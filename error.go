package nu

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/vmihailenco/msgpack/v5"
)

/* perhaps too clever?
func NewError(args ...any) Error {
	r := Error{}
	for _, arg := range args {
		switch a := arg.(type) {
		case Error:
			r.Inner = append(r.Inner, a)
		case *Error:
			r.Inner = append(r.Inner, *a)
		case Label:
			r.Labels = append(r.Labels, a)
		case error:
			r.Err = a
		case string:
			if _, err := url.Parse(a); err == nil {
				r.Url = a
			} else if strings.Count(a, "::") > 0 {
				// if Code is already assigned keep the one which has more "::"?
				r.Code = a
			} else if r.Err == nil {
				r.Err = errors.New(a)
			} else {
				r.Help = a
			}
		}
	}
	return r
}*/

/*
Error is a generic type of error used by Nu for interfacing with external code,
such as scripts and plugins. It represents the [LabeledError] in Nu.

[LabeledError]: https://www.nushell.sh/contributor-book/plugin_protocol_reference.html#labelederror
*/
type Error struct {
	Err    error   // The main message for the error
	Code   string  // A unique machine- and search-friendly error code to associate to the error. (e.g. nu::shell::missing_config_value)
	Url    string  // A link to documentation about the error, used in conjunction with "code"
	Help   string  // Additional help for the error, usually a hint about what the user might try
	Labels []Label // Labeled spans attached to the error, demonstrating to the user where the problem is
	Inner  []Error // Errors that are related to or caused this error
}

/*
Error implements Go "error" interface.
*/
func (e Error) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	if e.Code != "" {
		return e.Code
	}
	if e.Help != "" {
		return e.Help
	}
	return ""
}

func (e Error) Unwrap() error { return errors.Unwrap(e.Err) }

func (e *Error) AddLabel(text string, span Span) Error {
	e.Labels = append(e.Labels, Label{Text: text, Span: span})
	return *e
}

func (e *Error) encodeMsgpack(enc *msgpack.Encoder) error {
	cnt := 1 + bval(e.Code != "") + bval(e.Help != "") + bval(e.Url != "") + bval(len(e.Inner) > 0) + bval(len(e.Labels) > 0)
	if err := enc.EncodeMapLen(cnt); err != nil {
		return err
	}

	if err := encodeString(enc, "msg", e.Err.Error()); err != nil {
		return err
	}
	if e.Code != "" {
		if err := encodeString(enc, "code", e.Code); err != nil {
			return err
		}
	}
	if e.Help != "" {
		if err := encodeString(enc, "help", e.Help); err != nil {
			return err
		}
	}
	if e.Url != "" {
		if err := encodeString(enc, "url", e.Url); err != nil {
			return err
		}
	}

	if len(e.Labels) > 0 {
		if err := enc.EncodeString("labels"); err != nil {
			return err
		}
		if err := enc.EncodeArrayLen(len(e.Labels)); err != nil {
			return err
		}
		for _, v := range e.Labels {
			if err := v.encodeMsgpack(enc); err != nil {
				return err
			}
		}
	}

	if len(e.Inner) > 0 {
		if err := enc.EncodeString("inner"); err != nil {
			return err
		}
		if err := enc.EncodeArrayLen(len(e.Inner)); err != nil {
			return err
		}
		for _, v := range e.Inner {
			if err := v.encodeMsgpack(enc); err != nil {
				return err
			}
		}
	}

	return nil
}

func decodeLabeledError(dec *msgpack.Decoder) (le Error, _ error) {
	cnt, err := dec.DecodeMapLen()
	if err != nil {
		return le, err
	}
	for idx := range cnt {
		key, err := dec.DecodeString()
		if err != nil {
			return le, fmt.Errorf("decode key %d/%d", idx, cnt)
		}
		switch key {
		case "msg":
			var msg string
			msg, err = dec.DecodeString()
			le.Err = errors.New(msg)
		case "code":
			le.Code, err = dec.DecodeString()
		case "help":
			le.Help, err = dec.DecodeString()
		case "url":
			le.Url, err = dec.DecodeString()
		case "labels":
			var l int
			if l, err = dec.DecodeArrayLen(); err != nil {
				return le, fmt.Errorf("decode labels count: %w", err)
			}
			le.Labels = make([]Label, l)
			for i := range l {
				if err = le.Labels[i].decodeMsgpack(dec); err != nil {
					return le, fmt.Errorf("decode label %d of %d: %w", i, l, err)
				}
			}
		case "inner":
			var l int
			if l, err = dec.DecodeArrayLen(); err != nil {
				return le, fmt.Errorf("decode labels count: %w", err)
			}
			le.Inner = make([]Error, 0, l)
			for i := range l {
				e, err := decodeLabeledError(dec)
				if err != nil {
					return le, fmt.Errorf("decode inner error %d of %d: %w", i, l, err)
				}
				le.Inner = append(le.Inner, e)
			}
		}
		if err != nil {
			return le, fmt.Errorf("decoding value of %q: %w", key, err)
		}
	}
	return le, nil
}

/*
flatten the error chain into single Error, suitable for serialization.
*/
func flattenError(err error) (r *Error) {
	setErr := func(e *Error) {
		if r == nil {
			r = &Error{
				Err:    err,
				Url:    e.Url,
				Code:   e.Code,
				Help:   e.Help,
				Labels: slices.Clone(e.Labels),
				Inner:  slices.Clone(e.Inner),
			}
		} else {
			r.Inner = append(r.Inner, *e)
		}
	}

	for ce := err; ce != nil; ce = errors.Unwrap(ce) {
		switch e := ce.(type) {
		case Error:
			setErr(&e)
		case *Error:
			setErr(e)
		case interface{ Unwrap() []error }:
			newErr := errors.New(strings.Replace(err.Error(), ce.Error(), "there are multiple errors", 1))
			if r == nil {
				r = &Error{Err: newErr}
			} else {
				r.Err = newErr
			}
			for _, v := range e.Unwrap() {
				r.Inner = append(r.Inner, *flattenError(v))
			}
		}
	}

	if r == nil {
		return &Error{Err: err}
	}
	return r
}

/*
Label is "label" type for [Error].
*/
type Label struct {
	Text string // The message for the label.
	Span Span   // The span in the source code that the label should point to.
}

func (l Label) encodeMsgpack(enc *msgpack.Encoder) error {
	if err := enc.EncodeMapLen(2); err != nil {
		return err
	}
	if err := encodeString(enc, "text", l.Text); err != nil {
		return err
	}
	if err := enc.EncodeString("span"); err != nil {
		return err
	}
	return l.Span.encodeMsgpack(enc)
}

func (l *Label) decodeMsgpack(dec *msgpack.Decoder) error {
	cnt, err := dec.DecodeMapLen()
	if err != nil {
		return err
	}
	if cnt != 2 {
		return fmt.Errorf("expected ErrorLabel to contain 2 keys, got %d", cnt)
	}
	for idx := range cnt {
		key, err := dec.DecodeString()
		if err != nil {
			return fmt.Errorf("decode key %d of %d", idx, cnt)
		}
		switch key {
		case "text":
			l.Text, err = dec.DecodeString()
		case "span":
			err = l.Span.decodeMsgpack(dec)
		}
		if err != nil {
			return fmt.Errorf("decoding value of %q: %w", key, err)
		}
	}
	return nil
}
