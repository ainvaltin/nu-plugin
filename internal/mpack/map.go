package mpack

import (
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

/*
EncodeMapStart outputs a map with single key named "key", caller
must output the value:

	{ key: | }
*/
func EncodeMapStart(enc *msgpack.Encoder, key string) error {
	if err := enc.EncodeMapLen(1); err != nil {
		return fmt.Errorf("encoding map length: %w", err)
	}
	if err := enc.EncodeString(key); err != nil {
		return fmt.Errorf("encoding map key %q: %w", key, err)
	}
	return nil
}

/*
DecodeWrapperMap reads the "single item map" whose key is string - the
key name is returned and decoder is ready to read the value.
It is suitable to read what EncodeMapStart produced.
*/
func DecodeWrapperMap(dec *msgpack.Decoder) (string, error) {
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
