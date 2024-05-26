package nu

import (
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

type hello struct {
	Protocol string   `msgpack:"protocol"`
	Version  string   `msgpack:"version"`
	Features features `msgpack:"features"`
}

type features struct {
	LocalSocket bool
}

var _ msgpack.CustomEncoder = (*hello)(nil)

func (h *hello) EncodeMsgpack(enc *msgpack.Encoder) error {
	if err := encodeMapStart(enc, "Hello"); err != nil {
		return err
	}

	if err := enc.EncodeMapLen(3); err != nil {
		return err
	}
	if err := enc.EncodeString("protocol"); err != nil {
		return err
	}
	if err := enc.EncodeString(h.Protocol); err != nil {
		return err
	}
	if err := enc.EncodeString("version"); err != nil {
		return err
	}
	if err := enc.EncodeString(h.Version); err != nil {
		return err
	}
	if err := enc.EncodeString("features"); err != nil {
		return err
	}
	if err := h.EncodeMsgpackFeatures(enc); err != nil {
		return fmt.Errorf("encoding features: %w", err)
	}

	return nil
}

func (h *hello) EncodeMsgpackFeatures(enc *msgpack.Encoder) error {
	cnt := 0
	if h.Features.LocalSocket {
		cnt++
	}
	if err := enc.EncodeArrayLen(cnt); err != nil {
		return err
	}
	if h.Features.LocalSocket {
		if err := enc.EncodeMapLen(1); err != nil {
			return err
		}
		if err := enc.EncodeString("name"); err != nil {
			return err
		}
		if err := enc.EncodeString("LocalSocket"); err != nil {
			return err
		}
	}
	return nil
}

var _ msgpack.CustomDecoder = (*features)(nil)

func (f *features) DecodeMsgpack(dec *msgpack.Decoder) error {
	cnt, err := dec.DecodeArrayLen()
	if err != nil {
		return err
	}
	if cnt < 1 {
		return nil
	}
	for idx := 0; idx < cnt; idx++ {
		ftre, err := dec.DecodeMap()
		if err != nil {
			return err
		}
		f.LocalSocket = f.LocalSocket || ftre["name"] == "LocalSocket"
	}
	return nil
}
