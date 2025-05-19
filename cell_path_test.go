package nu

import (
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func Test_CellPath_Encode(t *testing.T) {
	cp := CellPath{}
	cp.AddStringSpan("title", false, Span{10, 20})
	cp.AddInteger(2, true)
	v := Value{Value: cp}

	p := &Plugin{}

	bin, err := p.serialize(&v)
	if err != nil {
		t.Errorf("encoding %#v: %v", v.Value, err)
	}
	var dv Value
	if err := p.deserialize(bin, &dv); err != nil {
		t.Errorf("decoding %#v: %v", v.Value, err)
	}

	if diff := cmp.Diff(dv, v, cmpopts.EquateComparable(pathItem[uint]{}, pathItem[string]{})); diff != "" {
		t.Errorf("encoding %T mismatch (-input +output):\n%s", v.Value, diff)
	}
}

func Test_CellPath_Add(t *testing.T) {

	expectedLength := func(t *testing.T, cp CellPath, cnt int) {
		t.Helper()
		if l := cp.Length(); l != cnt {
			t.Errorf("expected length %d, got %d", cnt, l)
		}
	}

	t.Run("AddInteger", func(t *testing.T) {
		cp := CellPath{}
		expectedLength(t, cp, 0)
		cp.AddInteger(0, false)
		expectedLength(t, cp, 1)
		cp.AddInteger(1, true)
		expectedLength(t, cp, 2)
		if s := cp.String(); s != "0.1?" {
			t.Errorf("expected path to be '0.1?', got %q", s)
		}

		cp.AddIntegerSpan(3, true, Span{Start: 1, End: 2})
		expectedLength(t, cp, 3)
		cp.AddIntegerSpan(2, false, Span{Start: 3, End: 4})
		expectedLength(t, cp, 4)
		if s := cp.String(); s != "0.1?.3?.2" {
			t.Errorf("expected path to be '0.1?.3?.2', got %q", s)
		}
	})

	t.Run("AddString", func(t *testing.T) {
		cp := CellPath{}
		expectedLength(t, cp, 0)

		cp.AddString("foo", false)
		expectedLength(t, cp, 1)
		cp.AddString("bar", true)
		expectedLength(t, cp, 2)
		if s := cp.String(); s != "foo.bar?" {
			t.Errorf("expected path to be 'foo.bar?', got %q", s)
		}

		cp.AddStringSpan("zoo", false, Span{5, 6})
		expectedLength(t, cp, 3)
		cp.AddStringSpan("buz", true, Span{8, 9})
		expectedLength(t, cp, 4)
		if s := cp.String(); s != "foo.bar?.zoo.buz?" {
			t.Errorf("expected path to be 'foo.bar?', got %q", s)
		}
	})
}

func Test_CellPath_read(t *testing.T) {

	checkItemInt := func(t *testing.T, item PathMember, v uint, opt bool) {
		t.Helper()
		if i := item.Type(); i != PathVariantInt {
			t.Fatalf("expected type to be Int, got %d", i)
		}
		if i := item.PathInt(); i != v {
			t.Fatalf("expected value to be %d, got %d", v, i)
		}
		if s := item.PathStr(); s != "" {
			t.Fatalf("expected string to be empty, got %q", s)
		}
		if o := item.Optional(); o != opt {
			t.Fatalf("expected Optional to be %t, got %t", opt, o)
		}
	}

	checkItemStr := func(t *testing.T, item PathMember, v string, opt bool) {
		t.Helper()
		if i := item.Type(); i != PathVariantString {
			t.Fatalf("expected type to be String, got %d", i)
		}
		if i := item.PathInt(); i != math.MaxUint {
			t.Fatalf("expected Integer to be MaxUint, got %x", i)
		}
		if s := item.PathStr(); s != v {
			t.Fatalf("expected value to be %q, got %q", v, s)
		}
		if o := item.Optional(); o != opt {
			t.Fatalf("expected Optional to be %t, got %t", opt, o)
		}
	}

	cp := CellPath{}
	cp.AddInteger(8, false)
	cp.AddString("first", false)
	cp.AddInteger(4, true)
	cp.AddString("second", true)

	checkItemInt(t, cp.Members[0], 8, false)
	checkItemStr(t, cp.Members[1], "first", false)
	checkItemInt(t, cp.Members[2], 4, true)
	checkItemStr(t, cp.Members[3], "second", true)
}
