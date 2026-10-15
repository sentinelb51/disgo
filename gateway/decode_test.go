package gateway_test

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/disgo/gateway"
)

// The golden dumps were written by upstream disgo's decoder: a change to decoding must leave every one identical,
// and both transports must write the same
var update = flag.Bool("update", false, "rewrite testdata/frames/*.golden")

type fixture struct {
	name  string
	frame []byte
}

// fixtures are gateway frames as Discord sends them: indented for review, compacted for the wire
func fixtures(t testing.TB) []fixture {

	t.Helper()

	paths, err := filepath.Glob("testdata/frames/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}

	loaded := make([]fixture, 0, len(paths))
	for _, path := range paths {
		indented, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		var frame bytes.Buffer
		if err = json.Compact(&frame, indented); err != nil {
			t.Fatalf("%s: %v", path, err)
		}

		loaded = append(loaded, fixture{strings.TrimSuffix(filepath.Base(path), ".json"), frame.Bytes()})
	}

	return loaded
}

// transports read a frame: decoder as upstream disgo's transport does, a json.Decoder per frame, and stream as this
// fork's does, one reused jsontext.Decoder. Both run either way, so a benchmark compares the two
var transports = []struct {
	name   string
	decode func(frame []byte) (*gateway.Message, error)
}{
	{"decoder", func(frame []byte) (*gateway.Message, error) {
		var message gateway.Message
		err := json.NewDecoder(bytes.NewReader(frame)).Decode(&message)
		return &message, err
	}},
	{"stream", func(frame []byte) (*gateway.Message, error) {
		var message gateway.Message
		reused.Reset(bytes.NewReader(frame), streamOptions)
		err := jsonv2.UnmarshalDecode(reused, &message)
		return &message, err
	}},
}

var (
	reused        = jsontext.NewDecoder(bytes.NewReader(nil))
	streamOptions = jsonv2.JoinOptions(json.DefaultOptionsV1(), json.ReportErrorsWithLegacySemantics(false))
)

func TestDecodeGolden(t *testing.T) {
	for _, fixture := range fixtures(t) {
		for _, transport := range transports {
			t.Run(transport.name+"/"+fixture.name, func(t *testing.T) {

				message, err := transport.decode(fixture.frame)
				if err != nil {
					t.Fatal(err)
				}

				var builder strings.Builder
				dump(&builder, reflect.ValueOf(message).Elem(), 0)

				path := filepath.Join("testdata", "frames", fixture.name+".golden")
				if *update {
					if err = os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}

				golden, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("%v (generate with -update)", err)
				}

				if got := builder.String(); got != string(golden) {
					t.Errorf("decoded differently from upstream disgo:\n%s", difference(string(golden), got))
				}
			})
		}
	}
}

func BenchmarkDecode(b *testing.B) {
	for _, transport := range transports {
		for _, fixture := range fixtures(b) {
			b.Run(transport.name+"/"+fixture.name, func(b *testing.B) {
				b.SetBytes(int64(len(fixture.frame)))
				b.ReportAllocs()
				for b.Loop() {
					if _, err := transport.decode(fixture.frame); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

var timeType = reflect.TypeFor[time.Time]()

// dump writes every field, exported or not, with map keys sorted and no addresses, so two decoders
// that build the same values write the same text. Kind accessors work on unexported fields; Interface doesn't
func dump(builder *strings.Builder, value reflect.Value, depth int) {

	var pad = strings.Repeat("  ", depth+1)

	switch value.Kind() {
	case reflect.Invalid:
		builder.WriteString("<nil>")
	case reflect.Pointer, reflect.Interface:
		if value.IsNil() {
			builder.WriteString("nil")
			return
		}
		if value.Kind() == reflect.Interface {
			fmt.Fprintf(builder, "(%s) ", value.Elem().Type())
		} else {
			builder.WriteByte('&')
		}
		dump(builder, value.Elem(), depth)
	case reflect.Struct:
		if value.Type() == timeType {
			builder.WriteString(instant(value).Format(time.RFC3339Nano))
			return
		}
		fmt.Fprintf(builder, "%s{\n", value.Type())
		for index := range value.NumField() {
			fmt.Fprintf(builder, "%s%s: ", pad, value.Type().Field(index).Name)
			dump(builder, value.Field(index), depth+1)
			builder.WriteByte('\n')
		}
		builder.WriteString(pad[2:] + "}")
	case reflect.Map:
		if value.IsNil() {
			builder.WriteString("nil")
			return
		}
		keys := value.MapKeys()
		slices.SortFunc(keys, func(a, b reflect.Value) int { return strings.Compare(scalar(a), scalar(b)) })
		builder.WriteString("{\n")
		for _, key := range keys {
			fmt.Fprintf(builder, "%s%s: ", pad, scalar(key))
			dump(builder, value.MapIndex(key), depth+1)
			builder.WriteByte('\n')
		}
		builder.WriteString(pad[2:] + "}")
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice && value.IsNil() {
			builder.WriteString("nil")
			return
		}
		if value.Type().Elem().Kind() == reflect.Uint8 {
			fmt.Fprintf(builder, "bytes(%q)", value.Bytes())
			return
		}
		builder.WriteString("[\n")
		for index := range value.Len() {
			builder.WriteString(pad)
			dump(builder, value.Index(index), depth+1)
			builder.WriteByte('\n')
		}
		builder.WriteString(pad[2:] + "]")
	default:
		builder.WriteString(scalar(value))
	}
}

func scalar(value reflect.Value) string {
	switch value.Kind() {
	case reflect.String:
		return fmt.Sprintf("%q", value.String())
	case reflect.Bool:
		return fmt.Sprint(value.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprint(value.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return fmt.Sprint(value.Uint())
	case reflect.Float32, reflect.Float64:
		return fmt.Sprint(value.Float())
	}
	return "?" + value.Kind().String()
}

// instant rebuilds a time.Time from its fields, as it may sit in an unexported one; a parsed time has no
// monotonic reading, so ext is seconds since year 1. The zone is dropped: Parse picks Local when its offset matches
func instant(value reflect.Value) time.Time {
	const unixToInternal = (1969*365 + 1969/4 - 1969/100 + 1969/400) * 24 * 60 * 60
	wall, ext := value.FieldByName("wall").Uint(), value.FieldByName("ext").Int()
	return time.Unix(ext-unixToInternal, int64(wall&(1<<30-1))).UTC()
}

// difference names the first line that differs, with a little context either side
func difference(want, got string) string {

	var (
		wanted = strings.Split(want, "\n")
		lines  = strings.Split(got, "\n")
	)

	for index := range max(len(wanted), len(lines)) {
		if index < len(wanted) && index < len(lines) && wanted[index] == lines[index] {
			continue
		}
		from := max(index-3, 0)
		return fmt.Sprintf("line %d\nwant:\n%s\ngot:\n%s", index+1,
			strings.Join(wanted[from:min(index+3, len(wanted))], "\n"), strings.Join(lines[from:min(index+3, len(lines))], "\n"))
	}

	return ""
}
