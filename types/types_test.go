package types

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/vmihailenco/msgpack/v5"
)

func Test_Type_encodeRoundtrip(t *testing.T) {
	var testCases = []Type{
		Any(), Binary(), Block(), Bool(), Date(), Duration(),
		Custom("typeName"),
		List(String()), List(Any()),
		OneOf(), OneOf(Int()), OneOf(Number(), Glob()),
		Record(RecordDef{"strF": String()}),
		Record(RecordDef{"strF": String(), "closure": Closure()}),
		Table(RecordDef{"col1": Custom("ct")}),
		Table(RecordDef{"col2": Range(), "col1": Nothing()}),
	}

	enc := msgpack.GetEncoder()
	defer msgpack.PutEncoder(enc)
	dec := msgpack.GetDecoder()
	defer msgpack.PutDecoder(dec)
	buf := bytes.NewBuffer(nil)

	for i, tc := range testCases {
		buf.Reset()
		enc.Reset(buf)
		if err := tc.EncodeMsgpack(enc); err != nil {
			t.Errorf("failed to encode %s: %v", tc, err)
			continue
		}

		dec.Reset(buf)
		out, err := DecodeMsgpack(dec)
		if err != nil {
			t.Errorf("failed to decode %s: %v", tc, err)
			continue
		}
		if out == nil {
			t.Errorf("decoder returned nil for %s", tc)
			continue
		}
		if diff := cmp.Diff(tc, out, cmp.Comparer(compareTypes)); diff != "" {
			t.Errorf("[%d] mismatch (-want +got):\n%s\ninput: %s\noutput: %s", i, diff, tc, out)
		}
	}
}

func Test_Type_String(t *testing.T) {
	var testCases = []struct {
		typ Type
		str string
	}{
		{Any(), "Any"},
		{Binary(), "Binary"},
		{Block(), "Block"},
		{Bool(), "Bool"},
		{CellPath(), "CellPath"},
		{Closure(), "Closure"},
		{Date(), "Date"},
		{Duration(), "Duration"},
		{Error(), "Error"},
		{Filesize(), "Filesize"},
		{Float(), "Float"},
		{Int(), "Int"},
		{Nothing(), "Nothing"},
		{Number(), "Number"},
		{Range(), "Range"},
		{String(), "String"},
		{Glob(), "Glob"},
		{Custom("typeName"), "Custom(typeName)"},
		{List(String()), "List(String)"},
		{List(Custom("foo")), "List(Custom(foo))"},
		{OneOf(String()), "OneOf([String])"},
		{OneOf(Int(), Float()), "OneOf([Int, Float])"},
		{Record(RecordDef{"fName": String()}), "Record([(fName, String)])"},
		{Record(RecordDef{"name": String(), "value": Closure()}), "Record([(name, String), (value, Closure)])"},
		{Table(RecordDef{"A": String(), "B": Date()}), "Table([(A, String), (B, Date)])"},
	}

	for _, tc := range testCases {
		if s := tc.typ.String(); s != tc.str {
			t.Errorf("expected %q got %q", tc.str, s)
		}
	}
}
