package mpack

import (
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

type MsgpackEncFunc func(enc *msgpack.Encoder) error

type MapItems []MsgpackEncFunc

/*
EncodeMap encodes fixed size map using the enc - it writes the number of items and
then calls each item to serialize the actual key - value pair.
*/
func (ef MapItems) EncodeMap(enc *msgpack.Encoder) error {
	if err := enc.EncodeMapLen(len(ef)); err != nil {
		return fmt.Errorf("encoding map length: %w", err)
	}
	for _, f := range ef {
		if err := f(enc); err != nil {
			return err
		}
	}
	return nil
}

/*func (ef MapItems) EncodeArray(enc *msgpack.Encoder) error {
	if err := enc.EncodeArrayLen(len(ef)); err != nil {
		return fmt.Errorf("encoding array length: %w", err)
	}
	for _, v := range ef {
		if err := v(enc); err != nil {
			return fmt.Errorf("encoding array item: %w", err)
		}
	}
	return nil
}*/

/*
AddOptionalStr adds the key/value only if the value is non empty string.
*/
func (ef *MapItems) AddOptionalStr(key, value string) {
	if value != "" {
		*ef = append(*ef, EncoderFuncString(key, value))
	}
}

/*
AddOptionalEncoder adds the encoder function only if the add flag is true.
*/
func (ef *MapItems) AddOptionalEncoder(add bool, encFunc MsgpackEncFunc) {
	if add {
		*ef = append(*ef, encFunc)
	}
}

/*
EncoderFuncString returns function which encodes string key-value pair.
Meant to be used with MapItems to store string value.
*/
func EncoderFuncString(key, value string) MsgpackEncFunc {
	return func(enc *msgpack.Encoder) (err error) {
		if err = enc.EncodeString(key); err != nil {
			return fmt.Errorf("encoding key %q: %w", key, err)
		}
		if err = enc.EncodeString(value); err != nil {
			return fmt.Errorf("encoding value of the key %q: %w", key, err)
		}
		return nil
	}
}

func EncoderFuncInt(key string, value int) MsgpackEncFunc {
	return func(enc *msgpack.Encoder) (err error) {
		if err = enc.EncodeString(key); err != nil {
			return fmt.Errorf("encoding key %q: %w", key, err)
		}
		if err = enc.EncodeInt(int64(value)); err != nil {
			return fmt.Errorf("encoding value of the key %q: %w", key, err)
		}
		return nil
	}
}

func EncoderFuncBool(key string, value bool) MsgpackEncFunc {
	return func(enc *msgpack.Encoder) (err error) {
		if err = enc.EncodeString(key); err != nil {
			return fmt.Errorf("encoding key %q: %w", key, err)
		}
		if err = enc.EncodeBool(value); err != nil {
			return fmt.Errorf("encoding value of the key %q: %w", key, err)
		}
		return nil
	}
}

/*
EncoderFuncArray writes "named array".
  - key : the name of the array, ie it's the key whose value will be the array;
  - items : slice of items to store as array values;
  - itemEnc : function which encodes item, ie enc.EncodeString

Returns function which can be used with MapItems.
*/
func EncoderFuncArray[T any](key string, items []T, itemEnc func(T) error) MsgpackEncFunc {
	return func(enc *msgpack.Encoder) (err error) {
		if err := enc.EncodeString(key); err != nil {
			return fmt.Errorf("encoding array name %q: %w", key, err)
		}
		if err := enc.EncodeArrayLen(len(items)); err != nil {
			return fmt.Errorf("encoding array %s length: %w", key, err)
		}
		for _, v := range items {
			if err := itemEnc(v); err != nil {
				return fmt.Errorf("encoding array %s item: %w", key, err)
			}
		}
		return nil
	}
}

/*
Callback is the "standard marshaler" signature.
*/
func EncoderFuncMarshal(key string, wf MsgpackEncFunc) MsgpackEncFunc {
	return func(enc *msgpack.Encoder) (err error) {
		if err = enc.EncodeString(key); err != nil {
			return fmt.Errorf("encoding key %q: %w", key, err)
		}
		if err = wf(enc); err != nil {
			return fmt.Errorf("encoding value of the key %q: %w", key, err)
		}
		return nil
	}
}
