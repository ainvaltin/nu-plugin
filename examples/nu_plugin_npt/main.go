package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ainvaltin/nu-plugin"
	"github.com/ainvaltin/nu-plugin/types"
)

func main() {
	p, err := nu.New(
		[]*nu.Command{
			{
				Signature: nu.PluginSignature{
					Name:        "npt",
					Desc:        "Demo & test plugin",
					Description: "Serves as example for how to use nu-plugin package, see the subcommands for more information about different features.",
					Category:    "Debug",
					SearchTerms: []string{"nu-plugin"},
					InputOutputTypes: []nu.InOutTypes{
						{In: types.Nothing(), Out: types.Nothing()},
					},
					AllowMissingExamples: true,
				},
				Examples: []nu.Example{},
				OnRun: func(ctx context.Context, ec *nu.ExecCommand) error {
					helpMsg, err := ec.GetHelp(ctx)
					if err != nil {
						return fmt.Errorf("loading the help of the plugin: %w", err)
					}
					return ec.ReturnValue(ctx, nu.ToValue(helpMsg))
				},
			},
			// actual demo commands
			cmdCompletion(),
			cmdEcho(),
		},
		"0.0.1",
		nil,
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to create plugin:", err)
		return
	}
	if err := p.Run(quitSignalContext()); err != nil && !errors.Is(err, nu.ErrGoodbye) {
		fmt.Fprintln(os.Stderr, "plugin exited with error:", err)
	}
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
