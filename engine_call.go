package nu

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"

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
			return fmt.Errorf("decoding ValueMap of EngineCallResponse: %w", err)
		}
		cr.Response = m
	case "Identifier":
		if cr.Response, err = dec.DecodeUint(); err != nil {
			return fmt.Errorf("decoding Identifier response: %w", err)
		}
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
	return enterForeground(pgid)
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
EvalClosure implements [EvalClosure engine call].

Pass a [Closure] and optional arguments to the engine to be evaluated. Returned
value follows the same rules as Input field of the [ExecCommand] (ie it could
be nil, Value or stream).

[EvalClosure engine call]: https://www.nushell.sh/contributor-book/plugin_protocol_reference.html#evalclosure-engine-call
*/
func (ec *ExecCommand) EvalClosure(ctx context.Context, closure Value, args ...EvalArgument) (any, error) {
	if _, ok := closure.Value.(Closure); !ok {
		return nil, fmt.Errorf("closure argument must be of type Closure, got %T", closure.Value)
	}

	cfg, err := newEvalArguments(ec.p, args)
	if err != nil {
		return nil, fmt.Errorf("init evaluation config: %w", err)
	}
	if len(cfg.named) > 0 {
		return nil, fmt.Errorf("closures don't support NamedParameters")
	}

	type param struct {
		Call *evalClosure `msgpack:"EvalClosure"`
	}
	ch, err := ec.p.engineCall(ctx, ec.callID, param{&evalClosure{closure: closure, cfg: cfg}})
	if err != nil {
		return nil, fmt.Errorf("engine call: %w", err)
	}

	go cfg.run(ctx)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case v := <-ch:
		return ec.p.getInput(ctx, v)
	}
}

type evalClosure struct {
	closure Value
	cfg     *evalArguments
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
	if err := enc.EncodeArrayLen(len(ec.cfg.positional)); err != nil {
		return err
	}
	for x, v := range ec.cfg.positional {
		if err := v.EncodeMsgpack(enc); err != nil {
			return fmt.Errorf("encoding positional argument [%d]: %w", x, err)
		}
	}

	return ec.cfg.encodeCommonFields(enc)
}

// ErrDeclNotFound is returned by the [ExecCommand.FindDeclaration] method if the
// command with the given name couldn't be found in the scope of the plugin call.
var ErrDeclNotFound = errors.New("command not found")

/*
FindDeclaration implements [FindDecl engine call].

Returns [ErrDeclNotFound] if the command with the given name couldn't be found
in the scope of the plugin call (NB! use [errors.Is] to check for the error as
it might be wrapped into more descriptive error).

In case of success the returned Declaration can be used to call the command.

[FindDecl engine call]: https://www.nushell.sh/contributor-book/plugin_protocol_reference.html#finddecl-engine-call
*/
func (ec *ExecCommand) FindDeclaration(ctx context.Context, name string) (*Declaration, error) {
	type param struct {
		Name string `msgpack:"FindDecl"`
	}
	ch, err := ec.p.engineCall(ctx, ec.callID, param{Name: name})
	if err != nil {
		return nil, fmt.Errorf("engine call: %w", err)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case v := <-ch:
		switch tv := v.(type) {
		case nil, empty:
			return nil, fmt.Errorf("%q: %w", name, ErrDeclNotFound)
		case uint:
			return &Declaration{id: tv, ec: ec}, nil
		default:
			return nil, fmt.Errorf("unexpected return value of type %T", tv)
		}
	}
}

/*
Declaration represents Nu command which can be called from plugin.
Use [ExecCommand.FindDeclaration] to obtain the Declaration.
*/
type Declaration struct {
	id uint
	ec *ExecCommand
}

/*
Call implements [CallDecl engine call]. Use [ExecCommand.FindDeclaration] to
obtain the Declaration.

Note that [NamedParams] can be used as argument of Call in addition to the
[Positional], [InputValue] and other [EvalArgument]s.

[CallDecl engine call]: https://www.nushell.sh/contributor-book/plugin_protocol_reference.html#calldecl-engine-call
*/
func (d Declaration) Call(ctx context.Context, args ...EvalArgument) (any, error) {
	cfg, err := newEvalArguments(d.ec.p, args)
	if err != nil {
		return nil, fmt.Errorf("init evaluation config: %w", err)
	}

	type param struct {
		Call *callDecl `msgpack:"CallDecl"`
	}
	ch, err := d.ec.p.engineCall(ctx, d.ec.callID, param{&callDecl{d.id, cfg}})
	if err != nil {
		return nil, fmt.Errorf("engine call: %w", err)
	}

	go cfg.run(ctx)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case v := <-ch:
		return d.ec.p.getInput(ctx, v)
	}
}

