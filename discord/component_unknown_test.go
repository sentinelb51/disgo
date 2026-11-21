package discord

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"strings"
	"testing"
	"testing/iotest"
)

// An unknown component keeps its own bytes: decoded from a stream, they were the decoder's buffer, which the values
// read after it overwrote
func TestUnknownComponentKeepsData(t *testing.T) {

	const unknown = `{"type":99,"id":7,"label":"from a future version"}`
	stream := `[` + unknown + `]` + strings.Repeat(` ["filler that the decoder reads into its buffer"]`, 64)

	dec := jsontext.NewDecoder(iotest.OneByteReader(bytes.NewReader([]byte(stream))), decodeOptions)

	var components []UnmarshalComponent
	if err := jsonv2.UnmarshalDecode(dec, &components); err != nil {
		t.Fatal(err)
	}
	for {
		var filler []string
		if err := jsonv2.UnmarshalDecode(dec, &filler); err != nil {
			break
		}
	}

	component, ok := components[0].Component.(UnknownComponent)
	if !ok {
		t.Fatalf("decoded as %T", components[0].Component)
	}
	if string(component.Data) != unknown {
		t.Errorf("Data: %s", component.Data)
	}
}
