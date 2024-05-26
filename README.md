# Nushell Plugin

The aim of this package is to make it simple to create 
[Nushell](https://www.nushell.sh/)
[Plugins](https://www.nushell.sh/contributor-book/plugins.html) 
using [Go](https://go.dev/).

## Status

WIP. Good enough to write simple plugins.
See [example project](https://github.com/ainvaltin/nu_plugin_plist) which implements 
commands to convert to/from plist and encode/decode base85.

Nushell [protocol](https://www.nushell.sh/contributor-book/plugin_protocol_reference.html)
`0.93.0`. Only stdio mode with message pack encoding is supported.

### Unsupported messages
- EngineCall
- Option

### Unsupported Values
- Range
- Glob
- Block
- Closure
- CellPath
- Custom
