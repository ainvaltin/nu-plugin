package operator

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/vmihailenco/msgpack/v5"
)

func Test_Operator_DecodeMsgpack(t *testing.T) {
	enc := msgpack.GetEncoder()
	buf := bytes.Buffer{}
	opBytes := func(class, op string) ([]byte, error) {
		buf.Reset()
		enc.Reset(&buf)
		if err := enc.EncodeMapLen(1); err != nil {
			return nil, err
		}
		if err := enc.EncodeString(class); err != nil {
			return nil, err
		}
		if err := enc.EncodeString(op); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}

	t.Run("Valid", func(t *testing.T) {
		dec := msgpack.GetDecoder()
		dec.UsePreallocateValues(true)
		for _, class := range op_classes {
			for _, op := range class.op {
				b, err := opBytes(class.class, op)
				if err != nil {
					t.Errorf("encoding %s.%s as msgpack: %v", class.class, op, err)
					continue
				}
				dec.Reset(bytes.NewReader(b))

				var o Operator
				if err := o.DecodeMsgpack(dec); err != nil {
					t.Errorf("unexpected error when decoding %s.%s: %v", class.class, op, err)
				}
				if s := o.String(); s != class.class+"."+op {
					t.Errorf("unexpected string %q for %q.%q", s, class.class, op)
				}
				if o.Class() != class.cid {
					t.Errorf("expected class %x, got %x", class.cid, o.Class())
				}
			}
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		var testCases = []struct {
			getData func() ([]byte, error)
			errMsg  string
		}{
			{
				getData: func() ([]byte, error) { return opBytes("foobar", "Equal") },
				errMsg:  `unknown Operator class "foobar"`,
			},
			{
				getData: func() ([]byte, error) { return opBytes("Bits", "foobar") },
				errMsg:  `unknown Operator "foobar" in class "Bits"`,
			},
			{
				getData: func() ([]byte, error) { return msgpack.Marshal([]byte{1, 2, 3}) },
				errMsg:  `reading map length: msgpack: unexpected code=c4 decoding map length`,
			},
			{
				getData: func() ([]byte, error) {
					return msgpack.Marshal(map[string]string{"Bits": "BitOr", "Boolean": "Or"})
				},
				errMsg: `wrapper map is expected to contain one item, got 2`,
			},
			{
				getData: func() ([]byte, error) { return msgpack.Marshal(map[int]string{1: "BitOr"}) },
				errMsg:  `reading map key: msgpack: invalid code=1 decoding string/bytes length`,
			},
			{
				getData: func() ([]byte, error) { return msgpack.Marshal(map[string]int{"Bits": 1}) },
				errMsg:  `msgpack: invalid code=1 decoding string/bytes length`,
			},
		}

		dec := msgpack.GetDecoder()
		dec.UsePreallocateValues(true)

		for i, tc := range testCases {
			b, err := tc.getData()
			if err != nil {
				t.Errorf("[%d] encoding as msgpack: %v", i, err)
				continue
			}
			dec.Reset(bytes.NewReader(b))
			var o Operator
			if err := o.DecodeMsgpack(dec); err == nil {
				t.Errorf("[%d] expected error, got: %v", i, o)
			} else if diff := cmp.Diff(tc.errMsg, err.Error()); diff != "" {
				t.Errorf("[%d] mismatch (-want +got):\n%s", i, diff)
			}
		}
	})
}
