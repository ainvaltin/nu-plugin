package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"go.etcd.io/bbolt"

	"github.com/ainvaltin/nu-plugin"
	"github.com/ainvaltin/nu-plugin/operator"
)

const (
	kindUnknown = 0
	kindBucket  = 1
	kindKey     = 2
)

var _ nu.CustomValue = boltValue{}

type boltValue struct {
	db   *bbolt.DB
	name []boltItem
	kind uint8
}

func (r boltValue) Name() string { return "bbolt" }

func (r boltValue) NotifyOnDrop() bool { return false }

func (r boltValue) Dropped(ctx context.Context) error { return nil }

func (r boltValue) Save(ctx context.Context, path string) error {
	buf, err := r.value()
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to open destination file: %w", err)
	}
	if _, err = f.Write(buf); err != nil {
		return fmt.Errorf("writing value to file: %w", err)
	}
	return nil
}

func (r boltValue) FollowPathInt(ctx context.Context, item uint, optional bool) (nu.Value, error) {
	return nu.Value{}, fmt.Errorf("int path not supported")
}

func (r boltValue) FollowPathString(ctx context.Context, item string, optional, caseSensitive bool) (nu.Value, error) {
	// The optional and caseSensitive parameters are ignored in this implementation.
	switch item {
	case "db":
		return nu.Value{Value: r.db.Path()}, nil
	case "item":
		name := make([][]byte, 0, len(r.name))
		for _, v := range r.name {
			name = append(name, v.name)
		}
		return nu.ToValue(name), nil
	case "name":
		if len(r.name) == 0 {
			return nu.Value{Value: nil}, nil // root bucket
		}
		return nu.Value{Value: r.name[len(r.name)-1].name}, nil
	case "type":
		switch r.kind {
		case kindBucket:
			return nu.Value{Value: "bucket"}, nil
		case kindKey:
			return nu.Value{Value: "value"}, nil
		}
		return nu.Value{Value: "unknown"}, nil
	case "value":
		buf, err := r.value()
		return nu.Value{Value: buf}, err
	case "size":
		v := nu.Value{}
		err := r.db.View(func(tx *bbolt.Tx) error {
			b, err := r.goToBucket(tx)
			if err != nil {
				return err
			}
			if r.kind == kindKey {
				v.Value = nu.Filesize(len(b.Get(r.name[len(r.name)-1].name)))
			} else {
				v.Value = b.Inspect().KeyN
			}
			return nil
		})
		return v, err
	case "values":
		var items []nu.Value
		err := r.db.View(func(tx *bbolt.Tx) error {
			b, err := r.goToBucket(tx)
			if err != nil {
				return err
			}
			return b.ForEach(func(k, v []byte) error {
				if v != nil {
					items = append(items, r.child(kindKey, slices.Clone(k)))
				}
				return nil
			})
		})
		return nu.ToValue(items), err
	case "buckets":
		var items []nu.Value
		err := r.db.View(func(tx *bbolt.Tx) error {
			b, err := r.goToBucket(tx)
			if err != nil {
				return err
			}
			return b.ForEachBucket(func(k []byte) error {
				items = append(items, r.child(kindBucket, slices.Clone(k)))
				return nil
			})
		})
		return nu.ToValue(items), err
	}
	return nu.Value{}, fmt.Errorf("unknown property %q", item)
}

func (r boltValue) Operation(ctx context.Context, op operator.Operator, rhs nu.Value) (nu.Value, error) {
	switch op {
	case operator.Math_Add:
		switch v := rhs.Value.(type) {
		case string:
			return r.addBucket([]byte(v), rhs.Span)
		case []byte:
			return r.addBucket(v, rhs.Span)
		case nu.Record:
			return r.addValue(v["key"], v["value"])
		default:
			return nu.Value{}, nu.Error{
				Err:    errors.New("unsupported value type"),
				Help:   "Supported types are String, Binary and Record{key: ..., value: ...}",
				Labels: []nu.Label{{Text: fmt.Sprintf("unsupported type %T", rhs.Value), Span: rhs.Span}},
			}
		}
	case operator.Math_Subtract:
		switch v := rhs.Value.(type) {
		case string:
			return r.asValue(), r.deleteItem([]byte(v))
		case []byte:
			return r.asValue(), r.deleteItem(v)
		default:
			return nu.Value{}, nu.Error{
				Err:    errors.New("unsupported value type"),
				Help:   "Supported types are String and Binary",
				Labels: []nu.Label{{Text: fmt.Sprintf("unsupported type %T", rhs.Value), Span: rhs.Span}},
			}
		}
	}
	return nu.Value{}, fmt.Errorf("operation %s %s %T not supported", r.Name(), op, rhs.Value)
}

