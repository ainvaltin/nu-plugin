package nu

import (
	"context"
	"fmt"
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
* /
//TODO: unsupported Value type "Closure"
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
} //*/

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
	// TODO: if EnterForeground called Setpgid we should call Setpgid(0) here
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
		default:
			return nil, fmt.Errorf("unexpected return value of type %T", tv)
		}
	}
}
