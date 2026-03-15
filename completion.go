package nu

import (
	"fmt"

	"github.com/ainvaltin/nu-plugin/internal/mpack"
	"github.com/ainvaltin/nu-plugin/types"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/vmihailenco/msgpack/v5/msgpcode"
)

/*
DynamicSuggestion is the data structure used in the response of the
GetCompletion message to provide autocompletion items for a flag or
positional argument.

https://docs.rs/nu-protocol/latest/nu_protocol/struct.DynamicSuggestion.html
*/
type DynamicSuggestion struct {
	Value            string   // String replacement that will be introduced to the the buffer
	Display          string   // If given, overrides value as text displayed to user
	Description      string   // Optional description for the replacement
	Extra            []string // Optional vector of strings in the suggestion. These can be used to represent examples coming from a suggestion
	AppendWhitespace bool     // Whether to append a space after selecting this suggestion. This helps to avoid that a completer repeats the complete suggestion.
	MatchIndices     []uint64 // Indices of the graphemes in the suggestion that matched the typed text. Useful if using fuzzy matching.
	Span             *Span    // Replacement span in the buffer, if any.
	//Kind Option<SuggestionKind> https://docs.rs/nu-protocol/latest/nu_protocol/enum.SuggestionKind.html
}

func (ds DynamicSuggestion) encodeMsgpack(enc *msgpack.Encoder, p *Plugin) (err error) {
	items := mpack.MapItems{
		mpack.EncoderFuncString("value", ds.Value),
		mpack.EncoderFuncBool("append_whitespace", ds.AppendWhitespace),
	}
	items.AddOptionalStr("display_override", ds.Display)
	items.AddOptionalStr("description", ds.Description)
	items.AddOptionalEncoder(len(ds.Extra) > 0, mpack.EncoderFuncArray("extra", ds.Extra, enc.EncodeString))
	items.AddOptionalEncoder(len(ds.MatchIndices) > 0, mpack.EncoderFuncArray("match_indices", ds.MatchIndices, enc.EncodeUint64))
	if ds.Span != nil {
		items = append(items,
			func(enc *msgpack.Encoder) error {
				if err := enc.EncodeString("span"); err != nil {
					return err
				}
				return ds.Span.encodeMsgpack(enc)
			})
	}

	if err = items.EncodeMap(enc); err != nil {
		return fmt.Errorf("encoding DynamicSuggestion: %w", err)
	}
	return nil
}

/*
Message sent by the engine to request completion items.
Contains the state of the command line when completion is triggered.
*/
type getCompletion struct {
	Name    string  `msgpack:"name"`     // name of the plugin command
	ArgType argType `msgpack:"arg_type"` // which argument is to be completed
	Call    struct {
		// https://docs.rs/nu-protocol/latest/nu_protocol/struct.DynamicCompletionCallRef.html
		Call struct {
			// https://docs.rs/nu-protocol/latest/nu_protocol/ast/struct.Call.html
			DeclID     uint               `msgpack:"decl_id"`
			Arguments  []argument         `msgpack:"arguments"`
			ParserInfo msgpack.RawMessage `msgpack:"parser_info"` // HashMap<String, Expression>
			Head       Span               `msgpack:"head"`
		} `msgpack:"call"`
		Strip bool `msgpack:"strip"`
		Pos   uint `msgpack:"pos"`
	} `msgpack:"call"`
}

// https://docs.rs/nu-protocol/latest/nu_protocol/engine/enum.ArgType.html
type argType struct {
	Flag       string `msgpack:"Flag"`       // name of the flag
	Positional *uint  `msgpack:"Positional"` // position index
}

func (a argType) String() string {
	s := "ArgType{"
	if a.Positional != nil {
		s += fmt.Sprintf("Positional: %d", *a.Positional)
	}
	if a.Flag != "" {
		s += fmt.Sprintf("Flag: %s", a.Flag)
	}
	return s + "}"
}

/*
Parsed command arguments
https://docs.rs/nu-protocol/latest/nu_protocol/ast/enum.Argument.html
*/
type argument struct {
	// A positional argument (that is not Argument::Spread)
	Positional *expression
	// A named/flag argument that can optionally receive a Value as an Expression
	Named *named
	// unknown argument used in “fall-through” signatures
	Unknown *expression
	// a list spread to fill in rest arguments
	Spread *expression
}

func (a argument) String() string {
	s := "Argument{"
	if a.Positional != nil {
		s += fmt.Sprintf("Positional: %s", a.Positional)
	}
	if a.Named != nil {
		s += fmt.Sprintf("Named: %s", a.Named)
	}
	if a.Unknown != nil {
		s += fmt.Sprintf("Unknown: %s", a.Unknown)
	}
	if a.Spread != nil {
		s += fmt.Sprintf("Spread: %s", a.Spread)
	}
	return s + "}"
}

/*
https://docs.rs/nu-protocol/latest/nu_protocol/ast/struct.Expression.html
*/
type expression struct {
	Expr    expr       `msgpack:"expr"`
	Span    Span       `msgpack:"span"`
	Span_id uint       `msgpack:"span_id"`
	Ty      types.Type `msgpack:"ty"`
}

