package nu

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/vmihailenco/msgpack/v5"
)

func Test_IntRange_EndBound(t *testing.T) {
	t.Run("input equals output", func(t *testing.T) {
		// cases where encode - decode sycle results in
		// the exact same value as the original input
		cases := []IntRange{
			{End: 0, Bound: Unbounded},
			{End: -1, Bound: Included},
			{End: 0, Bound: Included},
			{End: 1, Bound: Included},
			{End: -1, Bound: Excluded},
			{End: 0, Bound: Excluded},
			{End: 1, Bound: Excluded},
			{End: 1024, Bound: Excluded},
		}

		enc := msgpack.GetEncoder()
		dec := msgpack.GetDecoder()
		buf := bytes.NewBuffer(nil)
		for x, tc := range cases {
			buf.Reset()
			enc.Reset(buf)
			if err := tc.encodeEndBound(enc); err != nil {
				t.Error("encoding:", err)
				continue
			}

			dec.Reset(buf)
			v := IntRange{}
			if err := v.decodeEndBound(dec); err != nil {
				t.Error("decoding:", err)
				continue
			}

			if diff := cmp.Diff(tc, v); diff != "" {
				t.Errorf("[%d] encoding mismatch (-input +output):\n%s", x, diff)
			}
		}
	})

	t.Run("input not equal to output", func(t *testing.T) {
		// cases where output of the encode - decode sycle will be
		// different from the original input
		cases := []struct{ in, out IntRange }{
			// the End value will be disacarded for Unbounded
			{in: IntRange{End: 1, Bound: Unbounded}, out: IntRange{End: 0, Bound: Unbounded}},
			// only End and Bound are encoded/decoded by these methods
			{in: IntRange{Start: 1, Step: 2, End: 3, Bound: Unbounded}, out: IntRange{Bound: Unbounded}},
			{in: IntRange{Start: 1, Step: 2, End: 3, Bound: Included}, out: IntRange{End: 3, Bound: Included}},
			{in: IntRange{Start: 1, Step: 2, End: 3, Bound: Excluded}, out: IntRange{End: 3, Bound: Excluded}},
		}

		enc := msgpack.GetEncoder()
		dec := msgpack.GetDecoder()
		buf := bytes.NewBuffer(nil)
		for x, tc := range cases {
			buf.Reset()
			enc.Reset(buf)
			if err := tc.in.encodeEndBound(enc); err != nil {
				t.Error("encoding:", err)
				continue
			}

			dec.Reset(buf)
			v := IntRange{}
			if err := v.decodeEndBound(dec); err != nil {
				t.Error("decoding:", err)
				continue
			}

			if diff := cmp.Diff(v, tc.out); diff != "" {
				t.Errorf("[%d] en/decoding mismatch (-expected +got):\n%s", x, diff)
			}
		}
	})

	t.Run("invalid", func(t *testing.T) {
		// fail to encode unexpected Bound value
		v := IntRange{Bound: 10}
		enc := msgpack.NewEncoder(bytes.NewBuffer(nil))
		expectErrorMsg(t, v.encodeEndBound(enc), `unsupported bound value: 10`)
	})
}
