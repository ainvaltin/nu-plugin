package nu

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"syscall"

	"github.com/vmihailenco/msgpack/v5"
)

type engineCall struct {
	ID      int `msgpack:"id"`      // engine_call_id
	Context int `msgpack:"context"` // ID of the call which makes the engine call
	Call    any `msgpack:"call"`
}

type engineCallResponse struct {
	ID       int // engine_call_id
	Response any
}

var _ msgpack.CustomDecoder = (*engineCallResponse)(nil)

func (cr *engineCallResponse) DecodeMsgpack(dec *msgpack.Decoder) (err error) {
	if cr.ID, err = decodeTupleStart(dec); err != nil {
		return fmt.Errorf("decoding EngineCallResponse tuple: %w", err)
	}
	name, err := decodeWrapperMap(dec)
	if err != nil {
		return fmt.Errorf("decode value type of EngineCallResponse: %w", err)
	}
	switch name {
	case "PipelineData":
		pd := pipelineData{}
		if err := pd.DecodeMsgpack(dec); err != nil {
			return fmt.Errorf("decoding PipelineData of EngineCallResponse: %w", err)
		}
		cr.Response = pd
	case "ValueMap":
		m := map[string]Value{}
		if err = dec.DecodeValue(reflect.ValueOf(&m)); err != nil {
			return err
		}
		cr.Response = m
	case "Config":
		m, err := dec.DecodeMap()
		if err != nil {
			return fmt.Errorf("decoding Config response: %w", err)
		}
		cr.Response = m
	case "Error":
		e := LabeledError{}
		if err := dec.DecodeValue(reflect.ValueOf(&e)); err != nil {
			return err
		}
		cr.Response = e
	default:
		return fmt.Errorf("unexpected EngineCallResponse key %q", name)
	}
	return nil
}

/*
GetConfig engine call.

Get the Nushell engine configuration.
* /
//TODO: need to implement decoding the response struct, the msgpack lib's
//generic decode map doesn't seem to work...
func (ec *ExecCommand) GetConfig(ctx context.Context) (any, error) {
	ch, err := ec.p.engineCall(ctx, ec.callID, "GetConfig")
	if err != nil {
		return nil, fmt.Errorf("engine call: %w", err)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case v := <-ch:
		switch tv := v.(type) {
		case nil, empty:
			return nil, nil
		case Value:
			return &tv, nil
		default:
			return nil, fmt.Errorf("unexpected return value of type %T", tv)
		}
	}
} //*/

/*
GetPluginConfig engine call.

Get the configuration for the plugin, from its section in $env.config.plugins.NAME
if present. Returns nil if there is no configuration for the plugin set.

If the plugin configuration was specified as a closure, the engine will evaluate
that closure and return the result, which may cause an error response.
*/
func (ec *ExecCommand) GetPluginConfig(ctx context.Context) (*Value, error) {
	return ec.engineCallValueReturn(ctx, "GetPluginConfig")
}

/*
AddEnvVar engine call.

Set an environment variable in the caller's scope. The environment variable can only
be propagated to the caller's scope if called before the plugin call response is sent.
*/
func (ec *ExecCommand) AddEnvVar(ctx context.Context, name string, value Value) error {
	type param struct {
		Var []any `msgpack:"AddEnvVar"`
	}
	v, err := ec.engineCallValueReturn(ctx, param{Var: []any{name, &value}})
	if err != nil {
		return err
	}
	if v == nil {
		return nil
	}
	return fmt.Errorf("unexpected return value %v", v.Value)
}

/*
GetEnvVar engine call.

Get an environment variable from the caller's scope, returns nil if the environment
variable is not present.
*/
func (ec *ExecCommand) GetEnvVar(ctx context.Context, name string) (*Value, error) {
	type param struct {
		Name string `msgpack:"GetEnvVar"`
	}
	return ec.engineCallValueReturn(ctx, param{Name: name})
}

/*
GetEnvVars engine call.

Get all environment variables from the caller's scope.
*/
func (ec *ExecCommand) GetEnvVars(ctx context.Context) (map[string]Value, error) {
	ch, err := ec.p.engineCall(ctx, ec.callID, "GetEnvVars")
	if err != nil {
		return nil, fmt.Errorf("engine call: %w", err)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case v := <-ch:
		switch tv := v.(type) {
		case nil, empty:
			return nil, nil
		case map[string]Value:
			return tv, nil
		default:
			return nil, fmt.Errorf("unexpected return value of type %T", tv)
		}
	}
}

