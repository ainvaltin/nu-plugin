
## [2024-12-08]
- Introduce `syntaxshape` package. Until now where SyntaxShape is required we used `string`
  type but that allows only simple syntax shapes to be described.
- Rename `Flag` field `Arg` to `Shape` and change it's type from `string` to `syntaxshape.SyntaxShape`.
- Change `PositionalArg` field `Shape` type from `string` to `syntaxshape.SyntaxShape`.
- Rename `PluginSignature` field `Usage` to `Desc` and `UsageEx` to `Description`.