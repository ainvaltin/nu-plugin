package nu

import (
	"bytes"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/vmihailenco/msgpack/v5"
)

func Test_Data_DeEncode_happy(t *testing.T) {
	// encode Data as message pack, then decode the binary
	// and see did we get back (the same) struct
	testCases := []data{
		{ID: 3, Data: Value{Value: "Hello, world!", Span: Span{Start: 40000, End: 40015}}},
		{ID: 7, Data: []byte{0xf0, 0xff, 0x00}},
		{ID: 8, Data: Error{Err: errors.New("disconnected")}},
	}

	p := Plugin{}

	for x, tc := range testCases {
		bin, err := p.serialize(&tc)
		if err != nil {
			t.Errorf("[%d] encoding %#v: %v", x, tc, err)
			continue
		}

		dec := msgpack.NewDecoder(bytes.NewBuffer(bin))
		dec.SetMapDecoder(p.decodeInputMsg)
		dv, err := dec.DecodeInterface()
		if err != nil {
			t.Errorf("[%d] decoding %#v: %v", x, tc, err)
			continue
		}

		if diff := cmp.Diff(tc, dv, cmp.Comparer(compareErrors)); diff != "" {
			t.Errorf("[%d] mismatch (-want +got):\n%s", x, diff)
		}
	}
}

func Test_StreamMsgs_DeEncode_happy(t *testing.T) {
	// encode stream messages as message pack, then decode the binary
	// and see did we get back (the same) struct
	testCases := []any{
		end{ID: 4},
		ack{ID: 2},
		drop{ID: 42},
	}

	p := Plugin{}

	for x, tc := range testCases {
		bin, err := msgpack.Marshal(&tc)
		if err != nil {
			t.Errorf("[%d] encoding %#v: %v", x, tc, err)
			continue
		}

		dec := msgpack.NewDecoder(bytes.NewBuffer(bin))
		dec.SetMapDecoder(p.decodeInputMsg)
		dv, err := dec.DecodeInterface()
		if err != nil {
			t.Errorf("[%d] decoding %#v: %v", x, tc, err)
			continue
		}

		if diff := cmp.Diff(tc, dv); diff != "" {
			t.Errorf("[%d] mismatch (-want +got):\n%s", x, diff)
		}
	}
}
