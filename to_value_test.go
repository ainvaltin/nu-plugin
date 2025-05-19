package nu

import (
	"math"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func Test_ToValue(t *testing.T) {
	t.Run("simple types", func(t *testing.T) {
		testCases := []struct {
			kind  reflect.Kind // the expected type of the "value"
			value any          // value under test
			out   Value        // expected nu Value
		}{
			// boolean
			{kind: reflect.Bool, value: false, out: Value{Value: false}},
			{kind: reflect.Bool, value: true, out: Value{Value: true}},
			// unsigned int
			{kind: reflect.Uint, value: uint(0), out: Value{Value: int64(0)}},
			{kind: reflect.Uint8, value: uint8(2), out: Value{Value: int64(2)}},
			{kind: reflect.Uint16, value: uint16(3), out: Value{Value: int64(3)}},
			{kind: reflect.Uint32, value: uint32(4), out: Value{Value: int64(4)}},
			{kind: reflect.Uint64, value: uint64(8), out: Value{Value: int64(8)}},
			{kind: reflect.Uint64, value: uint64(math.MaxInt64), out: Value{Value: int64(math.MaxInt64)}},
			// signed int
			{kind: reflect.Int, value: -1, out: Value{Value: int64(-1)}},
			{kind: reflect.Int, value: 0, out: Value{Value: int64(0)}},
			{kind: reflect.Int, value: int(5), out: Value{Value: int64(5)}},
			{kind: reflect.Int8, value: int8(6), out: Value{Value: int64(6)}},
			{kind: reflect.Int16, value: int16(7), out: Value{Value: int64(7)}},
			{kind: reflect.Int32, value: int32(9), out: Value{Value: int64(9)}},
			{kind: reflect.Int64, value: int64(10), out: Value{Value: int64(10)}},
			{kind: reflect.Int64, value: int64(math.MaxInt64), out: Value{Value: int64(math.MaxInt64)}},
			{kind: reflect.Int64, value: int64(math.MinInt64), out: Value{Value: int64(math.MinInt64)}},
			// floats
			{kind: reflect.Float32, value: float32(1.5), out: Value{Value: 1.5}},
			{kind: reflect.Float64, value: 1.5, out: Value{Value: 1.5}},
			//
			{kind: reflect.String, value: "foo bar", out: Value{Value: "foo bar"}},
			{kind: reflect.Invalid, value: nil, out: Value{Value: nil}},
		}

		for x, tc := range testCases {
			// check that the test is correctly constructed, ie the value is of correct kind
			refV := reflect.ValueOf(tc.value)
			if tc.kind != refV.Kind() {
				t.Errorf("[%d] kind should be %s but is %s", x, tc.kind, refV.Kind())
				continue
			}

			v := ToValue(tc.value)
			if diff := cmp.Diff(tc.out, v); diff != "" {
				t.Errorf("[%d] encoding %T mismatch (-expected +actual):\n%s", x, tc.value, diff)
			}
		}
	})

	t.Run("nu types", func(t *testing.T) {
		testCases := []any{
			Block(1),
			Closure{BlockID: 2, Captures: []byte{0, 0, 0}},
			Filesize(1000),
			Glob{Value: "**", NoExpand: true},
			IntRange{Start: 1, Step: 2, End: 3},
			Record{},
			CellPath{},
			cvt{42, nil},
			[]Value{{Value: "item"}},
		}

		for x, tc := range testCases {
			v := ToValue(tc)
			if diff := cmp.Diff(tc, v.Value, cmpopts.EquateComparable(cvt{})); diff != "" {
				t.Errorf("[%d] encoding %T mismatch (-expected +actual):\n%s", x, tc, diff)
			}
		}
	})
}

func Test_rv2nv(t *testing.T) {
	// in some tests here we also call ToValue, to make sure both return the same Value

	t.Run("unsupported values", func(t *testing.T) {
		// simple cases of unsupported values - the input value itself is
		// invalid and  we expect to get back nu.Value which contains error
		testCases := []struct {
			kind  reflect.Kind // the expected type of the "value"
			value any          // value under test
			err   string       // expected error message
		}{
			{kind: reflect.Uintptr, value: uintptr(42), err: `unsupported value type uintptr`},
			{kind: reflect.Complex64, value: complex64(1i), err: `unsupported value type complex64`},
			{kind: reflect.Complex128, value: 1i, err: `unsupported value type complex128`},
			{kind: reflect.Func, value: func() {}, err: `unsupported value type func()`},
			{kind: reflect.Func, value: func(int) error { return nil }, err: `unsupported value type func(int) error`},
			{kind: reflect.Chan, value: make(chan int), err: `unsupported value type chan int`},
			{kind: reflect.Chan, value: make(<-chan int), err: `unsupported value type <-chan int`},
			{kind: reflect.Chan, value: make(chan<- int), err: `unsupported value type chan<- int`},
			{kind: reflect.Map, value: make(map[int]any), err: `map key type must be string, got map[int]interface {}`},
			{kind: reflect.Uint64, value: uint64(math.MaxInt64 + 1), err: `uint 9223372036854775808 is too large for int64`},
			{kind: reflect.Uint, value: uint(math.MaxInt64 + 1), err: `uint 9223372036854775808 is too large for int64`},
			//{kind: reflect.Pointer, value: reflect.PointerTo(int), err: ``},
			//{kind: reflect.UnsafePointer, value: , err: ``},
		}

		for x, tc := range testCases {
			// check that the test is correctly constructed, ie the value is of expected kind
			refV := reflect.ValueOf(tc.value)
			if tc.kind != refV.Kind() {
				t.Errorf("[%d] kind should be %s but is %s", x, tc.kind, refV.Kind())
				continue
			}

			v := rv2nv(refV)
			err, ok := v.Value.(error)
			if !ok {
				t.Errorf("[%d] returned value is not error but %T", x, v.Value)
				continue
			}
			if diff := cmp.Diff(tc.err, err.Error()); diff != "" {
				t.Errorf("[%d] encoding %T mismatch (-expected +actual):\n%s", x, tc.value, diff)
			}

			v = ToValue(tc.value)
			err, ok = v.Value.(error)
			if !ok {
				t.Errorf("[%d] returned value is not error but %T", x, v.Value)
				continue
			}
			if diff := cmp.Diff(tc.err, err.Error()); diff != "" {
				t.Errorf("[%d] encoding %T mismatch (-expected +actual):\n%s", x, tc.value, diff)
			}
		}
	})

	t.Run("simple types", func(t *testing.T) {
		testCases := []struct {
			kind  reflect.Kind // the expected type of the "value"
			value any          // value under test
			out   Value        // expected nu Value
		}{
			// boolean
			{kind: reflect.Bool, value: false, out: Value{Value: false}},
			{kind: reflect.Bool, value: true, out: Value{Value: true}},
			// unsigned int
			{kind: reflect.Uint, value: uint(0), out: Value{Value: int64(0)}},
			{kind: reflect.Uint8, value: uint8(2), out: Value{Value: int64(2)}},
			{kind: reflect.Uint16, value: uint16(3), out: Value{Value: int64(3)}},
			{kind: reflect.Uint32, value: uint32(4), out: Value{Value: int64(4)}},
			{kind: reflect.Uint64, value: uint64(8), out: Value{Value: int64(8)}},
			{kind: reflect.Uint64, value: uint64(math.MaxInt64), out: Value{Value: int64(math.MaxInt64)}},
			// signed int
			{kind: reflect.Int, value: -1, out: Value{Value: int64(-1)}},
			{kind: reflect.Int, value: 0, out: Value{Value: int64(0)}},
			{kind: reflect.Int, value: int(5), out: Value{Value: int64(5)}},
			{kind: reflect.Int8, value: int8(6), out: Value{Value: int64(6)}},
			{kind: reflect.Int16, value: int16(7), out: Value{Value: int64(7)}},
			{kind: reflect.Int32, value: int32(9), out: Value{Value: int64(9)}},
			{kind: reflect.Int64, value: int64(10), out: Value{Value: int64(10)}},
			{kind: reflect.Int64, value: int64(math.MaxInt64), out: Value{Value: int64(math.MaxInt64)}},
			{kind: reflect.Int64, value: int64(math.MinInt64), out: Value{Value: int64(math.MinInt64)}},
			// floats
			{kind: reflect.Float32, value: float32(1.5), out: Value{Value: 1.5}},
			{kind: reflect.Float64, value: 1.5, out: Value{Value: 1.5}},
			//
			{kind: reflect.String, value: "foo bar", out: Value{Value: "foo bar"}},
			//{kind: reflect.Interface, value: nil, out: Value{Value: "nested"}},
		}

		for x, tc := range testCases {
			// check that the test is correctly constructed, ie the value is of correct kind
			refV := reflect.ValueOf(tc.value)
			if tc.kind != refV.Kind() {
				t.Errorf("[%d] kind should be %s but is %s", x, tc.kind, refV.Kind())
				continue
			}

			v := rv2nv(refV)
			if diff := cmp.Diff(tc.out, v); diff != "" {
				t.Errorf("[%d] encoding %T mismatch (-expected +actual):\n%s", x, tc.value, diff)
			}

			v = ToValue(tc.value)
			if diff := cmp.Diff(tc.out, v); diff != "" {
				t.Errorf("[%d] encoding %T mismatch (-expected +actual):\n%s", x, tc.value, diff)
			}
		}
	})

	t.Run("unsupported map", func(t *testing.T) {
		// nested value is unsupported map type - the nested value contains error
		m := map[string]any{"nested": map[int]int{1: 0}}
		v := rv2nv(reflect.ValueOf(m))
		vm, ok := v.Value.(Record)
		if !ok {
			t.Fatalf("expected map, got %T", v.Value)
		}
		if v, ok = vm["nested"]; !ok {
			t.Fatalf("record doesn't contain key 'nested': %#v", vm)
		}
		err, ok := v.Value.(error)
		if !ok {
			t.Fatalf("expected error, got %T", v)
		}
		if err.Error() != "map key type must be string, got map[int]int" {
			t.Error("unexpected error:", err)
		}
	})

	t.Run("supported map types", func(t *testing.T) {
		// map key must be string
		testCases := []struct {
			value any    // value under test
			out   Record // expected Value.Value
		}{
			{
				value: map[string]any{
					"str": "str value",
					"int": 2000,
					"map": map[string]int{"one": 1, "two": 2},
				},
				out: Record{
					"str": Value{Value: "str value"},
					"int": Value{Value: int64(2000)},
					"map": Value{Value: Record{"one": Value{Value: int64(1)}, "two": Value{Value: int64(2)}}},
				},
			},
			{
				value: map[string]string{"one": "üks", "two": "kaks"},
				out: Record{
					"one": Value{Value: "üks"},
					"two": Value{Value: "kaks"},
				},
			},
		}

		for x, tc := range testCases {
			v := rv2nv(reflect.ValueOf(tc.value))
			if diff := cmp.Diff(tc.out, v.Value); diff != "" {
				t.Errorf("[%d] encoding %T mismatch (-expected +actual):\n%s", x, tc.value, diff)
			}
		}
	})

	t.Run("CellPath", func(t *testing.T) {
		cp := CellPath{}
		cp.AddInteger(10, false)
		cp.AddString("field", true)

		v := rv2nv(reflect.ValueOf(cp))
		if diff := cmp.Diff(cp, v.Value, cmpopts.EquateComparable(pathItem[uint]{}, pathItem[string]{})); diff != "" {
			t.Errorf("encoding mismatch (-expected +actual):\n%s", diff)
		}
	})

	t.Run("structs", func(t *testing.T) {
		// structs are mapped to Record
		type simple struct {
			A int
			S string
			p []byte
			X any
		}
		type recFld struct {
			A simple
			S string
		}

		t.Run("empty struct", func(t *testing.T) {
			// empty struct -> empty Record
			v := rv2nv(reflect.ValueOf(struct{}{}))
			if diff := cmp.Diff(Record{}, v.Value); diff != "" {
				t.Errorf("encoding mismatch (-expected +actual):\n%s", diff)
			}
		})

		t.Run("simple struct", func(t *testing.T) {
			in := simple{A: 1, S: "str", p: []byte{2}}
			out := Record{
				"A": Value{Value: int64(1)},
				"S": Value{Value: "str"},
				"p": Value{Value: []byte{2}},
				"X": Value{},
			}
			v := rv2nv(reflect.ValueOf(in))
			if diff := cmp.Diff(out, v.Value); diff != "" {
				t.Errorf("encoding mismatch (-expected +actual):\n%s", diff)
			}
		})

		t.Run("struct as field", func(t *testing.T) {
			in := recFld{
				S: "outer",
				A: simple{
					A: 7,
					S: "inner",
				},
			}
			out := Record{
				"S": Value{Value: "outer"},
				"A": Value{Value: Record{
					"A": Value{Value: int64(7)},
					"S": Value{Value: "inner"},
					"p": Value{Value: []byte(nil)},
					"X": Value{},
				}},
			}
			v := rv2nv(reflect.ValueOf(in))
			if diff := cmp.Diff(out, v.Value); diff != "" {
				t.Errorf("encoding mismatch (-expected +actual):\n%s", diff)
			}
		})

		t.Run("nested struct", func(t *testing.T) {
			type nested struct {
				simple
				S string
			}
			in := nested{
				simple: simple{
					A: 1,
					S: "nested",
					p: []byte{5, 5},
				},
				S: "outer",
			}

			out := Record{
				"S": Value{Value: "outer"},
				"simple": Value{Value: Record{
					"A": Value{Value: int64(1)},
					"S": Value{Value: "nested"},
					"p": Value{Value: []byte{5, 5}},
					"X": Value{},
				}},
			}

			v := rv2nv(reflect.ValueOf(in))
			if diff := cmp.Diff(out, v.Value); diff != "" {
				t.Errorf("encoding mismatch (-expected +actual):\n%s", diff)
			}
		})

		t.Run("inline struct", func(t *testing.T) {
			in := struct {
				I int
				N struct{ I int }
			}{
				I: 1,
				N: struct{ I int }{I: 2},
			}

			out := Record{
				"I": Value{Value: int64(1)},
				"N": Value{Value: Record{
					"I": Value{Value: int64(2)},
				}},
			}
			v := rv2nv(reflect.ValueOf(in))
			if diff := cmp.Diff(out, v.Value); diff != "" {
				t.Errorf("encoding mismatch (-expected +actual):\n%s", diff)
			}
		})
	})

	t.Run("slices and arrays", func(t *testing.T) {
		testCases := []struct {
			in  any
			out Value
		}{
			// []byte should return Binary, uint8 == byte
			{in: []byte(nil), out: Value{Value: []byte(nil)}},
			{in: []byte{}, out: Value{Value: []byte{}}},
			{in: []byte{1, 2, 3}, out: Value{Value: []byte{1, 2, 3}}},
			{in: []uint8{1, 2, 3}, out: Value{Value: []byte{1, 2, 3}}},
			{in: [3]byte{1, 2, 3}, out: Value{Value: []byte{1, 2, 3}}},
			// slices of other types are converted to list of Values
			{in: []int(nil), out: Value{Value: []Value{}}},
			{in: []int{}, out: Value{Value: []Value{}}},
			{in: []int8{1, 2, 3}, out: Value{Value: []Value{{Value: int64(1)}, {Value: int64(2)}, {Value: int64(3)}}}},
			{in: []int{1, 2, 3}, out: Value{Value: []Value{{Value: int64(1)}, {Value: int64(2)}, {Value: int64(3)}}}},
			{in: []string{"foo"}, out: Value{Value: []Value{{Value: "foo"}}}},
			{in: [3]int{1, 2, 3}, out: Value{Value: []Value{{Value: int64(1)}, {Value: int64(2)}, {Value: int64(3)}}}},
			{in: []cvt{{1, nil}}, out: Value{Value: []Value{{Value: cvt{1, nil}}}}},
		}

		for x, tc := range testCases {
			v := rv2nv(reflect.ValueOf(tc.in))
			if diff := cmp.Diff(tc.out, v, cmpopts.EquateComparable(cvt{})); diff != "" {
				t.Errorf("[%d] encoding %T mismatch (-expected +actual):\n%s", x, tc.in, diff)
			}

			v = ToValue(tc.in)
			if diff := cmp.Diff(tc.out, v, cmpopts.EquateComparable(cvt{})); diff != "" {
				t.Errorf("[%d] encoding %T mismatch (-expected +actual):\n%s", x, tc.in, diff)
			}
		}
	})
}

type cvt struct {
	f int
	CustomValue
}
