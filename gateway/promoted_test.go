package gateway_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// encoding/json/v2 prefers UnmarshalJSONFrom to UnmarshalJSON, promoted or not: a type that decodes itself in
// UnmarshalJSON but embeds a type with an UnmarshalJSONFrom is decoded by the embedded type's alone, unless it has an
// UnmarshalJSONFrom of its own. Checked over the packages that decode Discord's payloads
func TestNoPromotedUnmarshalJSONFrom(t *testing.T) {

	type typeName = string // package.Type

	var (
		methods  = map[typeName]map[string]bool{}
		embedded = map[typeName][]typeName{}
	)

	for _, dir := range []string{"../discord", "."} {
		paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}

		for _, path := range paths {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}

			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			file, err := parser.ParseFile(token.NewFileSet(), path, source, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}

			pkg := file.Name.Name
			qualify := func(expr ast.Expr) typeName {
				if star, ok := expr.(*ast.StarExpr); ok {
					expr = star.X
				}
				switch expr := expr.(type) {
				case *ast.Ident:
					return pkg + "." + expr.Name
				case *ast.SelectorExpr:
					if ident, ok := expr.X.(*ast.Ident); ok {
						return ident.Name + "." + expr.Sel.Name
					}
				}
				return ""
			}

			for _, decl := range file.Decls {
				switch decl := decl.(type) {
				case *ast.FuncDecl:
					if decl.Recv == nil || len(decl.Recv.List) == 0 {
						continue
					}
					receiver := qualify(decl.Recv.List[0].Type)
					if methods[receiver] == nil {
						methods[receiver] = map[string]bool{}
					}
					methods[receiver][decl.Name.Name] = true

				case *ast.GenDecl:
					for _, spec := range decl.Specs {
						typeSpec, ok := spec.(*ast.TypeSpec)
						if !ok {
							continue
						}
						structType, ok := typeSpec.Type.(*ast.StructType)
						if !ok {
							continue
						}
						for _, field := range structType.Fields.List {
							if len(field.Names) == 0 {
								name := pkg + "." + typeSpec.Name.Name
								embedded[name] = append(embedded[name], qualify(field.Type))
							}
						}
					}
				}
			}
		}
	}

	// hasFrom is whether a type's method set has UnmarshalJSONFrom, its own or promoted
	var hasFrom func(name typeName, seen map[typeName]bool) bool
	hasFrom = func(name typeName, seen map[typeName]bool) bool {
		if methods[name]["UnmarshalJSONFrom"] {
			return true
		}
		if seen[name] {
			return false
		}
		seen[name] = true
		for _, inner := range embedded[name] {
			if hasFrom(inner, seen) {
				return true
			}
		}
		return false
	}

	for name, own := range methods {
		if !own["UnmarshalJSON"] || own["UnmarshalJSONFrom"] {
			continue
		}
		for _, inner := range embedded[name] {
			if hasFrom(inner, map[typeName]bool{}) {
				t.Errorf("%s has an UnmarshalJSON but not an UnmarshalJSONFrom, which %s's would stand in for", name, inner)
			}
		}
	}

	if len(methods) == 0 || len(embedded) == 0 {
		t.Fatal("nothing parsed")
	}
}
