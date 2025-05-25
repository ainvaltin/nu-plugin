package nu

import (
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/vmihailenco/msgpack/v5/msgpcode"
)

/*
Block is Nushell [Block Value] type.

[Block Value]: https://www.nushell.sh/contributor-book/plugin_protocol_reference.html#block
*/
type Block uint64

/*
Filesize is Nushell [Filesize Value] type.

[Filesize Value]: https://www.nushell.sh/contributor-book/plugin_protocol_reference.html#filesize
*/
type Filesize int64

/*
Glob is Nushell [Glob Value] type - a filesystem glob, selecting multiple files or
directories depending on the expansion of wildcards.

Note that [Go stdlib glob] implementation doesn't support doublestar / globstar
pattern but thirdparty libraries which do exist.

[Glob Value]: https://www.nushell.sh/contributor-book/plugin_protocol_reference.html#glob
[Go stdlib glob]: https://pkg.go.dev/path/filepath#Glob
*/
type Glob struct {
	Value string
	// If true, the expansion of wildcards is disabled and Value should be treated
	// as a literal path.
	NoExpand bool
}

// encode Glob value minus the Span member of the Value
func (glob Glob) encodeGlob(enc *msgpack.Encoder) error {
	if err := enc.EncodeString("Glob"); err != nil {
		return err
	}
	if err := enc.EncodeMapLen(3); err != nil {
		return err
	}
	if err := enc.EncodeString("val"); err != nil {
		return err
	}
	if err := enc.EncodeString(glob.Value); err != nil {
		return err
	}
	if err := enc.EncodeString("no_expand"); err != nil {
		return err
	}
	if err := enc.EncodeBool(glob.NoExpand); err != nil {
		return err
	}
	return nil
}

// the enclosing map has been red and we need to decode the struct itself.
func decodeGlob(dec *msgpack.Decoder, value *Value) error {
	n, err := dec.DecodeMapLen()
	if err != nil {
		return err
	}
	if n == -1 {
		return nil
	}

	g := Glob{}
	for idx := 0; idx < n; idx++ {
		fieldName, err := dec.DecodeString()
		if err != nil {
			return fmt.Errorf("decoding field name [%d/%d] of Glob: %w", idx+1, n, err)
		}
		switch fieldName {
		case "val":
			g.Value, err = dec.DecodeString()
		case "no_expand":
			g.NoExpand, err = dec.DecodeBool()
		case "span":
			err = value.Span.decodeMsgpack(dec)
		default:
			return fmt.Errorf("unsupported Glob Value field %q", fieldName)
		}
		if err != nil {
			return fmt.Errorf("decoding field %s of Glob: %w", fieldName, err)
		}
	}
	value.Value = g
	return nil
}

/*
[Record] is an associative key-value map.
If records are contained in a list, this renders as a table.
The keys are always strings, but the values may be any type.

[Record]: https://www.nushell.sh/contributor-book/plugin_protocol_reference.html#record
*/
type Record map[string]Value

func (r Record) encodeMsgpack(enc *msgpack.Encoder, p *Plugin) error {
	if err := startValue(enc, "Record"); err != nil {
		return err
	}
	if err := enc.EncodeMapLen(len(r)); err != nil {
		return err
	}
	for k, v := range r {
		if err := enc.EncodeString(k); err != nil {
			return err
		}
		if err := v.encodeMsgpack(enc, p); err != nil {
			return fmt.Errorf("encode record field %s value: %w", k, err)
		}
	}
	return nil
}

func decodeRecord(dec *msgpack.Decoder, p *Plugin) (rec Record, err error) {
	var cnt int
	if cnt, err = dec.DecodeMapLen(); err != nil {
		return rec, fmt.Errorf("decoding Record field count: %w", err)
	}
	var name string
	rec = Record{}
	for range cnt {
		if name, err = dec.DecodeString(); err != nil {
			return rec, fmt.Errorf("decoding field name: %w", err)
		}
		var v Value
		if err = v.decodeMsgpack(dec, p); err != nil {
			return rec, fmt.Errorf("decoding field %s value: %w", name, err)
		}
		rec[name] = v
	}
	return rec, nil
}

/*
Closure [Value] is a reference to a parsed block of Nushell code, with variables
captured from scope.

The plugin should not try to inspect the contents of the closure. It is recommended
that this is only used as an argument to the [ExecCommand.EvalClosure] engine call.
*/
type Closure struct {
	BlockID  uint
	Captures msgpack.RawMessage
}

func (c Closure) encodeMsgpack(enc *msgpack.Encoder) error {
	if err := enc.EncodeMapLen(2); err != nil {
		return err
	}
	if err := enc.EncodeString("block_id"); err != nil {
		return err
	}
	if err := enc.EncodeUint(uint64(c.BlockID)); err != nil {
		return err
	}
	if err := enc.EncodeString("captures"); err != nil {
		return err
	}
	if c.Captures == nil {
		return enc.EncodeNil()
	} else {
		return c.Captures.EncodeMsgpack(enc)
	}
}

func decodeClosure(dec *msgpack.Decoder) (c Closure, _ error) {
	cnt, err := dec.DecodeMapLen()
	if err != nil {
		return c, err
	}
	if cnt != 2 {
		return c, fmt.Errorf("expected Closure to contain 2 keys, got %d", cnt)
	}

	var code byte
	for range cnt {
		key, err := dec.DecodeString()
		if err != nil {
			return c, err
		}
		switch key {
		case "block_id":
			c.BlockID, err = dec.DecodeUint()
		case "captures":
			code, err = dec.PeekCode()
			if err != nil {
				return c, fmt.Errorf("peeking 'captures' value type: %w", err)
			}
			switch code {
			case msgpcode.Nil:
				err = dec.DecodeNil()
			default:
				err = c.Captures.DecodeMsgpack(dec)
			}
		}
		if err != nil {
			return c, fmt.Errorf("decoding key %q: %w", key, err)
		}
	}
	return c, nil
}
