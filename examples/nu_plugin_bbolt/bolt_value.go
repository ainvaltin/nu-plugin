package main

import (
	"context"
	"fmt"
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
	name [][]byte
	kind uint8
}

func (r boltValue) Name() string { return "bbolt" }

func (r boltValue) NotifyOnDrop() bool { return false }

func (r boltValue) Dropped(ctx context.Context) error { return nil }

func (r boltValue) FollowPathInt(ctx context.Context, item uint) (nu.Value, error) {
	return nu.Value{}, fmt.Errorf("int path not supported")
}

func (r boltValue) FollowPathString(ctx context.Context, item string) (nu.Value, error) {
	switch item {
	case "name":
		if len(r.name) == 0 {
			return nu.Value{Value: nil}, nil // root bucket
		}
		return nu.Value{Value: r.name[len(r.name)-1]}, nil
	case "type":
		switch r.kind {
		case kindBucket:
			return nu.Value{Value: "bucket"}, nil
		case kindKey:
			return nu.Value{Value: "key"}, nil
		}
		return nu.Value{Value: "unknown"}, nil
	case "value":
		var buf []byte
		err := r.db.View(func(tx *bbolt.Tx) error {
			b, err := r.goToBucket(tx)
			if err != nil {
				return err
			}
			buf = slices.Clone(b.Get(r.name[len(r.name)-1]))
			return nil
		})
		return nu.Value{Value: buf}, err
	case "keys":
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
			return r.addBucket([]byte(v))
		case []byte:
			return r.addBucket(v)
		case nu.Record:
			return r.addValue(v["key"], v["value"])
		}
	case operator.Math_Subtract:
		switch v := rhs.Value.(type) {
		case string:
			return r.asValue(), r.deleteItem([]byte(v))
		case []byte:
			return r.asValue(), r.deleteItem(v)
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
			if r := slices.Compare(v, rhs[i]); r != 0 {
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
	return nu.Value{Value: nu.Record{
		"db":   nu.Value{Value: r.db.Path()},
		"item": nu.ToValue(r.name),
	}}, nil
}

func (r boltValue) asValue() nu.Value { return nu.Value{Value: r} }

func (r boltValue) child(kind uint8, name []byte) nu.Value {
	return boltValue{db: r.db, name: append(r.name, name), kind: kind}.asValue()
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

func (r boltValue) addBucket(name []byte) (nu.Value, error) {
	err := r.db.Update(func(tx *bbolt.Tx) error {
		b, err := r.goToBucket(tx)
		if err != nil {
			return err
		}
		_, err = b.CreateBucket(name)
		return err
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
		if b = b.Bucket(v); b == nil {
			return nil, fmt.Errorf("bucket %x not found", v)
		}
	}
	return b, nil
}
