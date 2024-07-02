package nu

import (
	"fmt"
	"reflect"
	"time"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/vmihailenco/msgpack/v5/msgpcode"
)

/*
Value represents [Nushell Value].

Generally type switch or type assertion has to be used to access the
"underling value", ie

	switch data := in.Value.(type) {
	case []byte:
		buf = data
	case string:
		buf = []byte(data)
	default:
		return fmt.Errorf("unsupported Value type %T", data)
	}

Incoming data is encoded as follows:

  - Nothing -> nil
  - Bool -> bool
  - Binary -> []byte
  - String -> string
  - Int -> int64
  - Float -> float64
  - Filesize -> [Filesize]
  - Duration -> [time.Duration]
  - Date -> [time.Time]
  - Record -> [Record]
  - List -> []Value
  - Glob -> [Glob]
  - Closure -> [Closure]

Outgoing values are encoded as:

  - nil -> Nothing
  - int, int8, int16, int32, int64 -> Int
  - uint, uint8, uint16, uint32, uint64 -> Int
  - float64, float32 -> Float
  - bool -> Bool
  - []byte -> Binary
  - string -> String
  - [Filesize] -> Filesize
  - [time.Duration] -> Duration
  - [time.Time] -> Date
  - [Record] -> Record
  - []Value -> List
  - [Glob] -> Glob
  - [Closure] -> Closure
  - error -> LabeledError

[Nushell Value]: https://www.nushell.sh/contributor-book/plugin_protocol_reference.html#value-types
*/
type Value struct {
	Value any
	Span  Span
}

type Span struct {
	Start int `msgpack:"start"`
	End   int `msgpack:"end"`
}

/*
Filesize is Nushell [Filesize Value] type.

[Filesize Value]: https://www.nushell.sh/contributor-book/plugin_protocol_reference.html#filesize
*/
type Filesize int64

/*
Glob is Nushell [Glob Value] type - a filesystem glob, selecting multiple files or
directories depending on the expansion of wildcards.

Note that [Go stdlib glob] implementation doesn't support doublestar / globstar
pattern but thirdparty libraries which do exist.

[Glob Value]: https://www.nushell.sh/contributor-book/plugin_protocol_reference.html#glob
[Go stdlib glob]: https://pkg.go.dev/path/filepath#Glob
*/
type Glob struct {
	Value string
	// If true, the expansion of wildcards is disabled and Value should be treated
	// as a literal path.
	NoExpand bool
}

type Record map[string]Value

/*
Closure [Value] is a reference to a parsed block of Nushell code, with variables
captured from scope.

The plugin should not try to inspect the contents of the closure. It is recommended
that this is only used as an argument to the [ExecCommand.EvalClosure] engine call.
*/
type Closure struct {
	BlockID  uint               `msgpack:"block_id"`
	Captures msgpack.RawMessage `msgpack:"captures"`
}

var _ msgpack.CustomEncoder = (*Value)(nil)

