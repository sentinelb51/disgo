package discord

import (
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"testing"
)

func TestPayloadWithFiles_FileName(t *testing.T) {
	for _, name := range []string{"transcript.txt", `say "hi".txt`, `back\slash.txt`} {
		t.Run(name, func(t *testing.T) {
			body, err := PayloadWithFiles(MessageCreate{Content: "file"}, NewFile(name, "", strings.NewReader("data")))
			if err != nil {
				t.Fatal(err)
			}

			_, params, err := mime.ParseMediaType(body.ContentType)
			if err != nil {
				t.Fatal(err)
			}
			reader := multipart.NewReader(body.Buffer, params["boundary"])
			if _, err = reader.NextPart(); err != nil { // payload_json
				t.Fatal(err)
			}
			part, err := reader.NextPart()
			if err != nil {
				t.Fatal(err)
			}
			if got := part.FileName(); got != name {
				t.Errorf("file name %q, want %q", got, name)
			}
			if data, _ := io.ReadAll(part); string(data) != "data" {
				t.Errorf("file data %q", data)
			}
		})
	}
}

// TestPayloadWithFiles_HeaderInjection sends a file name with a line break, which must not end the header
func TestPayloadWithFiles_HeaderInjection(t *testing.T) {
	body, err := PayloadWithFiles(MessageCreate{}, NewFile("a.txt\r\nX-Injected: 1", "", strings.NewReader("data")))
	if err != nil {
		t.Fatal(err)
	}

	_, params, err := mime.ParseMediaType(body.ContentType)
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(body.Buffer, params["boundary"])
	if _, err = reader.NextPart(); err != nil {
		t.Fatal(err)
	}
	part, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if injected := part.Header.Get("X-Injected"); injected != "" {
		t.Errorf("file name wrote header X-Injected: %s", injected)
	}
}
