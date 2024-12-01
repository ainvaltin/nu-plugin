package nu

import (
	"context"
	"fmt"
	"io"
)

func newInputStreamRaw(id int) *rawStreamIn {
	out := &rawStreamIn{
		id:  id,
		buf: make(chan []byte, 10),
	}
	out.rdr, out.data = io.Pipe()
	return out
}

type rawStreamIn struct {
	id    int
	buf   chan []byte
	onAck func(ctx context.Context, id int) // plugin has consumed the latest Data msg
	data  io.WriteCloser
	rdr   io.ReadCloser
}

func (lsi *rawStreamIn) Run(ctx context.Context) {
	up := make(chan struct{})

	go func() {
		defer lsi.data.Close()
		close(up)
		for {
			select {
			case in, ok := <-lsi.buf:
				if !ok {
					return
				}
				// todo: check for error - user closed the reader to signal to drop the stream?
				lsi.data.Write(in)
				lsi.onAck(ctx, lsi.id)
			case <-ctx.Done():
				return
			}
		}
	}()

	<-up
}

func (lsi *rawStreamIn) received(ctx context.Context, v any) error {
	in, ok := v.([]byte)
	if !ok {
		return fmt.Errorf("raw stream input must be of type []byte, got %T", v)
	}
	lsi.buf <- in
	return nil
}

func (lsi *rawStreamIn) endOfData() {
	close(lsi.buf)
}

func newInputStreamList(id int) *listStreamIn {
	in := &listStreamIn{
		id:   id,
		data: make(chan Value),
		buf:  make(chan Value, 10),
	}
	return in
}

type listStreamIn struct {
	id   int
	data chan Value // incoming data to be consumed by plugin

	buf chan Value

	// this callback is triggered to signal that the last item received
	// has been processed, consumer is ready for the next one
	onAck func(ctx context.Context, id int)
}

// return (readonly) chan to the command's Run handler
func (lsi *listStreamIn) InputStream() <-chan Value {
	return lsi.data
}

func (lsi *listStreamIn) Run(ctx context.Context) {
	// hackish way to make sure that when this func returns the
	// goroutine is running. otherwise ie tests are flaky...
	up := make(chan struct{})

	go func() {
		defer close(lsi.data)
		close(up)
		for {
			select {
			case in, ok := <-lsi.buf:
				if !ok {
					return
				}
				select {
				case lsi.data <- in:
					lsi.onAck(ctx, lsi.id)
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	<-up
}

// main loop calls on Data msg to given stream
func (lsi *listStreamIn) received(ctx context.Context, v any) error {
	in, ok := v.(Value)
	if !ok {
		return fmt.Errorf("list stream input must be of type Value, got %T", v)
	}
	lsi.buf <- in
	return nil
}

// main loop signals there will be no more data for the stream
// ctx with timeout for how long wait?
func (lsi *listStreamIn) endOfData() {
	close(lsi.buf)
}
