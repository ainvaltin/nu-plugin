package nu

import (
	"fmt"
	"io"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/vmihailenco/msgpack/v5/msgpcode"
)

func dumpMsgPack(dec *msgpack.Decoder, w io.Writer, prefix string) error {
	c, err := dec.PeekCode()
	if err != nil {
		return err
	}
	switch {
	case msgpcode.IsFixedMap(c) || c == msgpcode.Map16 || c == msgpcode.Map32:
		dumpMsgPackMap(dec, w, prefix)
	case msgpcode.IsString(c):
		s, err := dec.DecodeString()
		if err != nil {
			return fmt.Errorf("decoding string: %w", err)
		}
		fmt.Fprintf(w, "%sSTR: %s\n", prefix, s)
	case msgpcode.IsFixedNum(c):
		n, err := dec.DecodeInt64()
		if err != nil {
			return fmt.Errorf("decoding int: %w", err)
		}
		fmt.Fprintf(w, "%sINT: %d\n", prefix, n)
	case msgpcode.IsFixedArray(c):
		dumpMsgPackArray(dec, w, prefix)
	case c == msgpcode.Nil:
		fmt.Fprintf(w, "%sNIL\n", prefix)
	default:
		switch c {
		case msgpcode.Int8, msgpcode.Int16, msgpcode.Int32, msgpcode.Int64,
			msgpcode.Uint8, msgpcode.Uint16, msgpcode.Uint32, msgpcode.Uint64:
			n, err := dec.DecodeInt64()
			if err != nil {
				return fmt.Errorf("decoding int: %w", err)
			}
			fmt.Fprintf(w, "%sINT: %d\n", prefix, n)
		default:
			fmt.Fprintf(w, "%sCODE(%x)\n", prefix, c)
			if err = dec.Skip(); err != nil {
				return err
			}
		}
	}
	return nil
}

func dumpMsgPackMap(dec *msgpack.Decoder, w io.Writer, prefix string) error {
	ml, err := dec.DecodeMapLen()
	if err != nil {
		return fmt.Errorf("decode map length: %w", err)
	}
	fmt.Fprintf(w, "%sMAP(%d)\n", prefix, ml)
	prefix += "\t"
	for n := 1; n <= ml; n++ {
		fmt.Fprintf(w, "%sKEY[%d] ", prefix, n)
		if err := dumpMsgPack(dec, w, " "); err != nil {
			return err
		}
		fmt.Fprintf(w, "%sVAL[%d] ", prefix, n)
		if err := dumpMsgPack(dec, w, " "); err != nil {
			return err
		}
	}
	return nil
}

func dumpMsgPackArray(dec *msgpack.Decoder, w io.Writer, prefix string) error {
	al, err := dec.DecodeArrayLen()
	if err != nil {
		return fmt.Errorf("decode array length: %w", err)
	}
	fmt.Fprintf(w, "ARRAY(%d)\n", al)
	for ; al > 0; al-- {
		fmt.Fprintf(w, "%sA[%d] ", prefix, al)
		if err := dumpMsgPack(dec, w, prefix+"\t"); err != nil {
			return err
		}
	}
	return nil
}
