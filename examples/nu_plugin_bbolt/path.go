package main

import (
	"bytes"
	"errors"
	"fmt"
	"iter"
	"regexp"
	"slices"

	"go.etcd.io/bbolt"

	"github.com/ainvaltin/nu-plugin"
)

type boltValues iter.Seq2[boltValue, error]

func getBoltValues(in any) (boltValues, error) {
	switch i := in.(type) {
	case nu.Record:
		return getBoltValuesRec(i, nu.Span{})
	case nu.Value:
		switch t := i.Value.(type) {
		case nu.Record:
			return getBoltValuesRec(t, i.Span)
		case []nu.Value:
			var iters []boltValues
			for _, v := range t {
				it, err := getBoltValues(v)
				if err != nil {
					return nil, err
				}
				iters = append(iters, it)
			}
			return func(yield func(boltValue, error) bool) {
				for _, i := range iters {
					for v, err := range i {
						if !yield(v, err) {
							return
						}
					}
				}
			}, nil
		//case string: // db path; root bucket?
		default:
			return nil, nu.Error{
				Err:    fmt.Errorf("input Value must be Record, got %T", i.Value),
				Help:   `Input must be a Record {db:"path/to/bolt.db", item:itemName} where the "item" is optional`,
				Labels: []nu.Label{{Text: "unsupported type", Span: i.Span}},
			}
		}
	case nil:
		return nil, nu.Error{
			Err:  fmt.Errorf("missing input"),
			Help: `Command must have either input or argument(s)`,
		}
	}
	return nil, nu.Error{Err: fmt.Errorf("unsupported input type %T", in)}
}

func getBoltValuesRec(in nu.Record, span nu.Span) (boltValues, error) {
	dbn, ok := in["db"]
	if !ok {
		return nil, nu.Error{
			Err:    errors.New("missing 'db' field"),
			Help:   `Record describing Bolt item must have field "db" which is path to bolt database file.`,
			Labels: []nu.Label{{Text: "missing 'db' field", Span: span}},
		}
	}

	switch len(in) {
	case 1: // db field only
	case 2: // the other field must be "item"
		if _, ok := in["item"]; !ok {
			return nil, nu.Error{
				Err:    errors.New("missing 'item' field"),
				Help:   `Supported fields are "db" and "item"`,
				Labels: unsupportedFields(in),
			}
		}
	default:
		return nil, nu.Error{
			Err:    fmt.Errorf("expected one or two fields, got %d", len(in)),
			Help:   `Record describing Bolt item may have only two fields, "db" and "item"`,
			Labels: unsupportedFields(in),
		}
	}

	db, err := getDB(dbn.Value.(string))
	if err != nil {
		return nil, nu.Error{
			Err:    err,
			Labels: []nu.Label{{Text: `invalid db name`, Span: dbn.Span}},
		}
	}
	m, err := compilePath(in["item"])
	if err != nil {
		return nil, fmt.Errorf("compiling path: %w", err)
	}
	return matchItems(db, m), nil
}

func unsupportedFields(r nu.Record) (l []nu.Label) {
	for k, v := range r {
		if k != "db" && k != "item" {
			l = append(l, nu.Label{Text: `unsupported field`, Span: v.Span})
		}
	}
	return l
}

func matchItems(db *bbolt.DB, m pathMatcher) boltValues {
	return func(yield func(boltValue, error) bool) {
		_ = db.View(func(tx *bbolt.Tx) error {
			b := pathItem{bucket: tx.Cursor().Bucket()}
			for item, err := range m(&b) {
				if err != nil {
					if !yield(boltValue{}, fmt.Errorf("resolving path: %w", err)) {
						return nil
					}
				}
				if item != nil {
					r := boltValue{db: db, name: item.asPath(), kind: kindBucket}
					if item.bucket == nil {
						r.kind = kindKey
					}
					if !yield(r, nil) {
						return nil
					}
				}
			}
			return nil
		})
	}
}

type buckets iter.Seq2[*pathItem, error]