/*
GetCurrentDir engine call.

Get the current directory path in the caller's scope. This always returns an absolute path.
*/
func (ec *ExecCommand) GetCurrentDir(ctx context.Context) (string, error) {
	v, err := ec.engineCallValueReturn(ctx, "GetCurrentDir")
	if err != nil {
		return "", err
	}
	if v == nil {
		return "", nil
	}
	return v.Value.(string), nil
}

/*
GetHelp engine call.

Get fully formatted help text for the current command. This can help with
implementing top-level commands that just list their subcommands, rather
than implementing any specific functionality.
*/
func (ec *ExecCommand) GetHelp(ctx context.Context) (string, error) {
	v, err := ec.engineCallValueReturn(ctx, "GetHelp")
	if err != nil {
		return "", err
	}
	if v == nil {
		return "", nil
	}
	return v.Value.(string), nil
}

/*
EnterForeground engine call.

Moves the plugin to the foreground group for direct terminal access, in an operating
system-defined manner. This should be called when the plugin is going to drive the
terminal in raw mode, for example to implement a terminal UI.

This call will fail with an error if the plugin is already in the foreground.
The plugin should call LeaveForeground when it no longer needs to be in the foreground.
*/
func (ec *ExecCommand) EnterForeground(ctx context.Context) error {
	v, err := ec.engineCallValueReturn(ctx, "EnterForeground")
	if err != nil {
		return err
	}
	if v == nil {
		return nil
	}
	pgid, ok := v.Value.(int64)
	if !ok {
		return fmt.Errorf("expected pgid to be int, got %T", v.Value)
	}
	return syscall.Setpgid(syscall.Getpid(), int(pgid))
}

/*
LeaveForeground engine call - resets the state set by EnterForeground.
*/
func (ec *ExecCommand) LeaveForeground(ctx context.Context) error {
	v, err := ec.engineCallValueReturn(ctx, "LeaveForeground")
	if err != nil {
		return err
	}
	if v != nil {
		return fmt.Errorf("unexpected non-empty response: %v", v.Value)
	}
	// TODO: if EnterForeground called Setpgid we should call Setpgid(0) here?
	return nil
}

/*
GetSpanContents engine call.

Get the contents of a Span from the engine. This can be used for viewing the source code
that generated a value.
*/
func (ec *ExecCommand) GetSpanContents(ctx context.Context, span Span) ([]byte, error) {
	type param struct {
		Span Span `msgpack:"GetSpanContents"`
	}
	v, err := ec.engineCallValueReturn(ctx, param{span})
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	return v.Value.([]byte), nil
}

func (ec *ExecCommand) engineCallValueReturn(ctx context.Context, arg any) (*Value, error) {
	ch, err := ec.p.engineCall(ctx, ec.callID, arg)
	if err != nil {
		return nil, fmt.Errorf("engine call: %w", err)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case v := <-ch:
		switch tv := v.(type) {
		case nil, empty:
			return nil, nil
		case Value:
			return &tv, nil
		case LabeledError:
			return nil, &tv
		default:
			return nil, fmt.Errorf("unexpected return value of type %T", tv)
		}
	}
}

