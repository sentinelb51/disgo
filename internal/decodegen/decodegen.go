// Package decodegen writes the discord package's decoders (decode_gen.go): for each struct type named as a root, and
// each method-free struct type of the package its fields reach, a function reading the object token by token, each
// field by the helper for its type (decode.go), without reflection. It reads the package's source with go/types,
// leaving the generated file out, so a stale one never stops it.
package decodegen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
)

// Output is the file Generate writes, in the package's directory
const Output = "decode_gen.go"

const (
	snowflakePath = "github.com/disgoorg/snowflake/v2"
	jsontextPath  = "encoding/json/jsontext"
)

// Roots are the types decoded through generated functions, by name: the wire types of the package's
// UnmarshalJSONFrom methods, and the types decoded by reflection at the top of an event. Every method-free struct
// type of the package their fields reach is generated too
var Roots = []string{
	"messageWire",
	"embedWire",
	"gatewayGuildWire",
	"guildThread",
	"guildTextChannelWire",
	"guildNewsChannelWire",
	"guildCategoryChannelWire",
	"guildVoiceChannelWire",
	"guildStageVoiceChannelWire",
	"guildForumChannelWire",
	"guildMediaChannelWire",
	"dmChannel",
	"groupDMChannel",
	"applicationCommandInteractionWire",
	"componentInteractionWire",
	"autocompleteInteractionWire",
	"modalSubmitInteractionWire",
	"rawInteraction",
	"resolvedData",
}

// TestOutput is the test file Generate writes beside Output: a decoder by each generated function and by reflection,
// for every generated type, for the package's tests to compare
const TestOutput = "decode_gen_test.go"

// Files are what Generate writes: Output's source and TestOutput's
type Files struct {
	Source, Test []byte
}

// Generate writes the decoders of the package in dir
func Generate(dir string) (Files, error) {

	pkg, err := check(dir)
	if err != nil {
		return Files{}, err
	}

	g := &generator{pkg: pkg, done: map[*types.Named]bool{}, unsupported: map[*types.Named]string{}}
	for _, name := range Roots {
		object := pkg.Scope().Lookup(name)
		if object == nil {
			return Files{}, fmt.Errorf("decodegen: no type %s", name)
		}
		named, ok := object.Type().(*types.Named)
		if !ok {
			return Files{}, fmt.Errorf("decodegen: %s is no named type", name)
		}
		if reason := g.why(named); reason != "" {
			return Files{}, fmt.Errorf("decodegen: root %s can't be generated: %s", name, reason)
		}
		g.queue = append(g.queue, named)
	}

	for len(g.queue) > 0 {
		named := g.queue[0]
		g.queue = g.queue[1:]
		if g.done[named] {
			continue
		}
		g.done[named] = true
		if err := g.object(named); err != nil {
			return Files{}, err
		}
	}

	source, err := g.file()
	if err != nil {
		return Files{}, err
	}
	test, err := g.testFile()
	if err != nil {
		return Files{}, err
	}
	return Files{Source: source, Test: test}, nil
}

// check type-checks the package in dir, its generated files left out
func check(dir string) (*types.Package, error) {

	fset := token.NewFileSet()
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}

	var files []*ast.File
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") || filepath.Base(path) == Output {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	// The package calls the functions it is about to get, which go undefined: every other error stops it
	var failed []error
	config := types.Config{Importer: importer.ForCompiler(fset, "source", nil), Error: func(err error) {
		if typeErr, ok := err.(types.Error); ok && strings.HasPrefix(typeErr.Msg, "undefined: decode") {
			return
		}
		failed = append(failed, err)
	}}
	pkg, _ := config.Check(files[0].Name.Name, fset, files, nil)
	if len(failed) > 0 {
		return nil, fmt.Errorf("decodegen: type-check %s: %w", dir, failed[0])
	}
	return pkg, nil
}

type generator struct {
	pkg         *types.Package
	queue       []*types.Named
	done        map[*types.Named]bool
	unsupported map[*types.Named]string // why a struct type isn't generated, by type

	funcs     []string // each generated function, by type
	generated []string // each generated type's name
	imports   map[string]bool
}

// field is a struct type's field as encoding/json decodes it: by name, at the end of path
type field struct {
	name  string
	path  []string
	typ   types.Type
	depth int
	tag   bool
}

