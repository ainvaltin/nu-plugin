/*
Package operator defines Operator type and constants for Nu Custom Value support.

It is used by the [github.com/ainvaltin/nu-plugin] to support implementing CustomValue.
*/
package operator

import (
	"fmt"
	"slices"

	"github.com/vmihailenco/msgpack/v5"
)

/*
Operator is argument type for the Operation CustomValueOp call.

https://docs.rs/nu-protocol/latest/nu_protocol/ast/enum.Operator.html
*/
type Operator uint32

// Operator "Classes"
const (
	Comparison = iota << 16
	Math
	Boolean
	Bits
	Assignment
)

const (
	Boolean_Or  Operator = Boolean + iota
	Boolean_Xor          // xor
	Boolean_And          // and
)

const (
	Bits_Or         Operator = Bits + iota
	Bits_Xor                 // bit-xor
	Bits_And                 // bit-and
	Bits_ShiftLeft           // bit-shl
	Bits_ShiftRight          // bit-shr
)

const (
	Assignment_Assign      Operator = Assignment + iota // plain assignment
	Assignment_Add                                      // +=
	Assignment_Subtract                                 // -=
	Assignment_Multiply                                 // *=
	Assignment_Divide                                   // /=
	Assignment_Concatenate                              // +=
)

const (
	Comparison_Equal    Operator = Comparison + iota // ==
	Comparison_NotEqual                              // !=
	Comparison_LessThan
	Comparison_GreaterThan
	Comparison_LessThanOrEqual
	Comparison_GreaterThanOrEqual
	Comparison_RegexMatch    // =~ or like
	Comparison_NotRegexMatch // !~ or not-like
	Comparison_In            // in
	Comparison_NotIn         // not-in
	Comparison_Has           // has
	Comparison_NotHas        // not-has
	Comparison_StartsWith    // starts-with
	Comparison_EndsWith      // ends-with
)

const (
	Math_Add         Operator = Math + iota // +
	Math_Subtract                           // -
	Math_Multiply                           // *
	Math_Divide                             // /
	Math_FloorDivide                        // //
	Math_Modulo                             // mod
	Math_Pow                                // **
	Math_Concatenate
)

/*
Class returns [Comparison], [Math], [Boolean], [Bits] or [Assignment]
*/
func (op Operator) Class() int {
	return int(op & 0xFFFF0000)
}

func (op Operator) String() string {
	return op_classes[op>>16].class + "." + op_classes[op>>16].op[op&0xFFFF]
}

func (op *Operator) DecodeMsgpack(dec *msgpack.Decoder) error {
	// single item map like {"Math": "Plus"}
	className, err := decodeWrapperMap(dec)
	if err != nil {
		return err
	}
	idx := slices.Index(op_class_names, className)
	if idx == -1 {
		return fmt.Errorf("unknown Operator class %q", className)
	}
	*op = Operator(idx << 16)

	opName, err := dec.DecodeString()
	if err != nil {
		return err
	}
	if idx = slices.Index(op_classes[idx].op, opName); idx == -1 {
		return fmt.Errorf("unknown Operator %q in class %q", opName, className)
	}
	*op += Operator(idx)
	return nil
}

var op_class_names = []string{"Comparison", "Math", "Boolean", "Bits", "Assignment"}

var op_classes = [...]struct {
	cid   int
	class string
	op    []string
}{
	{cid: Comparison, class: "Comparison", op: []string{"Equal", "NotEqual", "LessThan", "GreaterThan", "LessThanOrEqual", "GreaterThanOrEqual", "RegexMatch", "NotRegexMatch", "In", "NotIn", "Has", "NotHas", "StartsWith", "EndsWith"}},
	{cid: Math, class: "Math", op: []string{"Add", "Subtract", "Multiply", "Divide", "FloorDivide", "Modulo", "Pow", "Concatenate"}},
	{cid: Boolean, class: "Boolean", op: []string{"Or", "Xor", "And"}},
	{cid: Bits, class: "Bits", op: []string{"BitOr", "BitXor", "BitAnd", "ShiftLeft", "ShiftRight"}},
	{cid: Assignment, class: "Assignment", op: []string{"Assign", "AddAssign", "SubtractAssign", "MultiplyAssign", "DivideAssign", "ConcatenateAssign"}},
}

/*
decodeWrapperMap reads the "single item map" whose key is string - the
key name is returned and decoder is ready to read the value.
*/
func decodeWrapperMap(dec *msgpack.Decoder) (string, error) {
	cnt, err := dec.DecodeMapLen()
	if err != nil {
		return "", fmt.Errorf("reading map length: %w", err)
	}
	if cnt != 1 {
		return "", fmt.Errorf("wrapper map is expected to contain one item, got %d", cnt)
	}

	keyName, err := dec.DecodeString()
	if err != nil {
		return "", fmt.Errorf("reading map key: %w", err)
	}
	return keyName, nil
}
