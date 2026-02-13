package nu

import (
	"context"
	"encoding/binary"
	"fmt"
	"reflect"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/vmihailenco/msgpack/v5/msgpcode"

	"github.com/ainvaltin/nu-plugin/operator"
)

/*
Ordering is result type for the PartialCmp CustomValueOp call.

Predefined constants [Incomparable], [Less], [Equal] and [Greater] should
be used by CustomValue implementations.
*/
type Ordering int8

const (
	Incomparable Ordering = -128 // the values can't be compared
	Less         Ordering = -1   // left hand side is less than right hand side
	Equal        Ordering = 0    // both values are equal
	Greater      Ordering = 1    // left hand side is greater than right hand side
)

func (op Ordering) encodeMsgpack(enc *msgpack.Encoder) error {
	if err := encodeMapStart(enc, "Ordering"); err != nil {
		return err
	}
	switch op {
	case Incomparable:
		return enc.EncodeNil()
	case Less:
		return enc.EncodeString("Less")
	case Equal:
		return enc.EncodeString("Equal")
	case Greater:
		return enc.EncodeString("Greater")
	}
	return fmt.Errorf("unsupported Ordering value %d", op)
}

/*
CustomValue is the interface user defined types have to implement to be used as Nu [Custom Value].

The [CustomValueOp] plugin calls are routed to the appropriate method of the variable.

[Custom Value]: https://www.nushell.sh/contributor-book/plugin_protocol_reference.html#custom
[CustomValueOp]: https://www.nushell.sh/contributor-book/plugin_protocol_reference.html#customvalueop-plugin-call
*/
type CustomValue interface {
	// The human-readable name of the custom value emitted by the plugin.
	Name() string
	// Whether the engine should send drop notification about this variable.
	NotifyOnDrop() bool
	// This method is called to notify the plugin that a CustomValue that had notify_on_drop set to
	// true (ie the NotifyOnDrop method returns true) was dropped in the engine - i.e., all copies
	// of it have gone out of scope.
	Dropped(ctx context.Context) error
	// Returns the result of following a numeric cell path (e.g. $custom_value.0) on the custom value.
	// This is most commonly used with custom types that act like lists or tables.
	// The result may be another custom value.
	FollowPathInt(ctx context.Context, item uint) (Value, error)
	// Returns the result of following a string cell path (e.g. $custom_value.field) on the custom value.
	// This is most commonly used with custom types that act like lists or tables.
	// The result may be another custom value.
	FollowPathString(ctx context.Context, item string) (Value, error)
	// Returns the result of evaluating an Operator on this custom value with another value.
	// The rhs Value may be any value - not just the same custom value type.
	// The result may be another custom value.
	Operation(ctx context.Context, op operator.Operator, rhs Value) (Value, error)
	// Compares the custom value to another value and returns the Ordering that should be used, if any.
	// The argument may be any value - not just the same custom value type.
	PartialCmp(ctx context.Context, v Value) Ordering
	// Saves the custom value to a file at the given path.
	Save(ctx context.Context, path string) error
	// Returns a plain value that is representative of the custom value, or an error if this is not possible.
	// Sending a custom value back for this operation is not allowed.
	ToBaseValue(ctx context.Context) (Value, error)
}

func encodeCustomValue(enc *msgpack.Encoder, id uint32, value CustomValue) error {
	notifyDrop := value.NotifyOnDrop()
	cnt := 3 + bval(notifyDrop)
	if err := enc.EncodeMapLen(cnt); err != nil {
		return err
	}

	if err := encodeString(enc, "type", "PluginCustomValue"); err != nil {
		return err
	}

	if err := encodeString(enc, "name", value.Name()); err != nil {
		return err
	}

	if err := enc.EncodeString("data"); err != nil {
		return err
	}
	if err := enc.EncodeBytes(binary.BigEndian.AppendUint32(nil, id)); err != nil {
		return err
	}

	if notifyDrop {
		if err := encodeBoolean(enc, "notify_on_drop", true); err != nil {
			return err
		}
	}

	return nil
}

func decodeCustomValue(dec *msgpack.Decoder, p *Plugin) (cv CustomValue, _ error) {
	return cv, decodeMap("CustomValue", dec, func(dec *msgpack.Decoder, key string) (err error) {
		switch key {
		case "type", "name":
			_, err = dec.DecodeString()
		case "data":
			id, ok := uint32(0), false
			if id, err = readCVID(dec); err == nil {
				if cv, ok = p.cvals[id]; !ok {
					return fmt.Errorf("no CustomValue with id %d", id)
				}
			}
		case "notify_on_drop":
			_, err = dec.DecodeBool()
		default:
			err = errUnknownField
		}
		return err
	})
}

type (
	dropped struct{}

	toBaseValue struct{}

	followPathInt struct {
		Item uint `msgpack:"item"`
		Span Span `msgpack:"span"`
	}

	followPathString struct {
		Item string `msgpack:"item"`
		Span Span   `msgpack:"span"`
	}

	partialCmp struct{ value Value }

	operation struct {
		op    operator.Operator
		value Value
	}

	save struct {
		Path struct {
			Item string `msgpack:"item"`
			Span Span   `msgpack:"span"`
		} `msgpack:"path"`
	}
)

type customValueOp struct {
	name string
	id   uint32
	span Span
	op   any
}

func (cvo *customValueOp) decodeMsgpack(dec *msgpack.Decoder, p *Plugin) error {
	cnt, err := dec.DecodeArrayLen()
	if err != nil {
		return fmt.Errorf("reading CustomValueOp tuple length: %w", err)
	}
	if cnt != 2 {
		return fmt.Errorf("expected 2-tuple, got %d", cnt)
	}

	// first map with item + span
	if err := cvo.readValue(dec); err != nil {
		return err
	}

	// then the op
	return cvo.readOperation(dec, p)
}

