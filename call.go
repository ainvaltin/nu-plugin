package nu

import (
	"fmt"
	"reflect"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/vmihailenco/msgpack/v5/msgpcode"
)

type (
	call struct {
		ID   int
		Call any
	}

	signature struct{}

	// share the same struct for both input and output for now
	metadata struct {
		Version string `msgpack:"version,omitempty"`
	}

	run struct {
		Name  string        `msgpack:"name"`
		Call  evaluatedCall `msgpack:"call"`
		Input any           `msgpack:"input,omitempty"`
	}

	evaluatedCall struct {
		Head       Span        `msgpack:"head"`
		Positional []Value     `msgpack:"positional"`
		Named      NamedParams `msgpack:"named"`
	}

	NamedParams map[string]Value

	callResponse struct {
		ID       int
		Response any
	}
)

type (
	empty struct{}

	listStream struct {
		ID   int  `msgpack:"id"`
		Span Span `msgpack:"span"`
	}

	byteStream struct {
		ID   int    `msgpack:"id"`
		Span Span   `msgpack:"span"`
		Type string `msgpack:"type"`
	}

	// A successful result with a Nu Value or stream. The body is a PipelineDataHeader.
	pipelineData struct {
		Data any `msgpack:"PipelineData"`
	}
)

func decodeCall(dec *msgpack.Decoder) (any, error) {
	var err error
	m := call{}
	if m.ID, err = decodeTupleStart(dec); err != nil {
		return nil, fmt.Errorf("decoding Call tuple: %w", err)
	}

	c, err := dec.PeekCode()
	if err != nil {
		return nil, err
	}
	switch {
	case msgpcode.IsFixedString(c), msgpcode.IsString(c):
		s, err := dec.DecodeString()
		if err != nil {
			return nil, err
		}
		switch s {
		case "Signature":
			m.Call = signature{}
		case "Metadata":
			m.Call = metadata{}
		default:
			return nil, fmt.Errorf("unknown Call command %q", s)
		}
	case msgpcode.IsFixedMap(c):
		name, err := decodeWrapperMap(dec)
		if err != nil {
			return nil, err
		}
		switch name {
		case "Run":
			r := run{Call: evaluatedCall{Named: NamedParams{}}}
			if err := r.DecodeMsgpack(dec); err != nil {
				return nil, fmt.Errorf("decoding Run: %w", err)
			}
			m.Call = r
		default:
			return nil, fmt.Errorf("unknown Call type %q", name)
		}
	default:
		return nil, fmt.Errorf("unsupported Call value: %d", c)
	}

	return m, nil
}

var _ msgpack.CustomDecoder = (*run)(nil)

func (r *run) DecodeMsgpack(dec *msgpack.Decoder) error {
	cnt, err := dec.DecodeMapLen()
	if err != nil {
		return fmt.Errorf("reading Run map length: %w", err)
	}
	for idx := 0; idx < cnt; idx++ {
		key, err := dec.DecodeString()
		if err != nil {
			return fmt.Errorf("reading Run key: %w", err)
		}
		switch key {
		case "name":
			r.Name, err = dec.DecodeString()
		case "call":
			err = dec.DecodeValue(reflect.ValueOf(&r.Call))
		case "input":
			r.Input, err = decodePipelineDataHeader(dec)
		default:
			return fmt.Errorf("unknown key %q under Run", key)
		}
		if err != nil {
			return fmt.Errorf("decoding Run key %q: %w", key, err)
		}
	}
	return nil
}

func decodePipelineDataHeader(dec *msgpack.Decoder) (any, error) {
	c, err := dec.PeekCode()
	if err != nil {
		return nil, err
	}
	switch {
	case msgpcode.IsFixedString(c), msgpcode.IsString(c):
		name, err := dec.DecodeString()
		if err != nil {
			return nil, err
		}
		if name == "Empty" {
			return empty{}, nil
		}
		return nil, fmt.Errorf("expected PipelineHeader Empty, got %q", name)
	case msgpcode.IsFixedMap(c):
		name, err := decodeWrapperMap(dec)
		if err != nil {
			return nil, err
		}
		switch name {
		case "Value":
			v := Value{}
			if err := v.DecodeMsgpack(dec); err != nil {
				return nil, err
			}
			return v, nil
		case "ListStream":
			v := listStream{}
			if err := dec.DecodeValue(reflect.ValueOf(&v)); err != nil {
				return nil, fmt.Errorf("decoding ListStream: %w", err)
			}
			return v, nil
		case "ByteStream":
			v := byteStream{}
			if err := dec.DecodeValue(reflect.ValueOf(&v)); err != nil {
				return nil, fmt.Errorf("decoding ByteStream: %w", err)
			}
			return v, nil
		default:
			return nil, fmt.Errorf("unknown PipelineDataHeader value %q", name)
		}
	default:
		return nil, fmt.Errorf("unexpected type %x in PipelineDataHeader", c)
	}
}