type callDecl struct {
	decl_id uint //	The ID of the declaration to call.
	cfg     *evalArguments
}

func (cd *callDecl) EncodeMsgpack(enc *msgpack.Encoder) error {
	if err := enc.EncodeMapLen(5); err != nil {
		return err
	}

	// the ID of the declaration to call
	if err := enc.EncodeString("decl_id"); err != nil {
		return err
	}
	if err := enc.EncodeUint(uint64(cd.decl_id)); err != nil {
		return err
	}

	if err := enc.EncodeString("call"); err != nil {
		return err
	}
	call := evaluatedCall{Positional: cd.cfg.positional, Named: cd.cfg.named}
	if err := enc.EncodeValue(reflect.ValueOf(&call)); err != nil {
		return err
	}

	return cd.cfg.encodeCommonFields(enc)
}

type (
	// EvalArgument is type for [ExecCommand.EvalClosure] and [Declaration.Call] optional arguments.
	EvalArgument interface {
		apply(*evalArguments) error
	}

	evalArgument struct{ fn func(*evalArguments) error }

	evalArguments struct {
		named           NamedParams
		positional      []Value
		input           any
		redirect_stdout bool
		redirect_stderr bool

		p   *Plugin
		run func(ctx context.Context)
	}
)

func (opt evalArgument) apply(cfg *evalArguments) error { return opt.fn(cfg) }

func newEvalArguments(p *Plugin, args []EvalArgument) (*evalArguments, error) {
	cfg := &evalArguments{p: p, run: func(context.Context) {}, input: empty{}}
	for _, opt := range args {
		if err := opt.apply(cfg); err != nil {
			return nil, fmt.Errorf("invalid argument: %w", err)
		}
	}
	return cfg, nil
}

func (args *evalArguments) encodeCommonFields(enc *msgpack.Encoder) error {
	if err := enc.EncodeString("input"); err != nil {
		return err
	}
	if err := encodePipelineDataHeader(enc, args.input); err != nil {
		return fmt.Errorf("encode input: %w", err)
	}

	if err := enc.EncodeString("redirect_stdout"); err != nil {
		return err
	}
	if err := enc.EncodeBool(args.redirect_stdout); err != nil {
		return err
	}

	if err := enc.EncodeString("redirect_stderr"); err != nil {
		return err
	}
	if err := enc.EncodeBool(args.redirect_stderr); err != nil {
		return err
	}

	return nil
}

func (args *evalArguments) setInput(arg any) error {
	if _, ok := args.input.(empty); !ok {
		return fmt.Errorf("the Input parameter has already been set to %T", args.input)
	}

	args.input = arg
	return nil
}

// Positional arguments for the call.
func Positional(args ...Value) EvalArgument {
	return evalArgument{fn: func(ec *evalArguments) error {
		if ec.positional != nil {
			return errors.New("positional arguments have already been set")
		}
		ec.positional = args
		return nil
	}}
}

// InputValue allows to set single-value input for the call.
func InputValue(arg Value) EvalArgument {
	return evalArgument{fn: func(ec *evalArguments) error { return ec.setInput(arg) }}
}

func InputListStream(arg <-chan Value) EvalArgument {
	return evalArgument{fn: func(ec *evalArguments) error {
		out := newOutputListValue(ec.p)
		if err := ec.setInput(&listStream{ID: out.id}); err != nil {
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

func InputRawStream(arg io.Reader) EvalArgument {
	return evalArgument{fn: func(ec *evalArguments) error {
		out := newOutputListRaw(ec.p)
		if err := ec.setInput(&byteStream{ID: out.id, Type: "Unknown"}); err != nil {
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

/*
Whether to redirect stdout if the declared command ends in an external command.

Default is "false", this argument sets it to "true".
*/
func RedirectStdout() EvalArgument {
	return evalArgument{fn: func(ec *evalArguments) error { ec.redirect_stdout = true; return nil }}
}

/*
Whether to redirect stderr if the declared command ends in an external command.

Default is "false", this argument sets it to "true".
*/
func RedirectStderr() EvalArgument {
	return evalArgument{fn: func(ec *evalArguments) error { ec.redirect_stderr = true; return nil }}
}
