package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ainvaltin/nu-plugin"
	"github.com/ainvaltin/nu-plugin/syntaxshape"
	"github.com/ainvaltin/nu-plugin/types"
)

func cmdError() *nu.Command {
	return &nu.Command{
		Signature: nu.PluginSignature{
			Name:        "npt error",
			Desc:        "Returns error",
			Description: "An example how error returned from a plugin looks like.",
			Category:    "Debug",
			SearchTerms: []string{"error", "output"},
			InputOutputTypes: []nu.InOutTypes{
				{In: types.Any(), Out: types.Any()},
			},
			OptionalPositional: []nu.PositionalArg{
				{Name: "message", Desc: "Error message", Shape: syntaxshape.String()},
			},
			Named: []nu.Flag{
				{Long: "type", Short: 't', Shape: syntaxshape.String(), Desc: "Error type, either simple Go error or Nu error struct", Completions: nu.StaticCompletions{"go", "nu"}},
				{Long: "list", Short: 'l', Shape: syntaxshape.Int(), Desc: "how many list items to return before error"},
			},
			AllowMissingExamples: true,
		},
		OnRun: handleCmdError,
	}
}

func handleCmdError(ctx context.Context, ec *nu.ExecCommand) error {
	if cnt, ok := ec.FlagValue("list"); ok && cnt.Value.(int64) > 0 {
		return listError(ctx, ec, cnt.Value.(int64))
	}
	return buildError(ec)
}

func listError(ctx context.Context, ec *nu.ExecCommand, cnt int64) error {
	out, err := ec.ReturnListStream(ctx)
	if err != nil {
		return fmt.Errorf("creating return list: %w", err)
	}
	defer close(out)

	for v := range cnt {
		out <- nu.ToValue(v)
	}
	out <- nu.ToValue(buildError(ec))
	return nil
}

func buildError(ec *nu.ExecCommand) error {
	msg := "error returned from plugin"
	if len(ec.Positional) > 0 {
		msg = ec.Positional[0].Value.(string)
	}

	if kind, ok := ec.FlagValue("type"); ok && strings.ToLower(kind.Value.(string)) == "go" {
		return errors.New(msg)
	}

	err := nu.Error{
		Err:  errors.New(msg),
		Code: "npt::example::error",
		Help: "this command always return error",
	}
	for x, v := range ec.Positional {
		err.AddLabel(fmt.Sprintf("positional argument %d", x), v.Span)
	}
	for x, v := range ec.Named {
		err.AddLabel(fmt.Sprintf("flag %q", x), v.Span)
	}

	return err
}
