package nu

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	"github.com/vmihailenco/msgpack/v5"
)

// exit cause when plugin received Goodbye message
var ErrGoodbye = errors.New("Goodbye")

// context cancellation (command's OnRun handler) or stream close error when
// consumer sent Drop message (ie plugin should stop producing into output stream).
var ErrDropStream = errors.New("received Drop stream message")

/*
New creates new Nushell Plugin with given commands.

The cfg may be nil. In that case Error level logger is used (logs to stderr), to
disable logging completely provide Config with NOP logger.
*/
func New(cmd []*Command, cfg *Config) (*Plugin, error) {
	p := &Plugin{
		cmds: make(map[string]*Command),
		outs: make(map[int]outputStream),
		inls: make(map[int]inputStream),
		runs: commandsInFlight{},
		in:   cfg.streamIn(os.Stdin),
		out:  cfg.streamOut(os.Stdout),
		log:  cfg.logger(),
	}

	for _, v := range cmd {
		cmdName := v.Sig.Name
		if _, ok := p.cmds[cmdName]; ok {
			return nil, fmt.Errorf("command %q already registered", cmdName)
		}
		if err := v.Sig.Named.addHelp(); err != nil {
			p.log.Warn(fmt.Sprintf("adding help flag to %q command", cmdName), attrError(err))
		}
		if err := v.Validate(); err != nil {
			return nil, fmt.Errorf("invalid command %q: %w", cmdName, err)
		}
		p.cmds[cmdName] = v
	}

	if len(p.cmds) == 0 {
		return nil, fmt.Errorf("no commands registered")
	}
	return p, nil
}

/*
Plugin implements Nushell Plugin, single Plugin can implement multiple commands.

The zero value is not usable, the [New] constructor must be used to create Plugin.
*/
type Plugin struct {
	cmds map[string]*Command // available commands

	runs  commandsInFlight
	iom   sync.Mutex // to sync in and out maps
	outs  map[int]outputStream
	inls  map[int]inputStream
	idGen atomic.Uint32 // id generator

	in io.Reader
	// output might be accessed by multiple goroutines so guard it with mutex
	m   sync.Mutex
	out io.Writer

	log *slog.Logger
}

type inputStream interface {
	received(ctx context.Context, v any) error
	endOfData()
}

type outputStream interface {
	ack() error
	run(ctx context.Context) error
	drop()
	streamID() int
	pipelineDataHdr() any
	close() error
}

/*
Run starts the plugin.
It is blocking until Plugin exits (ie because plugin engine sent Goodbye
message, the ctx was cancelled or unrecoverable error happened).
*/
func (p *Plugin) Run(ctx context.Context) error {
	// send encoding type and Hello
	p.outputRaw(ctx, []byte(format_mpack))
	h := hello{Protocol: protocol_name, Version: protocol_version}
	if err := p.outputMsg(ctx, &h); err != nil {
		return fmt.Errorf("sending Hello: %w", err)
	}

	// wait for server to send Hello? ie do not start
	// main message loop before we have received Hello?

	// launch a watchdog which closes the input stream when
	// context is cancelled? As otherwise we could be stuck
	// waiting for next message data...

	err := p.mainMsgLoop(ctx)
	p.log.DebugContext(ctx, "main input loop exit", attrError(err))
	// make sure all commands exit?
	p.runs.CancelAndWait(err)
	// if err is Goodbye return nil?
	return err
}

func (p *Plugin) mainMsgLoop(ctx context.Context) error {
	dec := msgpack.NewDecoder(p.in)
	dec.SetMapDecoder(decodeInputMsg)

	for ctx.Err() == nil {
		v, err := dec.DecodeInterface()
		switch err {
		case nil:
		case io.EOF:
			return nil
		default:
			p.log.ErrorContext(ctx, "decoding top-level message", attrError(err))
			continue
		}

		if s, ok := v.(string); ok && s == "Goodbye" {
			return ErrGoodbye
		}

		if err := p.handleMessage(ctx, v); err != nil {
			p.log.ErrorContext(ctx, "handling message", attrError(err), attrMsg(v))
		}
	}
	return ctx.Err()
}

// handleMessage processes top level message
func (p *Plugin) handleMessage(ctx context.Context, msg any) error {
	p.log.DebugContext(ctx, "handleMessage", attrMsg(msg))
	switch m := msg.(type) {
	case call:
		if err := p.handleCall(ctx, m); err != nil {
			return p.handleCallError(ctx, m.ID, err)
		}
		return nil
	case ack:
		return p.handleAck(ctx, m.ID)
	case data:
		return p.handleData(ctx, m)
	case end:
		return p.handleEnd(ctx, m.ID)
	case drop:
		return p.handleDrop(ctx, m.ID)
	case hello:
		return nil
	default:
		return fmt.Errorf("unknown top-level message %T", msg)
	}
}

func (p *Plugin) handleCall(ctx context.Context, msg call) error {
	switch m := msg.Call.(type) {
	case signature:
		return p.handleSignature(ctx)
	case run:
		return p.handleRun(ctx, m, msg.ID)
	default:
		return fmt.Errorf("unknown Call message %T", m)
	}
}

func (p *Plugin) handleSignature(ctx context.Context) error {
	sigs := make([]*Command, 0, len(p.cmds))
	for _, v := range p.cmds {
		v := v
		sigs = append(sigs, v)
	}
	return p.outputMsg(ctx, &callResponse{Response: sigs})
}

