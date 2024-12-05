package nu

import (
	"context"
	"fmt"
	"io"
)

func newOutputListRaw(p *Plugin, opts ...RawStreamOption) *rawStreamOut {
	out := initOutputListRaw(int(p.idGen.Add(1)), opts...)
	out.sender = p.outputMsg

	return out
}

func initOutputListRaw(id int, opts ...RawStreamOption) *rawStreamOut {
	out := &rawStreamOut{
		id:   id,
		done: make(chan struct{}),
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
	sender func(ctx context.Context, data any) error
	done   chan struct{}
	onDrop func()
	cfg    rawStreamCfg
}

func (rc *rawStreamOut) streamID() int { return rc.id }

func (rc *rawStreamOut) pipelineDataHdr() any {
	return &byteStream{ID: rc.id, Type: rc.cfg.dataType, MD: rc.cfg.md}
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
		close(rc.done)
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
			if err := rc.sender(ctx, &data{ID: rc.id, Data: buf}); err != nil {
				return fmt.Errorf("sending data: %w", err)
			}

			select {
			case <-rc.sent:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return nil
}

func (rc *rawStreamOut) ack() error {
	select {
	case rc.sent <- struct{}{}:
		return nil
	default:
		return fmt.Errorf("received unexpected Ack")
	}
}

func (rc *rawStreamOut) close(ctx context.Context) error {
	<-rc.done
	return rc.sender(ctx, end{ID: rc.id})
}

func (rc *rawStreamOut) drop() {
	if rc.onDrop != nil {
		rc.onDrop()
	}
	rc.rdr.CloseWithError(ErrDropStream)
}

func newOutputListValue(p *Plugin) *listStreamOut {
	out := &listStreamOut{
		id:     int(p.idGen.Add(1)),
		done:   make(chan struct{}),
		sent:   make(chan struct{}, 1),
		data:   make(chan Value),
		sender: p.outputMsg,
	}
	return out
}

type listStreamOut struct {
	id     int
	done   chan struct{}
	sent   chan struct{}
	data   chan Value
	sender func(ctx context.Context, data any) error
	onDrop func()
}

func (rc *listStreamOut) streamID() int { return rc.id }

func (rc *listStreamOut) pipelineDataHdr() any { return &listStream{ID: rc.id} }

func (rc *listStreamOut) run(ctx context.Context) error {
	defer close(rc.done)
	for {
		select {
		case v, ok := <-rc.data:
			if !ok {
				return nil
			}
			if err := rc.sender(ctx, &data{ID: rc.id, Data: v}); err != nil {
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

func (rc *listStreamOut) close(ctx context.Context) error {
	<-rc.done
	return rc.sender(ctx, end{ID: rc.id})
}

func (rc *listStreamOut) drop() {
	// closing the chan will cause panic on send so don't do that!
	if rc.onDrop != nil {
		rc.onDrop()
	}
}