func (v *Value) EncodeMsgpack(enc *msgpack.Encoder) error {
	err := enc.EncodeMapLen(1)
	if err != nil {
		return err
	}

	switch tv := v.Value.(type) {
	case bool:
		if err := startValue(enc, "Bool"); err != nil {
			return err
		}
		err = enc.EncodeBool(tv)
	case Filesize:
		if err := startValue(enc, "Filesize"); err != nil {
			return err
		}
		err = enc.EncodeInt64(int64(tv))
	case time.Duration:
		if err := startValue(enc, "Duration"); err != nil {
			return err
		}
		err = enc.EncodeInt64(tv.Nanoseconds())
	case time.Time:
		if err := startValue(enc, "Date"); err != nil {
			return err
		}
		err = enc.EncodeString(tv.Format(time.RFC3339))
	case int:
		if err := startValue(enc, "Int"); err != nil {
			return err
		}
		err = enc.EncodeInt(int64(tv))
	case int8:
		if err := startValue(enc, "Int"); err != nil {
			return err
		}
		err = enc.EncodeInt8(tv)
	case int16:
		if err := startValue(enc, "Int"); err != nil {
			return err
		}
		err = enc.EncodeInt16(tv)
	case int32:
		if err := startValue(enc, "Int"); err != nil {
			return err
		}
		err = enc.EncodeInt32(tv)
	case int64:
		if err := startValue(enc, "Int"); err != nil {
			return err
		}
		err = enc.EncodeInt64(tv)
	case uint:
		if err := startValue(enc, "Int"); err != nil {
			return err
		}
		err = enc.EncodeUint(uint64(tv))
	case uint8:
		if err := startValue(enc, "Int"); err != nil {
			return err
		}
		err = enc.EncodeUint8(tv)
	case uint16:
		if err := startValue(enc, "Int"); err != nil {
			return err
		}
		err = enc.EncodeUint16(tv)
	case uint32:
		if err := startValue(enc, "Int"); err != nil {
			return err
		}
		err = enc.EncodeUint32(tv)
	case uint64:
		if err := startValue(enc, "Int"); err != nil {
			return err
		}
		err = enc.EncodeUint64(tv)
	case float32:
		if err := startValue(enc, "Float"); err != nil {
			return err
		}
		err = enc.EncodeFloat32(tv)
	case float64:
		if err := startValue(enc, "Float"); err != nil {
			return err
		}
		err = enc.EncodeFloat64(tv)
	case string:
		if err := startValue(enc, "String"); err != nil {
			return err
		}
		err = enc.EncodeString(tv)
	case []byte:
		if err := startValue(enc, "Binary"); err != nil {
			return err
		}
		err = enc.EncodeBytes(tv)
	case Record:
		if err := startValue(enc, "Record"); err != nil {
			return err
		}
		if err := enc.EncodeMapLen(len(tv)); err != nil {
			return err
		}
		for k, v := range tv {
			if err := enc.EncodeString(k); err != nil {
				return err
			}
			if err := enc.EncodeValue(reflect.ValueOf(&v)); err != nil {
				return err
			}
		}
	case []Value:
		err = encodeValueList(enc, tv)
	case Closure:
		if err := startValue(enc, "Closure"); err != nil {
			return err
		}
		err = enc.EncodeValue(reflect.ValueOf(&tv))
	case Glob:
		err = encodeGlob(enc, &tv)
	case error:
		err = encodeLabeledError(enc, AsLabeledError(tv))
	case LabeledError:
		err = encodeLabeledError(enc, &tv)
	case nil:
		if err := enc.EncodeString("Nothing"); err != nil {
			return err
		}
		if err := enc.EncodeMapLen(1); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported Value type %T", tv)
	}
	if err != nil {
		return fmt.Errorf("encoding %T Value", v.Value)
	}

	if err := enc.EncodeString("span"); err != nil {
		return err
	}
	if err := enc.EncodeValue(reflect.ValueOf(&v.Span)); err != nil {
		return fmt.Errorf("encoding span: %w", err)
	}

	return nil
}

/*
startValue outputs key "typeName" with value of map with two items of
which first key "val" is created too. So the caller has to output value
of "val" and one more key-value pair:

	"typeName": { "val": | }
*/
func startValue(enc *msgpack.Encoder, typeName string) error {
	if err := enc.EncodeString(typeName); err != nil {
		return err
	}
	if err := enc.EncodeMapLen(2); err != nil {
		return err
	}
	return enc.EncodeString("val")
}

func encodeValueList(enc *msgpack.Encoder, items []Value) error {
	if err := enc.EncodeString("List"); err != nil {
		return err
	}
	if err := enc.EncodeMapLen(2); err != nil {
		return err
	}
	if err := enc.EncodeString("vals"); err != nil {
		return err
	}
	if err := enc.EncodeArrayLen(len(items)); err != nil {
		return err
	}
	for _, v := range items {
		if err := v.EncodeMsgpack(enc); err != nil {
			return err
		}
	}
	return nil
}

// encode Glob value minus the Span member of the Value
func encodeGlob(enc *msgpack.Encoder, glob *Glob) error {
	if err := enc.EncodeString("Glob"); err != nil {
		return err
	}
	if err := enc.EncodeMapLen(3); err != nil {
		return err
	}
	if err := enc.EncodeString("val"); err != nil {
		return err
	}
	if err := enc.EncodeString(glob.Value); err != nil {
		return err
	}
	if err := enc.EncodeString("no_expand"); err != nil {
		return err
	}
	if err := enc.EncodeBool(glob.NoExpand); err != nil {
		return err
	}
	return nil
}

