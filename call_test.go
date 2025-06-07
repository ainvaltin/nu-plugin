package nu

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/vmihailenco/msgpack/v5"
)

func Test_pipelineMetadata_DeEncode(t *testing.T) {
	t.Run("input == output", func(t *testing.T) {
		testCases := []pipelineMetadata{
			{DataSource: ""},
			{DataSource: "None"},
			{DataSource: "None", ContentType: "application/json"},
			{DataSource: "Ls"},
			{DataSource: "FilePath"},
			{DataSource: "FilePath", FilePath: "/foo/bar.json"},
			{DataSource: "FilePath", FilePath: "", ContentType: "text/html"},
			{DataSource: "FilePath", FilePath: "test.html", ContentType: "text/html"},
		}

		for x, tc := range testCases {
			bin, err := msgpack.Marshal(&tc)
			if err != nil {
				t.Fatalf("[%d] failed to marshal: %v", x, err)
			}
			var mdB pipelineMetadata
			if err := msgpack.Unmarshal(bin, &mdB); err != nil {
				t.Fatalf("[%d] failed to unmarshal: %v", x, err)
			}
			if diff := cmp.Diff(tc, mdB); diff != "" {
				t.Fatalf("[%d] mismatch (-want +got):\n%s", x, diff)
			}
		}
	})

	t.Run("input != output", func(t *testing.T) {
		testCases := []struct {
			in  pipelineMetadata // data to be encoded
			out pipelineMetadata // what we expect to get after serialization roundtrip
		}{
			// FilePath property is only valid (and serialized) when DataSource=="FilePath"
			{
				in:  pipelineMetadata{DataSource: "", FilePath: "foo.htm", ContentType: "text/html"},
				out: pipelineMetadata{DataSource: "", FilePath: "", ContentType: "text/html"},
			},
			{
				in:  pipelineMetadata{DataSource: "Ls", FilePath: "foo.htm", ContentType: "text/html"},
				out: pipelineMetadata{DataSource: "Ls", FilePath: "", ContentType: "text/html"},
			},
			{
				in:  pipelineMetadata{DataSource: "None", FilePath: "foo.htm", ContentType: "text/html"},
				out: pipelineMetadata{DataSource: "None", FilePath: "", ContentType: "text/html"},
			},
		}

		for x, tc := range testCases {
			bin, err := msgpack.Marshal(&tc.in)
			if err != nil {
				t.Fatalf("[%d] failed to marshal: %v", x, err)
			}
			var mdB pipelineMetadata
			if err := msgpack.Unmarshal(bin, &mdB); err != nil {
				t.Fatalf("[%d] failed to unmarshal: %v", x, err)
			}
			if diff := cmp.Diff(tc.out, mdB); diff != "" {
				t.Fatalf("[%d] mismatch (-want +got):\n%s", x, diff)
			}
		}
	})
}

func Test_Call_DeEncode_happy(t *testing.T) {
	// encode Call as message pack, then decode the binary
	// and see did we get back (the same) struct
	testCases := []call{
		{ID: 1, Call: signature{}},
		{ID: 0, Call: run{Name: "inc", Input: empty{}, Call: evaluatedCall{Head: Span{Start: 40400, End: 40403}, Positional: []Value{}, Named: NamedParams{}}}},
		{ID: 0, Call: run{Name: "inc", Input: Value{Value: int64(2), Span: Span{Start: 9090, End: 9093}}, Call: evaluatedCall{Head: Span{Start: 40400, End: 40403}, Positional: []Value{}, Named: NamedParams{}}}},
		{ID: 0, Call: run{Name: "inc", Input: listStream{ID: 2}, Call: evaluatedCall{Head: Span{Start: 40400, End: 40403}, Positional: []Value{}, Named: NamedParams{}}}},
		{ID: 2, Call: run{Name: "inc", Input: empty{}, Call: evaluatedCall{Head: Span{Start: 40400, End: 40403}, Positional: []Value{{Value: "0.1.2", Span: Span{Start: 40407, End: 40415}}}, Named: NamedParams{}}}},
		{ID: 2, Call: run{Name: "inc", Input: empty{}, Call: evaluatedCall{Head: Span{Start: 40400, End: 40403}, Positional: []Value{{Value: "0.1.2", Span: Span{Start: 40407, End: 40415}}}, Named: NamedParams{"major": Value{Value: true, Span: Span{Start: 40404, End: 40406}}}}}},
	}

	p := Plugin{}

	for x, tc := range testCases {
		bin, err := p.serialize(&tc)
		if err != nil {
			t.Errorf("[%d] encoding %#v: %v", x, tc, err)
			continue
		}

		dec := msgpack.NewDecoder(bytes.NewBuffer(bin))
		dec.SetMapDecoder(decodeNuMsgAll(&p, p.handleMsgDecode))
		dv, err := p.decodeInputMsg(dec)
		if err != nil {
			t.Errorf("[%d] decoding %#v: %v", x, tc, err)
			continue
		}

		if diff := cmp.Diff(tc, dv); diff != "" {
			t.Errorf("[%d] mismatch (-want +got):\n%s", x, diff)
		}
	}
}

