package discord

import (
	"bytes"
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// generatedDecoder decodes one generated type by its function and by encoding/json/v2's reflection
// (decode_gen_test.go lists them)
type generatedDecoder struct {
	name      string
	generated func(dec *jsontext.Decoder) (any, error)
	reflected func(data []byte) (any, error)
}

// decodeCorpus is every object and array in the gateway's frames and this package's test payloads: each generated
// decoder is handed every one, its own type's or not
func decodeCorpus(t testing.TB) [][]byte {
	t.Helper()

	var corpus [][]byte
	var collect func(value jsontext.Value)
	collect = func(value jsontext.Value) {
		kind := value.Kind()
		if kind != '{' && kind != '[' {
			return
		}
		corpus = append(corpus, bytes.Clone(value))

		dec := jsontext.NewDecoder(bytes.NewReader(value))
		if _, err := dec.ReadToken(); err != nil {
			t.Fatal(err)
		}
		for dec.PeekKind() != '}' && dec.PeekKind() != ']' {
			if kind == '{' {
				if _, err := dec.ReadToken(); err != nil {
					t.Fatal(err)
				}
			}
			inner, err := dec.ReadValue()
			if err != nil {
				t.Fatal(err)
			}
			collect(inner)
		}
	}

	paths, err := filepath.Glob("../gateway/testdata/frames/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no frames: %v", err)
	}
	for _, path := range paths {
		indented, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var frame bytes.Buffer
		if err = jsonv1.Compact(&frame, indented); err != nil {
			t.Fatal(err)
		}
		collect(frame.Bytes())
	}

	for _, message := range benchmarkMessages {
		collect(message.data)
	}

	return corpus
}

// compareDecoders hands data to every generated decoder both ways: both must fail, or decode the same value
func compareDecoders(t *testing.T, data []byte) {
	t.Helper()

	for _, decoder := range generatedDecoders {
		dec := jsontext.NewDecoder(bytes.NewReader(data), decodeOptions)
		generated, generatedErr := decoder.generated(dec)
		if generatedErr == nil {
			// One value, and nothing after it, as Unmarshal takes
			if _, err := dec.ReadToken(); !errors.Is(err, io.EOF) {
				generatedErr = errors.New("more after the value")
			}
		}

		reflected, reflectedErr := decoder.reflected(data)
		switch {
		case (generatedErr == nil) != (reflectedErr == nil):
			t.Errorf("%s of %.200s: generated %v, reflection %v", decoder.name, data, generatedErr, reflectedErr)
		case generatedErr == nil && !sameDecoded(reflect.ValueOf(generated), reflect.ValueOf(reflected)):
			t.Errorf("%s of %.200s:\n generated %+v\nreflection %+v", decoder.name, data, generated, reflected)
		}
	}
}

var timeType = reflect.TypeFor[time.Time]()

// sameDecoded is reflect.DeepEqual but for times, which are the same instant, as a time's zone may come out as UTC
// one way and as a fixed +00:00 the other
func sameDecoded(a, b reflect.Value) bool {
	if a.Type() != b.Type() {
		return false
	}

	switch a.Kind() {
	case reflect.Pointer, reflect.Interface:
		if a.IsNil() || b.IsNil() {
			return a.IsNil() == b.IsNil()
		}
		return sameDecoded(a.Elem(), b.Elem())
	case reflect.Struct:
		if a.Type() == timeType {
			return instant(a).Equal(instant(b))
		}
		for index := range a.NumField() {
			if !sameDecoded(a.Field(index), b.Field(index)) {
				return false
			}
		}
		return true
	case reflect.Slice:
		if a.IsNil() != b.IsNil() || a.Len() != b.Len() {
			return false
		}
		for index := range a.Len() {
			if !sameDecoded(a.Index(index), b.Index(index)) {
				return false
			}
		}
		return true
	case reflect.Array:
		for index := range a.Len() {
			if !sameDecoded(a.Index(index), b.Index(index)) {
				return false
			}
		}
		return true
	case reflect.Map:
		if a.IsNil() != b.IsNil() || a.Len() != b.Len() {
			return false
		}
		for _, key := range a.MapKeys() {
			other := b.MapIndex(key)
			if !other.IsValid() || !sameDecoded(a.MapIndex(key), other) {
				return false
			}
		}
		return true
	case reflect.Func:
		return a.IsNil() && b.IsNil()
	}

	// Unexported fields are read through the value: compare their bits
	return reflect.DeepEqual(valueOf(a), valueOf(b))
}