/*
EvalClosure engine call.

Pass a [Closure] and arguments to the engine to be evaluated. Returned value follows
the same rules as Input field of the [ExecCommand] (ie it could be nil, Value or
stream).
*/
func (ec *ExecCommand) EvalClosure(ctx context.Context, closure Value, args ...EvalClosureArgument) (any, error) {
	if _, ok := closure.Value.(Closure); !ok {
		return nil, fmt.Errorf("closure value must be of type Closure, got %T", closure.Value)
	}

	cfg := &evalClosure{p: ec.p, closure: closure, input: empty{}, run: func(context.Context) {}}
	for _, opt := range args {
		if err := opt.apply(cfg); err != nil {
			return nil, fmt.Errorf("invalid Closure argument: %w", err)
		}
	}

	go cfg.run(ctx)

	type param struct {
		Call *evalClosure `msgpack:"EvalClosure"`
	}
	ch, err := ec.p.engineCall(ctx, ec.callID, param{cfg})
	if err != nil {
		return nil, fmt.Errorf("engine call: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case v := <-ch:
		return ec.p.getInput(ctx, v)
	}
}

type evalClosure struct {
	closure         Value   `msgpack:"closure"`
	positional      []Value `msgpack:"positional"`
	input           any     `msgpack:"input"`
	redirect_stdout bool    `msgpack:"redirect_stdout"`
	redirect_stderr bool    `msgpack:"redirect_stderr"`

	p   *Plugin
	run func(ctx context.Context)
}

var _ msgpack.CustomEncoder = (*evalClosure)(nil)

func (ec *evalClosure) EncodeMsgpack(enc *msgpack.Encoder) error {
	if err := enc.EncodeMapLen(5); err != nil {
		return err
	}

	// closure
	if err := enc.EncodeString("closure"); err != nil {
		return err
	}
	if err := enc.EncodeMapLen(2); err != nil {
		return err
	}
	if err := enc.EncodeString("item"); err != nil {
		return err
	}
	if err := enc.EncodeValue(reflect.ValueOf(ec.closure.Value)); err != nil {
		return fmt.Errorf("encoding closure data: %w", err)
	}
	if err := enc.EncodeString("span"); err != nil {
		return err
	}
	if err := enc.EncodeValue(reflect.ValueOf(&ec.closure.Span)); err != nil {
		return err
	}

	// positional parameters
	if err := enc.EncodeString("positional"); err != nil {
		return err
	}
	if err := enc.EncodeArrayLen(len(ec.positional)); err != nil {
		return err
	}
	for x, v := range ec.positional {
		if err := v.EncodeMsgpack(enc); err != nil {
			return fmt.Errorf("encoding positional argument [%d]: %w", x, err)
		}
	}

	if err := enc.EncodeString("input"); err != nil {
		return err
	}
	if err := encodePipelineDataHeader(enc, ec.input); err != nil {
		return err
	}

	if err := enc.EncodeString("redirect_stdout"); err != nil {
		return err
	}
	if err := enc.EncodeBool(ec.redirect_stdout); err != nil {
		return err
	}

	if err := enc.EncodeString("redirect_stderr"); err != nil {
		return err
	}
	if err := enc.EncodeBool(ec.redirect_stderr); err != nil {
		return err
	}

	return nil
}

func (ec *evalClosure) setInput(arg any) error {
	if _, ok := ec.input.(empty); !ok {
		return fmt.Errorf("the Input parameter has already been set to %T", ec.input)
	}

	ec.input = arg
	return nil
}

type (
	/*
		EvalClosureArgument is type for [ExecCommand.EvalClosure] optional arguments.

		https://www.nushell.sh/contributor-book/plugin_protocol_reference.html#evalclosure-engine-call
	*/
	EvalClosureArgument interface {
		apply(*evalClosure) error
	}

	evalClosureArgument struct{ fn func(*evalClosure) error }
)

func (opt evalClosureArgument) apply(cfg *evalClosure) error { return opt.fn(cfg) }

// Positional arguments for the closure.
func Positional(args ...Value) EvalClosureArgument {
	return evalClosureArgument{fn: func(ec *evalClosure) error {
		if ec.positional != nil {
			return errors.New("positional arguments have already been set")
		}
		ec.positional = args
		return nil
	}}
}

// InputValue allows to set single-value input for the closure.
func InputValue(arg Value) EvalClosureArgument {
	return evalClosureArgument{fn: func(ec *evalClosure) error { return ec.setInput(arg) }}
}

func InputListStream(arg <-chan Value) EvalClosureArgument {
	return evalClosureArgument{fn: func(ec *evalClosure) error {
		out := newOutputListValue(ec.p)
		if err := ec.setInput(&listStream{ID: out.id, Span: ec.closure.Span}); err != nil {
			return err
		}
		ec.run = func(ctx context.Context) {
			defer func() {
				close(out.data)
				out.close(ctx)
			}()
			ec.p.registerOutputStream(ctx, out)
			for v := range arg {
				select {
				case <-ctx.Done():
					return
				case out.data <- v:
				}
			}
		}
		return nil
	}}
}

func InputRawStream(arg io.Reader) EvalClosureArgument {
	return evalClosureArgument{fn: func(ec *evalClosure) error {
		out := newOutputListRaw(ec.p)
		if err := ec.setInput(&byteStream{ID: out.id, Span: ec.closure.Span, Type: "Unknown"}); err != nil {
			return err
		}
		ec.run = func(ctx context.Context) {
			defer out.close(ctx)
			ec.p.registerOutputStream(ctx, out)
			if n, err := io.Copy(out.data, arg); err != nil {
				ec.p.log.ErrorContext(ctx, fmt.Sprintf("raw stream error after %d bytes", n), attrError(err))
			}
		}
		return nil
	}}
}

func RedirectStdout() EvalClosureArgument {
	return evalClosureArgument{fn: func(ec *evalClosure) error { ec.redirect_stdout = true; return nil }}
}

func RedirectStderr() EvalClosureArgument {
	return evalClosureArgument{fn: func(ec *evalClosure) error { ec.redirect_stderr = true; return nil }}
}
