/*
Package types defines types and functions to describe Nu types in plugin signatures.
*/
package types

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/ainvaltin/nu-plugin/internal/mpack"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/vmihailenco/msgpack/v5/msgpcode"
)

/*
Type describes how [Values] are represented.

It corresponds to the Rust nu-protocol [enum Type].

Constructor functions (ie [String], [Int],...) are used to create Type instance.

[Values]: https://pkg.go.dev/github.com/ainvaltin/nu-plugin#Value
[enum Type]: https://docs.rs/nu-protocol/latest/nu_protocol/enum.Type.html
*/
type Type interface {
	String() string
	EncodeMsgpack(enc *msgpack.Encoder) error

	encodeMsgpack(enc *msgpack.Encoder) error
}

/*
RecordDef is the "field list" of the [Record] and [Table] Type.
The key is field name and value is the type of the field.
*/
type RecordDef map[string]Type

type nuType struct {
	typ     string
	itmType []Type
	fields  RecordDef
	name    string
}

/*
DecodeMsgpack decodes Type from decoder. The stream must be in a correct
position (in the beginning of a Type value).
*/
func DecodeMsgpack(dec *msgpack.Decoder) (Type, error) {
	t := &nuType{}
	return t, t.DecodeMsgpack(dec)
}

func (iot *nuType) DecodeMsgpack(dec *msgpack.Decoder) error {
	c, err := dec.PeekCode()
	if err != nil {
		return fmt.Errorf("peeking item type: %w", err)
	}

	switch {
	case msgpcode.IsFixedString(c), msgpcode.IsString(c):
		if iot.typ, err = dec.DecodeString(); err != nil {
			return fmt.Errorf("decoding type name: %w", err)
		}
		// todo: check do we recognize this type?
	case msgpcode.IsFixedMap(c):
		if iot.typ, err = mpack.DecodeWrapperMap(dec); err != nil {
			return err
		}

		switch iot.typ {
		case "Custom":
			if iot.name, err = dec.DecodeString(); err != nil {
				return fmt.Errorf("decode name of the Custom type: %w", err)
			}
		case "List":
			v := &nuType{}
			iot.itmType = []Type{v}
			return dec.Decode(v)
		case "OneOf": // OneOf(Box<[Type]>),
			cnt, err := dec.DecodeArrayLen()
			if err != nil {
				return fmt.Errorf("decoding number of types in the %s: %w", iot.typ, err)
			}
			iot.itmType = make([]Type, 0, cnt)
			for range cnt {
				i := nuType{}
				if err := i.DecodeMsgpack(dec); err != nil {
					return err
				}
				iot.itmType = append(iot.itmType, &i)
			}
		case "Record", "Table": // Record(Box<[(String, Type)]>), Table(Box<[(String, Type)]>),
			cnt, err := dec.DecodeArrayLen()
			if err != nil {
				return fmt.Errorf("decoding number of fields in the %s: %w", iot.typ, err)
			}
			iot.fields = make(RecordDef)
			for range cnt {
				name, typ, err := decodeRecordItem(dec)
				if err != nil {
					return err
				}
				iot.fields[name] = typ
			}
		default:
			return fmt.Errorf("unsupported nu Type name %q", iot.typ)
		}
	default:
		return fmt.Errorf("unsupported nu Type start code: %d", c)
	}

	return nil
}

func (ss *nuType) EncodeMsgpack(enc *msgpack.Encoder) error {
	return ss.encodeMsgpack(enc)
}

func (ss *nuType) encodeMsgpack(enc *msgpack.Encoder) error {
	switch ss.typ {
	case
		"Any",
		"Binary",
		"Block",
		"Bool",
		"CellPath",
		"Closure",
		"Date",
		"Duration",
		"Error",
		"Filesize",
		"Float",
		"Int",
		"Nothing",
		"Number",
		"Range",
		"String",
		"Glob":
		return enc.EncodeString(ss.typ)
	case "Custom": // Custom(Box<str>),
		if err := mpack.EncodeMapStart(enc, ss.typ); err != nil {
			return err
		}
		return enc.EncodeString(ss.name)
	case "List": // List(Box<Type>),
		if err := mpack.EncodeMapStart(enc, ss.typ); err != nil {
			return err
		}
		return ss.itmType[0].encodeMsgpack(enc)
	case "OneOf": // OneOf(Box<[Type]>),
		if err := mpack.EncodeMapStart(enc, ss.typ); err != nil {
			return err
		}
		if err := enc.EncodeArrayLen(len(ss.itmType)); err != nil {
			return err
		}
		for _, v := range ss.itmType {
			if err := v.encodeMsgpack(enc); err != nil {
				return err
			}
		}
	case "Record", "Table": // Record(Box<[(String, Type)]>), Table(Box<[(String, Type)]>),
		if err := mpack.EncodeMapStart(enc, ss.typ); err != nil {
			return err
		}
		if err := enc.EncodeArrayLen(len(ss.fields)); err != nil {
			return err
		}
		for k, v := range ss.fields {
			if err := encodeRecordItem(enc, k, v); err != nil {
				return fmt.Errorf("encoding %s field %s: %w", ss.typ, k, err)
			}
		}
	default:
		return fmt.Errorf("unsupported Type: %q", ss.typ)
	}
	return nil
}