// object writes the decoder of named
func (g *generator) object(named *types.Named) error {

	fields, err := jsonFields(named.Underlying().(*types.Struct))
	if err != nil {
		return fmt.Errorf("decodegen: %s: %w", named.Obj().Name(), err)
	}

	var body strings.Builder
	name := named.Obj().Name()

	// A root is entered from the package, with a decoder: a state of the pool reads it
	if slices.Contains(Roots, name) {
		// The package declares no such function itself
		if object := g.pkg.Scope().Lookup(funcName(named)); object != nil {
			return fmt.Errorf("decodegen: %s's decoder, %s, would shadow the package's own", name, funcName(named))
		}
		fmt.Fprintf(&body, "func %s(dec *jsontext.Decoder, v *%s) error {\n", funcName(named), name)
		fmt.Fprintf(&body, "\td := enterDecode(dec)\n\terr := d.%s(v)\n\tleaveDecode(d)\n\treturn err\n}\n\n", funcName(named))
	}

	fmt.Fprintf(&body, "func (d *decodeState) %s(v *%s) error {\n", funcName(named), name)
	body.WriteString("\tif more, err := beginObject(d, v); !more {\n\t\treturn err\n\t}\n\n")
	body.WriteString("\tfor d.dec.PeekKind() == '\"' {\n\t\tname, err := d.dec.ReadValue()\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\n")
	body.WriteString("\t\tswitch string(name) {\n")

	for _, f := range fields {
		statement, err := g.statement("&v."+strings.Join(f.path, "."), f.typ)
		if err != nil {
			return fmt.Errorf("decodegen: %s.%s: %w", name, strings.Join(f.path, "."), err)
		}
		fmt.Fprintf(&body, "\t\tcase %s:\n\t\t\terr = %s\n", "`\""+f.name+"\"`", statement)
	}

	body.WriteString("\t\tdefault:\n\t\t\terr = otherMember(d, name, v)\n\t\t}\n")
	body.WriteString("\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n\n\treturn endObject(d)\n}\n")

	g.funcs = append(g.funcs, body.String())
	g.generated = append(g.generated, name)
	return nil
}

// statement decodes into the field at target, of typ. A pointer or slice is decoded in place, its elements by
// their own decoder
func (g *generator) statement(target string, typ types.Type) (string, error) {
	switch t := typ.(type) {
	case *types.Pointer:
		elem, err := g.decoder(t.Elem())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("decodePtr(d, %s, %s)", target, elem), nil
	case *types.Slice:
		elem, err := g.decoder(t.Elem())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("decodeSlice(d, %s, %s)", target, elem), nil
	case *types.Map:
		if isID(t.Key()) {
			elem, err := g.decoder(t.Elem())
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("decodeMapID(d, %s, %s)", target, elem), nil
		}
	}

	decoder, err := g.decoder(typ)
	if err != nil {
		return "", err
	}
	// A generated type's decoder is a method expression: called as one
	if method, ok := strings.CutPrefix(decoder, "(*decodeState)."); ok {
		return fmt.Sprintf("d.%s(%s)", method, target), nil
	}
	return fmt.Sprintf("%s(d, %s)", decoder, target), nil
}

// method reports whether typ, or its pointer, decodes itself
func (g *generator) method(typ types.Type) bool {
	return hasMethod(typ, "UnmarshalJSONFrom") || hasMethod(typ, "UnmarshalJSON") || hasMethod(typ, "UnmarshalText")
}

