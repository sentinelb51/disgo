package gateway

import "encoding/json/jsontext"

// DecodeFrame decodes a frame as the transports do, keeping its data as RawD only when keepRaw
func DecodeFrame(dec *jsontext.Decoder, message *Message, keepRaw bool) error {
	return message.decode(dec, keepRaw)
}
