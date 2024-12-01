package nu

import (
	"context"
	"crypto/rand"
	"hash/crc64"
	"io"
	"sync"
	"testing"
	"time"
)

func Test_rawStreamIn(t *testing.T) {
	t.Run("input must be byte slice", func(t *testing.T) {
		rs := newInputStreamRaw(11)

		err := rs.received(context.Background(), 33)
		expectErrorMsg(t, err, `raw stream input must be of type []byte, got int`)

		err = rs.received(context.Background(), nil)
		expectErrorMsg(t, err, `raw stream input must be of type []byte, got <nil>`)
	})

	t.Run("data sent without Ack", func(t *testing.T) {
		t.Skip("engine doesn't wait for Ack before sending next Data msg")
		rs := newInputStreamRaw(1)
		rs.onAck = func(ctx context.Context, id int) { t.Error("unexpected call") }
		rs.Run(context.Background())
		if err := rs.received(context.Background(), []byte{1}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// receiving next one before Ack-ing previous one
		err := rs.received(context.Background(), []byte{2})
		expectErrorMsg(t, err, `received new Data before Ack-ing previous one?`)
	})

	t.Run("attempt to write after end of data signal", func(t *testing.T) {
		rs := newInputStreamRaw(1)
		rs.onAck = func(ctx context.Context, id int) { t.Error("unexpected call") }
		rs.Run(context.Background())
		rs.endOfData()
		_, err := rs.data.Write([]byte{8})
		expectErrorMsg(t, err, `io: read/write on closed pipe`)
	})

	t.Run("producer and consumer", func(t *testing.T) {
		acked := make(chan struct{})
		rs := newInputStreamRaw(20)
		rs.onAck = func(ctx context.Context, id int) { acked <- struct{}{} }
		rs.Run(context.Background())

		var sumW uint64
		go func() {
			buf := make([]byte, 10)
			cc := crc64.New(crc64.MakeTable(crc64.ISO))
			for i := 0; i < 20; i++ {
				if _, err := rand.Read(buf); err != nil {
					t.Errorf("reading rand: %v", err)
				}
				if _, err := cc.Write(buf); err != nil {
					t.Errorf("writing to CRC: %v", err)
				}
				if err := rs.received(context.Background(), buf); err != nil {
					t.Errorf("sending data to stream: %v", err)
				}
				<-acked
			}
			sumW = cc.Sum64()
			rs.endOfData()
		}()

		cc := crc64.New(crc64.MakeTable(crc64.ISO))
		if _, err := io.Copy(cc, rs.rdr); err != nil {
			t.Errorf("reading input: %v", err)
		}

		if sumR := cc.Sum64(); sumR != sumW {
			t.Errorf("CRC doesn't match: expected %d, got %d", sumW, sumR)
		}
	})
}

func Test_listStreamIn(t *testing.T) {
	t.Run("input must be of type Value", func(t *testing.T) {
		ls := newInputStreamList(1)

		err := ls.received(context.Background(), &Value{Value: 2})
		expectErrorMsg(t, err, `list stream input must be of type Value, got *nu.Value`)

		err = ls.received(context.Background(), 7)
		expectErrorMsg(t, err, `list stream input must be of type Value, got int`)
	})

	t.Run("data sent without Ack", func(t *testing.T) {
		t.Skip("engine doesn't wait for Ack before sending next Data msg")
		ls := newInputStreamList(1)
		ls.onAck = func(ctx context.Context, id int) {}
		ls.Run(context.Background())
		if err := ls.received(context.Background(), Value{Value: 2}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		// receiving next one before Ack-ing previous one
		err := ls.received(context.Background(), Value{Value: 3})
		expectErrorMsg(t, err, `received new Data before Ack-ing previous one?`)
	})

	t.Run("Acking before next receive", func(t *testing.T) {
		// normal use case, check that onAck event is triggered when data is consumed
		onAckCalled := make(chan struct{})
		ls := newInputStreamList(1)
		ls.onAck = func(ctx context.Context, id int) {
			if id != 1 {
				t.Errorf("expected Ack callback for stream with ID 1, got %d", id)
			}
			close(onAckCalled)
		}
		ls.Run(context.Background())

		if err := ls.received(context.Background(), Value{Value: 2}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// consumer reads the input
		v := <-ls.InputStream()
		if v.Value != 2 {
			t.Errorf("expected to get value 2, got %v", v.Value)
		}
		select {
		case <-onAckCalled:
		case <-time.After(time.Second):
			t.Error("no ACK")
		}

		// should be able to send next value
		if err := ls.received(context.Background(), Value{Value: 3}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("signal end of data", func(t *testing.T) {
		// signaling end of data before last item has been consumed mustn't lose
		// the last item (even tho EOD should be singnalled only after Ack?)
		onAckCalled := make(chan struct{})
		ls := newInputStreamList(1)
		ls.onAck = func(ctx context.Context, id int) {
			close(onAckCalled)
		}
		ls.Run(context.Background())

		if err := ls.received(context.Background(), Value{Value: 8}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		ls.endOfData()

		// consumer reads the input
		v := <-ls.InputStream()
		if v.Value != 8 {
			t.Errorf("expected to get value 2, got %v", v.Value)
		}
		select {
		case <-onAckCalled:
		case <-time.After(time.Second):
			t.Error("no ACK")
		}
		// stream must get closed now than the item has been consumed
		select {
		case v, ok := <-ls.InputStream():
			if ok {
				t.Errorf("got unexpected value %#v", v)
			}
		case <-time.After(time.Second):
			t.Error("stream not closed")
		}
	})

	t.Run("producer and consumer", func(t *testing.T) {
		acked := make(chan struct{})

		ls := newInputStreamList(20)
		ls.onAck = func(ctx context.Context, id int) { acked <- struct{}{} }
		ls.Run(context.Background())
		wg := sync.WaitGroup{}
		wg.Add(2)

		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				if err := ls.received(context.Background(), Value{Value: i}); err != nil {
					t.Errorf("sending Value to stream: %v", err)
				}
				<-acked
			}
			ls.endOfData()
		}()

		var sum int
		go func() {
			defer wg.Done()
			for v := range ls.InputStream() {
				sum += v.Value.(int)
			}
		}()

		wg.Wait()
		if sum != 190 {
			t.Errorf("expected 190, got %d", sum)
		}
	})
}