func (r boltValue) PartialCmp(ctx context.Context, v nu.Value) nu.Ordering {
	if rhs, err := toPath(v); err == nil {
		for i, v := range r.name {
			if i >= len(rhs) {
				return nu.Greater // shorter (rhs) is less
			}
			if r := slices.Compare(v.name, rhs[i].name); r != 0 {
				return nu.Ordering(r)
			}
		}
		if len(r.name) == len(rhs) {
			return nu.Equal
		}
		return nu.Less // rhs is longer
	}

	return nu.Incomparable
}

func (r boltValue) ToBaseValue(ctx context.Context) (nu.Value, error) {
	name := []byte{}
	if len(r.name) > 0 {
		name = r.name[len(r.name)-1].name
	}
	return nu.Value{Value: fmt.Sprintf("%x@%s", name, filepath.Base(r.db.Path()))}, nil
}

func (r boltValue) asValue() nu.Value { return nu.Value{Value: r} }

func (r boltValue) value() ([]byte, error) {
	switch r.kind {
	case kindBucket:
		return nil, errors.New("bucket doesn't have value")
	case kindKey:
		var buf []byte
		err := r.db.View(func(tx *bbolt.Tx) error {
			b, err := r.goToBucket(tx)
			if err != nil {
				return err
			}
			buf = slices.Clone(b.Get(r.name[len(r.name)-1].name))
			return nil
		})
		return buf, err
	}
	return nil, errors.New("item kind is unknown")
}

func (r boltValue) child(kind uint8, name []byte) nu.Value {
	return boltValue{db: r.db, name: append(slices.Clone(r.name), boltItem{name: name}), kind: kind}.asValue()
}

func (r boltValue) addValue(keyn, value nu.Value) (nu.Value, error) {
	kn, err := toBytes(keyn)
	if err != nil {
		return nu.Value{}, fmt.Errorf("invalid key: %w", err)
	}
	val, err := toBytes(value)
	if err != nil {
		return nu.Value{}, fmt.Errorf("invalid value: %w", err)
	}
	err = r.db.Update(func(tx *bbolt.Tx) error {
		b, err := r.goToBucket(tx)
		if err != nil {
			return err
		}
		return b.Put(kn, val)
	})
	return r.child(kindKey, kn), err
}

func (r boltValue) addBucket(name []byte, span nu.Span) (nu.Value, error) {
	err := r.db.Update(func(tx *bbolt.Tx) error {
		b, err := r.goToBucket(tx)
		if err != nil {
			return err
		}
		if _, err = b.CreateBucket(name); err != nil {
			return nu.Error{
				Err:    fmt.Errorf("create bucket %x: %w", name, err),
				Labels: []nu.Label{{Text: err.Error(), Span: span}},
			}
		}
		return nil
	})
	if err != nil {
		return nu.Value{}, err
	}
	return r.child(kindBucket, name), nil
}

func (r boltValue) deleteItem(name []byte) error {
	return r.db.Update(func(tx *bbolt.Tx) error {
		b, err := r.goToBucket(tx)
		if err != nil {
			return err
		}
		if b.Bucket(name) == nil {
			return b.Delete(name)
		}
		return b.DeleteBucket(name)
	})
}

func (r boltValue) goToBucket(tx *bbolt.Tx) (*bbolt.Bucket, error) {
	path := r.name
	if r.kind == kindKey {
		path = path[:len(path)-1]
	}
	b := tx.Cursor().Bucket()
	for _, v := range path {
		if b = b.Bucket(v.name); b == nil {
			return nil, (&nu.Error{Err: fmt.Errorf("bucket %x doesn't exist", v.name)}).AddLabel("no such bucket", v.span)
		}
	}
	return b, nil
}
