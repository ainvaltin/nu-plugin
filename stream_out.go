package nu

import (
	"context"
	"fmt"
	"io"
	"sync"
)

func newOutputListRaw(id int, opts ...RawStreamOption) *rawStreamOut {
	out := &rawStreamOut{
		id:   id,
		sent: make(chan struct{}, 1),
		cfg:  rawStreamCfg{bufSize: 1024, dataType: "Unknown"},
	}
	out.rdr, out.data = io.Pipe()

	for _, opt := range opts {
		opt.apply(&out.cfg)
	}

	return out
}

type rawStreamOut struct {
	id     int
	data   io.WriteCloser // input from plugin
	rdr    *io.PipeReader
	sent   chan struct{} // has the latest Data msg been Ack-ed?
	onSend func(ID int, b []byte) error
	_close func()
	onDrop func()
	cfg    rawStreamCfg
}

func (rc *rawStreamOut) setOnDone(onDone func(id int)) {
	rc._close = sync.OnceFunc(func() { onDone(rc.id) })
}

func (rc *rawStreamOut) streamID() int { return rc.id }

func (rc *rawStreamOut) pipelineDataHdr() any {
	return &byteStream{ID: rc.id, Type: rc.cfg.dataType}
}

func (rc *rawStreamOut) read() ([]byte, error) {
	buf := make([]byte, rc.cfg.bufSize)
	sp := 0
	for {
		n, err := rc.rdr.Read(buf[sp:])
		sp += n
		if sp == len(buf) || err != nil {
			return buf[:sp], err
		}
	}
}

func (rc *rawStreamOut) run(ctx context.Context) error {
	defer func() {
		rc.rdr.Close()
		rc.data.Close()
	}()

	for eof := false; !eof; {
		buf, err := rc.read()
		switch err {
		case nil:
		case io.EOF:
			eof = true
		default:
			return fmt.Errorf("reading data: %w", err)
		}
		if len(buf) > 0 {
			if err := rc.onSend(rc.id, buf); err != nil {
				return fmt.Errorf("sending data: %w", err)
			}
			// use select and check for cxt.Done?
			<-rc.sent
		}
	}

	return rc.close()
}

func (rc *rawStreamOut) ack() error {
	select {
	case rc.sent <- struct{}{}:
		return nil
	default:
		return fmt.Errorf("received unexpected Ack")
	}
}

func (rc *rawStreamOut) close() error {
	rc._close()
	return nil
}

func (rc *rawStreamOut) drop() {
	if rc.onDrop != nil {
		rc.onDrop()
	}
	rc.rdr.CloseWithError(ErrDropStream)
}

func newOutputListValue(id int) *listStreamOut {
	out := &listStreamOut{
		id:   id,
		sent: make(chan struct{}, 1),
		data: make(chan Value),
	}
	return out
}

type listStreamOut struct {
	id     int
	sent   chan struct{}
	data   chan Value
	onSend func(ID int, v Value) error
	onDrop func()
	_close func()
}

func (rc *listStreamOut) setOnDone(onDone func(id int)) {
	rc._close = sync.OnceFunc(func() { onDone(rc.id) })
}

func (rc *listStreamOut) streamID() int { return rc.id }

func (rc *listStreamOut) pipelineDataHdr() any { return &listStream{ID: rc.id} }

func (rc *listStreamOut) run(ctx context.Context) error {
main_loop:
	for {
		select {
		case v, ok := <-rc.data:
			if !ok {
				break main_loop
			}
			if err := rc.onSend(rc.id, v); err != nil {
				return fmt.Errorf("send: %w", err)
			}
		case <-ctx.Done():
			return ctx.Err()
		}

		select {
		case <-rc.sent:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return rc.close()
}

// main loop calls to signal that last send was ack-ed
func (rc *listStreamOut) ack() error {
	select {
	case rc.sent <- struct{}{}:
		return nil
	default:
		return fmt.Errorf("received unexpected Ack")
	}
}

func (rc *listStreamOut) close() error {
	rc._close()
	return nil
}

func (rc *listStreamOut) drop() {
	// closing the chan will cause panic on send so don't do that!
	if rc.onDrop != nil {
		rc.onDrop()
	}
}