func (exp *expression) DecodeMsgpack(dec *msgpack.Decoder) error {
	return decodeMap("expression", dec, func(dec *msgpack.Decoder, key string) (err error) {
		switch key {
		case "expr":
			err = dec.Decode(&exp.Expr)
		case "span":
			err = exp.Span.decodeMsgpack(dec)
		case "span_id":
			exp.Span_id, err = dec.DecodeUint()
		case "ty":
			exp.Ty, err = types.DecodeMsgpack(dec)
		default:
			return errUnknownField
		}
		return err
	})
}

func (exp *expression) String() string {
	s := "Expression{"
	s += fmt.Sprintf("Expr: %v", exp.Expr)
	if exp.Ty != nil {
		s += fmt.Sprintf(" Ty: %s", exp.Ty)
	}
	s += fmt.Sprintf(" span_id: %d", exp.Span_id)
	s += fmt.Sprintf(" span: {%d, %d}", exp.Span.Start, exp.Span.End)
	return s + "}"
}

type spanned[T any] struct {
	Item T    `msgpack:"item"`
	Span Span `msgpack:"span"`
}

func (s *spanned[T]) String() string {
	return fmt.Sprintf("{item: %v, span:{%d, %d}}", s.Item, s.Span.Start, s.Span.End)
}

/*
A named/flag argument that can optionally receive a Value as an Expression

The optional second Spanned<String> refers to the short-flag version if used

my_cmd --flag
my_cmd -f
my_cmd --flag-with-value <expr>

Named((Spanned<String>, Option<Spanned<String>>, Option<Expression>)),
*/
type named struct {
	A spanned[string]
	B *spanned[string]
	C *expression
}

func (n *named) String() string {
	s := "Named{ A: " + n.A.String()
	if n.B != nil {
		s += fmt.Sprintf(" B: %s", n.B)
	}
	if n.C != nil {
		s += fmt.Sprintf(" C: %s", n.C)
	}
	return s + "}"
}

func (n *named) DecodeMsgpack(dec *msgpack.Decoder) error {
	err := decodeArrayItems(dec,
		func(d *msgpack.Decoder) error { return d.Decode(&n.A) },
		func(d *msgpack.Decoder) error {
			n.B = &spanned[string]{}
			return d.Decode(&n.B)
		},
		func(d *msgpack.Decoder) error {
			n.C = &expression{}
			return n.C.DecodeMsgpack(d)
		},
	)
	if err != nil {
		return fmt.Errorf("decoding 'named' tuple elements: %w", err)
	}
	return nil
}

func decodeArrayItems(dec *msgpack.Decoder, itemDecF ...func(*msgpack.Decoder) error) error {
	cnt, err := dec.DecodeArrayLen()
	if err != nil {
		return err
	}
	if len(itemDecF) < cnt {
		return fmt.Errorf("array has %d items but we have only %d decoders", cnt, len(itemDecF))
	}

	for n := range cnt {
		if err := itemDecF[n](dec); err != nil {
			return fmt.Errorf("decoding array element %d: %w", n, err)
		}
	}
	return nil
}

type expr struct {
	name string // enum variant name
	str  string // string value
	b    bool   // bool value or flag
	i    int64
	raw  msgpack.RawMessage
}

func (exp *expr) String() string {
	s := "Expr{" + exp.name
	switch exp.name {
	case "Nothing", "Garbage":
	case "String", "RawString":
		s += ": " + exp.str
	case "Bool":
		s += fmt.Sprintf(": %t", exp.b)
	case "Int":
		s += fmt.Sprintf(": %d", exp.i)
	case "Filepath", "Directory", "GlobPattern":
		s += fmt.Sprintf(": %s, %t", exp.str, exp.b)
	default:
		s += fmt.Sprintf("raw: %x", exp.raw)
	}
	return s + "}"
}

func (iot *expr) DecodeMsgpack(dec *msgpack.Decoder) error {
	c, err := dec.PeekCode()
	if err != nil {
		return fmt.Errorf("peeking item type: %w", err)
	}

	switch {
	case msgpcode.IsFixedString(c), msgpcode.IsString(c):
		if iot.name, err = dec.DecodeString(); err != nil {
			return fmt.Errorf("decoding type name: %w", err)
		}
		// todo: check do we recognize this type? Expecting:
		// Nothing, Garbage
	case msgpcode.IsFixedMap(c):
		if iot.name, err = mpack.DecodeWrapperMap(dec); err != nil {
			return err
		}

		switch iot.name {
		case "String", "RawString":
			if iot.str, err = dec.DecodeString(); err != nil {
				return fmt.Errorf("decode %s value: %w", iot.name, err)
			}
		case "Bool":
			if iot.b, err = dec.DecodeBool(); err != nil {
				return fmt.Errorf("decode %s value: %w", iot.name, err)
			}
		case "Int":
			if iot.i, err = dec.DecodeInt64(); err != nil {
				return fmt.Errorf("decode %s value: %w", iot.name, err)
			}
		default:
			iot.raw, err = dec.DecodeRaw()
		}
	default:
		return fmt.Errorf("unsupported Expr start code: %d", c)
	}

	return nil
}
