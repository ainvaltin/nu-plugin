package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.etcd.io/bbolt"

	"github.com/ainvaltin/nu-plugin"
)

func Test_compilePath(t *testing.T) {
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "db.db"), 0600, nil)
	if err != nil {
		t.Fatalf("opening DB: %v", err)
	}
	defer db.Close()

	err = db.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucket([]byte("bucket A"))
		if err != nil {
			return err
		}
		for v := range 10 {
			var b2 *bbolt.Bucket
			if b2, err = b.CreateBucket([]byte{0, byte(v)}); err != nil {
				return err
			}
			if err = b2.Put([]byte("key"), []byte("value")); err != nil {
				return err
			}
		}

		b, err = tx.CreateBucket([]byte("bucket B"))
		if err != nil {
			return err
		}
		for v := range 10 {
			if err = b.Put([]byte{1, byte(v)}, fmt.Appendf(nil, "bucket B -> 0x01%02x", v)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("populate database: %v", err)
	}

	getPathsErr := func(m pathMatcher) (r [][]boltItem, _ error) {
		return r, db.View(func(tx *bbolt.Tx) error {
			b := pathItem{bucket: tx.Cursor().Bucket()}
			for i, err := range m(&b) {
				if err != nil {
					return fmt.Errorf("getting path: %w", err)
				}
				if i != nil {
					r = append(r, i.asPath())
				}
			}
			return nil
		})
	}

	getPaths := func(t *testing.T, m pathMatcher) [][]boltItem {
		r, err := getPathsErr(m)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	t.Run("misc", func(t *testing.T) {
		var testCases = []struct {
			in  nu.Value
			out [][]boltItem
		}{
			{in: nu.Value{}, out: [][]boltItem{{}}},
			{in: nu.Value{Value: "bucket A"}, out: [][]boltItem{{{name: []uint8("bucket A")}}}},
		}

		for _, tc := range testCases {
			m, err := compilePath(tc.in)
			if err != nil {
				t.Fatalf("compile matcher (%v): %v", tc.in, err)
			}
			out := getPaths(t, m)
			if diff := cmp.Diff(out, tc.out, cmp.AllowUnexported(boltItem{})); diff != "" {
				t.Errorf("%v mismatch (-expected +got):\n%s", tc.in, diff)
			}
		}
	})

	t.Run("exact match", func(t *testing.T) {
		var testCases = []struct {
			in  []any
			out []boltItem
		}{
			{in: []any{"bucket A"}, out: []boltItem{{name: []uint8("bucket A")}}},
			{in: []any{"bucket B"}, out: []boltItem{{name: []uint8("bucket B")}}},
			{in: []any{"bucket A", []byte{0, 0}}, out: []boltItem{{name: []uint8("bucket A")}, {name: []uint8{0, 0}}}},
			{in: []any{"bucket B", []byte{1, 5}}, out: []boltItem{{name: []uint8("bucket B")}, {name: []uint8{1, 5}}}},
		}

		for _, tc := range testCases {
			m, err := compilePath(nu.ToValue(tc.in))
			if err != nil {
				t.Fatalf("compile matcher: %v", err)
			}
			out := getPaths(t, m)
			if diff := cmp.Diff(out[0], tc.out, cmp.AllowUnexported(boltItem{})); diff != "" {
				t.Errorf("%v mismatch (-input +output):\n%s", tc.in, diff)
			}
		}
	})

	t.Run("CellPath", func(t *testing.T) {
		var testCases = []struct {
			in  []any
			out []boltItem
		}{
			{in: []any{newCP().int(0, false).cellPath()}, out: []boltItem{{name: []uint8("bucket A")}}},
			{in: []any{newCP().int(1, false).cellPath()}, out: []boltItem{{name: []uint8("bucket B")}}},
			{in: []any{newCP().str("bucket A", false, true).int(1, false).cellPath()}, out: []boltItem{{name: []uint8("bucket A")}, {name: []uint8{0, 1}}}},
			{in: []any{newCP().str("bucket B", false, true).int(2, false).cellPath()}, out: []boltItem{{name: []uint8("bucket B")}, {name: []uint8{1, 2}}}},
			{in: []any{newCP().int(1, false).int(0, false).cellPath()}, out: []boltItem{{name: []uint8("bucket B")}, {name: []uint8{1, 0}}}},
			{in: []any{newCP().int(0, false).int(9, false).str("key", false, true).cellPath()}, out: []boltItem{{name: []uint8("bucket A")}, {name: []uint8{0, 9}}, {name: []uint8("key")}}},
		}

		for _, tc := range testCases {
			m, err := compilePath(nu.ToValue(tc.in))
			if err != nil {
				t.Fatalf("compile matcher: %v", err)
			}
			out := getPaths(t, m)
			if diff := cmp.Diff(out[0], tc.out, cmp.AllowUnexported(boltItem{})); diff != "" {
				t.Errorf("%v mismatch (-input +output):\n%s", tc.in, diff)
			}
		}
	})

	t.Run("RegExp", func(t *testing.T) {
		var testCases = []struct {
			in  []any
			out [][]boltItem
		}{
			{in: []any{`\A.*A\z`}, out: [][]boltItem{{{name: []uint8("bucket A")}}}},
			{in: []any{`\Abucket.*\z`}, out: [][]boltItem{{{name: []uint8("bucket A")}}, {{name: []uint8("bucket B")}}}},
			{in: []any{`\Abucket.*\z`, `\A.*\x03\z`}, out: [][]boltItem{{{name: []uint8("bucket A")}, {name: []uint8{0, 3}}}, {{name: []uint8("bucket B")}, {name: []uint8{1, 3}}}}},
		}

		for _, tc := range testCases {
			m, err := compilePath(nu.ToValue(tc.in))
			if err != nil {
				t.Fatalf("compile matcher: %v", err)
			}
			out := getPaths(t, m)
			if diff := cmp.Diff(out, tc.out, cmp.AllowUnexported(boltItem{})); diff != "" {
				t.Errorf("%v mismatch (-input +output):\n%s", tc.in, diff)
			}
		}
	})

	t.Run("mixed", func(t *testing.T) {
		var testCases = []struct {
			in  []any
			out [][]boltItem
		}{
			{
				in: []any{`\Abucket.*\z`, newCP().int(0, true).cellPath()},
				out: [][]boltItem{
					{{name: []uint8("bucket A")}, {name: []uint8{0, 0}}},
					{{name: []uint8("bucket B")}, {name: []uint8{1, 0}}},
				},
			},
			{
				in: []any{`(?i:\A.* a\z)`, newCP().int(0, true).cellPath(), "key"},
				out: [][]boltItem{
					{{name: []uint8("bucket A")}, {name: []uint8{0, 0}}, {name: []uint8("key")}},
				},
			},
		}

		for _, tc := range testCases {
			m, err := compilePath(nu.ToValue(tc.in))
			if err != nil {
				t.Fatalf("compile matcher: %v", err)
			}
			out := getPaths(t, m)
			if diff := cmp.Diff(out, tc.out, cmp.AllowUnexported(boltItem{})); diff != "" {
				t.Errorf("%v mismatch (-input +output):\n%s", tc.in, diff)
			}
		}
	})

	t.Run("error", func(t *testing.T) {
		var testCases = []struct {
			in  []any
			out error
		}{
			{in: []any{`nope`}, out: errors.New(`getting path: bucket  doesn't contain item named 6e6f7065`)},
		}

		for _, tc := range testCases {
			m, err := compilePath(nu.ToValue(tc.in))
			if err != nil {
				t.Fatalf("compile matcher: %v", err)
			}
			out, err := getPathsErr(m)
			if err == nil {
				t.Errorf("expected to get error, got paths: %v", out)
				continue
			}
			if diff := cmp.Diff(err.Error(), tc.out.Error(), cmp.AllowUnexported(boltItem{})); diff != "" {
				t.Errorf("%v mismatch (-input +output):\n%s\npaths: %v", tc.in, diff, out)
			}
		}
	})
}

type cpBuilder struct {
	cp nu.CellPath
}

func newCP() cpBuilder {
	return cpBuilder{cp: nu.CellPath{}}
}

func (b cpBuilder) cellPath() nu.CellPath {
	return b.cp
}

func (b cpBuilder) int(i uint, opt bool) cpBuilder {
	b.cp.AddInteger(i, opt)
	return b
}

func (b cpBuilder) str(s string, opt, cs bool) cpBuilder {
	b.cp.AddString(s, opt, cs)
	return b
}
