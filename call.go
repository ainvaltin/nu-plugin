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
		Head       Span             `msgpack:"head"`
		Positional positionalParams `msgpack:"positional"`
		Named      NamedParams      `msgpack:"named"`
	}

	positionalParams []Value

	NamedParams map[string]Value

	callResponse struct {
		ID       int
		Response any
	}

	signal struct {
		Signal string `msgpack:"Signal"`
	}
)

type (
	empty struct{}

	// Value tuple variant as used by PipelineDataHeader
	pipelineValue struct {
		V Value
		M pipelineMetadata
	}

	listStream struct {
		ID   int              `msgpack:"id"`
		Span Span             `msgpack:"span"`
		MD   pipelineMetadata `msgpack:"metadata"`
	}

	byteStream struct {
		ID   int              `msgpack:"id"`
		Span Span             `msgpack:"span"`
		Type string           `msgpack:"type"`
		MD   pipelineMetadata `msgpack:"metadata"`
	}

	// A successful result with a Nu Value or stream. The body is a PipelineDataHeader.
	pipelineData struct {
		Data any `msgpack:"PipelineData"`
	}

	pipelineMetadata struct {
		DataSource  string
		FilePath    string // assigned when DataSource == FilePath
		ContentType string
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
			return nil, fmt.Errorf("decoding PipelineHeader map: %w", err)
		}
		switch name {
		case "Value":
			v := pipelineValue{}
			if err := v.DecodeMsgpack(dec); err != nil {
				return nil, fmt.Errorf("decoding pipelineValue: %w", err)
			}
			return v.V, nil
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
	case Value:
		return (&pipelineValue{V: dt}).EncodeMsgpack(enc)
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

func (pp *positionalParams) EncodeMsgpack(enc *msgpack.Encoder) error {
	if pp == nil || len(*pp) == 0 {
		return enc.EncodeArrayLen(0)
	}

	if err := enc.EncodeArrayLen(len(*pp)); err != nil {
		return err
	}
	for _, v := range *pp {
		if err := v.EncodeMsgpack(enc); err != nil {
			return err
		}
	}
	return nil
}

// to implement EvalArgument
func (np NamedParams) apply(cfg *evalArguments) error { cfg.named = np; return nil }

var _ msgpack.CustomEncoder = (*NamedParams)(nil)

func (np *NamedParams) EncodeMsgpack(enc *msgpack.Encoder) error {
	if np == nil || len(*np) == 0 {
		return enc.EncodeArrayLen(0)
	}

	if err := enc.EncodeArrayLen(len(*np)); err != nil {
		return err
	}
	var parName npName
	for name, v := range *np {
		if err := enc.EncodeArrayLen(2); err != nil {
			return err
		}
		parName.Name = name
		if err := enc.EncodeValue(reflect.ValueOf(&parName)); err != nil {
			return fmt.Errorf("writing named params [%s] key: %w", name, err)
		}
		if err := v.EncodeMsgpack(enc); err != nil {
			return err
		}
	}
	return nil
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

func (pv *pipelineValue) EncodeMsgpack(enc *msgpack.Encoder) error {
	if err := encodeMapStart(enc, "Value"); err != nil {
		return err
	}
	if err := enc.EncodeArrayLen(2); err != nil {
		return fmt.Errorf("encoding PipelineDataHeader Value tuple length: %w", err)
	}
	if err := pv.V.EncodeMsgpack(enc); err != nil {
		return fmt.Errorf("encoding PipelineDataHeader of Value: %w", err)
	}
	return pv.M.EncodeMsgpack(enc)
}

func (pv *pipelineValue) DecodeMsgpack(dec *msgpack.Decoder) error {
	dLen, err := dec.DecodeArrayLen()
	if err != nil {
		return fmt.Errorf("decode tuple length of Value: %w", err)
	}
	if dLen != 2 {
		return fmt.Errorf("expected two item tuple, got %d items", dLen)
	}
	if err = pv.V.DecodeMsgpack(dec); err != nil {
		return fmt.Errorf("decoding Value: %w", err)
	}
	if err = pv.M.DecodeMsgpack(dec); err != nil {
		return fmt.Errorf("decoding Value's metadata: %w", err)
	}
	return nil
}

func (md *pipelineMetadata) EncodeMsgpack(enc *msgpack.Encoder) error {
	if md.DataSource == "" && md.ContentType == "" {
		return enc.EncodeNil()
	}

	if err := enc.EncodeMapLen(2); err != nil {
		return err
	}

	if err := enc.EncodeString("data_source"); err != nil {
		return err
	}
	switch md.DataSource {
	case "FilePath":
		if err := encodeMapStart(enc, "FilePath"); err != nil {
			return err
		}
		if err := enc.EncodeString(md.FilePath); err != nil {
			return err
		}
	default:
		if err := enc.EncodeString(md.DataSource); err != nil {
			return err
		}
	}

	if err := enc.EncodeString("content_type"); err != nil {
		return err
	}
	if md.ContentType == "" {
		return enc.EncodeNil()
	} else {
		if err := enc.EncodeString(md.ContentType); err != nil {
			return err
		}
	}

	return nil
}

func (md *pipelineMetadata) DecodeMsgpack(dec *msgpack.Decoder) error {
	c, err := dec.PeekCode()
	if err != nil {
		return err
	}
	switch {
	case c == msgpcode.Nil:
		return dec.DecodeNil()
	case msgpcode.IsFixedMap(c):
		ml, err := dec.DecodeMapLen()
		if err != nil {
			return fmt.Errorf("decode map length: %w", err)
		}
		for ; ml > 0; ml-- {
			key, err := dec.DecodeString()
			if err != nil {
				return fmt.Errorf("decoding metadata key: %w", err)
			}

			c, err := dec.PeekCode()
			if err != nil {
				return fmt.Errorf("peeking value type of %q: %w", key, err)
			}
			switch key {
			case "data_source":
				switch {
				case msgpcode.IsString(c):
					if md.DataSource, err = dec.DecodeString(); err != nil {
						return fmt.Errorf("decoding DataSource value: %w", err)
					}
				case msgpcode.IsFixedMap(c):
					// must be map(1): "FilePath"=<path>
					ml, err := dec.DecodeMapLen()
					if err != nil {
						return fmt.Errorf("decode map length: %w", err)
					}
					if ml != 1 {
						return fmt.Errorf("expected value of data_source to be map(1) but got %d items", ml)
					}
					if md.DataSource, err = dec.DecodeString(); err != nil {
						return fmt.Errorf("decoding DataSource value: %w", err)
					}
					if md.FilePath, err = dec.DecodeString(); err != nil {
						return fmt.Errorf("decoding %q value: %w", md.DataSource, err)
					}
				default:
					return fmt.Errorf("unexpected value code %x for %q", c, key)
				}
			case "content_type":
				switch {
				case msgpcode.IsString(c):
					if md.ContentType, err = dec.DecodeString(); err != nil {
						return fmt.Errorf("decoding ContentType value: %w", err)
					}
				case c == msgpcode.Nil:
					if err = dec.DecodeNil(); err != nil {
						return err
					}
				default:
					return fmt.Errorf("unexpected value code %x for %q", c, key)
				}
			default:
				return fmt.Errorf("unexpected metadata key %q", key)
			}
		}
		return nil
	default:
		return fmt.Errorf("unexpected Value metadata, code %x", c)
	}
}
