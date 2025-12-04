/*
Package types defines types and functions to describe Nu types in plugin signatures.
*/
package types

import (
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

/*
Type describes how [Values] are represented.

https://docs.rs/nu-protocol/latest/nu_protocol/enum.Type.html

[Values]: https://pkg.go.dev/github.com/ainvaltin/nu-plugin#Value
*/
type Type interface {
	EncodeMsgpack(enc *msgpack.Encoder) error

	encodeMsgpack(enc *msgpack.Encoder) error
}

/*
RecordDef is the "field list" of the [Record] and [Table] Type.
The key is field name and value is the type of the field.
*/
type RecordDef map[string]Type

type nuType struct {
	typ      string
	itmTypes []Type
	fields   RecordDef
	name     string
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
		"ListStream",
		"Nothing",
		"Number",
		"Range",
		"Signature",
		"String",
		"Glob":
		return enc.EncodeString(ss.typ)
	case "Custom": // Custom(Box<str>),
		if err := encodeMapStart(enc, ss.typ); err != nil {
			return err
		}
		return enc.EncodeString(ss.name)
	case "List": // List(Box<Type>),
		if err := encodeMapStart(enc, ss.typ); err != nil {
			return err
		}
		return ss.itmTypes[0].encodeMsgpack(enc)
	case "OneOf": // OneOf(Box<[Type]>)
		if err := encodeMapStart(enc, ss.typ); err != nil {
			return err
		}
		if err := enc.EncodeArrayLen(len(ss.itmTypes)); err != nil {
			return err
		}
		for _, t := range ss.itmTypes {
			if err := t.encodeMsgpack(enc); err != nil {
				return err
			}
		}
	case "Record", "Table": // Record(Box<[(String, Type)]>), Table(Box<[(String, Type)]>),
		if err := encodeMapStart(enc, ss.typ); err != nil {
			return err
		}
		if err := enc.EncodeArrayLen(len(ss.fields)); err != nil {
			return err
		}
		for k, v := range ss.fields {
			if err := encodeRecordItem(enc, k, v); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported Type: %q", ss.typ)
	}
	return nil
}

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
	return &nuType{typ: "List", itmTypes: []Type{itemType}}
}

func ListStream() Type {
	return &nuType{typ: "ListStream"}
}

func Nothing() Type {
	return &nuType{typ: "Nothing"}
}

func Number() Type {
	return &nuType{typ: "Number"}
}

func OneOf(itemTypes ...Type) Type {
	return &nuType{typ: "OneOf", itmTypes: itemTypes}
}

func Range() Type {
	return &nuType{typ: "Range"}
}

func Record(fields RecordDef) Type {
	return &nuType{typ: "Record", fields: fields}
}

func Signature() Type {
	return &nuType{typ: "Signature"}
}

func String() Type {
	return &nuType{typ: "String"}
}

func Table(fields RecordDef) Type {
	return &nuType{typ: "Table", fields: fields}
}

func encodeMapStart(enc *msgpack.Encoder, key string) error {
	if err := enc.EncodeMapLen(1); err != nil {
		return err
	}
	if err := enc.EncodeString(key); err != nil {
		return err
	}
	return nil
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