// decoder is an expression of type decodeFunc[typ]
func (g *generator) decoder(typ types.Type) (string, error) {

	name := g.typeString(typ)
	if isID(typ) {
		return "decodeID", nil
	}

	switch typ.(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Array:
		// An element of a pointer, slice or map that is one (statement takes them as fields), its pointer method
		// set not the element's
		return fmt.Sprintf("reflected[%s]", name), nil
	}

	switch {
	case hasMethod(typ, "UnmarshalJSONFrom"):
		return fmt.Sprintf("fromMethod[%s]", name), nil
	case hasMethod(typ, "UnmarshalJSON"):
		return fmt.Sprintf("jsonMethod[%s]", name), nil
	case hasMethod(typ, "UnmarshalText"):
		return fmt.Sprintf("reflected[%s]", name), nil
	}

	switch t := typ.Underlying().(type) {
	case *types.Basic:
		switch {
		case t.Info()&types.IsString != 0:
			return fmt.Sprintf("decodeString[%s]", name), nil
		case t.Info()&types.IsBoolean != 0:
			return fmt.Sprintf("decodeBool[%s]", name), nil
		case t.Info()&types.IsInteger != 0 && t.Info()&types.IsUnsigned == 0:
			return fmt.Sprintf("decodeInt[%s]", name), nil
		case t.Info()&types.IsFloat != 0:
			return fmt.Sprintf("decodeFloat[%s]", name), nil
		}
	case *types.Struct:
		if named, ok := typ.(*types.Named); ok && named.Obj().Pkg() == g.pkg && named.TypeParams().Len() == 0 {
			if reason := g.why(named); reason == "" {
				if !g.done[named] {
					g.queue = append(g.queue, named)
				}
				return "(*decodeState)." + funcName(named), nil
			}
		}
	}

	// Unsigned numbers, a slice or pointer as an element, maps but by snowflake, interfaces and others' structs:
	// none that Discord's hot payloads carry, decoded by reflection
	return fmt.Sprintf("reflected[%s]", name), nil
}

// why says why named can't be generated, or nothing when it can: it decodes itself, has a field encoding/json would
// decode by an option the generator doesn't, or embeds a pointer
func (g *generator) why(named *types.Named) string {

	if reason, known := g.unsupported[named]; known {
		return reason
	}

	reason := ""
	structType, isStruct := named.Underlying().(*types.Struct)
	switch {
	case !isStruct:
		reason = "not a struct"
	case g.method(named):
		reason = "decodes itself"
	default:
		if _, err := jsonFields(structType); err != nil {
			reason = err.Error()
		}
	}

	g.unsupported[named] = reason
	return reason
}

// jsonFields are the fields encoding/json decodes in a struct: exported ones by their tag's name or their own, and
// those of embedded structs, the shallowest winning a name, or the tagged one among equals. A struct type embedded
// twice, or a name no field wins, is left to encoding/json, whose rules for them the generator doesn't follow
func jsonFields(structType *types.Struct) ([]field, error) {

	var all []field
	seen := map[*types.Struct]bool{structType: true}
	var walk func(s *types.Struct, path []string, depth int) error
	walk = func(s *types.Struct, path []string, depth int) error {
		for index := range s.NumFields() {
			variable := s.Field(index)
			tag := reflect.StructTag(s.Tag(index)).Get("json")
			if tag == "-" {
				continue
			}

			name, options, _ := strings.Cut(tag, ",")
			for option := range strings.SplitSeq(options, ",") {
				switch option {
				case "", "omitempty", "omitzero":
				default:
					return fmt.Errorf("field %s has option %q", variable.Name(), option)
				}
			}

			fieldPath := append(slices.Clone(path), variable.Name())
			if variable.Embedded() && name == "" {
				embedded := variable.Type()
				if _, pointer := embedded.(*types.Pointer); pointer {
					return fmt.Errorf("embeds a pointer, %s", variable.Name())
				}
				if inner, ok := embedded.Underlying().(*types.Struct); ok {
					if seen[inner] {
						return fmt.Errorf("embeds %s twice", variable.Name())
					}
					seen[inner] = true
					if err := walk(inner, fieldPath, depth+1); err != nil {
						return err
					}
					continue
				}
			}

			if !variable.Exported() {
				continue
			}
			if name == "" {
				name = variable.Name()
			}
			if strings.ToLower(name) != name || !plainName(name) {
				return fmt.Errorf("field %s is named %q, matched only by encoding/json's folding", variable.Name(), name)
			}

			all = append(all, field{name: name, path: fieldPath, typ: variable.Type(), depth: depth, tag: tag != ""})
		}
		return nil
	}

	if err := walk(structType, nil, 0); err != nil {
		return nil, err
	}

	// The dominant field of each name, in the order fields come
	var fields []field
	for _, candidate := range all {
		if slices.ContainsFunc(fields, func(f field) bool { return f.name == candidate.name }) {
			continue
		}
		var rivals []field
		for _, f := range all {
			if f.name == candidate.name {
				rivals = append(rivals, f)
			}
		}
		shallowest := slices.MinFunc(rivals, func(a, b field) int { return a.depth - b.depth }).depth
		rivals = slices.DeleteFunc(rivals, func(f field) bool { return f.depth != shallowest })
		if len(rivals) > 1 {
			tagged := slices.DeleteFunc(slices.Clone(rivals), func(f field) bool { return !f.tag })
			if len(tagged) != 1 {
				return nil, fmt.Errorf("no field wins the name %q", candidate.name)
			}
			rivals = tagged
		}
		fields = append(fields, rivals[0])
	}
	return fields, nil
}