type pathItem struct {
	parent *pathItem
	bucket *bbolt.Bucket // when nil it's a key
	name   []byte
	span   nu.Span
}

func (p pathItem) asPath() []boltItem {
	r := []boltItem{}
	for c := &p; c != nil && c.name != nil; c = c.parent {
		r = append(r, boltItem{name: c.name, span: c.span})
	}
	slices.Reverse(r)
	return r
}

func compilePath(v nu.Value) (pathMatcher, error) {
	switch p := v.Value.(type) {
	case nil: // match root bucket
		return func(b *pathItem) buckets {
			return func(yield func(*pathItem, error) bool) {
				yield(b, nil)
			}
		}, nil
	case []nu.Value:
		mf := []pathMatcher{}
		for _, v := range p {
			pm, err := toPathMatcher(v)
			if err != nil {
				return nil, err
			}
			mf = append(mf, pm)
		}
		return foldMatchers(mf), nil
	default:
		return toPathMatcher(v)
	}
}

// (?flags:re)
var isRegexp = regexp.MustCompile(`\(\?[imsU-]+:\\A.*\\z\)|\\A.*\\z`)

func toPathMatcher(v nu.Value) (pathMatcher, error) {
	switch p := v.Value.(type) {
	case []byte:
		return exactBytesMatcher(p, v.Span), nil
	case string:
		if isRegexp.Match([]byte(p)) {
			re, err := regexp.Compile(p)
			if err != nil {
				return nil, nu.Error{
					Err:    fmt.Errorf("compiling regular expression: %w", err),
					Code:   "go::regexp::syntax",
					Url:    "https://pkg.go.dev/regexp/syntax",
					Help:   "See Go documentation about supported regular expression syntax",
					Labels: []nu.Label{{Text: err.Error(), Span: v.Span}},
				}
			}
			return regexpMatcher(re, v.Span), nil
		}
		return exactBytesMatcher([]byte(p), v.Span), nil
	case nu.CellPath:
		return cellPathMatcher(p), nil
	case []nu.Value:
		var r []byte
		for _, v := range p {
			b, err := toBytes(v)
			if err != nil {
				return nil, err
			}
			r = append(r, b...)
		}
		return exactBytesMatcher(r, v.Span), nil
	default:
		return nil, nu.Error{
			Err:    fmt.Errorf("can't convert value %T to bbolt item name", p),
			Help:   "Supported types are Binary, String and CellPath",
			Labels: []nu.Label{{Text: fmt.Sprintf("unsupported type %T", p), Span: v.Span}},
		}
	}
}

func exactBytesMatcher(name []byte, span nu.Span) pathMatcher {
	return func(parent *pathItem) buckets {
		return func(yield func(*pathItem, error) bool) {
			r := pathItem{
				parent: parent,
				bucket: parent.bucket.Bucket(name),
				name:   name,
				span:   span,
			}
			if r.bucket != nil {
				yield(&r, nil)
				return
			}
			if k, _ := parent.bucket.Cursor().Seek(name); k != nil {
				if bytes.Equal(k, name) {
					yield(&r, nil)
					return
				}
			}
			yield(nil, nu.Error{
				Err:    fmt.Errorf("bucket %x doesn't contain item named %x", parent.name, name),
				Labels: []nu.Label{{Text: "not found", Span: span}},
			})
		}
	}
}

func regexpMatcher(expr *regexp.Regexp, span nu.Span) pathMatcher {
	return func(parent *pathItem) buckets {
		if parent.bucket == nil {
			return notBucketErr(parent)
		}
		return func(yield func(*pathItem, error) bool) {
			parent.bucket.ForEach(func(k, v []byte) error {
				if expr.Match(k) {
					r := pathItem{
						parent: parent,
						bucket: parent.bucket.Bucket(k),
						name:   slices.Clone(k),
						span:   span,
					}
					if !yield(&r, nil) {
						return errors.New("stop iterating")
					}
				}
				return nil
			})
		}
	}
}

