[![Go Reference](https://pkg.go.dev/badge/github.com/ainvaltin/nu-plugin.svg)](https://pkg.go.dev/github.com/ainvaltin/nu-plugin)

# Nushell Plugin

The aim of this package is to make it simple to create 
[Nushell](https://www.nushell.sh/)
[Plugins](https://www.nushell.sh/contributor-book/plugins.html) 
using [Go](https://go.dev/).

## Status

WIP. Good enough to write simple plugins.

Nushell [protocol](https://www.nushell.sh/contributor-book/plugin_protocol_reference.html)
`0.105.0`. Only message pack encoding is supported.

### Unsupported Engine Calls
- GetConfig - the config struct type is a moving target.

### Unsupported Values
- Range (partially, Int ranges are supported, Float ranges are not)

### Example plugins

Example projects outside this repository, ie not those in the examples directory:

- [convert between formats](https://github.com/ainvaltin/nu_plugin_plist) like plist, base58, base85.
- [bolt database](https://github.com/ainvaltin/nu_plugin_boltdb) operations.

To discover more projects using this package see the 
[dependents reported by GitHub](https://github.com/ainvaltin/nu-plugin/network/dependents)
and 
[importers reported by go package registry](https://pkg.go.dev/github.com/ainvaltin/nu-plugin?tab=importedby).