// plainName mirrors decode.go's: a name the generated switch matches exactly
func plainName(name string) bool {
	for _, c := range []byte(name) {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return name != ""
}

// isID reports whether typ is snowflake.ID
func isID(typ types.Type) bool {
	named, ok := typ.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == snowflakePath && named.Obj().Name() == "ID"
}

func hasMethod(typ types.Type, name string) bool {
	if _, pointer := typ.(*types.Pointer); !pointer {
		typ = types.NewPointer(typ)
	}
	object, _, _ := types.LookupFieldOrMethod(typ, true, nil, name)
	_, isFunc := object.(*types.Func)
	return isFunc
}

func funcName(named *types.Named) string {
	name := named.Obj().Name()
	return "decode" + strings.ToUpper(name[:1]) + name[1:]
}

func (g *generator) typeString(typ types.Type) string {
	return types.TypeString(typ, func(pkg *types.Package) string {
		if pkg == g.pkg {
			return ""
		}
		if g.imports == nil {
			g.imports = map[string]bool{}
		}
		g.imports[pkg.Path()] = true
		return pkg.Name()
	})
}

func (g *generator) file() ([]byte, error) {

	var out bytes.Buffer
	out.WriteString("// Code generated by internal/decodegen; DO NOT EDIT.\n\npackage discord\n\nimport (\n")

	// The standard library's first, then the others, as goimports groups them
	standard, others := []string{jsontextPath}, []string(nil)
	for path := range g.imports {
		switch {
		case path == jsontextPath:
		case strings.Contains(strings.Split(path, "/")[0], "."):
			others = append(others, path)
		default:
			standard = append(standard, path)
		}
	}
	sort.Strings(standard)
	sort.Strings(others)
	for _, path := range standard {
		fmt.Fprintf(&out, "\t%q\n", path)
	}
	if len(others) > 0 {
		out.WriteString("\n")
		for _, path := range others {
			fmt.Fprintf(&out, "\t%q\n", path)
		}
	}
	out.WriteString(")\n\n")

	sort.Strings(g.funcs)
	for _, f := range g.funcs {
		out.WriteString(f)
		out.WriteString("\n")
	}

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return out.Bytes(), fmt.Errorf("decodegen: format: %w", err)
	}
	return formatted, nil
}

// testFile lists each generated type's decoders, by its function and by encoding/json/v2's reflection
func (g *generator) testFile() ([]byte, error) {

	var out bytes.Buffer
	out.WriteString("// Code generated by internal/decodegen; DO NOT EDIT.\n\npackage discord\n\n")
	out.WriteString("import (\n\t\"encoding/json/jsontext\"\n\tjsonv2 \"encoding/json/v2\"\n)\n\n")
	out.WriteString("// generatedDecoders decode each generated type by its function, and by reflection\n")
	out.WriteString("var generatedDecoders = []generatedDecoder{\n")

	names := slices.Clone(g.generated)
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&out, "\t{%q,\n", name)
		fmt.Fprintf(&out, "\t\tfunc(dec *jsontext.Decoder) (any, error) {\n\t\t\tv, d := new(%s), enterDecode(dec)\n\t\t\tdefer leaveDecode(d)\n\t\t\treturn v, d.%s(v)\n\t\t},\n", name, "decode"+strings.ToUpper(name[:1])+name[1:])
		fmt.Fprintf(&out, "\t\tfunc(data []byte) (any, error) { v := new(%s); return v, jsonv2.Unmarshal(data, v, decodeOptions) }},\n", name)
	}
	out.WriteString("}\n")

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return out.Bytes(), fmt.Errorf("decodegen: format the test file: %w", err)
	}
	return formatted, nil
}

// Write writes the decoders of the package in dir to its Output, and their test table to TestOutput
func Write(dir string) error {
	files, err := Generate(dir)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, Output), files.Source, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, TestOutput), files.Test, 0o644)
}
