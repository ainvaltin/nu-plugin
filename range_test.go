package nu

import (
	"bytes"
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/vmihailenco/msgpack/v5"
)

func Test_IntRange_String(t *testing.T) {
	var testCases = []struct {
		r IntRange
		s string
	}{
		{r: IntRange{}, s: "0..0..0"},
		{r: IntRange{Start: 0, Step: 1, End: 1, Bound: Included}, s: "0..1..1"},
		{r: IntRange{Start: 0, Step: 1, End: 1, Bound: Excluded}, s: "0..1..<1"},
		{r: IntRange{Start: 0, Step: -1, End: -1, Bound: Included}, s: "0..-1..-1"},
		{r: IntRange{Start: 0, Step: -1, End: -1, Bound: Excluded}, s: "0..-1..<-1"},
		{r: IntRange{Start: 8, Step: 5, End: 0, Bound: Unbounded}, s: "8..13.."},
		{r: IntRange{Start: -10, Step: -5, End: -15, Bound: Included}, s: "-10..-15..-15"},
		{r: IntRange{Start: -10, Step: 5, End: 15, Bound: Excluded}, s: "-10..-5..<15"},
	}

	for x, tc := range testCases {
		if diff := cmp.Diff(tc.r.String(), tc.s); diff != "" {
			t.Errorf("[%d] String mismatch (-expected +got):\n%s", x, diff)
		}
	}
}