func (p *Plugin) handleRun(ctx context.Context, msg run, callID int) error {
	cmd, ok := p.cmds[msg.Name]
	if !ok {
		return fmt.Errorf("unknown Run target %q", msg.Name)
	}

	exec := &ExecCommand{
		p:      p,
		callID: callID,
		Name:   msg.Name,
		Call:   msg.Call,
	}

	switch it := msg.Input.(type) {
	case Empty, nil:
		exec.Input = Empty{}
	case Value:
		exec.Input = it
	case listStream:
		ls := newInputStreamList(it.ID)
		ls.ack = func(ID int) {
			if err := p.outputMsg(ctx, ack{ID: ID}); err != nil {
				p.log.ErrorContext(ctx, "sending Ack", attrError(err), attrStreamID(ID))
			}
		}
		p.iom.Lock()
		p.inls[it.ID] = ls
		p.iom.Unlock()
		exec.Input = ls.InputStream()
	case externalStream:
		p.log.DebugContext(ctx, fmt.Sprintf("input stdout: %#v", it.Stdout))
		ls := newInputStreamRaw(it.Stdout.ID)
		ls.ack = func(ID int) {
			if err := p.outputMsg(ctx, ack{ID: ID}); err != nil {
				p.log.ErrorContext(ctx, "sending Ack", attrError(err), attrStreamID(ID))
			}
		}
		p.iom.Lock()
		p.inls[ls.id] = ls
		p.iom.Unlock()
		exec.Input = ls.rdr
	default:
		return fmt.Errorf("running %q with unsupported input type: %T", msg.Name, it)
	}

	ctx, exec.cancel = context.WithCancelCause(ctx)
	p.runs.registerInFlight(exec)
	go func() {
		defer p.runs.removeInFlight(exec)
		if err := cmd.OnRun(ctx, exec); err != nil {
			if err := exec.returnError(ctx, err); err != nil {
				p.log.ErrorContext(ctx, "sending error response", attrError(err), attrCallID(callID))
			}
			// the stream might still be open so attempt to close it
			exec.closeOutputStream()
			return
		}

		if out := exec.output.Load(); out == nil {
			if err := exec.returnNothing(ctx); err != nil {
				p.log.ErrorContext(ctx, "sending 'Empty' response", attrError(err), attrCallID(callID))
			}
		}
	}()

	return nil
}

func (p *Plugin) handleAck(_ context.Context, id int) error {
	p.iom.Lock()
	out, ok := p.outs[id]
	p.iom.Unlock()

	if ok {
		return out.ack()
	}
	return fmt.Errorf("no output stream with id %d", id)
}

func (p *Plugin) handleData(ctx context.Context, data data) error {
	p.iom.Lock()
	in, ok := p.inls[data.ID]
	p.iom.Unlock()
	if !ok {
		return fmt.Errorf("unknown input stream %d", data.ID)
	}
	return in.received(ctx, data.Data)
}

func (p *Plugin) handleEnd(ctx context.Context, id int) error {
	p.iom.Lock()
	in, ok := p.inls[id]
	delete(p.inls, id)
	p.iom.Unlock()
	if !ok {
		return fmt.Errorf("unknown input stream %d", id)
	}
	in.endOfData()
	return p.outputMsg(ctx, drop{ID: id})
}

func (p *Plugin) handleDrop(_ context.Context, id int) error {
	p.iom.Lock()
	out, ok := p.outs[id]
	delete(p.outs, id)
	p.iom.Unlock()

	if !ok {
		return fmt.Errorf("no output stream with id %d", id)
	}

	out.drop()
	return nil
}

func (p *Plugin) registerOutput(ctx context.Context, callID int, stream outputStream) error {
	p.iom.Lock()
	p.outs[stream.streamID()] = stream
	p.iom.Unlock()

	if err := p.outputMsg(ctx, &callResponse{ID: callID, Response: &pipelineData{stream.pipelineDataHdr()}}); err != nil {
		return fmt.Errorf("sending CallResponse{%d} PipelineData Stream{%d}: %w", callID, stream.streamID(), err)
	}

	go func() {
		if err := stream.run(ctx); err != nil {
			p.log.ErrorContext(ctx, "output stream run exit", attrError(err), attrStreamID(stream.streamID()))
		}
	}()

	return nil
}

/*
handleCallError sends error response with given Call ID (ie processing this call
caused error).
*/
func (p *Plugin) handleCallError(ctx context.Context, callID int, callErr error) error {
	// if the response stream has been started we must send error inside the stream?
	p.log.ErrorContext(ctx, "responding with error to a Call", attrError(callErr), attrCallID(callID))
	if err := p.outputMsg(ctx, &callResponse{ID: callID, Response: callErr}); err != nil {
		return fmt.Errorf("sending error response to a Call: %w", err)
	}
	return nil
}

/*
Encode data as message pack and send it out.
*/
func (p *Plugin) outputMsg(ctx context.Context, data any) error {
	b, err := msgpack.Marshal(data)
	if err != nil {
		return fmt.Errorf("serializing %T: %w", data, err)
	}
	return p.outputRaw(ctx, b)
}

func (p *Plugin) outputRaw(ctx context.Context, data []byte) error {
	p.m.Lock()
	defer p.m.Unlock()
	p.log.DebugContext(ctx, "output", "msg", data)

	if _, err := p.out.Write(data); err != nil {
		return fmt.Errorf("writing to output: %w", err)
	}
	return nil
}
