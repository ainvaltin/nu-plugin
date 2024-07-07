package nu

import (
	"fmt"
	"log/slog"
	"reflect"

	"github.com/vmihailenco/msgpack/v5"
)

func attrError(err error) slog.Attr {
	return slog.Any("error", err)
}

func attrMsg(msg any) slog.Attr {
	switch reflect.TypeOf(msg).Kind() {
	case reflect.Struct:
		return slog.Any("message", fmt.Sprintf("%#v", msg))
	}
	return slog.Any("message", msg)
}

func attrStreamID(id int) slog.Attr {
	return slog.Int("stream_id", id)
}

func attrCallID(id int) slog.Attr {
	return slog.Int("call_id", id)
}

/*
encodeMapStart outputs a map with single key named "key", caller
must output the value:

	{ key: | }
*/
func encodeMapStart(enc *msgpack.Encoder, key string) error {
	if err := enc.EncodeMapLen(1); err != nil {
		return err
	}
	if err := enc.EncodeString(key); err != nil {
		return err
	}
	return nil
}

/*
encodeTupleInMap outputs map with single key "key" whose value is tuple
[id, ? ], the caller must output tuple's second item.

	{ key: [ id, | ] }
*/
func encodeTupleInMap(enc *msgpack.Encoder, key string, id int) error {
	if err := enc.EncodeMapLen(1); err != nil {
		return err
	}
	if err := enc.EncodeString(key); err != nil {
		return err
	}
	if err := enc.EncodeArrayLen(2); err != nil {
		return err
	}
	if err := enc.EncodeInt(int64(id)); err != nil {
		return err
	}
	return nil
}

/*
decodeTupleStart reads the start of a "2-tuple (array): [id, <something>]" and
returns the ID part, the decoder is in the start of <something> when nil error
is returned.
*/
func decodeTupleStart(d *msgpack.Decoder) (int, error) {
	n, err := d.DecodeArrayLen()
	if err != nil {
		return 0, fmt.Errorf("reading tuple array length: %w", err)
	}
	if n != 2 {
		return 0, fmt.Errorf("unexpected tuple array length %d", n)
	}

	id, err := d.DecodeInt()
	if err != nil {
		return 0, fmt.Errorf("reading data stream ID: %w", err)
	}
	return id, nil
}

/*
decodeWrapperMap reads the "single item map" whose key is string - the
key name is returned and decoder is ready to read the value.
*/
func decodeWrapperMap(dec *msgpack.Decoder) (string, error) {
	cnt, err := dec.DecodeMapLen()
	if err != nil {
		return "", fmt.Errorf("reading map length: %w", err)
	}
	if cnt != 1 {
		return "", fmt.Errorf("wrapper map is expected to contain one item, got %d", cnt)
	}

	keyName, err := dec.DecodeString()
	if err != nil {
		return "", fmt.Errorf("reading map key: %w", err)
	}
	return keyName, nil
}