func (cvo *customValueOp) readOperation(dec *msgpack.Decoder, p *Plugin) error {
	c, err := dec.PeekCode()
	if err != nil {
		return err
	}
	switch {
	case msgpcode.IsFixedString(c), msgpcode.IsString(c):
		s, err := dec.DecodeString()
		if err != nil {
			return err
		}
		switch s {
		case "ToBaseValue":
			cvo.op = toBaseValue{}
		case "Dropped":
			cvo.op = dropped{}
		default:
			return fmt.Errorf("unknown CustomValueOp command %q", s)
		}
	case msgpcode.IsFixedMap(c):
		name, err := decodeWrapperMap(dec)
		if err != nil {
			return err
		}
		switch name {
		case "FollowPathInt":
			v := followPathInt{}
			err = dec.DecodeValue(reflect.ValueOf(&v))
			cvo.op = v
		case "FollowPathString":
			v := followPathString{}
			err = dec.DecodeValue(reflect.ValueOf(&v))
			cvo.op = v
		case "PartialCmp":
			v := partialCmp{}
			err = v.value.decodeMsgpack(dec, p)
			cvo.op = v
		case "Operation":
			v := operation{}
			err = v.decodeMsgpack(dec, p)
			cvo.op = v
		case "Save":
			v := save{}
			err = dec.DecodeValue(reflect.ValueOf(&v))
			cvo.op = v
		default:
			return fmt.Errorf("unknown CustomValueOp[1] type %q", name)
		}
		if err != nil {
			return fmt.Errorf("decoding CustomValueOp[1].%s: %w", name, err)
		}
	default:
		return fmt.Errorf("unsupported CustomValueOp[1] value: %d", c)
	}

	return nil
}

/*
read the first item in the duple, item and span
*/
func (cvo *customValueOp) readValue(dec *msgpack.Decoder) error {
	cnt, err := dec.DecodeMapLen()
	if err != nil {
		return fmt.Errorf("reading CustomValueOp[0] map len: %w", err)
	}
	for range cnt {
		key, err := dec.DecodeString()
		if err != nil {
			return fmt.Errorf("reading CustomValueOp[0] key: %w", err)
		}
		switch key {
		case "item":
			err = cvo.readCustomValueData(dec)
		case "span":
			err = cvo.span.decodeMsgpack(dec)
		default:
			return fmt.Errorf("unknown key %q under CustomValueOp[0]", key)
		}
		if err != nil {
			return fmt.Errorf("decoding CustomValueOp[0] key %q: %w", key, err)
		}
	}
	return nil
}

func (cvo *customValueOp) readCustomValueData(dec *msgpack.Decoder) error {
	cnt, err := dec.DecodeMapLen()
	if err != nil {
		return fmt.Errorf("reading CustomValueOp.item map len: %w", err)
	}
	for range cnt {
		key, err := dec.DecodeString()
		if err != nil {
			return fmt.Errorf("reading CustomValueOp.item key: %w", err)
		}
		switch key {
		case "name":
			cvo.name, err = dec.DecodeString()
		case "data":
			cvo.id, err = readCVID(dec)
		case "notify_on_drop":
			_, err = dec.DecodeBool()
		default:
			return fmt.Errorf("unknown key %q under CustomValueOp.item", key)
		}
		if err != nil {
			return fmt.Errorf("decoding CustomValueOp.item key %q: %w", key, err)
		}
	}
	return nil
}

func readCVID(dec *msgpack.Decoder) (uint32, error) {
	n, err := dec.DecodeArrayLen()
	if err != nil {
		return 0, fmt.Errorf("reading Binary array length: %w", err)
	}
	if n < 1 {
		return 0, nil
	}
	// just "dec.ReadFull(buf)" won't work as uint8 might be encoded using
	// two bytes per value but ArrayLen gives us count of items (not bytes)
	buf := make([]byte, n)
	for i := range n {
		if buf[i], err = dec.DecodeUint8(); err != nil {
			return 0, fmt.Errorf("reading array item [%d]: %w", i, err)
		}
	}
	if len(buf) != 4 {
		return 0, fmt.Errorf("expected CustomValue data to be 4 bytes, got %d", len(buf))
	}
	return binary.BigEndian.Uint32(buf), nil
}

func (op *operation) decodeMsgpack(dec *msgpack.Decoder, p *Plugin) error {
	cnt, err := dec.DecodeArrayLen()
	if err != nil {
		return fmt.Errorf("reading Operation tuple length: %w", err)
	}
	if cnt != 2 {
		return fmt.Errorf("expected 2-tuple, got %d", cnt)
	}

	// first map with item + span
	cnt, err = dec.DecodeMapLen()
	if err != nil {
		return fmt.Errorf("reading Operation map len: %w", err)
	}
	for range cnt {
		key, err := dec.DecodeString()
		if err != nil {
			return fmt.Errorf("reading Operation key: %w", err)
		}
		switch key {
		case "item":
			// single item map like {"Math": "Plus"}
			err = op.op.DecodeMsgpack(dec)
		case "span":
			err = (&Span{}).decodeMsgpack(dec)
		default:
			return fmt.Errorf("unknown key %q under Operation", key)
		}
		if err != nil {
			return fmt.Errorf("decoding Operation key %q: %w", key, err)
		}
	}

	// Value
	return op.value.decodeMsgpack(dec, p)
}
