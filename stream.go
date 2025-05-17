package nu

import (
	"fmt"
	"reflect"

	"github.com/vmihailenco/msgpack/v5"
)

type (
	// This message is sent from producer to consumer.
	data struct {
		ID   int
		Data any
	}

	/*
		This message is sent from consumer to producer.

		Sent by the consumer in reply to each Data message, indicating that the consumer
		has finished processing that message. ack is used for flow control. If a consumer
		does not need to process a stream immediately, or is having trouble keeping up,
		it should not send ack messages until it is ready to process more Data.
	*/
	ack struct {
		ID int `msgpack:"Ack"`
	}

	/*
		This message is sent from producer to consumer.

		Must be sent at the end of a stream by the producer. The producer must not send any
		more Data messages after the end of the stream.

		The consumer must send Drop in reply unless the stream ended because the consumer
		chose to drop the stream.
	*/
	end struct {
		ID int `msgpack:"End"`
	}

	/*
		This message is sent from consumer to producer.

		Sent by the consumer to indicate disinterest in further messages from a stream.
		The producer may send additional Data messages after drop has been received, but
		should make an effort to stop sending messages and End the stream as soon as possible.

		The consumer should not consider Data messages sent after drop to be an error,
		unless End has already been received.

		The producer must send End in reply unless the stream ended because the producer
		ended the stream.
	*/
	drop struct {
		ID int `msgpack:"Drop"`
	}
)

func (d *data) decodeMsgpack(dec *msgpack.Decoder, p *Plugin) error {
	id, err := decodeTupleStart(dec)
	if err != nil {
		return err
	}
	d.ID = id

	keyName, err := decodeWrapperMap(dec)
	if err != nil {
		return fmt.Errorf("reading the data map: %w", err)
	}
	switch keyName {
	case "List":
		v := Value{}
		if err := v.decodeMsgpack(dec, p); err != nil {
			return err
		}
		d.Data = v
	case "Raw":
		// contains either Ok or Err map
		if keyName, err = decodeWrapperMap(dec); err != nil {
			return fmt.Errorf("reading sub-map of Raw: %w", err)
		}
		switch keyName {
		case "Ok":
			if d.Data, err = decodeBinary(dec); err != nil {
				return fmt.Errorf("reading raw data: %w", err)
			}
		case "Err":
			e := LabeledError{}
			if err := dec.DecodeValue(reflect.ValueOf(&e)); err != nil {
				return err
			}
			d.Data = e
		default:
			return fmt.Errorf("unexpected key %q under Raw", keyName)
		}
	default:
		return fmt.Errorf("unexpected key %q under Data", keyName)
	}

	return nil
}

func (d *data) encodeMsgpack(enc *msgpack.Encoder, p *Plugin) error {
	if err := encodeTupleInMap(enc, "Data", d.ID); err != nil {
		return err
	}
	switch v := d.Data.(type) {
	case Value:
		if err := encodeMapStart(enc, "List"); err != nil {
			return err
		}
		return v.encodeMsgpack(enc, p)
	case []byte:
		if err := encodeMapStart(enc, "Raw"); err != nil {
			return err
		}
		if err := encodeMapStart(enc, "Ok"); err != nil {
			return err
		}
		return enc.EncodeBytes(v)
	case error:
		// if the Data contains error it must be a Raw stream, in case of
		// List stream the error must be wrapped into a Value.
		return encodeLabeledErrorToRawStream(enc, AsLabeledError(v))
	case LabeledError:
		return encodeLabeledErrorToRawStream(enc, AsLabeledError(&v))
	default:
		return fmt.Errorf("unsupported Data value: %T", v)
	}
}

func encodeLabeledErrorToRawStream(enc *msgpack.Encoder, le *LabeledError) error {
	if err := encodeMapStart(enc, "Raw"); err != nil {
		return err
	}
	if err := encodeMapStart(enc, "Err"); err != nil {
		return err
	}
	return enc.EncodeValue(reflect.ValueOf(le))
}
