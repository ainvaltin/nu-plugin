package nu

import (
	"reflect"

	"github.com/vmihailenco/msgpack/v5"
)

type LabeledError struct {
	Msg    string         `msgpack:"msg"`              // The main message for the error.
	Labels []ErrorLabel   `msgpack:"labels,omitempty"` // Labeled spans attached to the error, demonstrating to the user where the problem is.
	Code   string         `msgpack:"code,omitempty"`   // A unique machine- and search-friendly error code to associate to the error. (e.g. nu::shell::missing_config_value)
	Url    string         `msgpack:"url,omitempty"`    // A link to documentation about the error, used in conjunction with code
	Help   string         `msgpack:"help,omitempty"`   // Additional help for the error, usually a hint about what the user might try
	Inner  []LabeledError `msgpack:"inner,omitempty"`  // Errors that are related to or caused this error
}

/*
ErrorLabel is "label" type for [LabeledError].
*/
type ErrorLabel struct {
	Text string `msgpack:"text"` // The message for the label.
	Span Span   `msgpack:"span"` // The span in the source code that the label should point to.
}

/*
AsLabeledError "converts" error to LabeledError - if the
error is already LabeledError it will be "unwrapped",
otherwise new LabeledError will be created wrapping the err.
*/
func AsLabeledError(err error) *LabeledError {
	if le, ok := err.(*LabeledError); ok {
		return le
	}
	return &LabeledError{Msg: err.Error()}
}

/*
Error implements Go "error" interface.

The "Msg" is returned as error message.
*/
func (le *LabeledError) Error() string {
	return le.Msg
}

func (le *LabeledError) encodeMsgpack(enc *msgpack.Encoder) error {
	if err := enc.EncodeString("Error"); err != nil {
		return err
	}
	if err := enc.EncodeMapLen(2); err != nil {
		return err
	}
	if err := enc.EncodeString("error"); err != nil {
		return err
	}
	return enc.EncodeValue(reflect.ValueOf(le))
}
