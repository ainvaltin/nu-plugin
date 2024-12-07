package syntaxshape

import (
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

/*
The syntactic shapes that describe how a sequence should be parsed.

Use functions in this package to construct concrete syntax shape.

https://docs.rs/nu-protocol/latest/nu_protocol/enum.SyntaxShape.html
*/
type SyntaxShape interface {
	EncodeMsgpack(enc *msgpack.Encoder) error

	encodeMsgpack(enc *msgpack.Encoder) error
}

/*
RecordDef is the "field list" of the [Record] and [Table] Syntax Shapes.
The key is field name and value is the type of the field.
*/
type RecordDef map[string]SyntaxShape

type syntaxShape struct {
	typ     string
	itmType []SyntaxShape
	fields  RecordDef
}

func (ss *syntaxShape) EncodeMsgpack(enc *msgpack.Encoder) error {
	return ss.encodeMsgpack(enc)
}

func (ss *syntaxShape) encodeMsgpack(enc *msgpack.Encoder) error {
	switch ss.typ {
	case "Any",
		"Binary",
		"Block",
		"Boolean",
		"CellPath",
		"DateTime",
		"Directory",
		"Duration",
		"Error",
		"Expression",
		"ExternalArgument",
		"Filepath",
		"Filesize",
		"Float",
		"FullCellPath",
		"GlobPattern",
		"Int",
		"ImportPattern",
		"MathExpression",
		"MatchBlock",
		"Nothing",
		"Number",
		"Operator",
		"Range",
		"RowCondition",
		"Signature",
		"String",
		"VarWithOptType":
		return enc.EncodeString(ss.typ)
	case "Closure": // Closure(Option<Vec<SyntaxShape>>)
		if err := encodeMapStart(enc, "Closure"); err != nil {
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
	case "List": // List(Box<SyntaxShape>)
		if err := encodeMapStart(enc, "List"); err != nil {
			return err
		}
		return ss.itmType[0].encodeMsgpack(enc)
	case "OneOf": // OneOf(Vec<SyntaxShape>)
		if err := encodeMapStart(enc, "OneOf"); err != nil {
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
	case "Record", "Table": // Record(Vec<(String, SyntaxShape)>)
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
		return fmt.Errorf("unsupported SyntaxShape: %q", ss.typ)
	}
	return nil
}

// Any syntactic form is allowed.
func Any() SyntaxShape {
	return &syntaxShape{typ: "Any"}
}

// A binary literal.
func Binary() SyntaxShape {
	return &syntaxShape{typ: "Binary"}
}

func Block() SyntaxShape {
	return &syntaxShape{typ: "Block"}
}

func Boolean() SyntaxShape {
	return &syntaxShape{typ: "Boolean"}
}

func CellPath() SyntaxShape {
	return &syntaxShape{typ: "CellPath"}
}

func Closure(args ...SyntaxShape) SyntaxShape {
	return &syntaxShape{typ: "Closure"}
}

func DateTime() SyntaxShape {
	return &syntaxShape{typ: "DateTime"}
}

func Directory() SyntaxShape {
	return &syntaxShape{typ: "Directory"}
}

func Duration() SyntaxShape {
	return &syntaxShape{typ: "Duration"}
}

func Error() SyntaxShape {
	return &syntaxShape{typ: "Error"}
}

func Expression() SyntaxShape {
	return &syntaxShape{typ: "Expression"}
}

func ExternalArgument() SyntaxShape {
	return &syntaxShape{typ: "ExternalArgument"}
}

func Filepath() SyntaxShape {
	return &syntaxShape{typ: "Filepath"}
}

func Filesize() SyntaxShape {
	return &syntaxShape{typ: "Filesize"}
}

func Float() SyntaxShape {
	return &syntaxShape{typ: "Float"}
}

func FullCellPath() SyntaxShape {
	return &syntaxShape{typ: "FullCellPath"}
}

func GlobPattern() SyntaxShape {
	return &syntaxShape{typ: "GlobPattern"}
}

func Int() SyntaxShape {
	return &syntaxShape{typ: "Int"}
}

func ImportPattern() SyntaxShape {
	return &syntaxShape{typ: "ImportPattern"}
}

func List(itemType SyntaxShape) SyntaxShape {
	return &syntaxShape{typ: "List", itmType: []SyntaxShape{itemType}}
}

func MathExpression() SyntaxShape {
	return &syntaxShape{typ: "MathExpression"}
}

func MatchBlock() SyntaxShape {
	return &syntaxShape{typ: "MatchBlock"}
}

func Nothing() SyntaxShape {
	return &syntaxShape{typ: "Nothing"}
}

// Only a numeric (integer or float) value is allowed
func Number() SyntaxShape {
	return &syntaxShape{typ: "Number"}
}

func OneOf(itemType ...SyntaxShape) SyntaxShape {
	return &syntaxShape{typ: "OneOf", itmType: itemType}
}

func Operator() SyntaxShape {
	return &syntaxShape{typ: "Operator"}
}

func Range() SyntaxShape {
	return &syntaxShape{typ: "Range"}
}

/*
Record describes record type, ie

	Shape: syntaxshape.Record(syntaxshape.RecordDef{"a": syntaxshape.Int(), "b": syntaxshape.String()})
*/
func Record(fields RecordDef) SyntaxShape {
	return &syntaxShape{typ: "Record", fields: fields}
}

func RowCondition() SyntaxShape {
	return &syntaxShape{typ: "RowCondition"}
}

func Signature() SyntaxShape {
	return &syntaxShape{typ: "Signature"}
}

func String() SyntaxShape {
	return &syntaxShape{typ: "String"}
}

func Table(fields RecordDef) SyntaxShape {
	return &syntaxShape{typ: "Table", fields: fields}
}

// A variable with optional type, `x` or `x: int`
func VarWithOptType() SyntaxShape {
	return &syntaxShape{typ: "VarWithOptType"}
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

func encodeRecordItem(enc *msgpack.Encoder, name string, typ SyntaxShape) error {
	if err := enc.EncodeArrayLen(2); err != nil {
		return err
	}
	if err := enc.EncodeString(name); err != nil {
		return err
	}
	return typ.encodeMsgpack(enc)
}
