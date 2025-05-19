package nu

import (
	"fmt"
	"math"
	"reflect"

	"github.com/vmihailenco/msgpack/v5"
)

/*
[CellPath] member types, ie values returned by the Type() method of a [PathMember].
*/
const (
	PathVariantString = 1
	PathVariantInt    = 2
)

/*
PathMember describes [CellPath] member.
*/
type PathMember interface {
	// Type indicates whether the path member is uint or string,
	// return either PathVariantInt or PathVariantString
	Type() uint

	// PathInt returns uint value of the path member, should be called
	// only when Type returns PathVariantInt
	PathInt() uint

	// PathStr returns string value of the path member, should be called
	// only when Type returns PathVariantString
	PathStr() string

	// Optional path members will not cause errors if they can't be
	// accessed - the path access will just return Nothing instead.
	Optional() bool

	Span() Span

	string() string
	encodeMsgpack(enc *msgpack.Encoder, p *Plugin) error
}

/*
Represents a path into subfields of lists, records, and tables.
*/
type CellPath struct {
	Members []PathMember
	Span    Span
}

func (cp CellPath) String() string {
	s := ""
	for i, v := range cp.Members {
		if i > 0 {
			s += "."
		}
		s += v.string()
	}
	return s
}

func (cp CellPath) Length() int {
	return len(cp.Members)
}

func (cp *CellPath) AddInteger(value uint, optional bool) {
	cp.Members = append(cp.Members, pathItem[uint]{value: value, optional: optional})
}

func (cp *CellPath) AddIntegerSpan(value uint, optional bool, span Span) {
	cp.Members = append(cp.Members, pathItem[uint]{value: value, optional: optional, span: span})
}

func (cp *CellPath) AddString(value string, optional bool) {
	cp.Members = append(cp.Members, pathItem[string]{value: value, optional: optional})
}

func (cp *CellPath) AddStringSpan(value string, optional bool, span Span) {
	cp.Members = append(cp.Members, pathItem[string]{value: value, optional: optional, span: span})
}

func (cp *CellPath) decodeMsgpack(dec *msgpack.Decoder, p *Plugin) error {
	key, err := decodeWrapperMap(dec)
	if err != nil {
		return err
	}
	if key != "members" {
		return fmt.Errorf("expected key 'members', got %q", key)
	}
	cnt, err := dec.DecodeArrayLen()
	if err != nil {
		return err
	}
	for idx := range cnt {
		m, err := decodePathMember(dec, p)
		if err != nil {
			return fmt.Errorf("decode CellPath member [%d/%d]: %w", idx, cnt, err)
		}
		cp.Members = append(cp.Members, m)
	}
	return nil
}

func (cp *CellPath) encodeMsgpack(enc *msgpack.Encoder, p *Plugin) error {
	if err := enc.EncodeMapLen(1); err != nil {
		return err
	}
	if err := enc.EncodeString("members"); err != nil {
		return err
	}
	if err := enc.EncodeArrayLen(len(cp.Members)); err != nil {
		return err
	}
	for _, v := range cp.Members {
		if err := v.encodeMsgpack(enc, p); err != nil {
			return err
		}
	}
	return nil
}

type pathItem[T uint | string] struct {
	value    T
	optional bool
	span     Span
}

func (pi pathItem[T]) string() string {
	opt := ""
	if pi.optional {
		opt = "?"
	}
	return fmt.Sprintf("%v%s", pi.value, opt)
}

func (pi pathItem[T]) Type() uint {
	if _, ok := any(pi.value).(uint); ok {
		return PathVariantInt
	}
	return PathVariantString
}

func (pi pathItem[T]) PathInt() uint {
	if v, ok := any(pi.value).(uint); ok {
		return v
	}
	return math.MaxUint
}

func (pi pathItem[T]) PathStr() string {
	if v, ok := any(pi.value).(string); ok {
		return v
	}
	return ""
}

func (pi pathItem[T]) Optional() bool { return pi.optional }

func (pi pathItem[T]) Span() Span { return pi.span }

func (pi pathItem[T]) encodeMsgpack(enc *msgpack.Encoder, p *Plugin) error {
	vtyp := "String"
	if pi.Type() == PathVariantInt {
		vtyp = "Int"
	}
	if err := encodeMapStart(enc, vtyp); err != nil {
		return err
	}
	if err := enc.EncodeMapLen(3); err != nil {
		return err
	}
	if err := enc.EncodeString("val"); err != nil {
		return err
	}
	if err := enc.Encode(pi.value); err != nil {
		return err
	}
	if err := enc.EncodeString("span"); err != nil {
		return err
	}
	if err := enc.EncodeValue(reflect.ValueOf(&pi.span)); err != nil {
		return fmt.Errorf("encoding span: %w", err)
	}
	return encodeBoolean(enc, "optional", pi.optional)
}

func decodePathMember(dec *msgpack.Decoder, p *Plugin) (PathMember, error) {
	itemType, err := decodeWrapperMap(dec)
	if err != nil {
		return nil, fmt.Errorf("decode PathMember type key: %w", err)
	}
	cnt, err := dec.DecodeMapLen()
	if err != nil {
		return nil, err
	}

	sVal, iVal, span, opt := "", uint(0), Span{}, false
	for idx := range cnt {
		key, err := dec.DecodeString()
		if err != nil {
			return nil, fmt.Errorf("decode [%d/%d] pathItem key: %w", idx, cnt, err)
		}
		switch key {
		case "val":
			switch itemType {
			case "Int":
				iVal, err = dec.DecodeUint()
			case "String":
				sVal, err = dec.DecodeString()
			default:
				return nil, fmt.Errorf("unsupported CellPath val type %s", itemType)
			}
		case "span":
			err = dec.DecodeValue(reflect.ValueOf(&span))
		case "optional":
			opt, err = dec.DecodeBool()
		}
		if err != nil {
			return nil, fmt.Errorf("decoding key %q: %w", key, err)
		}
	}

	switch itemType {
	case "Int":
		return pathItem[uint]{value: iVal, span: span, optional: opt}, nil
	case "String":
		return pathItem[string]{value: sVal, span: span, optional: opt}, nil
	}
	return nil, fmt.Errorf("unsupported CellPath member type %s", itemType)
}
