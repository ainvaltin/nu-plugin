package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go.etcd.io/bbolt"

	"github.com/ainvaltin/nu-plugin"
	"github.com/ainvaltin/nu-plugin/syntaxshape"
	"github.com/ainvaltin/nu-plugin/types"
)

func main() {
	p, err := nu.New(
		[]*nu.Command{{
			Signature: nu.PluginSignature{
				Name:        "boltval",
				Category:    "Database",
				Desc:        "bbolt database objects - Nu custom value demo",
				Description: "Demo implementation of Custom Value type - each value is a item (bucket or key) in the bbolt database and properties and operators allow to act on them.",
				SearchTerms: []string{"custom value"},
				InputOutputTypes: []nu.InOutTypes{
					{In: types.Nothing(), Out: types.List(types.Custom("bbolt"))},
					{In: types.Any(), Out: types.List(types.Custom("bbolt"))},
				},
				OptionalPositional: []nu.PositionalArg{
					{Name: "file", Shape: syntaxshape.Filepath(), Desc: `Name of the Bolt database file.`},
					{Name: "path", Shape: syntaxshape.OneOf(syntaxshape.List(syntaxshape.Any()), syntaxshape.Binary(), syntaxshape.String(), syntaxshape.CellPath()), Desc: `Either bucket or key name, if not given then root bucket.`},
				},
				AllowMissingExamples: true,
			},
			Examples: []nu.Example{
				{Description: "List of root buckets", Example: "boltval /path/to.db | each {$in.buckets}", Result: &nu.Value{Value: []nu.Value{{Value: nu.Record{"db": nu.Value{Value: "/path/to.db"}, "item": nu.Value{Value: []byte{1, 2, 3}}}}}}},
				{Description: `Add bucket "bar", then add key "foo" into that bucket with value 0x[0102030405]`, Example: `boltval /path/to.db | $in.0 + bar + {key: foo, value: 0x[0102030405]}`, Result: &nu.Value{Value: nu.Record{"db": nu.Value{Value: "/path/to.db"}, "item": nu.ToValue([][]byte{{98, 97, 114}, {102, 111, 111}})}}},
				{Description: "Value of the key 'foo' in the bucket 'bar'.", Example: "boltval /path/to.db [bar, foo] | $in.0.value", Result: &nu.Value{Value: []byte{0, 1, 2, 3, 4, 5}}},
			},
			OnRun: boltCmdHandler,
		}},
		"0.0.1",
		nil,
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to create plugin", err)
		return
	}
	if err := p.Run(quitSignalContext()); err != nil && !errors.Is(err, nu.ErrGoodbye) {
		fmt.Fprintln(os.Stderr, "plugin exited with error:", err)
	}
}

func boltCmdHandler(ctx context.Context, call *nu.ExecCommand) error {
	var values boltValues
	var err error
	if len(call.Positional) > 0 {
		r := nu.Record{"db": call.Positional[0], "item": nu.Value{}}
		if len(call.Positional) > 1 {
			r["item"] = call.Positional[1]
		}
		values, err = getBoltValues(r)
	} else {
		values, err = getBoltValues(call.Input)
	}
	if err != nil {
		return err
	}

	out, err := call.ReturnListStream(ctx)
	if err != nil {
		return fmt.Errorf("opening result stream: %w", err)
	}
	defer close(out)

	for v, err := range values {
		if err != nil {
			return err
		}
		out <- nu.Value{Value: v}
	}
	return nil
}

var dbr map[string]*bbolt.DB

func getDB(dbName string) (_ *bbolt.DB, err error) {
	if dbName, err = filepath.Abs(dbName); err != nil {
		return nil, err
	}
	if db, ok := dbr[dbName]; ok {
		return db, nil
	}

	db, err := bbolt.Open(dbName, 0600, &bbolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening bolt db: %w", err)
	}
	if dbr == nil {
		dbr = map[string]*bbolt.DB{}
	}
	dbr[dbName] = db
	return db, nil
}

func quitSignalContext() context.Context {
	ctx, cancel := context.WithCancelCause(context.Background())

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(sigChan)
		sig := <-sigChan
		cancel(fmt.Errorf("got quit signal: %s", sig))
	}()

	return ctx
}