func Test_IntRange_EndBound(t *testing.T) {
	t.Run("input equals output", func(t *testing.T) {
		// cases where encode - decode cycle results in
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
		// cases where output of the encode - decode cycle will be
		// different from the original input
		cases := []struct{ in, out IntRange }{
			// the End value will be discarded for Unbounded
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

func Test_IntRange_Iterator(t *testing.T) {
	t.Run("invalid ranges", func(t *testing.T) {
		// invalid range should produce empty list
		cases := []IntRange{
			{}, // Step is zero
			{Start: 1, Step: 1, End: 0, Bound: Included},  // count up, Start > End
			{Start: 1, Step: 1, End: 0, Bound: Excluded},  // count up, Start > End
			{Start: 1, Step: 1, End: -1, Bound: Included}, // count up, Start > End
		}
		for x, tc := range cases {
			if err := tc.Validate(); err == nil {
				t.Errorf("[%d] expected error for invalid IntRange %#v", x, tc)
				continue
			}
			if diff := cmp.Diff([]int64(nil), slices.Collect(tc.All())); diff != "" {
				t.Errorf("[%d] sequence mismatch for %#v (-expected +got):\n%s", x, tc, diff)
			}
		}
	})

	t.Run("valid range but produces no items", func(t *testing.T) {
		cases := []IntRange{
			{Start: 1, Step: 1, End: 1, Bound: Excluded},
			{Start: -10, Step: 1, End: -10, Bound: Excluded},
		}
		for x, tc := range cases {
			if err := tc.Validate(); err != nil {
				t.Errorf("[%d] unexpected expected error for IntRange %#v: %v", x, tc, err)
				continue
			}
			if diff := cmp.Diff([]int64(nil), slices.Collect(tc.All())); diff != "" {
				t.Errorf("[%d] sequence mismatch for %#v (-expected +got):\n%s", x, tc, diff)
			}
		}
	})

	t.Run("counting up", func(t *testing.T) {
		cases := []struct {
			r   IntRange
			out []int64
		}{
			// positive range
			{r: IntRange{Start: 1, Step: 1, End: 1, Bound: Included}, out: []int64{1}},
			{r: IntRange{Start: 1, Step: 1, End: 4, Bound: Included}, out: []int64{1, 2, 3, 4}},
			{r: IntRange{Start: 1, Step: 1, End: 4, Bound: Excluded}, out: []int64{1, 2, 3}},
			{r: IntRange{Start: 1, Step: 2, End: 5, Bound: Included}, out: []int64{1, 3, 5}},
			{r: IntRange{Start: 1, Step: 2, End: 5, Bound: Excluded}, out: []int64{1, 3}},
			{r: IntRange{Start: 1, Step: 3, End: 8, Bound: Included}, out: []int64{1, 4, 7}},
			{r: IntRange{Start: 1, Step: 3, End: 8, Bound: Excluded}, out: []int64{1, 4, 7}},
			{r: IntRange{Start: 1, Step: 3, End: 7, Bound: Excluded}, out: []int64{1, 4}},
			{r: IntRange{Start: math.MaxInt64 - 1, Step: 1, End: math.MaxInt64, Bound: Excluded}, out: []int64{math.MaxInt64 - 1}},
			{r: IntRange{Start: math.MaxInt64 - 1, Step: 3, End: math.MaxInt64, Bound: Excluded}, out: []int64{math.MaxInt64 - 1}},
			// starts at zero
			{r: IntRange{Start: 0, Step: 3, End: 7, Bound: Excluded}, out: []int64{0, 3, 6}},
			// ends at zero
			{r: IntRange{Start: -6, Step: 2, End: 0, Bound: Included}, out: []int64{-6, -4, -2, 0}},
			{r: IntRange{Start: -6, Step: 2, End: 0, Bound: Excluded}, out: []int64{-6, -4, -2}},
			// negative range
			{r: IntRange{Start: -6, Step: 2, End: -2, Bound: Included}, out: []int64{-6, -4, -2}},
			{r: IntRange{Start: -6, Step: 2, End: -2, Bound: Excluded}, out: []int64{-6, -4}},
			{r: IntRange{Start: -6, Step: 2, End: -1, Bound: Excluded}, out: []int64{-6, -4, -2}},
			{r: IntRange{Start: -6, Step: 1, End: -1, Bound: Excluded}, out: []int64{-6, -5, -4, -3, -2}},
			// from negative to positive
			{r: IntRange{Start: math.MinInt64, Step: math.MaxInt64, End: math.MaxInt64, Bound: Included}, out: []int64{math.MinInt64, -1, math.MaxInt64 - 1}},
			// unbounded
			{r: IntRange{Start: math.MaxInt64 - 2, Step: 1, Bound: Unbounded}, out: []int64{math.MaxInt64 - 2, math.MaxInt64 - 1, math.MaxInt64}},
			{r: IntRange{Start: math.MaxInt64 - 2, Step: 2, Bound: Unbounded}, out: []int64{math.MaxInt64 - 2, math.MaxInt64}},
			{r: IntRange{Start: math.MaxInt64 - 2, Step: 3, Bound: Unbounded}, out: []int64{math.MaxInt64 - 2}},
		}

		for x, tc := range cases {
			if err := tc.r.Validate(); err != nil {
				t.Errorf("[%d] invalid IntRange %#v: %v", x, tc.r, err)
				continue
			}
			if diff := cmp.Diff(tc.out, slices.Collect(tc.r.All())); diff != "" {
				t.Errorf("[%d] sequence mismatch for %#v (-expected +got):\n%s", x, tc.r, diff)
			}
		}
	})

	t.Run("counting down", func(t *testing.T) {
		cases := []struct {
			r   IntRange
			out []int64
		}{
			// positive range
			{r: IntRange{Start: 5, Step: -1, End: 4, Bound: Included}, out: []int64{5, 4}},
			{r: IntRange{Start: 5, Step: -1, End: 4, Bound: Excluded}, out: []int64{5}},
			// ends at zero
			{r: IntRange{Start: 1, Step: -1, End: 0, Bound: Included}, out: []int64{1, 0}},
			{r: IntRange{Start: 1, Step: -1, End: 0, Bound: Excluded}, out: []int64{1}},
			// starts at zero
			{r: IntRange{Start: 0, Step: -1, End: -3, Bound: Included}, out: []int64{0, -1, -2, -3}},
			{r: IntRange{Start: 0, Step: -1, End: -3, Bound: Excluded}, out: []int64{0, -1, -2}},
			// from positive to negative
			{r: IntRange{Start: 10, Step: -3, End: -2, Bound: Included}, out: []int64{10, 7, 4, 1, -2}},
			{r: IntRange{Start: 10, Step: -3, End: -2, Bound: Excluded}, out: []int64{10, 7, 4, 1}},
			// negative range
			{r: IntRange{Start: -1, Step: -1, End: -4, Bound: Included}, out: []int64{-1, -2, -3, -4}},
			{r: IntRange{Start: -1, Step: -1, End: -4, Bound: Excluded}, out: []int64{-1, -2, -3}},
			// unbounded
			{r: IntRange{Start: math.MinInt64 + 2, Step: -1, Bound: Unbounded}, out: []int64{math.MinInt64 + 2, math.MinInt64 + 1, math.MinInt64}},
			{r: IntRange{Start: math.MinInt64 + 2, Step: -2, Bound: Unbounded}, out: []int64{math.MinInt64 + 2, math.MinInt64}},
			{r: IntRange{Start: math.MinInt64 + 2, Step: -3, Bound: Unbounded}, out: []int64{math.MinInt64 + 2}},
		}

		for x, tc := range cases {
			if err := tc.r.Validate(); err != nil {
				t.Errorf("[%d] invalid IntRange %#v: %v", x, tc.r, err)
				continue
			}
			if diff := cmp.Diff(tc.out, slices.Collect(tc.r.All())); diff != "" {
				t.Errorf("[%d] sequence mismatch for %#v (-expected +got):\n%s", x, tc.r, diff)
			}
		}
	})
}

func ExampleIntRange() {
	var values []int64
	// end bound defaults to Included
	rng := IntRange{Start: -1, Step: 2, End: 5}
	for v := range rng.All() {
		values = append(values, v)
	}
	fmt.Printf("Included: %v\n", values)

	// exclude end bound
	rng.Bound = Excluded
	values = slices.Collect(rng.All())
	fmt.Printf("Excluded: %v\n", values)
	// Output:
	// Included: [-1 1 3 5]
	// Excluded: [-1 1 3]
}
