package nu

import (
	"context"
	"fmt"
	"reflect"

	"github.com/vmihailenco/msgpack/v5"

	"github.com/ainvaltin/nu-plugin/syntaxshape"
)

/*
Command describes an command provided by the plugin.
*/
type Command struct {
	Signature PluginSignature `msgpack:"sig"`
	Examples  Examples        `msgpack:"examples"`

	// callback executed on command invocation
	OnRun func(context.Context, *ExecCommand) error `msgpack:"-"`
}

func (c Command) Validate() error {
	if err := c.Signature.Validate(); err != nil {
		return err
	}
	if c.OnRun == nil {
		return fmt.Errorf("command must have on-run handler")
	}
	return nil
}

type PluginSignature struct {
	Name string `msgpack:"name"`
	// This should be a single sentence as it is the part shown for example in the completion menu.
	Desc string `msgpack:"description"`
	// Additional documentation of the command.
	Description        string         `msgpack:"extra_description"`
	SearchTerms        []string       `msgpack:"search_terms"`
	Category           string         `msgpack:"category"` // https://docs.rs/nu-protocol/latest/nu_protocol/enum.Category.html
	RequiredPositional PositionalArgs `msgpack:"required_positional"`
	OptionalPositional PositionalArgs `msgpack:"optional_positional,"`
	RestPositional     *PositionalArg `msgpack:"rest_positional,omitempty"`

	// The "help" (short "h") flag will be added automatically when plugin
	// is created, do not use these names for other flags or arguments.
	Named                Flags      `msgpack:"named"`
	InputOutputTypes     [][]string `msgpack:"input_output_types"` // https://docs.rs/nu-protocol/latest/nu_protocol/enum.Type.html
	IsFilter             bool       `msgpack:"is_filter"`
	CreatesScope         bool       `msgpack:"creates_scope"`
	AllowsUnknownArgs    bool       `msgpack:"allows_unknown_args"`
	AllowMissingExamples bool       `msgpack:"allow_variants_without_examples"`
}

type (
	PositionalArg struct {
		Name    string                  `msgpack:"name"`
		Desc    string                  `msgpack:"desc"`
		Shape   syntaxshape.SyntaxShape `msgpack:"shape"`
		VarId   uint                    `msgpack:"var_id,omitempty"`
		Default *Value                  `msgpack:"default_value,omitempty"`
	}
	PositionalArgs []PositionalArg
)

type (
	/*
		Flag is a definition of a flag (Shape is unassigned) or named argument (Shape assigned).
	*/
	Flag struct {
		Long     string                  `msgpack:"long"`
		Short    string                  `msgpack:"short,omitempty"` // must be single character!
		Shape    syntaxshape.SyntaxShape `msgpack:"arg,omitempty"`
		Required bool                    `msgpack:"required"`
		Desc     string                  `msgpack:"desc"`
		VarId    uint                    `msgpack:"var_id,omitempty"`
		Default  *Value                  `msgpack:"default_value,omitempty"`
	}
	Flags []Flag
)

type (
	Example struct {
		Example     string `msgpack:"example"`
		Description string `msgpack:"description"`
		Result      *Value `msgpack:"result,omitempty"`
	}
	Examples []Example
)

func (sig PluginSignature) Validate() error {
	if sig.Name == "" {
		return fmt.Errorf("command must have name")
	}
	if sig.Category == "" {
		return fmt.Errorf("command must have Category")
	}
	if sig.Desc == "" {
		return fmt.Errorf("command Desc must have value")
	}
	if len(sig.SearchTerms) == 0 {
		return fmt.Errorf("command Search Terms must have value")
	}
	if len(sig.InputOutputTypes) == 0 {
		return fmt.Errorf("command Input-Output types must be specified")
	}

	return sig.Named.Validate()
}

/*
Decode top-level "plugin input" message, the message must be "map".
*/
func decodeInputMsg(dec *msgpack.Decoder) (interface{}, error) {
	name, err := decodeWrapperMap(dec)
	if err != nil {
		return nil, fmt.Errorf("decode message's map: %w", err)
	}
	return handleMsgDecode(dec, name)
}

func handleMsgDecode(dec *msgpack.Decoder, name string) (_ interface{}, err error) {
	switch name {
	case "Call":
		return decodeCall(dec)
	case "Data":
		m := data{}
		return m, m.DecodeMsgpack(dec)
	case "Ack":
		m := ack{}
		m.ID, err = dec.DecodeInt()
		return m, err
	case "End":
		m := end{}
		m.ID, err = dec.DecodeInt()
		return m, err
	case "Drop":
		m := drop{}
		m.ID, err = dec.DecodeInt()
		return m, err
	case "EngineCallResponse":
		m := engineCallResponse{}
		return m, m.DecodeMsgpack(dec)
	case "Hello":
		m := hello{}
		return m, dec.DecodeValue(reflect.ValueOf(&m))
	case "Signal":
		m := signal{}
		if m.Signal, err = dec.DecodeString(); m.Signal == "Interrupt" {
			return nil, ErrInterrupt
		}
		return m, err
	default:
		return nil, fmt.Errorf("unknown message %q", name)
	}
}

var _ msgpack.CustomEncoder = (*PositionalArgs)(nil)

func (pa *PositionalArgs) EncodeMsgpack(enc *msgpack.Encoder) error {
	if pa == nil || len(*pa) == 0 {
		return enc.EncodeArrayLen(0)
	}
	if err := enc.EncodeArrayLen(len(*pa)); err != nil {
		return err
	}
	for _, v := range *pa {
		if err := enc.EncodeValue(reflect.ValueOf(&v)); err != nil {
			return err
		}
	}
	return nil
}

var _ msgpack.CustomEncoder = (*Flags)(nil)

func (flags *Flags) EncodeMsgpack(enc *msgpack.Encoder) error {
	if flags == nil || len(*flags) == 0 {
		return enc.EncodeArrayLen(0)
	}
	if err := enc.EncodeArrayLen(len(*flags)); err != nil {
		return err
	}
	for _, v := range *flags {
		if err := enc.EncodeValue(reflect.ValueOf(&v)); err != nil {
			return err
		}
	}
	return nil
}

func (flags *Flags) addHelp() error {
	for _, v := range *flags {
		if v.Long == "help" || v.Short == "h" {
			return fmt.Errorf("help flag is already registered")
		}
	}
	*flags = append(*flags, Flag{Long: "help", Short: "h", Desc: "Display the help message for this command"})
	return nil
}

func (flags *Flags) Validate() error {
	for _, v := range *flags {
		if len(v.Short) > 1 {
			return fmt.Errorf("flag's short name must be single character, got %q", v.Short)
		}
	}
	return nil
}

var _ msgpack.CustomEncoder = (*Examples)(nil)

func (ex *Examples) EncodeMsgpack(enc *msgpack.Encoder) error {
	if ex == nil || len(*ex) == 0 {
		return enc.EncodeArrayLen(0)
	}
	if err := enc.EncodeArrayLen(len(*ex)); err != nil {
		return err
	}
	for _, v := range *ex {
		if err := enc.EncodeValue(reflect.ValueOf(&v)); err != nil {
			return err
		}
	}
	return nil
}
