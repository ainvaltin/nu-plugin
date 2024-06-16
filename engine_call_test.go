package nu_test

import (
	"context"
	"fmt"

	"github.com/ainvaltin/nu-plugin"
)

// example of a command which sends list stream as a input to closure
func ExampleInputListStream() {
	_ = &nu.Command{
		Signature: nu.PluginSignature{
			Name: "demo",
			RequiredPositional: nu.PositionalArgs{
				nu.PositionalArg{
					Name:  "closure",
					Desc:  "Closure to be evaluated",
					Shape: "Any",
				},
			},
		},
		Examples: nu.Examples{
			{Description: `Closure which adds +1 to each item in input stream and returns stream`, Example: `demo { each {|n| $n + 1} }`},
		},

		OnRun: func(ctx context.Context, call *nu.ExecCommand) error {
			// EvalClosure will block until the closure returns something so generate the
			// input stream in goroutine
			closureIn := make(chan nu.Value)
			go func() {
				defer close(closureIn)
				for v := range 10 {
					closureIn <- nu.Value{Value: v}
				}
			}()

			closureOut, err := call.EvalClosure(ctx, call.Positional[0], nu.InputListStream(closureIn))
			if err != nil {
				return fmt.Errorf("evaluating closure: %w", err)
			}
			switch data := closureOut.(type) {
			case <-chan nu.Value:
				out, err := call.ReturnListStream(ctx)
				if err != nil {
					return fmt.Errorf("opening output stream: %w", err)
				}
				for v := range data {
					out <- v
				}
				close(out)
			default:
				return fmt.Errorf("unsupported closure output type %T", data)
			}
			return nil
		},
	}
}
