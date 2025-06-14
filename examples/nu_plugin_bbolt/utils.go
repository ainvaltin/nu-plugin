package main

import (
	"errors"
	"fmt"

	"github.com/ainvaltin/nu-plugin"
)

/*
boltItem is bucket or key name in bbolt database.
*/
type boltItem struct {
	name []byte
	span nu.Span
}

func toPath(v nu.Value) (path []boltItem, _ error) {
	switch t := v.Value.(type) {
	case []nu.Value:
		for _, v := range t {
			b, err := toBytes(v)
			if err != nil {
				return nil, err
			}
			path = append(path, boltItem{name: b, span: v.Span})
		}
		return path, nil
	case nu.CellPath:
		for _, v := range t.Members {
			if v.Type() != nu.PathVariantString {
				return nil, (&nu.Error{Err: errors.New("only string path members are supported")}).AddLabel("integer members not supported", v.Span())
			}
			if v.Optional() {
				// support optional items as last member(s)?
				return nil, (&nu.Error{Err: errors.New("optional path members are not supported")}).AddLabel("optional members not supported", v.Span())
			}
			if !v.CaseSensitive() {
				return nil, nu.Error{Err: errors.New("case-insensitive path members are not supported"), Labels: []nu.Label{{Text: "case-insensitive members not supported", Span: v.Span()}}}
			}
			path = append(path, boltItem{name: []byte(v.PathStr()), span: v.Span()})
		}
		return path, nil
	default:
		b, err := toBytes(v)
		return []boltItem{{name: b, span: v.Span}}, err
	}
}

func toBytes(v nu.Value) ([]byte, error) {
	switch t := v.Value.(type) {
	case []byte:
		return t, nil
	case string:
		return []byte(t), nil
	case int64:
		if t < 256 {
			return []byte{uint8(t)}, nil
		}
		return nil, nu.Error{
			Err:    fmt.Errorf("integer values must fit into byte, got %d", t),
			Labels: []nu.Label{{Text: "value out of range (max allowed is 255)", Span: v.Span}},
		}
	case []nu.Value:
		var r []byte
		for _, v := range t {
			b, err := toBytes(v)
			if err != nil {
				return nil, err
			}
			r = append(r, b...)
		}
		return r, nil
	default:
		return nil, nu.Error{
			Err:    errors.New("can't convert value to bytes"),
			Labels: []nu.Label{{Text: fmt.Sprintf("unsupported type %T", t), Span: v.Span}},
		}
	}
}
