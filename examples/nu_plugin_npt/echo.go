package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ainvaltin/nu-plugin"
	"github.com/ainvaltin/nu-plugin/syntaxshape"
	"github.com/ainvaltin/nu-plugin/types"
)

func cmdEcho() *nu.Command {
	return &nu.Command{
		Signature: nu.PluginSignature{
			Name:        "npt echo",
			Desc:        "Returns the input as output",
			Description: "An example how to read input and return result from a plugin.",
			Category:    "Debug",
			SearchTerms: []string{"input", "output"},
			InputOutputTypes: []nu.InOutTypes{
				{In: types.Any(), Out: types.Any()},
			},
			OptionalPositional: []nu.PositionalArg{
				{Name: "input", Desc: "input for the command", Shape: syntaxshape.Any()},
			},
			AllowMissingExamples: true,
		},
		Examples: []nu.Example{
			{Example: "npt echo foobar", Description: "Input (and output) is Value", Result: &nu.Value{Value: "foobar"}},
			{Example: "42 | npt echo", Description: "Input (and output) is Value", Result: &nu.Value{Value: 42}},
			{Example: "open --raw file.txt | npt echo", Description: "Input (and output) is RawStream", Result: &nu.Value{Value: []byte("content of the file")}},
			{Example: "[1, 2, 3] | npt echo", Description: "Input (and output) is ListStream", Result: &nu.Value{Value: []nu.Value{nu.ToValue(1), nu.ToValue(2), nu.ToValue(3)}}},
		},
		OnRun: handleCmdEcho,
	}
}

func handleCmdEcho(ctx context.Context, ec *nu.ExecCommand) error {
	switch in := ec.Input.(type) {
	case nu.Value:
		return ec.ReturnValue(ctx, in)
	case io.Reader:
		out, err := ec.ReturnRawStream(ctx)
		if err != nil {
			return fmt.Errorf("opening return stream: %w", err)
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	case <-chan nu.Value:
		out, err := ec.ReturnListStream(ctx)
		if err != nil {
			return fmt.Errorf("opening return list: %w", err)
		}
		defer close(out)

		for {
			select {
			case v, ok := <-in:
				if !ok {
					return nil // input closed, all OK
				}
				select {
				case out <- v:
				case <-ctx.Done():
					return ctxErr(ctx)
				}
			case <-ctx.Done():
				return ctxErr(ctx)
			}
		}
	}

	return fmt.Errorf("unsupported input type %T", ec.Input)
}

func ctxErr(ctx context.Context) error {
	// if we got signal from the engine that we should drop the stream
	// then it's not error
	err := context.Cause(ctx)
	if errors.Is(err, nu.ErrDropStream) {
		return nil
	}
	return err
}
