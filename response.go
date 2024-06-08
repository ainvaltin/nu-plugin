package nu

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

/*
ExecCommand is passed as argument to the plugin command's OnRun handler.

It allows to make engine calls, access command's input (see Input, Named
and Positional fields) and send response (see Return* methods).
*/
type ExecCommand struct {
	Name string

	// Span of the command invocation
	Head Span
	// Values of positional arguments
	Positional []Value
	// Names and values of named arguments
	Named NamedParams

	/*
		Input to the command. Is one of:

		- nil: no input;
		- Value: single value input;
		- <-chan Value: stream of Values;
		- io.ReadCloser: raw stream;
	*/
	Input any

	p      *Plugin
	callID int // call ID which launched the cmd
	cancel context.CancelCauseFunc
	output atomic.Value
}

/*
ReturnValue should be used when command returns single Value.
*/
func (ec *ExecCommand) ReturnValue(ctx context.Context, v Value) error {
	if !ec.output.CompareAndSwap(nil, v) {
		return fmt.Errorf("response has been already sent")
	}

	rsp := callResponse{ID: ec.callID, Response: &pipelineData{Data: &v}}
	return ec.p.outputMsg(ctx, &rsp)
}

/*
ReturnListStream should be used when command returns multiple nu.Values.

To signal the end of data chan must be closed.
*/
func (ec *ExecCommand) ReturnListStream(ctx context.Context) (chan<- Value, error) {
	out := newOutputListValue(int(ec.p.idGen.Add(1)))
	if !ec.output.CompareAndSwap(nil, out) {
		if es, ok := ec.output.Load().(*listStreamOut); ok {
			return es.data, nil
		}
		return nil, fmt.Errorf("response has been already sent")
	}

	out.onSend = func(id int, v Value) error {
		return ec.p.outputMsg(ctx, &data{ID: id, Data: v})
	}
	out.setOnDone(func(id int) { ec.p.outputMsg(ctx, end{ID: id}) })
	out.onDrop = func() { ec.cancel(ErrDropStream) }

	if err := ec.p.registerOutput(ctx, ec.callID, out); err != nil {
		return nil, err
	}

	return out.data, nil
}

/*
ReturnRawStream should be used when command returns raw stream.

To signal the end of data Writer must be closed.

Cancelling the context (ctx) will also "stop" the output stream, ie it
signals that the plugin is about to quit and all work has to be abandoned.
*/
func (ec *ExecCommand) ReturnRawStream(ctx context.Context, opts ...RawStreamOption) (io.WriteCloser, error) {
	out := newOutputListRaw(int(ec.p.idGen.Add(1)), opts...)
	if !ec.output.CompareAndSwap(nil, out) {
		if es, ok := ec.output.Load().(*rawStreamOut); ok {
			return es.data, nil
		}
		return nil, fmt.Errorf("response has been already sent")
	}

	out.onSend = func(ID int, b []byte) error {
		return ec.p.outputMsg(ctx, &data{ID: ID, Data: b})
	}
	out.setOnDone(func(id int) { ec.p.outputMsg(ctx, end{ID: id}) })
	out.onDrop = func() { ec.cancel(ErrDropStream) }

	if err := ec.p.registerOutput(ctx, ec.callID, out); err != nil {
		return nil, err
	}

	return out.data, nil
}

func (ec *ExecCommand) returnNothing(ctx context.Context) error {
	return ec.p.outputMsg(ctx, &callResponse{ID: ec.callID, Response: &pipelineData{Data: empty{}}})
}

func (ec *ExecCommand) returnError(ctx context.Context, callErr error) error {
	out := ec.output.Load()
	switch s := out.(type) {
	case nil, *Value:
		if err := ec.p.outputMsg(ctx, &callResponse{ID: ec.callID, Response: callErr}); err != nil {
			return fmt.Errorf("sending error response to a Call: %w", err)
		}
		return nil
	case *rawStreamOut:
		return ec.p.outputMsg(ctx, &data{ID: s.id, Data: callErr})
	case *listStreamOut:
		return ec.p.outputMsg(ctx, &data{ID: s.id, Data: Value{Value: callErr}})
	default:
		return fmt.Errorf("unsupported output type %T", s)
	}
}

func (ec *ExecCommand) closeOutputStream() {
	out := ec.output.Load()
	if closer, ok := out.(interface{ close() error }); ok {
		closer.close()
	}
}

type (
	RawStreamOption interface {
		apply(*rawStreamCfg)
	}

	rawStreamCfg struct {
		bufSize  uint
		dataType string // the expected type of the stream
		//span     Span
	}
	rawStreamOpt struct{ fn func(*rawStreamCfg) }
)

func (opt rawStreamOpt) apply(cfg *rawStreamCfg) { opt.fn(cfg) }

/*
BufferSize allows to hint the desired buffer size (but it is not guaranteed
that buffer will be exactly that big).
Writes are collected into buffer before sending to the consumer.
*/
func BufferSize(size uint) RawStreamOption {
	return rawStreamOpt{fn: func(rc *rawStreamCfg) { rc.bufSize = max(size, 512) }}
}

/*
BinaryStream indicates that the stream contains binary data of unknown encoding,
and should be treated as a binary value. See also [StringStream].
*/
func BinaryStream() RawStreamOption {
	return rawStreamOpt{fn: func(rc *rawStreamCfg) { rc.dataType = "Binary" }}
}

/*
StringStream indicates that the stream contains text data that is valid UTF-8,
and should be treated as a string value. See also [BinaryStream].
*/
func StringStream() RawStreamOption {
	return rawStreamOpt{fn: func(rc *rawStreamCfg) { rc.dataType = "String" }}
}

type commandsInFlight struct {
	runs []*ExecCommand
	m    sync.Mutex
	wg   sync.WaitGroup
}

func (cf *commandsInFlight) registerInFlight(cmd *ExecCommand) {
	cf.m.Lock()
	defer cf.m.Unlock()

	cf.wg.Add(1)
	for i := range cf.runs {
		if cf.runs[i] == nil {
			cf.runs[i] = cmd
			return
		}
	}

	cf.runs = append(cf.runs, cmd)
}

func (cf *commandsInFlight) removeInFlight(cmd *ExecCommand) {
	cf.m.Lock()
	defer cf.m.Unlock()

	for i := range cf.runs {
		if cf.runs[i] == cmd {
			cf.runs[i] = nil
			cf.wg.Done()
			return
		}
	}
}

func (cf *commandsInFlight) stopAll(cause error) {
	cf.m.Lock()
	defer cf.m.Unlock()

	for i := range cf.runs {
		if cf.runs[i] != nil {
			cf.runs[i].cancel(cause)
		}
	}
}

func (cf *commandsInFlight) CancelAndWait(cause error) {
	cf.stopAll(cause)
	// should have some timeout for the wait? ctx as param?
	cf.wg.Wait()
}
