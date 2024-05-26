package nu

type LabeledError struct {
	Msg    string         `msgpack:"msg"`
	Labels []ErrorLabel   `msgpack:"labels,omitempty"`
	Code   string         `msgpack:"code,omitempty"`
	Url    string         `msgpack:"url,omitempty"`
	Help   string         `msgpack:"help,omitempty"`
	Inner  []LabeledError `msgpack:"inner,omitempty"`
}

type ErrorLabel struct {
	Text string `msgpack:"text"`
	Span Span   `msgpack:"span"`
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

func (le *LabeledError) Error() string {
	return le.Msg
}
