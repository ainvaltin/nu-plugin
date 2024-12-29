package nu

import (
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/vmihailenco/msgpack/v5/msgpcode"
)

type RangeBound uint8

const (
	Unbounded RangeBound = 0
	Included  RangeBound = 1
	Excluded  RangeBound = 2
)

/*
IntRange is IntRange variant of [Nushell Range] type.

[Nushell Range]: https://www.nushell.sh/contributor-book/plugin_protocol_reference.html#range
*/
type IntRange struct {
	Start int64
	Step  int64
	End   int64
	Bound RangeBound // end bound kind of the range
}

func (v *IntRange) String() string {
	s := ""
	switch v.Bound {
	case Included:
		s = fmt.Sprintf("%d", v.End)
	case Excluded:
		s = fmt.Sprintf("<%d", v.End)
	}
	return fmt.Sprintf("%d..%d..%s", v.Start, v.Start+v.Step, s)
}

var _ msgpack.CustomEncoder = (*IntRange)(nil)

func (v *IntRange) EncodeMsgpack(enc *msgpack.Encoder) error {
	if err := encodeMapStart(enc, "IntRange"); err != nil {
		return err
	}

	if err := enc.EncodeMapLen(3); err != nil {
		return err
	}
	if err := enc.EncodeString("start"); err != nil {
		return err
	}
	if err := enc.EncodeInt(v.Start); err != nil {
		return err
	}
	if err := enc.EncodeString("step"); err != nil {
		return err
	}
	if err := enc.EncodeInt(v.Step); err != nil {
		return err
	}
	if err := enc.EncodeString("end"); err != nil {
		return err
	}
	if err := v.encodeEndBound(enc); err != nil {
		return err
	}
	return nil
}

func (v *IntRange) encodeEndBound(enc *msgpack.Encoder) (err error) {
	if v.Bound == Unbounded {
		return enc.EncodeString("Unbounded")
	}

	if err := enc.EncodeMapLen(1); err != nil {
		return err
	}
	switch v.Bound {
	case Included:
		err = enc.EncodeString("Included")
	case Excluded:
		err = enc.EncodeString("Excluded")
	default:
		return fmt.Errorf("unsupported bound value: %d", v.Bound)
	}
	if err != nil {
		return err
	}
	return enc.EncodeInt(v.End)
}

func (v *IntRange) decodeEndBound(dec *msgpack.Decoder) (err error) {
	code, err := dec.PeekCode()
	if err != nil {
		return fmt.Errorf("peek the type of the end bound of IntRange: %w", err)
	}
	var name string
	switch {
	case msgpcode.IsFixedMap(code) || code == msgpcode.Map16 || code == msgpcode.Map32:
		if n, err := dec.DecodeMapLen(); err != nil || n != 1 {
			return fmt.Errorf("expected single item map as end bound, got [%d] or error: %w", n, err)
		}
		name, err = dec.DecodeString()
	case msgpcode.IsString(code):
		name, err = dec.DecodeString()
	}
	if err != nil {
		return err
	}

	switch name {
	case "Unbounded":
		v.Bound = Unbounded
		return nil
	case "Included":
		v.Bound = Included
	case "Excluded":
		v.Bound = Excluded
	default:
		return fmt.Errorf("unsupported bound name %q", name)
	}
	v.End, err = dec.DecodeInt64()
	return err
}

var _ msgpack.CustomDecoder = (*IntRange)(nil)

func (v *IntRange) DecodeMsgpack(dec *msgpack.Decoder) error {
	n, err := dec.DecodeMapLen()
	if err != nil {
		return err
	}
	if n == -1 {
		return nil
	}

	for idx := 0; idx < n; idx++ {
		fieldName, err := dec.DecodeString()
		if err != nil {
			return fmt.Errorf("decoding field name [%d/%d] of IntRange: %w", idx+1, n, err)
		}
		switch fieldName {
		case "start":
			v.Start, err = dec.DecodeInt64()
		case "step":
			v.Step, err = dec.DecodeInt64()
		case "end":
			err = v.decodeEndBound(dec)
		default:
			return fmt.Errorf("unexpected key %q in IntRange", fieldName)
		}
		if err != nil {
			return fmt.Errorf("decode field %q: %w", fieldName, err)
		}
	}
	return nil
}

func decodeMsgpackRange(dec *msgpack.Decoder) (any, error) {
	name, err := decodeWrapperMap(dec)
	if err != nil {
		return nil, fmt.Errorf("decoding Range value kind: %w", err)
	}
	switch name {
	case "IntRange":
		v := IntRange{}
		return v, v.DecodeMsgpack(dec)
	case "FloatRange":
		return nil, fmt.Errorf("FloatRange is not implemented")
	default:
		return nil, fmt.Errorf("unsupported Range type: %q", name)
	}
}
