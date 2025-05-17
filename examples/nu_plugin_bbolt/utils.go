package main

import (
	"fmt"

	"github.com/ainvaltin/nu-plugin"
)

func toPath(v nu.Value) ([][]byte, error) {
	switch t := v.Value.(type) {
	case []nu.Value:
		var r [][]byte
		for _, v := range t {
			b, err := toBytes(v)
			if err != nil {
				return nil, err
			}
			r = append(r, b)
		}
		return r, nil
	default:
		b, err := toBytes(v)
		return [][]byte{b}, err
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
		return nil, fmt.Errorf("integer values must fit into byte, got %d", t)
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
		return nil, fmt.Errorf("unsupported type %T", t)
	}
}
