
## [2025-01-01]
- Implement `FindDecl` and `CallDecl` engine calls.
  Renamed `EvalClosureArgument` to `EvalArgument` as it is now used for both
  `EvalClosure` and `CallDecl`.

## [2024-12-29]
- Plugin protocol version 0.101.0
- Implement Stringer for IntRange;
- Implement iterator for `IntRange`. Minimum supported Go version is now 1.23.
- Fix loading empty list. Caused ie `GetEnvVar` or `GetEnvVars` call to fail
  when some env var contained empty list value.

## [2024-12-09]
- Implement `Keyword` SyntaxShape;
- Introduce `types` package - to define input / output types of the plugin.
  The `PluginSignature` field `InputOutputTypes` is now `[]InOutTypes`.

## [2024-12-08]
- Introduce `syntaxshape` package. Until now where SyntaxShape is required `string`
  type was used but that allows only simple syntax shapes to be described.
- Rename `Flag` field `Arg` to `Shape` and change it's type from `string` to `syntaxshape.SyntaxShape`.
- Change `PositionalArg` field `Shape` type from `string` to `syntaxshape.SyntaxShape`.
- Rename `PluginSignature` field `Usage` to `Desc` and `UsageEx` to `Description`.
- Introduce `ExecCommand.FlagValue` method.
- Drop `NamedParams.HasFlag` method, use `ExecCommand.FlagValue` instead. The way HasFlag 
  have been used allows stubble bug in case `--flag=false` is used.
- Drop `NamedParams.StringValue` method;