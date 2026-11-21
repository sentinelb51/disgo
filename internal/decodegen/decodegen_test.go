package decodegen

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"testing"
)

// The generated files are what the generator writes now: a struct changed since needs go generate ./discord
func TestGeneratedUpToDate(t *testing.T) {
	const dir = "../../discord"

	files, err := Generate(dir)
	if err != nil {
		t.Fatal(err)
	}

	for name, want := range map[string][]byte{Output: files.Source, TestOutput: files.Test} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is stale: go generate ./discord", name)
		}
	}
}

// Shapes the generator leaves to encoding/json, whose rules for them it doesn't follow, and an element that is a
// pointer to a type with methods: no wrong decoder, no code that doesn't compile
func TestShapes(t *testing.T) {
	const source = "package p\n" +
		"type Leaf struct { X int `json:\"x\"` }\n" +
		"type Common struct { Leaf }\n" +
		"type Left struct { Common }\n" +
		"type Right struct { Common }\n" +
		"type Twice struct { Left; Right }\n" +
		"type Conflict struct { A int `json:\"x\"`; B int `json:\"x\"` }\n" +
		"type Self struct{}\n" +
		"func (*Self) UnmarshalJSON([]byte) error { return nil }\n" +
		"type Pointers struct { S []*Self `json:\"s\"` }\n"

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := new(types.Config).Check("p", fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	structOf := func(name string) *types.Struct {
		return pkg.Scope().Lookup(name).Type().Underlying().(*types.Struct)
	}

	// Each was generated, the name x left out of it
	for name, want := range map[string]string{"Twice": "embeds Common twice", "Conflict": `no field wins the name "x"`} {
		if _, err := jsonFields(structOf(name)); err == nil || err.Error() != want {
			t.Errorf("%s: %v, want %s", name, err, want)
		}
	}

	// Was jsonMethod[*Self], which needs **Self to have the method
	g := &generator{pkg: pkg, done: map[*types.Named]bool{}, unsupported: map[*types.Named]string{}}
	if statement, err := g.statement("&v.S", structOf("Pointers").Field(0).Type()); err != nil || statement != "decodeSlice(d, &v.S, reflected[*Self])" {
		t.Errorf("[]*Self: %s, %v", statement, err)
	}
}