func encodePipelineDataHeader(enc *msgpack.Encoder, data any) error {
	switch dt := data.(type) {
	case *Value:
		if err := encodeMapStart(enc, "Value"); err != nil {
			return err
		}
		return dt.EncodeMsgpack(enc)
	case *listStream:
		if err := encodeMapStart(enc, "ListStream"); err != nil {
			return err
		}
		return enc.EncodeValue(reflect.ValueOf(dt))
	case *byteStream:
		if err := encodeMapStart(enc, "ByteStream"); err != nil {
			return err
		}
		return enc.EncodeValue(reflect.ValueOf(dt))
	case *empty, empty:
		return enc.EncodeString("Empty")
	default:
		return fmt.Errorf("unsupported PipelineDataHeader type %T", dt)
	}
}

var _ msgpack.CustomDecoder = (*NamedParams)(nil)

func (np *NamedParams) DecodeMsgpack(dec *msgpack.Decoder) error {
	count, err := dec.DecodeArrayLen()
	if err != nil {
		return fmt.Errorf("reading NamedParameter count: %w", err)
	}
	if count == -1 {
		return nil
	}

	for idx := 0; idx < count; idx++ {
		tl, err := dec.DecodeArrayLen()
		if err != nil {
			return fmt.Errorf("reading named params [%d] tuple length: %w", idx, err)
		}
		if tl != 2 {
			return fmt.Errorf("NamedParams tuple should have 2 items, got %d for [%d]", tl, idx)
		}

		// {item: str, span: Span}
		var name npName
		if err := dec.DecodeValue(reflect.ValueOf(&name)); err != nil {
			return fmt.Errorf("reading named params [%d] key: %w", idx, err)
		}

		v := Value{}
		c, err := dec.PeekCode()
		if err != nil {
			return err
		}
		if c == msgpcode.Nil {
			if err := dec.DecodeNil(); err != nil {
				return err
			}
		} else {
			if err = v.DecodeMsgpack(dec); err != nil {
				return fmt.Errorf("reading named params [%d] value: %w", idx, err)
			}
		}
		(*np)[name.Name] = v
	}
	return nil
}

type npName struct {
	Name string `msgpack:"item"`
	Span Span   `msgpack:"span"`
}

/*
StringValue returns value of the named parameter "name" if set or "def"
when not set.
*/
func (np NamedParams) StringValue(name, def string) string {
	v, ok := np[name]
	if !ok {
		return def
	}
	switch d := v.Value.(type) {
	case string:
		return d
	default:
		// TODO: either return error or panic?
		return fmt.Sprintf("parameter %q is of type %T not string", name, d)
	}
}

/*
Check if a flag (named parameter that does not take a value) is set.

Returns "true" if flag is set or passed true value, returns "false" if flag is
not set or passed "false" value. Returns error if passed value is not a boolean.

Note that the long name must be used to query the flags!
*/
func (np NamedParams) HasFlag(name string) (bool, error) {
	v, ok := np[name]
	if !ok {
		return false, nil
	}

	switch tv := v.Value.(type) {
	case nil: // just flag, without value, ie "cmd -f"
		return true, nil
	case bool: // flag with value, ie "cmd -f=false"
		return tv, nil
	default:
		// the nu makes actually good job making sure that only bool values are
		// accepted so could get rid of error return and panic (should never happen)?
		return false, fmt.Errorf("flag's value must be bool, got %T", tv)
	}
}

var _ msgpack.CustomEncoder = (*callResponse)(nil)

func (cr *callResponse) EncodeMsgpack(enc *msgpack.Encoder) error {
	if err := encodeTupleInMap(enc, "CallResponse", cr.ID); err != nil {
		return err
	}

	switch dt := cr.Response.(type) {
	case *Value:
		if err := encodeMapStart(enc, "Value"); err != nil {
			return err
		}
		return dt.EncodeMsgpack(enc)
	case *pipelineData:
		return dt.EncodeMsgpack(enc)
	case *LabeledError:
		return encodeErrorResponse(enc, dt)
	case error:
		return encodeErrorResponse(enc, AsLabeledError(dt))
	case metadata:
		if err := encodeMapStart(enc, "Metadata"); err != nil {
			return err
		}
		return enc.EncodeValue(reflect.ValueOf(&dt))
	case []*Command:
		if err := encodeMapStart(enc, "Signature"); err != nil {
			return err
		}
		if err := enc.EncodeArrayLen(len(dt)); err != nil {
			return err
		}
		for _, v := range dt {
			if err := enc.EncodeValue(reflect.ValueOf(&v)); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported type %T in CallResponse", dt)
	}
}

func encodeErrorResponse(enc *msgpack.Encoder, le *LabeledError) error {
	if err := encodeMapStart(enc, "Error"); err != nil {
		return err
	}
	return enc.EncodeValue(reflect.ValueOf(le))
}

var _ msgpack.CustomEncoder = (*pipelineData)(nil)

func (pd *pipelineData) EncodeMsgpack(enc *msgpack.Encoder) error {
	if err := encodeMapStart(enc, "PipelineData"); err != nil {
		return err
	}

	return encodePipelineDataHeader(enc, pd.Data)
}

var _ msgpack.CustomDecoder = (*pipelineData)(nil)

func (pd *pipelineData) DecodeMsgpack(dec *msgpack.Decoder) (err error) {
	pd.Data, err = decodePipelineDataHeader(dec)
	return err
}
