package nu

import (
	"errors"
	"fmt"
	"iter"
	"math"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/vmihailenco/msgpack/v5/msgpcode"
)

type RangeBound uint8

const (
	Included  RangeBound = 0
	Excluded  RangeBound = 1
	Unbounded RangeBound = 2
)

func (rb RangeBound) String() string {
	switch rb {
	case Included:
		return "Included"
	case Excluded:
		return "Excluded"
	case Unbounded:
		return "Unbounded"
	default:
		return fmt.Sprintf("RangeBound(%d)", int(rb))
	}
}

/*
IntRange is the IntRange variant of [Nushell Range] type.

When creating IntRange manually don't forget to assign Step as range with
zero stride would be invalid.

Bound defaults to "included" which is also default in Nushell.

To iterate over values in the range use [IntRange.All] method.

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

func (v IntRange) Validate() error {
	// should we check that End == 0 for Unbounded?
	switch {
	case v.Step > 0:
		if v.Bound != Unbounded && v.Start > v.End {
			return fmt.Errorf("start value must be smaller than end value, got %d..%d (step %d)", v.Start, v.End, v.Step)
		}
	case v.Step < 0:
		if v.Bound != Unbounded && v.Start <= v.End {
			return fmt.Errorf("start value must be greater than end value, got %d..%d (step %d)", v.Start, v.End, v.Step)
		}
	default:
		return errors.New("step must be non-zero")
	}

	return nil
}

/*
All generates all the values in the Range.

Invalid range doesn't generate any values.
*/
func (v IntRange) All() iter.Seq[int64] {
	switch {
	case v.Step > 0:
		return v.countUp()
	case v.Step < 0:
		return v.countDown()
	default:
		// one can manually construct invalid range where step == 0
		return func(yield func(int64) bool) {}
	}
}

func add(a, b int64) (int64, bool) {
	c := a + b
	return c, (c > a) == (b > 0)
}

func (v *IntRange) countUp() iter.Seq[int64] {
	return func(yield func(int64) bool) {
		var end int64
		switch v.Bound {
		case Unbounded:
			// 9223372036854775806..
			// returns just two values, ie it does not wrap over on overflow
			end = math.MaxInt64
		case Included:
			end = v.End
		case Excluded:
			end = v.End - 1
		}

		for i, ok := v.Start, true; i <= end && ok; i, ok = add(i, v.Step) {
			if !yield(i) {
				return
			}
		}
	}
}

func (v *IntRange) countDown() iter.Seq[int64] {
	return func(yield func(int64) bool) {
		var end int64
		switch v.Bound {
		case Unbounded:
			end = math.MinInt64
		case Included:
			end = v.End
		case Excluded:
			end = v.End + 1
		}

		for i, ok := v.Start, true; i >= end && ok; i, ok = add(i, v.Step) {
			if !yield(i) {
				return
			}
		}
	}
}

var _ msgpack.CustomEncoder = (*IntRange)(nil)

func (v *IntRange) EncodeMsgpack(enc *msgpack.Encoder) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("invalid IntRange definition: %w", err)
	}

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
	// validate? or we trust engine to send correct data?
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
