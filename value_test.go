package nu

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/vmihailenco/msgpack/v5"
)

func Test_Value_DeEncode(t *testing.T) {
	// encode Value as message pack, then decode the binary
	// and see did we get back (the same) expected Value
	// happy cases, ie we expect no en/decode errors
	testCases := []struct {
		in  Value // value encoded
		out Value // value we expect to get by decoding encoded "in"
	}{
		{in: Value{Value: nil}, out: Value{Value: nil}},                                                             // Nothing
		{in: Value{Value: nil, Span: Span{Start: 2, End: 7}}, out: Value{Value: nil, Span: Span{Start: 2, End: 7}}}, // Nothing
		{in: Value{Value: int(1)}, out: Value{Value: int64(1)}},
		{in: Value{Value: int(1), Span: Span{Start: 1020, End: 1050}}, out: Value{Value: int64(1), Span: Span{Start: 1020, End: 1050}}},
		{in: Value{Value: int8(2)}, out: Value{Value: int64(2)}},
		{in: Value{Value: int8(-1)}, out: Value{Value: int64(-1)}},
		{in: Value{Value: int8(127)}, out: Value{Value: int64(127)}},
		{in: Value{Value: int16(3)}, out: Value{Value: int64(3)}},
		{in: Value{Value: int32(4)}, out: Value{Value: int64(4)}},
		{in: Value{Value: int64(5)}, out: Value{Value: int64(5)}},
		{in: Value{Value: uint(6)}, out: Value{Value: int64(6)}},
		{in: Value{Value: uint8(7)}, out: Value{Value: int64(7)}},
		{in: Value{Value: uint16(8)}, out: Value{Value: int64(8)}},
		{in: Value{Value: uint32(9)}, out: Value{Value: int64(9)}},
		{in: Value{Value: uint64(10)}, out: Value{Value: int64(10)}},
		{in: Value{Value: float32(1.0 / 32.0)}, out: Value{Value: float64(1.0 / 32.0)}},
		{in: Value{Value: float64(1.0 / 32.0)}, out: Value{Value: float64(1.0 / 32.0)}},
		{in: Value{Value: true}, out: Value{Value: true}},
		{in: Value{Value: false}, out: Value{Value: false}},
		{in: Value{Value: ""}, out: Value{Value: ""}},
		{in: Value{Value: "foo bar"}, out: Value{Value: "foo bar"}},
		{in: Value{Value: []byte{0, 1, 2, 127, 128, 254, 255}}, out: Value{Value: []byte{0, 1, 2, 127, 128, 254, 255}}},
		{in: Value{Value: Filesize(1001)}, out: Value{Value: Filesize(1001)}},
		{in: Value{Value: 11 * time.Minute}, out: Value{Value: 11 * time.Minute}},
		{in: Value{Value: time.Date(2024, 05, 25, 14, 55, 06, 0, time.UTC)}, out: Value{Value: time.Date(2024, 05, 25, 14, 55, 06, 0, time.UTC)}},
		{in: Value{Value: Record{"foo": Value{Value: "bar"}, "int": Value{Value: 12}}}, out: Value{Value: Record{"foo": Value{Value: "bar"}, "int": Value{Value: int64(12)}}}},
		{in: Value{Value: []Value{{Value: "first"}, {Value: 13}}}, out: Value{Value: []Value{{Value: "first"}, {Value: int64(13)}}}},
		{in: Value{Value: fmt.Errorf("oops")}, out: Value{Value: LabeledError{Msg: "oops"}}},
		{in: Value{Value: Closure{BlockID: 8}}, out: Value{Value: Closure{BlockID: 8}}},
		{in: Value{Value: Closure{BlockID: 8, Captures: []byte{144}}}, out: Value{Value: Closure{BlockID: 8, Captures: []byte{144}}}},
		{in: Value{Value: Glob{Value: "[a-z].txt", NoExpand: false}}, out: Value{Value: Glob{Value: "[a-z].txt", NoExpand: false}}},
		{in: Value{Value: Glob{Value: "**/*.txt", NoExpand: true}}, out: Value{Value: Glob{Value: "**/*.txt", NoExpand: true}}},
		{in: Value{Value: Glob{Value: "foo.txt"}, Span: Span{Start: 1, End: 8}}, out: Value{Value: Glob{Value: "foo.txt"}, Span: Span{Start: 1, End: 8}}},
	}

	for x, tc := range testCases {
		bin, err := msgpack.Marshal(&tc.in)
		if err != nil {
			t.Errorf("encoding %#v: %v", tc.in.Value, err)
			continue
		}
		var dv Value
		if err := msgpack.Unmarshal(bin, &dv); err != nil {
			t.Errorf("decoding %#v: %v", tc.in.Value, err)
			continue
		}

		if diff := cmp.Diff(dv, tc.out); diff != "" {
			t.Errorf("[%d] encoding %T mismatch (-input +output):\n%s", x, tc.in.Value, diff)
		}
	}
}

func Test_Value_Encode(t *testing.T) {
	t.Run("unsupported type", func(t *testing.T) {
		v := Value{Value: 10i}
		_, err := msgpack.Marshal(&v)
		expectErrorMsg(t, err, `unsupported Value type complex128`)

		v = Value{Value: struct{ Foo string }{"anon"}}
		_, err = msgpack.Marshal(&v)
		expectErrorMsg(t, err, `unsupported Value type struct { Foo string }`)
	})
}
