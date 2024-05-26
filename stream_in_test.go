package nu

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

func Test_listStreamIn(t *testing.T) {
	t.Run("data sent without Ack", func(t *testing.T) {
		ls := newInputStreamList(1)
		ls.ack = func(ID int) {}
		if err := ls.received(context.Background(), Value{Value: 2}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		// receiving next one before Ack-ing previous one
		if err := ls.received(context.Background(), Value{Value: 3}); err != nil {
			if err.Error() != `received new Data before Ack-ing previous one?` {
				t.Errorf("got error but unexpected message: %s", err)
			}
		} else {
			t.Error("expected error, not none")
		}
	})

	t.Run("Acking before next receive", func(t *testing.T) {
		acked := make(chan struct{})
		ls := newInputStreamList(1)
		ls.ack = func(ID int) {
			if ID != 1 {
				t.Errorf("expected Ack callback for stream 1, got %d", ID)
			}
			close(acked)
		}

		if err := ls.received(context.Background(), Value{Value: 2}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// consumer read the input
		v := <-ls.InputStream()
		if v.Value != 2 {
			t.Errorf("expected to get value 2, got %v", v.Value)
		}
		<-acked

		// should be able to send next value
		if err := ls.received(context.Background(), Value{Value: 3}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("producer and consumer", func(t *testing.T) {
		acked := make(chan struct{})

		ls := newInputStreamList(20)
		ls.ack = func(ID int) { acked <- struct{}{} }
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

		var sum atomic.Int64
		go func() {
			defer wg.Done()
			for v := range ls.InputStream() {
				sum.Add(int64(v.Value.(int)))
			}
		}()

		wg.Wait()
		if v := sum.Load(); v != 190 {
			t.Errorf("expected 190, got %d", v)
		}
	})
}
