package nu

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func Test_rawStreamOut(t *testing.T) {
	t.Run("sending data blocks until Ack-ed", func(t *testing.T) {
		consumer := bytes.NewBuffer(nil)
		ls := initOutputListRaw(1)
		ls.cfg.bufSize = 5
		ls.onSend = func(ctx context.Context, id int, v []byte) error { _, err := consumer.Write(v); return err }
		ls.setOnDone(func(ctx context.Context, id int) error { return nil })

		runDone := make(chan error)
		go func() {
			runDone <- ls.run(context.Background())
		}()

		// first write should be accepted without delay
		// it is exactly the buffer size so data will be sent to the
		// consumer and we'll wait for Ack
		ls.data.Write(bytes.Repeat([]byte{1}, int(ls.cfg.bufSize)))

		// second write should not succeed as previous has not been Ack-ed
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

		// Ack first send and second one should be accepted
		ls.ack()
		select {
		case <-secWrite:
		case <-time.After(500 * time.Millisecond):
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
		case <-time.After(time.Second):
			t.Error("run hasn't exited")
		}
		// did we get expected data in the consumer side
		if diff := cmp.Diff(consumer.Bytes(), slices.Concat(bytes.Repeat([]byte{1}, int(ls.cfg.bufSize)), bytes.Repeat([]byte{2}, int(ls.cfg.bufSize)))); diff != "" {
			t.Errorf("data mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("multiple writes collected and sent", func(t *testing.T) {
		ls := initOutputListRaw(1)
		ls.setOnDone(func(ctx context.Context, id int) error { return nil })
		// we set buf size so big that we do not expect any onSend calls
		ls.cfg.bufSize = 1024
		ls.onSend = func(ctx context.Context, id int, v []byte) error {
			t.Errorf("unexpected onSend call with %x", v)
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
		ls.onSend = func(ctx context.Context, id int, v []byte) error { _, err := consumer.Write(v); return err }
		if err := ls.data.Close(); err != nil {
			t.Errorf("unexpected error closing the writer: %v", err)
		}
		// ack allows "run" to exit and makes sure that the onSend has
		// time to happen (run triggers it in a goroutine)
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
		if diff := cmp.Diff(consumer.Bytes(), slices.Concat(bytes.Repeat([]byte{1}, 10), bytes.Repeat([]byte{2}, 10), bytes.Repeat([]byte{3}, 10))); diff != "" {
			t.Errorf("data mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("not sending anything", func(t *testing.T) {
		ls := initOutputListRaw(1)
		ls.onSend = func(ctx context.Context, id int, v []byte) error { t.Errorf("unexpected call: %v", v); return nil }

		var onDoneCalled atomic.Bool
		ls.setOnDone(func(ctx context.Context, id int) error { onDoneCalled.Store(true); return nil })

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

		if !onDoneCalled.Load() {
			t.Error("onDone hasn't been called")
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
		ls.onSend = func(ctx context.Context, id int, v Value) error { return nil }
		<-ls.closer
		ls.setOnDone(func(ctx context.Context, id int) error { return nil })

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

	t.Run("onDone is called", func(t *testing.T) {
		done := make(chan struct{})
		p := &Plugin{}
		p.idGen.Add(76)
		ls := newOutputListValue(p)
		ls.onSend = func(ctx context.Context, id int, v Value) error { return nil }
		<-ls.closer
		ls.setOnDone(func(ctx context.Context, id int) error {
			if id != 77 {
				t.Errorf("expected stream id 77, got %d", id)
			}
			close(done)
			return nil
		})

		runDone := make(chan error)
		go func() {
			runDone <- ls.run(context.Background())
		}()

		close(ls.data)

		// closing chan should cause "normal exit" with onDone called
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("haven't got onDone signal")
		}

		// and "run" should exit with nil error
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
		ls.onSend = func(ctx context.Context, id int, v Value) error { return nil }
		<-ls.closer
		ls.setOnDone(func(ctx context.Context, id int) error { t.Error("shouldn't be called"); return nil })

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
		ls.onSend = func(ctx context.Context, id int, v Value) error { return nil }
		<-ls.closer
		ls.setOnDone(func(ctx context.Context, id int) error { t.Error("shouldn't be called"); return nil })

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