func cellPathMatcher(cp nu.CellPath) pathMatcher {
	mf := []pathMatcher{}
	for _, m := range cp.Members {
		if m.Type() == nu.PathVariantInt {
			mf = append(mf, cellPathMemberIntMatcher(m))
		} else {
			if !m.CaseSensitive() {
				// use regexp matcher? but that doesn't play well with optional?
				return func(b *pathItem) buckets {
					return func(yield func(*pathItem, error) bool) {
						yield(nil, nu.Error{
							Err:    errors.New("case insensitive cell paths are not supported"),
							Labels: []nu.Label{{Text: "only case sensitive members can be used", Span: m.Span()}},
						})
					}
				}
			}
			mf = append(mf, cellPathMemberStrMatcher(m))
		}
	}

	return foldMatchers(mf)
}

func cellPathMemberIntMatcher(pm nu.PathMember) pathMatcher {
	return func(parent *pathItem) buckets {
		if parent.bucket == nil {
			return notBucketErr(parent)
		}

		return func(yield func(*pathItem, error) bool) {
			idx := uint(0)
			key := pm.PathInt()
			c := parent.bucket.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				if key == idx {
					r := pathItem{
						parent: parent,
						bucket: parent.bucket.Bucket(k),
						name:   slices.Clone(k),
						span:   pm.Span(),
					}
					yield(&r, nil)
					return
				}
				idx++
			}
			if pm.Optional() {
				//yield(parent, nil)
				return
			}
			yield(nil, nu.Error{
				Err:    fmt.Errorf("bucket %x contains %d items", parent.name, idx),
				Help:   "Items use zero based index, ie first item is $.0, second is $.1 etc",
				Labels: []nu.Label{{Text: "index out of range", Span: pm.Span()}},
			})
		}
	}
}

func cellPathMemberStrMatcher(pm nu.PathMember) pathMatcher {
	return func(parent *pathItem) buckets {
		if parent.bucket == nil {
			return notBucketErr(parent)
		}

		return func(yield func(*pathItem, error) bool) {
			name := []byte(pm.PathStr())
			r := pathItem{
				parent: parent,
				bucket: parent.bucket.Bucket(name),
				name:   name,
				span:   pm.Span(),
			}
			if r.bucket != nil {
				yield(&r, nil)
				return
			}
			// is it a key?
			if k, _ := parent.bucket.Cursor().Seek(name); k != nil {
				if bytes.Equal(k, name) {
					yield(&r, nil)
					return
				}
			}
			if pm.Optional() {
				// Optional path members will not cause errors if they can't be accessed - the path access will just return Nothing instead.
				//yield(parent, nil)
				return
			}
			yield(nil, nu.Error{
				Err:    fmt.Errorf("bucket %x doesn't contain item %x", parent.name, name),
				Labels: []nu.Label{{Text: "no such item", Span: pm.Span()}},
			})
		}
	}
}

type pathMatcher func(b *pathItem) buckets

func foldMatchers(mf []pathMatcher) pathMatcher {
	if len(mf) == 1 {
		return mf[0]
	}

	return func(b *pathItem) buckets {
		stack := []*pathItem{b}
		for _, f := range mf {
			ns := []*pathItem{}
			for _, b := range stack {
				for b, err := range f(b) {
					if err != nil {
						return func(yield func(*pathItem, error) bool) {
							yield(nil, err)
						}
					}
					ns = append(ns, b)
				}
			}
			stack = ns
		}
		return func(yield func(*pathItem, error) bool) {
			for _, v := range stack {
				if !yield(v, nil) {
					return
				}
			}
		}
	}
}

func notBucketErr(b *pathItem) buckets {
	return func(yield func(*pathItem, error) bool) {
		err := nu.Error{
			Err:    fmt.Errorf("item %x is a key and thus can't have children", b.name),
			Labels: []nu.Label{{Text: "not a bucket", Span: b.span}},
		}
		yield(nil, err)
	}
}
