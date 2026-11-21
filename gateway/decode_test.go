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

// transports read a frame: decoder as upstream disgo's transport does, a json.Decoder per frame; stream through
// Message's UnmarshalJSONFrom on one reused jsontext.Decoder, keeping RawD; and direct as this fork's transport does
// without raw events, d decoded as it streams in and RawD left nil. All run either way, so a benchmark compares them
var transports = []struct {
	name   string
	raw    bool
	decode func(frame []byte) (*gateway.Message, error)
}{
	{"decoder", true, func(frame []byte) (*gateway.Message, error) {
		var message gateway.Message
		err := json.NewDecoder(bytes.NewReader(frame)).Decode(&message)
		return &message, err
	}},
	{"stream", true, func(frame []byte) (*gateway.Message, error) {
		var message gateway.Message
		reused.Reset(bytes.NewReader(frame), streamOptions)
		err := jsonv2.UnmarshalDecode(reused, &message)
		return &message, err
	}},
	{"direct", false, func(frame []byte) (*gateway.Message, error) {
		var message gateway.Message
		reused.Reset(bytes.NewReader(frame), streamOptions)
		err := gateway.DecodeFrame(reused, &message, false)
		return &message, err
	}},
}

// withoutRawD is a dump without its RawD line, for a transport that doesn't keep it
func withoutRawD(dump string) string {
	lines := strings.Split(dump, "\n")
	return strings.Join(slices.DeleteFunc(lines, func(line string) bool { return strings.HasPrefix(line, "  RawD: ") }), "\n")
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

				if !transport.raw && message.RawD != nil {
					t.Errorf("RawD kept: %q", message.RawD)
				}

				var builder strings.Builder
				dump(&builder, reflect.ValueOf(message).Elem(), 0)

				path := filepath.Join("testdata", "frames", fixture.name+".golden")
				if *update {
					if !transport.raw {
						return
					}
					if err = os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}

				read, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("%v (generate with -update)", err)
				}

				golden, got := string(read), builder.String()
				if !transport.raw {
					golden, got = withoutRawD(golden), withoutRawD(got)
				}

				if got != golden {
					t.Errorf("decoded differently from upstream disgo:\n%s", difference(golden, got))
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

// Data arriving before op, or before t in a dispatch, is kept and decoded once the frame is read
func TestDecodeDataFirst(t *testing.T) {
	for _, frame := range []string{
		`{"d":{"heartbeat_interval":41250},"op":10}`,
		`{"op":0,"d":{"id":"5","channel_id":"6"},"t":"MESSAGE_DELETE","s":7}`,
	} {
		var message gateway.Message
		reused.Reset(strings.NewReader(frame), streamOptions)
		if err := gateway.DecodeFrame(reused, &message, false); err != nil {
			t.Fatalf("%s: %v", frame, err)
		}

		switch data := message.D.(type) {
		case gateway.MessageDataHello:
			if data.HeartbeatInterval != 41250 {
				t.Errorf("hello: %+v", data)
			}
		case gateway.EventMessageDelete:
			if data.ID != 5 || data.ChannelID != 6 || message.S != 7 {
				t.Errorf("message delete: %+v, s %d", data, message.S)
			}
		default:
			t.Errorf("%s: decoded as %T", frame, message.D)
		}
	}
}
