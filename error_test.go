package nu

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/vmihailenco/msgpack/v5"
)

func Test_Error_message(t *testing.T) {
	var testCases = []struct {
		err Error
		msg string // message we expect to get from the "err"
	}{
		{err: Error{Err: errors.New("some error")}, msg: "some error"},
		{
			err: Error{
				Err:  errors.New("some error"),
				Code: "err::code",
				Help: "helpful",
			},
			msg: "some error",
		},
		{
			err: Error{Code: "err::code", Help: "helpful"},
			msg: "err::code",
		},
		{
			err: Error{Help: "helpful"},
			msg: "helpful",
		},
		{err: Error{}, msg: ""},
	}

	for x, tc := range testCases {
		if diff := cmp.Diff(tc.msg, tc.err.Error()); diff != "" {
			t.Errorf("[%d] mismatch (-want +got):\n%s", x, diff)
		}
	}
}

func Test_Error_encode_decode(t *testing.T) {
	var testCases = []Error{
		{Err: errors.New("so bad")},
		{Err: errors.New("so bad"), Code: "C1"},
		{Err: errors.New("so bad"), Code: "C1", Url: "foo://bar"},
		{Err: errors.New("so bad"), Code: "C1", Url: "foo://bar", Help: "yes"},
		{Err: errors.New("so bad"), Code: "C1", Url: "foo://bar", Help: "Yes", Labels: []Label{{Text: "label"}}},
		{Err: errors.New("so bad"), Code: "C1", Url: "foo://bar", Help: "Yes", Labels: []Label{{Text: "label"}}, Inner: []Error{{Err: errors.New("inner")}}},
	}

	dec := msgpack.GetDecoder()
	enc := msgpack.GetEncoder()
	buf := bytes.Buffer{}
	for x, tc := range testCases {
		buf.Reset()
		enc.Reset(&buf)
		if err := tc.encodeMsgpack(enc); err != nil {
			t.Errorf("[%d] encode %#v: %v", x, tc, err)
			continue
		}

		dec.Reset(&buf)
		e, err := decodeLabeledError(dec)
		if err != nil {
			t.Errorf("[%d] decode %#v: %v", x, tc, err)
			continue
		}

		if diff := cmp.Diff(&e, &tc, cmp.Comparer(compareErrors)); diff != "" {
			t.Errorf("[%d] encoding %T mismatch (-input +output):\n%s", x, tc, diff)
		}
	}
}

func Test_flattenError(t *testing.T) {
	var testCases = []struct {
		in  error
		out Error
	}{
		{in: errors.New("msg"), out: Error{Err: errors.New("msg")}},
		{in: fmt.Errorf("msg"), out: Error{Err: errors.New("msg")}},
		{in: fmt.Errorf("wrapped: %w", errors.New("msg")), out: Error{Err: errors.New("wrapped: msg")}},
		{in: fmt.Errorf("wrapped: %w", Error{Err: errors.New("msg")}), out: Error{Err: errors.New("wrapped: msg")}},
		{
			in: fmt.Errorf("wrapped: %w", Error{
				Err:    fmt.Errorf("wrapped inner: %w", Error{Err: errors.New("msg")}),
				Help:   "something helpful",
				Labels: []Label{{Text: "label", Span: Span{Start: 10, End: 30}}},
			}),
			out: Error{
				Err:    errors.New("wrapped: wrapped inner: msg"),
				Help:   "something helpful",
				Labels: []Label{{Text: "label", Span: Span{Start: 10, End: 30}}},
				Inner:  []Error{{Err: errors.New("msg")}},
			},
		},
		// the outermost error is Join-ed
		{
			in: errors.Join(errors.New("first"), errors.New("second")),
			out: Error{
				Err:   fmt.Errorf("there are multiple errors"),
				Inner: []Error{{Err: errors.New("first")}, {Err: errors.New("second")}},
			},
		},
		{
			in: errors.Join(errors.New("first"), Error{Err: errors.New("second")}),
			out: Error{
				Err:   fmt.Errorf("there are multiple errors"),
				Inner: []Error{{Err: errors.New("first")}, {Err: errors.New("second")}},
			},
		},
		// inner error is Join-ed
		{
			in: fmt.Errorf("outer: %w", errors.Join(errors.New("first"), Error{Err: errors.New("second")})),
			out: Error{
				Err:   fmt.Errorf("outer: there are multiple errors"),
				Inner: []Error{{Err: errors.New("first")}, {Err: errors.New("second")}},
			},
		},
		{
			in: Error{Err: fmt.Errorf("outer: %w", errors.Join(errors.New("first"), &Error{Err: errors.New("second")}))},
			out: Error{
				Err:   fmt.Errorf("outer: there are multiple errors"),
				Inner: []Error{{Err: errors.New("first")}, {Err: errors.New("second")}},
			},
		},
	}

	for x, tc := range testCases {
		fe := flattenError(tc.in)
		if diff := cmp.Diff(&tc.out, fe, cmp.Comparer(compareErrors)); diff != "" {
			t.Errorf("[%d] encoding %T mismatch (-want +got):\n%s", x, tc.out, diff)
		}
	}
}

/*
implementation for cmp.Comparer to be used as cmp.Diff Option when comparing Error-s
*/
func compareErrors(a, b Error) bool {
	return a.equal(&b)
}

func (a *Error) equal(b *Error) bool {
	if a.Code != b.Code || a.Url != b.Url || a.Help != b.Help || a.Err.Error() != b.Err.Error() {
		return false
	}
	if !slices.Equal(a.Labels, b.Labels) {
		return false
	}
	if len(a.Inner) != len(b.Inner) {
		return false
	}
	for _, ae := range a.Inner {
		if !slices.ContainsFunc(b.Inner, func(be Error) bool { return be.equal(&ae) }) {
			return false
		}
	}
	return true
}
