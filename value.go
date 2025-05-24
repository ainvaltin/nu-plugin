package nu

import (
	"fmt"
	"math"
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
  - Block -> [Block]
  - Range -> [IntRange]
  - CellPath -> [CellPath]

Outgoing values are encoded as:

  - nil -> Nothing
  - int, int8, int16, int32, int64 -> Int
  - uint, uint8, uint16, uint32, uint64 -> Int
  - float64, float32 -> Float
  - bool -> Bool
  - []byte -> Binary
  - string -> String
  - error -> LabeledError
  - [Filesize] -> Filesize
  - [time.Duration] -> Duration
  - [time.Time] -> Date
  - [Record] -> Record
  - []Value -> List
  - [Glob] -> Glob
  - [Closure] -> Closure
  - [Block] -> Block
  - [IntRange] -> Range
  - [CustomValue] -> Custom
  - [CellPath] -> CellPath

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

func (v Span) encodeMsgpack(enc *msgpack.Encoder) error {
	if err := enc.EncodeMapLen(2); err != nil {
		return err
	}
	if err := enc.EncodeString("start"); err != nil {
		return err
	}
	if err := enc.EncodeInt(int64(v.Start)); err != nil {
		return err
	}
	if err := enc.EncodeString("end"); err != nil {
		return err
	}
	if err := enc.EncodeInt(int64(v.End)); err != nil {
		return err
	}
	return nil
}

func (v *Span) decodeMsgpack(dec *msgpack.Decoder) error {
	cnt, err := dec.DecodeMapLen()
	if err != nil {
		return err
	}
	if cnt != 2 {
		return fmt.Errorf("expected span map to contain two keys, got %d", cnt)
	}
	for range cnt {
		key, err := dec.DecodeString()
		if err != nil {
			return err
		}
		switch key {
		case "start":
			v.Start, err = dec.DecodeInt()
		case "end":
			v.End, err = dec.DecodeInt()
		}
		if err != nil {
			return fmt.Errorf("decoding %s value: %w", key, err)
		}
	}
	return nil
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

func (r Record) encodeMsgpack(enc *msgpack.Encoder, p *Plugin) error {
	if err := startValue(enc, "Record"); err != nil {
		return err
	}
	if err := enc.EncodeMapLen(len(r)); err != nil {
		return err
	}
	for k, v := range r {
		if err := enc.EncodeString(k); err != nil {
			return err
		}
		if err := v.encodeMsgpack(enc, p); err != nil {
			return fmt.Errorf("encode record field %s value: %w", k, err)
		}
	}
	return nil
}

func decodeRecord(dec *msgpack.Decoder, p *Plugin) (rec Record, err error) {
	var cnt int
	if cnt, err = dec.DecodeMapLen(); err != nil {
		return rec, fmt.Errorf("decoding Record field count: %w", err)
	}
	var name string
	rec = Record{}
	for range cnt {
		if name, err = dec.DecodeString(); err != nil {
			return rec, fmt.Errorf("decoding field name: %w", err)
		}
		var v Value
		if err = v.decodeMsgpack(dec, p); err != nil {
			return rec, fmt.Errorf("decoding field %s value: %w", name, err)
		}
		rec[name] = v
	}
	return rec, nil
}

/*
Closure [Value] is a reference to a parsed block of Nushell code, with variables
captured from scope.

The plugin should not try to inspect the contents of the closure. It is recommended
that this is only used as an argument to the [ExecCommand.EvalClosure] engine call.
*/
type Closure struct {
	BlockID  uint
	Captures msgpack.RawMessage
}

func (c Closure) encodeMsgpack(enc *msgpack.Encoder) error {
	if err := enc.EncodeMapLen(2); err != nil {
		return err
	}
	if err := enc.EncodeString("block_id"); err != nil {
		return err
	}
	if err := enc.EncodeUint(uint64(c.BlockID)); err != nil {
		return err
	}
	if err := enc.EncodeString("captures"); err != nil {
		return err
	}
	if c.Captures == nil {
		return enc.EncodeNil()
	} else {
		return c.Captures.EncodeMsgpack(enc)
	}
}

func decodeClosure(dec *msgpack.Decoder) (c Closure, _ error) {
	cnt, err := dec.DecodeMapLen()
	if err != nil {
		return c, err
	}
	if cnt != 2 {
		return c, fmt.Errorf("expected Closure to contain 2 keys, got %d", cnt)
	}

	var code byte
	for range cnt {
		key, err := dec.DecodeString()
		if err != nil {
			return c, err
		}
		switch key {
		case "block_id":
			c.BlockID, err = dec.DecodeUint()
		case "captures":
			code, err = dec.PeekCode()
			if err != nil {
				return c, fmt.Errorf("peeking 'captures' value type: %w", err)
			}
			switch code {
			case msgpcode.Nil:
				err = dec.DecodeNil()
			default:
				err = c.Captures.DecodeMsgpack(dec)
			}
		}
		if err != nil {
			return c, fmt.Errorf("decoding key %q: %w", key, err)
		}
	}
	return c, nil
}

/*
Block is Nushell [Block Value] type.

[Block Value]: https://www.nushell.sh/contributor-book/plugin_protocol_reference.html#block
*/
type Block uint64

func (v *Value) encodeMsgpack(enc *msgpack.Encoder, p *Plugin) error {
	err := enc.EncodeMapLen(1)
	if err != nil {
		return err
	}

	switch tv := v.Value.(type) {
	case int:
		err = encodeInt(enc, "Int", int64(tv))
	case int8:
		err = encodeInt(enc, "Int", int64(tv))
	case int16:
		err = encodeInt(enc, "Int", int64(tv))
	case int32:
		err = encodeInt(enc, "Int", int64(tv))
	case int64:
		err = encodeInt(enc, "Int", int64(tv))
	case uint:
		err = encodeUInt(enc, uint64(tv))
	case uint8:
		err = encodeInt(enc, "Int", int64(tv))
	case uint16:
		err = encodeInt(enc, "Int", int64(tv))
	case uint32:
		err = encodeInt(enc, "Int", int64(tv))
	case uint64:
		err = encodeUInt(enc, tv)
	case Filesize:
		err = encodeInt(enc, "Filesize", int64(tv))
	case time.Duration:
		err = encodeInt(enc, "Duration", tv.Nanoseconds())
	case Block:
		err = encodeInt(enc, "Block", int64(tv))
	case float32:
		if err = startValue(enc, "Float"); err == nil {
			err = enc.EncodeFloat32(tv)
		}
	case float64:
		if err = startValue(enc, "Float"); err == nil {
			err = enc.EncodeFloat64(tv)
		}
	case bool:
		if err = startValue(enc, "Bool"); err == nil {
			err = enc.EncodeBool(tv)
		}
	case time.Time:
		if err = startValue(enc, "Date"); err == nil {
			err = enc.EncodeString(tv.Format(time.RFC3339))
		}
	case string:
		if err = startValue(enc, "String"); err == nil {
			err = enc.EncodeString(tv)
		}
	case []byte:
		if err := startValue(enc, "Binary"); err != nil {
			return err
		}
		// EncodeBytes encodes nil slice as NIL but Nu doesn't like it:
		// Plugin failed to decode: invalid type: unit value, expected a sequence
		if tv == nil {
			err = enc.EncodeBytesLen(0)
		} else {
			err = enc.EncodeBytes(tv)
		}
	case Record:
		err = tv.encodeMsgpack(enc, p)
	case []Value:
		err = encodeValueList(enc, tv, p)
	case Closure:
		if err = startValue(enc, "Closure"); err == nil {
			err = tv.encodeMsgpack(enc)
		}
	case Glob:
		err = encodeGlob(enc, &tv)
	case IntRange:
		if err = startValue(enc, "Range"); err == nil {
			err = tv.encodeMsgpack(enc)
		}
	case error:
		err = encodeLabeledError(enc, AsLabeledError(tv))
	case LabeledError:
		err = encodeLabeledError(enc, &tv)
	case nil:
		if err = enc.EncodeString("Nothing"); err == nil {
			err = enc.EncodeMapLen(1)
		}
	case CustomValue:
		if err := startValue(enc, "Custom"); err != nil {
			return err
		}
		id := p.idGen.Add(1)
		if err = encodeCustomValue(enc, id, tv); err == nil {
			p.cvals[id] = tv
		}
	case CellPath:
		if err = startValue(enc, "CellPath"); err == nil {
			err = tv.encodeMsgpack(enc, p)
		}
	default:
		return fmt.Errorf("unsupported Value type %T", tv)
	}
	if err != nil {
		return fmt.Errorf("encoding %T Value: %w", v.Value, err)
	}

	if err := enc.EncodeString("span"); err != nil {
		return err
	}
	if err := v.Span.encodeMsgpack(enc); err != nil {
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

func encodeInt(enc *msgpack.Encoder, name string, v int64) error {
	if err := startValue(enc, name); err != nil {
		return err
	}
	return enc.EncodeInt(v)
}

func encodeUInt(enc *msgpack.Encoder, v uint64) error {
	if v > math.MaxInt64 {
		return fmt.Errorf("uint %d is too large for int64", v)
	}
	if err := startValue(enc, "Int"); err != nil {
		return err
	}
	return enc.EncodeUint(v)
}

func encodeValueList(enc *msgpack.Encoder, items []Value, p *Plugin) error {
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
		if err := v.encodeMsgpack(enc, p); err != nil {
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

func (v *Value) decodeMsgpack(dec *msgpack.Decoder, p *Plugin) error {
	name, err := decodeWrapperMap(dec)
	if err != nil {
		return fmt.Errorf("decodeWrapperMap: %w", err)
	}
	switch name {
	case "Glob":
		return decodeGlob(dec, v)
	default:
		return v.decodeValue(dec, name, p)
	}
}

func (v *Value) decodeValue(dec *msgpack.Decoder, typeName string, p *Plugin) error {
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
				v.Value, err = decodeRecord(dec, p)
			case "Closure":
				v.Value, err = decodeClosure(dec)
			case "Block":
				var id int64
				id, err = dec.DecodeInt64()
				v.Value = Block(id)
			case "Range":
				v.Value, err = decodeMsgpackRange(dec)
			case "Custom":
				v.Value, err = decodeCustomValue(dec, p)
			case "CellPath":
				c := CellPath{}
				err = c.decodeMsgpack(dec, p)
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
			lst := make([]Value, cnt)
			for i := 0; i < cnt; i++ {
				if err := lst[i].decodeMsgpack(dec, p); err != nil {
					return fmt.Errorf("decoding List item [%d/%d]: %w", i+1, cnt, err)
				}
			}
			v.Value = lst
		case "error":
			le := LabeledError{}
			err = dec.DecodeValue(reflect.ValueOf(&le))
			v.Value = le
		case "span":
			err = v.Span.decodeMsgpack(dec)
		default:
			return fmt.Errorf("unsupported field %q in %s Value", fieldName, typeName)
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
			err = value.Span.decodeMsgpack(dec)
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
