package nu

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/vmihailenco/msgpack/v5"
)

func Test_Hello_DeEncode_happy(t *testing.T) {
	// encode Hello as message pack, then decode the binary
	// and see did we get back (the same) struct
	testCases := []hello{
		{Protocol: "nu-plugin", Version: "0.90.2"},
		{Protocol: "nu-plugin", Version: "0.93.0", Features: features{LocalSocket: true}},
	}

	for x, tc := range testCases {
		bin, err := msgpack.Marshal(&tc)
		if err != nil {
			t.Errorf("[%d] encoding %#v: %v", x, tc, err)
			continue
		}

		dec := msgpack.NewDecoder(bytes.NewBuffer(bin))
		dec.SetMapDecoder(decodeInputMsg)
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
