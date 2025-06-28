# Custom Value demo

This plugin is a demo/test of Custom Values - it implements `bbolt` type
which is item (bucket or value) in a [bbolt database](https://github.com/etcd-io/bbolt).
Type defines fields and operations which allow to navigate and manipulate the database.
The command name registered by the plugin is `boltval`.

NB! [Plugin Garbage collection](https://www.nushell.sh/book/plugins.html#plugin-garbage-collector)
should be enabled for this plugin as it keeps lock on the bbolt db and if the
plugin is never garbage collected other tools can't access the db!

See https://github.com/ainvaltin/nu_plugin_boltdb for more classical take on a
plugin to operate with bbolt databases.

## Examples

### CustomValue methods
The examples to trigger CustomValue methods.

The command
```nushell
boltval ~/data/test.db | $in.buckets | sort
```
triggers `FollowPathString` (the `.buckets` access) and `PartialCmp` (the `sort` filter).
The `ToBaseValue` is called to get the value to show to the user.
It lists all buckets in the root bucket.

```nushell
boltval ~/data/test.db | $in + bar + {key: foo, value: 0x[0102030405]}
```
This triggers `Operation` on the custom value with `Math_Add` operator and string "bar"
and then on the resulting variable another add with record as right hand side value.
Implementation of the `Math_Add` on the Custom Value is that it will add bucket "bar",
then add key "foo" into that bucket with value `0x[0102030405]`.

### bbolt operations
Some example commands to explain how to use this plugin for "normal work".

To list all keys in all top level buckets
```nushell
boltval ~/data/test.db | $in.buckets | each {|| $in.values} | flatten
```
omitting the `path` argument means that the root bucket is selected so `$in.buckets` in the
next step lists all the root buckets... alternatively

```nushell
boltval ~/data/test.db \A.*\z | each {{name:($in.name|decode), type:$in.type, size:$in.size}}
```
here the `path` argument is regular expression which selects everything and thus we return
name, type and size of all the items in the root bucket.

Run `boltval --help` for more examples.