// instant rebuilds a time.Time from its fields, as it may sit in an unexported one; a parsed time has no
// monotonic reading, so ext is seconds since year 1
func instant(value reflect.Value) time.Time {
	const unixToInternal = (1969*365 + 1969/4 - 1969/100 + 1969/400) * 24 * 60 * 60
	wall, ext := value.FieldByName("wall").Uint(), value.FieldByName("ext").Int()
	return time.Unix(ext-unixToInternal, int64(wall&(1<<30-1))).UTC()
}

// valueOf reads a basic value, exported or not
func valueOf(v reflect.Value) any {
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint()
	case reflect.Float32, reflect.Float64:
		return v.Float()
	}
	return v.Kind().String()
}

// Every generated decoder decodes what reflection does, from every object and array of the payloads Discord sends
func TestGeneratedDecoders(t *testing.T) {
	corpus := decodeCorpus(t)
	if len(generatedDecoders) == 0 || len(corpus) < 100 {
		t.Fatalf("%d decoders, %d payloads", len(generatedDecoders), len(corpus))
	}

	for _, data := range corpus {
		compareDecoders(t, data)
	}

	// What the payloads hold rarely: escapes, invalid UTF-8, names in another case or escaped, duplicates, nulls
	// everywhere, numbers out of range or of the wrong kind, values of the wrong kind
	for _, data := range []string{
		`{"content":"line\nbreak \u00e9\ud83d\ude00 \"quoted\""}`,
		"{\"content\":\"bad \xff utf8\",\"username\":\"\xc3\"}",
		`{"ID":"5","Content":"case","\u0069d":"6","username":"a","USERNAME":"b"}`,
		`{"id":"1","id":"2","mentions":[{"id":"3"}],"mentions":[{"id":"4"},{"id":"5"}]}`,
		`{"id":null,"content":null,"mentions":null,"member":null,"timestamp":null,"tts":null,"type":null,"embeds":null}`,
		`{"type":99999999999999999999,"flags":-1,"position":1.5,"color":1e3,"size":"5"}`,
		`{"tts":"true","mentions":{},"embeds":"x","member":[],"author":5,"id":5,"id":"x","guild_id":"01"}`,
		`{"timestamp":"2026-09-30T22:14:05.512+02:00","edited_timestamp":"not a time"}`,
		`{"timestamp":"2026-09-30T22:14:05Z","edited_timestamp":"2026-09-30T22:14:05Z"}`,
		// A repeated array decodes into the last one's elements, up to its capacity, as they are
		`{"mention_roles":["1","2"],"mention_roles":["3"],"mention_roles":[null,null]}`,
		`{"mention_roles":["1","2"],"mention_roles":[],"mention_roles":[null,null]}`,
		`{"mentions":[{"id":"1","username":"a"},{"id":"2"}],"mentions":[{"id":"3"}],"mentions":[{},{}]}`,
		`{"users":{"1":{"id":"1"},"x":{"id":"2"},"":{"id":"3"},"99999999999999999999":{}},"members":null}`,
		`{"roles":{"1":null},"channels":[]}`,
		`[]`, `null`, `5`, `"string"`, `{}`, `{"unknown":{"deep":[1,{"a":null}]},"id":"7"}`,
	} {
		compareDecoders(t, []byte(data))
	}
}

// FuzzGeneratedDecoders mutates the payloads: go test ./discord -run ^$ -fuzz FuzzGeneratedDecoders
func FuzzGeneratedDecoders(f *testing.F) {
	for _, data := range decodeCorpus(f) {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		compareDecoders(t, data)
	})
}
