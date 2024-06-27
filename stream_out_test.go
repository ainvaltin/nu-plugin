package nu

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func Test_rawStreamOut(t *testing.T) {
	t.Run("sending data blocks until Ack-ed", func(t *testing.T) {
		consumer := bytes.NewBuffer(nil)
		ls := initOutputListRaw(1)
		ls.cfg.bufSize = 5
		ls.sender = func(ctx context.Context, d any) error {
			v := d.(*data)
			_, err := consumer.Write(v.Data.([]byte))
			return err
		}

		runDone := make(chan error)
		go func() {
			runDone <- ls.run(context.Background())
		}()

		// first write should be accepted without delay
		// it is exactly the buffer size so data will be sent to the
		// consumer and we'll wait for Ack
		ls.data.Write(bytes.Repeat([]byte{1}, int(ls.cfg.bufSize)))

		// second write should block as previous has not been Ack-ed
		secWrite := make(chan struct{})
		go func() {
			defer close(secWrite)
			ls.data.Write(bytes.Repeat([]byte{2}, int(ls.cfg.bufSize)))
		}()

		select {
		case <-secWrite:
			t.Fatalf("second write was accepted without Ack")
		case <-time.After(1000 * time.Millisecond):
		}

		// Ack first send and second write should procceed
		ls.ack()
		select {
		case <-secWrite:
		case <-time.After(1000 * time.Millisecond):
			t.Fatalf("second write was NOT accepted")
		}

		// Ack the second write too so run would react to writer being closed
		ls.ack()
		if err := ls.data.Close(); err != nil {
			t.Errorf("unexpected error closing the writer: %v", err)
		}
		select {
		case err := <-runDone:
			if err != nil {
				t.Errorf("run exited with unexpected error: %v", err)
			}
		case <-time.After(1000 * time.Millisecond):
			t.Error("run hasn't exited")
		}
		// did we get expected data in the consumer side
		expect := slices.Concat(bytes.Repeat([]byte{1}, int(ls.cfg.bufSize)), bytes.Repeat([]byte{2}, int(ls.cfg.bufSize)))
		if diff := cmp.Diff(consumer.Bytes(), expect); diff != "" {
			t.Errorf("data mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("multiple writes collected and sent", func(t *testing.T) {
		ls := initOutputListRaw(1)
		// we set buf size so big that we do not expect any send calls
		ls.cfg.bufSize = 1024
		ls.sender = func(ctx context.Context, d any) error {
			t.Errorf("unexpected Send call with %#v", d)
			return nil
		}

		runDone := make(chan error)
		go func() {
			runDone <- ls.run(context.Background())
		}()

		// multiple writes should succeed without blocking as buffer is
		// bigger than the data sent by the plugin
		ls.data.Write(bytes.Repeat([]byte{1}, 10))
		ls.data.Write(bytes.Repeat([]byte{2}, 10))
		ls.data.Write(bytes.Repeat([]byte{3}, 10))
		// closing the writer should trigger sending the data
		consumer := bytes.NewBuffer(nil)
		ls.sender = func(ctx context.Context, d any) error {
			v := d.(*data)
			_, err := consumer.Write(v.Data.([]byte))
			return err
		}
		if err := ls.data.Close(); err != nil {
			t.Errorf("unexpected error closing the writer: %v", err)
		}
		// ack allows "run" to exit
		ls.ack()
		select {
		case err := <-runDone:
			if err != nil {
				t.Errorf("run exited with unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("run hasn't exited")
		}
		// make sure we got the same data in the output we wrote into writer
		expect := slices.Concat(bytes.Repeat([]byte{1}, 10), bytes.Repeat([]byte{2}, 10), bytes.Repeat([]byte{3}, 10))
		if diff := cmp.Diff(consumer.Bytes(), expect); diff != "" {
			t.Errorf("data mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("simulate engine", func(t *testing.T) {
		// simulate communication between plugin and engine using goroutines
		pluginIn := bytes.Repeat([]byte{1, 2, 0}, 20)
		pluginOut := bytes.NewBuffer(nil)
		engine := make(chan []byte, 1)

		ls := initOutputListRaw(1)

		go func() {
			for v := range engine {
				pluginOut.Write(v)
				time.Sleep(100 * time.Millisecond)
				if err := ls.ack(); err != nil {
					t.Errorf("ACK: %v", err)
				}
			}
		}()

		// the buf size controls how many send operations will it take to send all the data
		ls.cfg.bufSize = uint(len(pluginIn) / 5)
		ls.sender = func(ctx context.Context, d any) error {
			go func() {
				time.Sleep(50 * time.Millisecond)
				v := d.(*data)
				engine <- v.Data.([]byte)
			}()
			return nil
		}

		runDone := make(chan error)
		go func() {
			runDone <- ls.run(context.Background())
		}()
		ls.data.Write(pluginIn)
		ls.data.Close()

		select {
		case err := <-runDone:
			if err != nil {
				t.Errorf("run exited with unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("run hasn't exited")
		}
		// make sure we got the same data in the output we wrote into writer
		if diff := cmp.Diff(pluginOut.Bytes(), pluginIn); diff != "" {
			t.Errorf("data mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("not sending anything", func(t *testing.T) {
		ls := initOutputListRaw(1)
		ls.sender = func(ctx context.Context, d any) error { t.Errorf("unexpected call: %#v", d); return nil }

		runDone := make(chan error)
		go func() {
			runDone <- ls.run(context.Background())
		}()

		// closing the writer should end the stream
		if err := ls.data.Close(); err != nil {
			t.Errorf("closing writer: %v", err)
		}

		select {
		case err := <-runDone:
			if err != nil {
				t.Errorf("run exited with unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("run hasn't exited")
		}
	})

	t.Run("two Ack-s in a row", func(t *testing.T) {
		ls := initOutputListRaw(77)
		if err := ls.ack(); err != nil {
			t.Errorf("first Ack should not have returned error but got: %v", err)
		}
		if err := ls.ack(); err != nil {
			if err.Error() != "received unexpected Ack" {
				t.Errorf("got error with unexpected message: %v", err)
			}
		} else {
			t.Error("second Ack should have returned error")
		}
	})
}

func Test_listStreamOut(t *testing.T) {
	t.Run("sending data blocks until Ack-ed", func(t *testing.T) {
		ls := newOutputListValue(&Plugin{})
		ls.sender = func(ctx context.Context, data any) error { return nil }

		runDone := make(chan error)
		go func() {
			runDone <- ls.run(context.Background())
		}()

		ch := ls.data
		// first send should be accepted without delay
		ch <- Value{Value: 1}

		// second send should not succeed as previous has not been Ack-ed
		select {
		case ch <- Value{Value: 2}:
			t.Fatalf("second send was accepted without Ack")
		case <-time.After(100 * time.Millisecond):
		}

		// Ack first send and second one should be accepted
		ls.ack()
		select {
		case ch <- Value{Value: 2}:
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("second send was NOT accepted")
		}

		// Ack the second send too so run would react to chan being closed
		ls.ack()
		close(ch)
		select {
		case err := <-runDone:
			if err != nil {
				t.Errorf("run exited with unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("run hasn't exited")
		}
	})

	t.Run("do not send anything", func(t *testing.T) {
		p := &Plugin{}
		p.idGen.Add(76)
		ls := newOutputListValue(p)

		runDone := make(chan error)
		go func() {
			runDone <- ls.run(context.Background())
		}()

		// closing chan should cause "normal exit"
		close(ls.data)

		select {
		case err := <-runDone:
			if err != nil {
				t.Errorf("run exited with unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("run hasn't exited")
		}
	})

	t.Run("ctx cancel stops the loop: waiting input", func(t *testing.T) {
		runDone := make(chan error)
		ls := newOutputListValue(&Plugin{})
		ls.sender = func(ctx context.Context, data any) error { return nil }

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			runDone <- ls.run(ctx)
		}()

		cancel()

		select {
		case err := <-runDone:
			if err == nil || !errors.Is(err, context.Canceled) {
				t.Errorf("run exited with unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("run hasn't exited")
		}
	})

	t.Run("ctx cancel stops the loop: waiting ack", func(t *testing.T) {
		runDone := make(chan error)
		ls := newOutputListValue(&Plugin{})
		ls.sender = func(ctx context.Context, data any) error { return nil }

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			runDone <- ls.run(ctx)
		}()

		ls.data <- Value{Value: 1}
		cancel()

		select {
		case err := <-runDone:
			if err == nil || !errors.Is(err, context.Canceled) {
				t.Errorf("run exited with unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("run hasn't exited")
		}
	})

	t.Run("two Ack-s in a row", func(t *testing.T) {
		ls := newOutputListValue(&Plugin{})
		if err := ls.ack(); err != nil {
			t.Errorf("first Ack should not have returned error but got: %v", err)
		}
		if err := ls.ack(); err != nil {
			if err.Error() != "received unexpected Ack" {
				t.Errorf("got error with unexpected message: %v", err)
			}
		} else {
			t.Error("second Ack should have returned error")
		}
	})
}
