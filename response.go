package nu

import (
	"context"
	"fmt"
	"io"
	"mime"
	"path/filepath"
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
FlagValue returns value of named parameter/flag.

The returned bool flag indicates was the flag set by user (true) or
not (false). When flag was not set by user the default value (defined
in plugin signature) is returned. When signature doesn't define
default value (or flag is not defined in the signature) then zero Value
and false is returned.

For toggle flags (Shape is not assigned in the flag definition) Bool
Value is always returned ie if user doesn't provide the flag or
"--flagName=false" is used then Value==false is returned.
*/
func (ec *ExecCommand) FlagValue(name string) (Value, bool) {
	v, ok := ec.Named[name]
	if ok {
		// shell doesn't run the command when "value flag" doesn't have
		// correct value so when value is nil it must be "toggle flag"?
		if v.Value == nil {
			return Value{Value: true}, true
		}
		// the flag was specified with value - could be toggle flag with
		// explicit boolean value too
		return v, true
	}

	// we need to know is it a "toggle flag" and whats the default
	cmd := ec.p.cmds[ec.Name]
	for _, flag := range cmd.Signature.Named {
		if flag.Long == name {
			// if it is toggle flag return false
			if flag.Shape == nil {
				return Value{Value: false}, false
			}
			if flag.Default != nil {
				// use ToValue as ie default defined as "nu.Value{Value: 1}"
				// has a type "int" but canonical value would be "int64"
				return ToValue(flag.Default.Value), false
			}
			break
		}
	}

	// v must be zero value here
	return v, false
}

/*
ReturnValue should be used when command returns single Value.
*/
func (ec *ExecCommand) ReturnValue(ctx context.Context, v Value) error {
	if !ec.output.CompareAndSwap(nil, v) {
		return fmt.Errorf("response has been already sent")
	}

	rsp := callResponse{ID: ec.callID, Response: &pipelineData{Data: v}}
	return ec.p.outputMsg(ctx, &rsp)
}

/*
ReturnListStream should be used when command returns multiple nu.Values.

When one of the values is [error] engine considers the plugin call to have
been failed and prints that error message.

To signal the end of data chan must be closed (even when sending error)!
*/
func (ec *ExecCommand) ReturnListStream(ctx context.Context) (chan<- Value, error) {
	out := newOutputListValue(ec.p)
	out.onDrop = func() { ec.cancel(ErrDropStream) }

	if !ec.output.CompareAndSwap(nil, out) {
		if es, ok := ec.output.Load().(*listStreamOut); ok {
			return es.data, nil
		}
		return nil, fmt.Errorf("response has been already sent")
	}

	if err := ec.startResponseStream(ctx, out); err != nil {
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
	out := newOutputListRaw(ec.p, opts...)
	out.onDrop = func() { ec.cancel(ErrDropStream) }

	if !ec.output.CompareAndSwap(nil, out) {
		if es, ok := ec.output.Load().(*rawStreamOut); ok {
			return es.data, nil
		}
		return nil, fmt.Errorf("response has been already sent")
	}

	if err := ec.startResponseStream(ctx, out); err != nil {
		return nil, err
	}

	return out.data, nil
}

/*
if response haven't been sent then send Empty
*/
func (ec *ExecCommand) returnNothing(ctx context.Context) error {
	if out := ec.output.Load(); out == nil {
		return ec.p.outputMsg(ctx, &callResponse{ID: ec.callID, Response: &pipelineData{Data: empty{}}})
	}
	return nil
}

func (ec *ExecCommand) returnError(ctx context.Context, callErr error) error {
	out := ec.output.Load()
	switch s := out.(type) {
	case nil, *Value, Value:
		// if we have already sent the Value response, will this get through?!
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

func (ec *ExecCommand) startResponseStream(ctx context.Context, out outputStream) error {
	ec.p.registerOutputStream(ctx, out)
	if err := ec.p.outputMsg(ctx, &callResponse{ID: ec.callID, Response: &pipelineData{out.pipelineDataHdr()}}); err != nil {
		return fmt.Errorf("sending CallResponse{%d} PipelineData Stream{%d}: %w", ec.callID, out.streamID(), err)
	}
	return nil
}

func (ec *ExecCommand) closeOutputStream(ctx context.Context) {
	out := ec.output.Load()
	if closer, ok := out.(closeCtx); ok {
		closer.close(ctx)
	}
}

type (
	RawStreamOption interface {
		apply(*rawStreamCfg)
	}

	rawStreamCfg struct {
		bufSize  uint
		dataType string // the expected type of the stream
		md       pipelineMetadata
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

/*
FilePath sets the stream metadata to "DataSource = FilePath" with given file name.
The "content type" field of the metadata is set based on the file's extension
using system mime type registry.
*/
func FilePath(fileName string) RawStreamOption {
	return rawStreamOpt{fn: func(rc *rawStreamCfg) {
		rc.md.FilePath = fileName
		rc.md.DataSource = "FilePath"
		rc.md.ContentType = mime.TypeByExtension(filepath.Ext(fileName))
	}}
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
			cf.runs[i].cancel(nil)
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
