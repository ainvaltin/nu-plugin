package nu

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/vmihailenco/msgpack/v5"
)

func Test_Plugin_Run(t *testing.T) {
	createPlugin := func(t *testing.T) *Plugin {
		p, err := New(
			[]*Command{{
				Signature: PluginSignature{
					Name:             "foo bar",
					Category:         "Experimental",
					Usage:            "test cmd",
					SearchTerms:      []string{"foo"},
					InputOutputTypes: [][]string{{"Any", "Any"}},
				},
				OnRun: func(ctx context.Context, exec *ExecCommand) error {
					return nil
				},
			}},
			&Config{Logger: logger(t)},
		)
		if err != nil {
			t.Fatalf("creating plugin: %v", err)
		}
		return p
	}

	t.Run("cancel context", func(t *testing.T) {
		p := createPlugin(t)
		r, w := io.Pipe()
		p.in = r
		p.out = bytes.NewBuffer(nil)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error)
		go func() {
			defer r.Close()
			done <- p.Run(ctx)
		}()

		// wait for plugin to send it's Hello. Potentially flaky but there
		// is no perfect way to detect when the Plugin's main loop started
		time.Sleep(time.Second)

		// cancelling the Run ctx doesn't stop it as it's waiting to decode
		// message from input...
		cancel()
		select {
		case err := <-done:
			if err == nil || !errors.Is(err, context.Canceled) {
				t.Errorf("unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			t.Log("as expected Run hasn't exited")
		}
		// sending something to input causes "main loop" to check the
		// context and exit
		enc := msgpack.NewEncoder(w)
		if err := enc.EncodeString("whatever"); err != nil {
			t.Errorf("sending message: %v", err)
		}
		select {
		case err := <-done:
			if err == nil || !errors.Is(err, context.Canceled) {
				t.Errorf("unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("Run hasn't exited")
		}
	})

	t.Run("close input", func(t *testing.T) {
		p := createPlugin(t)
		pOut := bytes.NewBuffer(nil)
		r, w := io.Pipe()
		p.in = r
		p.out = pOut

		done := make(chan error)
		go func() {
			defer r.Close()
			done <- p.Run(context.Background())
		}()

		// wait for plugin to send it's Hello. Potentially flaky but there
		// is no perfect way to detect when the Plugin's main loop started
		time.Sleep(time.Second)

		// Plugin is waiting for a message to decode, closing input
		// should cause EOF and exit with nil error
		if err := w.Close(); err != nil {
			t.Errorf("closing writer: %v", err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("Run hasn't exited")
		}
	})

	t.Run("Goodbye", func(t *testing.T) {
		p := createPlugin(t)
		p.out = bytes.NewBuffer(nil)
		r, w := io.Pipe()
		p.in = r

		done := make(chan error)
		go func() {
			defer r.Close()
			done <- p.Run(context.Background())
		}()

		enc := msgpack.NewEncoder(w)
		if err := enc.EncodeString("Goodbye"); err != nil {
			t.Errorf("sending Goodbye: %v", err)
		}

		select {
		case err := <-done:
			if err == nil || !errors.Is(err, ErrGoodbye) {
				t.Errorf("unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("Run hasn't exited")
		}
	})
}

func Test_Plugin_Signature(t *testing.T) {
	p, err := New([]*Command{
		{
			Signature: PluginSignature{
				Name:             "foo bar",
				Category:         "Experimental",
				Usage:            "test cmd",
				SearchTerms:      []string{"foo"},
				InputOutputTypes: [][]string{{"Any", "Any"}},
			},
			OnRun: func(ctx context.Context, exec *ExecCommand) error {
				return nil
			},
		},
	},
		&Config{Logger: logger(t)},
	)
	if err != nil {
		t.Fatalf("creating plugin: %v", err)
	}

	rsp, err := PluginResponse(context.Background(), p, &call{ID: 1, Call: signature{}})
	if err != nil {
		t.Errorf("getting plugin response: %v", err)
	}
	t.Logf("plugin response:\n0x[%x] | from msgpack", rsp)
}

func Test_Plugin_response(t *testing.T) {
	signature := PluginSignature{
		Name:             "inc",
		Category:         "Experimental",
		Usage:            "test cmd",
		SearchTerms:      []string{"foo"},
		InputOutputTypes: [][]string{{"Any", "Any"}},
	}

	t.Run("Error response", func(t *testing.T) {
		p, err := New([]*Command{
			{
				Signature: signature,
				OnRun: func(ctx context.Context, exec *ExecCommand) error {
					return fmt.Errorf("sorry")
				},
			},
		},
			&Config{Logger: logger(t)},
		)
		if err != nil {
			t.Fatalf("creating plugin: %v", err)
		}

		runEngine(t, p, append(protocolPrelude,
			msgDef{send: &call{ID: 1, Call: run{Name: "inc"}}},
			msgDef{recv: callResponse{ID: 1, Response: LabeledError{Msg: "sorry"}}},
		))
	})

	t.Run("Single Value response", func(t *testing.T) {
		p, err := New([]*Command{
			{
				Signature: signature,
				OnRun: func(ctx context.Context, exec *ExecCommand) error {
					return exec.ReturnValue(ctx, Value{Value: 42})
				},
			},
		},
			&Config{Logger: logger(t)},
		)
		if err != nil {
			t.Fatalf("creating plugin: %v", err)
		}

		runEngine(t, p, append(protocolPrelude,
			msgDef{send: &call{ID: 1, Call: run{Name: "inc"}}},
			msgDef{recv: callResponse{ID: 1, Response: pipelineData{Data: Value{Value: int64(42)}}}},
		))
	})

	t.Run("List of Values response", func(t *testing.T) {
		p, err := New([]*Command{
			{
				Signature: signature,
				OnRun: func(ctx context.Context, exec *ExecCommand) error {
					out, err := exec.ReturnListStream(ctx)
					if err != nil {
						return fmt.Errorf("getting the return list: %w", err)
					}
					out <- Value{Value: "v1"}
					out <- Value{Value: "v2"}
					close(out)
					return nil
				},
			},
		},
			&Config{Logger: logger(t)},
		)
		if err != nil {
			t.Fatalf("creating plugin: %v", err)
		}

		runEngine(t, p, append(protocolPrelude,
			msgDef{send: &call{ID: 1, Call: run{Name: "inc"}}},
			msgDef{recv: callResponse{ID: 1, Response: pipelineData{Data: listStream{ID: 1}}}},
			msgDef{recv: data{ID: 1, Data: Value{Value: "v1"}}},
			msgDef{send: &ack{ID: 1}},
			msgDef{recv: data{ID: 1, Data: Value{Value: "v2"}}},
			msgDef{send: &ack{ID: 1}},
			msgDef{recv: end{ID: 1}},
			msgDef{send: &drop{ID: 1}},
		))
	})

	t.Run("List of bytes response", func(t *testing.T) {
		p, err := New([]*Command{
			{
				Signature: signature,
				OnRun: func(ctx context.Context, exec *ExecCommand) error {
					out, err := exec.ReturnRawStream(ctx)
					if err != nil {
						return fmt.Errorf("getting output writer: %w", err)
					}
					out.Write([]byte("first"))
					out.Write([]byte("second"))
					return out.Close()
				},
			},
		},
			&Config{Logger: logger(t)},
		)
		if err != nil {
			t.Fatalf("creating plugin: %v", err)
		}

		runEngine(t, p, append(protocolPrelude,
			msgDef{send: &call{ID: 1, Call: run{Name: "inc"}}},
			msgDef{recv: callResponse{ID: 1, Response: pipelineData{byteStream{ID: 1, Type: "Unknown"}}}},
			msgDef{recv: data{ID: 1, Data: []byte("firstsecond")}},
			msgDef{send: &ack{ID: 1}},
			msgDef{recv: end{ID: 1}},
			msgDef{send: &drop{ID: 1}},
		))
	})
}

func Test_Plugin_input(t *testing.T) {
	signature := PluginSignature{
		Name:             "inc",
		Category:         "Experimental",
		Usage:            "test cmd",
		SearchTerms:      []string{"foo"},
		InputOutputTypes: [][]string{{"Any", "Any"}},
	}

	t.Run("Empty input", func(t *testing.T) {
		p, err := New([]*Command{
			{
				Signature: signature,
				OnRun: func(ctx context.Context, exec *ExecCommand) error {
					switch vt := exec.Input.(type) {
					case nil:
					default:
						t.Errorf("unexpected input type %T", vt)
					}
					return nil
				},
			},
		},
			&Config{Logger: logger(t)},
		)
		if err != nil {
			t.Fatalf("creating plugin: %v", err)
		}

		runEngine(t, p, append(protocolPrelude,
			msgDef{send: &call{ID: 1, Call: run{Name: "inc", Input: nil}}},
		))
	})

	t.Run("Single Value", func(t *testing.T) {
		p, err := New([]*Command{
			{
				Signature: signature,
				OnRun: func(ctx context.Context, exec *ExecCommand) error {
					switch vt := exec.Input.(type) {
					case Value:
						if vt.Value != "input" {
							t.Errorf("expected 'input' got %q", vt.Value)
						}
					default:
						t.Errorf("unexpected input type %T", vt)
					}
					return nil
				},
			},
		},
			&Config{Logger: logger(t)},
		)
		if err != nil {
			t.Fatalf("creating plugin: %v", err)
		}

		runEngine(t, p, append(protocolPrelude,
			msgDef{send: &call{ID: 1, Call: run{Name: "inc", Input: Value{Value: "input"}}}},
		))
	})

	t.Run("Value List", func(t *testing.T) {
		p, err := New([]*Command{
			{
				Signature: signature,
				OnRun: func(ctx context.Context, exec *ExecCommand) error {
					var in <-chan Value
					switch vt := exec.Input.(type) {
					case <-chan Value:
						if vt == nil {
							t.Errorf("input stream is nil")
						}
						in = vt
					default:
						t.Errorf("unexpected input type %T", vt)
					}
					for v := range in {
						if v.Value != "first" {
							t.Errorf("expected 'first' got %v", v.Value)
						}
					}
					return nil
				},
			},
		},
			&Config{Logger: logger(t)},
		)
		if err != nil {
			t.Fatalf("creating plugin: %v", err)
		}

		runEngine(t, p, append(protocolPrelude,
			msgDef{send: &call{ID: 1, Call: run{Name: "inc", Input: listStream{ID: 7}}}},
			msgDef{send: &data{ID: 7, Data: Value{Value: "first"}}},
			msgDef{recv: ack{ID: 7}},
			msgDef{send: &end{ID: 7}},
			msgDef{recv: drop{ID: 7}},
			msgDef{recv: callResponse{ID: 1, Response: pipelineData{empty{}}}},
		))
	})
}

func runEngine(t *testing.T, p *Plugin, msg []msgDef) {
	t.Helper()

	engineIn, pluginOut := io.Pipe()
	pluginIn, engineOut := io.Pipe()
	p.in, p.out = pluginIn, pluginOut

	errch := make(chan error, 3)
	var wg sync.WaitGroup
	wg.Add(3)

	engOut := make(chan []byte, 1)
	go func() {
		defer func() {
			engineOut.Close()
			wg.Done()
		}()
		for b := range engOut {
			if _, err := engineOut.Write(b); err != nil {
				errch <- fmt.Errorf("failed to write into engine output: %w", err)
			}
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		defer func() {
			pluginIn.Close()
			//io.ReadAll(pluginIn)
			pluginOut.Close()
			engineIn.Close()
			wg.Done()
		}()
		if err := p.Run(ctx); err != nil {
			errch <- fmt.Errorf("Run exited with error: %w", err)
		}
	}()

	go func() {
		defer func() {
			close(engOut)
			engineIn.Close()
			//io.ReadAll(engineIn)
			wg.Done()
		}()

		dec := msgpack.NewDecoder(engineIn)
		dec.SetMapDecoder(decodeNuMsgAll(handleMsgDecode))

		for k, v := range msg {
			if v.recv != nil {
				inmsg, err := dec.DecodeInterface()
				if err != nil {
					errch <- fmt.Errorf("decoding msg [%d]: %w", k, err)
				}
				if diff := cmp.Diff(v.recv, inmsg); diff != "" {
					errch <- fmt.Errorf("[%d] message mismatch (-want +got):\n%s", k, diff)
				}
			} else {
				buf, err := v.msgBytes()
				if err != nil {
					errch <- fmt.Errorf("encoding message [%d]: %w", k, err)
				}
				p.log.Debug("engine sends", "msg", buf)
				engOut <- buf
			}
		}

		// launch goroutine which dumps input into err?

		/*for {
			inmsg, err := dec.DecodeInterface()
			if err != nil {
				errch <- fmt.Errorf("decoding unexpected msg: %w", err)
				return
			}
			errch <- fmt.Errorf("received unexpected message from plugin: %#v", inmsg)
		}*/
	}()

	go func() {
		wg.Wait()
		close(errch)
	}()

	for e := range errch {
		t.Error(e)
		cancel()
	}
}

type msgDef struct {
	send any // engine sends message to plugin
	recv any // plugin sends message to engine
}

/*
msgBytes returns the message described by the def as bytes.
*/
func (md msgDef) msgBytes() ([]byte, error) {
	var msg any
	switch {
	case md.recv != nil:
		msg = md.recv
	case md.send != nil:
		msg = md.send
	}
	buf, err := msgpack.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("encoding msg as MessagePack: %v", err)
	}
	return buf, nil
}

// plugin protocol "handshake" sent and received by the Plugin.
// contains 10 messages [0..9] so the first message in test is index 10
var protocolPrelude = []msgDef{
	// length of encoding marker
	{recv: int8(7)},
	// "msgpack"
	{recv: int8(0x6d)},
	{recv: int8(0x73)},
	{recv: int8(0x67)},
	{recv: int8(0x70)},
	{recv: int8(0x61)},
	{recv: int8(0x63)},
	{recv: int8(0x6b)},
	{recv: hello{Protocol: protocol_name, Version: protocol_version, Features: features{LocalSocket: true}}},
	{send: &hello{Protocol: "nu-plugin", Version: "0.92.2"}},
}
