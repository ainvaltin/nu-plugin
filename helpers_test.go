package nu

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/neilotoole/slogt"
	"github.com/vmihailenco/msgpack/v5"
)

/*
PluginResponse returns plugin "p" response to the message "msg".
The message is pointer to Go nu-protocol message structure, ie

	PluginResponse(ctx, p, &Call{ID: 1, Call: Signature{}})
*/
func PluginResponse(ctx context.Context, p *Plugin, msg any) ([]byte, error) {
	outBuf := &bytes.Buffer{}
	p.out = outBuf

	r, w := io.Pipe()
	p.in = r

	done := make(chan error, 1)
	go func() {
		defer close(done)
		if err := p.mainMsgLoop(ctx); err != nil {
			done <- fmt.Errorf("plugin loop exited with error: %w", err)
		}
	}()

	if err := msgpack.NewEncoder(w).Encode(msg); err != nil {
		done <- fmt.Errorf("encoding the message: %w", err)
	}
	w.Close()

	var err error
	for e := range done {
		err = errors.Join(err, e)
	}
	return outBuf.Bytes(), err
}

/*
Parses all nu-plugin-protocol messages (both server and client).
*/
func decodeNuMsgAll(p *Plugin, next func(d *msgpack.Decoder, name string) (_ interface{}, err error)) func(*msgpack.Decoder) (interface{}, error) {
	return func(dec *msgpack.Decoder) (interface{}, error) {
		name, err := decodeWrapperMap(dec)
		if err != nil {
			return nil, fmt.Errorf("decode message: %w", err)
		}
		switch name {
		case "CallResponse":
			cr := callResponse{}
			return cr, cr.decodeMsgpack(dec, p)
		case "PipelineData":
			cr := pipelineData{}
			return cr, cr.decodeMsgpack(dec, p)
		default:
			return next(dec, name)
		}
	}
}

func logger(t *testing.T) *slog.Logger {
	return slogt.New(t)
}

func expectErrorMsg(t *testing.T, err error, msg string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got none")
	}

	if diff := cmp.Diff(err.Error(), msg); diff != "" {
		t.Errorf("error message mismatch (-want +got):\n%s", diff)
	}
}

func (p *Plugin) deserialize(data []byte, v any) error {
	type mpe interface {
		decodeMsgpack(*msgpack.Decoder, *Plugin) error
	}

	dec := msgpack.GetDecoder()
	defer msgpack.PutDecoder(dec)
	dec.UsePreallocateValues(true)
	dec.Reset(bytes.NewReader(data))
	if f, ok := v.(mpe); ok {
		return f.decodeMsgpack(dec, p)
	}

	return dec.Decode(v)
}