func (c *call) encodeMsgpack(enc *msgpack.Encoder, p *Plugin) error {
	if err := encodeTupleInMap(enc, "Call", c.ID); err != nil {
		return err
	}
	switch mt := c.Call.(type) {
	case signature:
		return enc.EncodeString("Signature")
	case run:
		if err := encodeMapStart(enc, "Run"); err != nil {
			return err
		}
		return mt.encodeMsgpack(enc, p)
	default:
		return fmt.Errorf("unsupported Call type %T", mt)
	}
}

func (r *run) encodeMsgpack(enc *msgpack.Encoder, p *Plugin) error {
	if err := enc.EncodeMapLen(3); err != nil {
		return err
	}
	if err := enc.EncodeString("name"); err != nil {
		return err
	}
	if err := enc.EncodeString(r.Name); err != nil {
		return err
	}
	if err := enc.EncodeString("call"); err != nil {
		return err
	}
	if err := r.Call.encodeMsgpack(enc, p); err != nil {
		return err
	}
	if err := enc.EncodeString("input"); err != nil {
		return err
	}

	switch iv := r.Input.(type) {
	case nil, empty, *empty:
		return enc.EncodeString("Empty")
	case Value:
		return (&pipelineValue{V: iv}).encodeMsgpack(enc, p)
	case listStream:
		if err := encodeMapStart(enc, "ListStream"); err != nil {
			return err
		}
		if err := encodeMapStart(enc, "id"); err != nil {
			return err
		}
		return enc.EncodeInt(int64(iv.ID))
	default:
		return fmt.Errorf("unsupported Input type %T", iv)
	}
}

func (ec *evaluatedCall) encodeMsgpack(enc *msgpack.Encoder, p *Plugin) error {
	if err := enc.EncodeMapLen(3); err != nil {
		return fmt.Errorf("writing evaluatedCall map length: %w", err)
	}
	if err := enc.EncodeString("head"); err != nil {
		return err
	}
	if err := ec.Head.encodeMsgpack(enc); err != nil {
		return err
	}
	if err := enc.EncodeString("positional"); err != nil {
		return err
	}
	if err := ec.Positional.encodeMsgpack(enc, p); err != nil {
		return err
	}
	if err := enc.EncodeString("named"); err != nil {
		return err
	}
	if err := ec.Named.encodeMsgpack(enc, p); err != nil {
		return err
	}
	return nil
}

func (cr *callResponse) decodeMsgpack(dec *msgpack.Decoder, p *Plugin) (err error) {
	if cr.ID, err = decodeTupleStart(dec); err != nil {
		return fmt.Errorf("decoding CallResponse tuple: %w", err)
	}
	name, err := decodeWrapperMap(dec)
	if err != nil {
		return err
	}
	switch name {
	case "PipelineData":
		pd := pipelineData{}
		if err := pd.decodeMsgpack(dec, p); err != nil {
			return err
		}
		cr.Response = pd
	case "Error":
		cr.Response, err = decodeLabeledError(dec)
		return err
	default:
		return fmt.Errorf("unexpected CallResponse key %q", name)
	}
	return nil
}