func encodeLabeledError(enc *msgpack.Encoder, le *LabeledError) error {
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

var _ msgpack.CustomDecoder = (*Value)(nil)

func (v *Value) DecodeMsgpack(dec *msgpack.Decoder) error {
	name, err := decodeWrapperMap(dec)
	if err != nil {
		return err
	}
	switch name {
	case "Glob":
		return decodeGlob(dec, v)
	default:
		return v.decodeValue(dec, name)
	}
}

func (v *Value) decodeValue(dec *msgpack.Decoder, typeName string) error {
	n, err := dec.DecodeMapLen()
	if err != nil {
		return err
	}
	if n == -1 {
		return nil
	}

	for idx := 0; idx < n; idx++ {
		fieldName, err := dec.DecodeString()
		if err != nil {
			return fmt.Errorf("decoding field name [%d/%d] of %s: %w", idx+1, n, typeName, err)
		}
		switch fieldName {
		case "val":
			switch typeName {
			case "Bool":
				v.Value, err = dec.DecodeBool()
			case "Binary":
				v.Value, err = decodeBinary(dec)
			case "String":
				v.Value, err = dec.DecodeString()
			case "Int":
				v.Value, err = dec.DecodeInt64()
			case "Float":
				v.Value, err = dec.DecodeFloat64()
			case "Filesize":
				var fs int64
				fs, err = dec.DecodeInt64()
				v.Value = Filesize(fs)
			case "Duration":
				var d int64
				d, err = dec.DecodeInt64()
				v.Value = time.Nanosecond * time.Duration(d)
			case "Date":
				var d string
				if d, err = dec.DecodeString(); err != nil {
					return fmt.Errorf("reading Date value as string: %w", err)
				}
				v.Value, err = time.Parse(time.RFC3339, d)
			case "Record":
				rec := Record{}
				err = dec.DecodeValue(reflect.ValueOf(&rec))
				v.Value = rec
			case "Closure":
				c := Closure{}
				err = dec.DecodeValue(reflect.ValueOf(&c))
				v.Value = c
			default:
				return fmt.Errorf("unsupported Value type %q", typeName)
			}
		case "vals":
			if typeName != "List" {
				return fmt.Errorf("expected type to be 'List', got %q", typeName)
			}
			cnt, err := dec.DecodeArrayLen()
			if err != nil {
				return err
			}
			if cnt < 1 {
				return nil
			}
			lst := make([]Value, cnt)
			for i := 0; i < cnt; i++ {
				if err := lst[i].DecodeMsgpack(dec); err != nil {
					return fmt.Errorf("decoding List item [%d/%d]: %w", i+1, cnt, err)
				}
			}
			v.Value = lst
		case "error":
			le := LabeledError{}
			err = dec.DecodeValue(reflect.ValueOf(&le))
			v.Value = le
		case "span":
			err = dec.DecodeValue(reflect.ValueOf(&v.Span))
		default:
			return fmt.Errorf("unsupported Value field %q", fieldName)
		}

		if err != nil {
			return fmt.Errorf("decoding field %s of %s: %w", fieldName, typeName, err)
		}
	}

	return nil
}

func decodeBinary(dec *msgpack.Decoder) ([]byte, error) {
	c, err := dec.PeekCode()
	if err != nil {
		return nil, fmt.Errorf("peeking Binary start code: %w", err)
	}
	switch {
	case msgpcode.IsBin(c):
		return dec.DecodeBytes()
	case msgpcode.IsFixedArray(c) || c == msgpcode.Array16 || c == msgpcode.Array32:
		n, err := dec.DecodeArrayLen()
		if err != nil {
			return nil, fmt.Errorf("reading Binary array length: %w", err)
		}
		if n < 1 {
			return nil, nil
		}
		// just "dec.ReadFull(buf)" won't work as uint8 might be encoded using
		// two bytes per value but ArrayLen gives us count of items (not bytes)
		buf := make([]byte, n)
		for i := 0; i < n; i++ {
			if buf[i], err = dec.DecodeUint8(); err != nil {
				return nil, fmt.Errorf("reading array item [%d]: %w", i, err)
			}
		}
		return buf, nil
	default:
		return nil, fmt.Errorf("unsupported Binary value starting %x", c)
	}
}

// the enclosing map has been red and we need to decode the struct itself.
func decodeGlob(dec *msgpack.Decoder, value *Value) error {
	n, err := dec.DecodeMapLen()
	if err != nil {
		return err
	}
	if n == -1 {
		return nil
	}

	g := Glob{}
	for idx := 0; idx < n; idx++ {
		fieldName, err := dec.DecodeString()
		if err != nil {
			return fmt.Errorf("decoding field name [%d/%d] of Glob: %w", idx+1, n, err)
		}
		switch fieldName {
		case "val":
			g.Value, err = dec.DecodeString()
		case "no_expand":
			g.NoExpand, err = dec.DecodeBool()
		case "span":
			err = dec.DecodeValue(reflect.ValueOf(&value.Span))
		default:
			return fmt.Errorf("unsupported Glob Value field %q", fieldName)
		}
		if err != nil {
			return fmt.Errorf("decoding field %s of Glob: %w", fieldName, err)
		}
	}
	value.Value = g
	return nil
}
