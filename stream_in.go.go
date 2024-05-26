package nu

import (
	"context"
	"fmt"
	"io"
	"time"
)

func newInputStreamRaw(id int) *rawStreamIn {
	out := &rawStreamIn{
		id:       id,
		inFlight: make(chan struct{}, 1),
	}
	out.rdr, out.data = io.Pipe()
	return out
}

type rawStreamIn struct {
	id       int
	inFlight chan struct{}
	ack      func(ID int) // plugin has consumed the last Data msg
	data     io.WriteCloser
	rdr      io.ReadCloser
}

func (lsi *rawStreamIn) received(ctx context.Context, v any) error {
	in, ok := v.([]byte)
	if !ok {
		return fmt.Errorf("raw stream input must be of type []byte, got %T", v)
	}
	select {
	case lsi.inFlight <- struct{}{}:
	default:
		return fmt.Errorf("received new Data before Ack-ing last one?")
	}

	go func() {
		lsi.data.Write(in)
		<-lsi.inFlight
		lsi.ack(lsi.id)
	}()

	return nil
}
func (lsi *rawStreamIn) endOfData() {
	go func() {
		select {
		case lsi.inFlight <- struct{}{}:
			lsi.data.Close()
		case <-time.After(10 * time.Second): // ctx param with TO?
		}
	}()
}

func newInputStreamList(id int) *listStreamIn {
	in := &listStreamIn{
		ID:       id,
		data:     make(chan Value),
		inFlight: make(chan struct{}, 1),
	}
	return in
}

type listStreamIn struct {
	ID       int
	data     chan Value // incoming data to be consumed by plugin
	inFlight chan struct{}
	ack      func(ID int) // plugin has consumed the last Data msg
}

func (lsi *listStreamIn) InputStream() <-chan Value {
	return lsi.data
}

// main loop calls on Data msg to given stream
func (lsi *listStreamIn) received(ctx context.Context, v any) error {
	in, ok := v.(Value)
	if !ok {
		return fmt.Errorf("list stream input must be of type Value, got %T", v)
	}

	select {
	case lsi.inFlight <- struct{}{}:
	default:
		return fmt.Errorf("received new Data before Ack-ing previous one?")
	}

	go func() {
		select {
		case lsi.data <- in:
			<-lsi.inFlight
			lsi.ack(lsi.ID)
		case <-ctx.Done():
			return
		}
	}()

	return nil
}

// main loop signals there will be no more data for the stream
// ctx with timeout for how long wait?
func (lsi *listStreamIn) endOfData() {
	go func() {
		select {
		case lsi.inFlight <- struct{}{}:
			close(lsi.data)
		case <-time.After(10 * time.Second): //panic!? ctx as a param?
		}
	}()
}