func (ss *nuType) String() string {
	switch ss.typ {
	case "Custom": // Custom(Box<str>),
		return fmt.Sprintf("Custom(%s)", ss.name)
	case "List": // List(Box<Type>),
		return fmt.Sprintf("List(%s)", ss.itmType[0])
	case "OneOf": // OneOf(Box<[Type]>),
		names := make([]string, len(ss.itmType))
		for x, v := range ss.itmType {
			names[x] = v.String()
		}
		return fmt.Sprintf("OneOf([%s])", strings.Join(names, ", "))
	case "Record", "Table": // Record(Box<[(String, Type)]>), Table(Box<[(String, Type)]>),
		names := make([]string, 0, len(ss.fields))
		// sort the keys to have a stable output
		for _, key := range slices.Sorted(maps.Keys(ss.fields)) {
			names = append(names, fmt.Sprintf("(%s, %s)", key, ss.fields[key].String()))
		}
		return fmt.Sprintf("%s([%s])", ss.typ, strings.Join(names, ", "))
	}

	return ss.typ
}

func (ss *nuType) equal(b *nuType) bool {
	if (ss == nil && b != nil) || (ss != nil && b == nil) {
		return false
	}
	if ss.typ != b.typ || ss.name != b.name {
		return false
	}
	if !slices.EqualFunc(ss.itmType, b.itmType, compareTypes) {
		return false
	}
	if !maps.EqualFunc(ss.fields, b.fields, compareTypes) {
		return false
	}
	return true
}

func compareTypes(a, b Type) bool {
	tA, okA := a.(*nuType)
	tB, okB := b.(*nuType)
	return (okA && okB) && tA.equal(tB)
}

// Top type, supertype of all types.
func Any() Type {
	return &nuType{typ: "Any"}
}

func Binary() Type {
	return &nuType{typ: "Binary"}
}

func Block() Type {
	return &nuType{typ: "Block"}
}

func Bool() Type {
	return &nuType{typ: "Bool"}
}

func CellPath() Type {
	return &nuType{typ: "CellPath"}
}

func Closure() Type {
	return &nuType{typ: "Closure"}
}

func Custom(name string) Type {
	return &nuType{typ: "Custom", name: name}
}

func Date() Type {
	return &nuType{typ: "Date"}
}

func Duration() Type {
	return &nuType{typ: "Duration"}
}

func Error() Type {
	return &nuType{typ: "Error"}
}

func Filesize() Type {
	return &nuType{typ: "Filesize"}
}

func Float() Type {
	return &nuType{typ: "Float"}
}

func Glob() Type {
	return &nuType{typ: "Glob"}
}

func Int() Type {
	return &nuType{typ: "Int"}
}

func List(itemType Type) Type {
	return &nuType{typ: "List", itmType: []Type{itemType}}
}

// Supertype of all types it contains.
func OneOf(types ...Type) Type {
	return &nuType{typ: "OneOf", itmType: types}
}

func Nothing() Type {
	return &nuType{typ: "Nothing"}
}

// Supertype of [Int] and [Float]. Equivalent to oneof<int, float>
func Number() Type {
	return &nuType{typ: "Number"}
}

func Range() Type {
	return &nuType{typ: "Range"}
}

func Record(fields RecordDef) Type {
	return &nuType{typ: "Record", fields: fields}
}

func String() Type {
	return &nuType{typ: "String"}
}

func Table(fields RecordDef) Type {
	return &nuType{typ: "Table", fields: fields}
}

func encodeRecordItem(enc *msgpack.Encoder, name string, typ Type) error {
	if err := enc.EncodeArrayLen(2); err != nil {
		return err
	}
	if err := enc.EncodeString(name); err != nil {
		return err
	}
	return typ.encodeMsgpack(enc)
}

func decodeRecordItem(dec *msgpack.Decoder) (string, Type, error) {
	cnt, err := dec.DecodeArrayLen()
	if err != nil {
		return "", nil, err
	}
	if cnt != 2 {
		return "", nil, fmt.Errorf("expected two item array, got %d", cnt)
	}

	name, err := dec.DecodeString()
	if err != nil {
		return "", nil, err
	}

	typ := &nuType{}
	if err = typ.DecodeMsgpack(dec); err != nil {
		return "", nil, err
	}

	return name, typ, nil
}
