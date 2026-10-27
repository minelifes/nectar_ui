package material

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"
)

// Every theme / style type has a CopyWith: adding a theme without running
// `go generate ./ui/material` fails here.
func TestEveryThemeTypeHasCopyWith(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var types, methods []string
	for _, p := range pkgs {
		for _, f := range p.Files {
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.GenDecl:
					for _, s := range d.Specs {
						ts, ok := s.(*ast.TypeSpec)
						if !ok || !ts.Name.IsExported() {
							continue
						}
						if _, ok := ts.Type.(*ast.StructType); !ok {
							continue
						}
						if n := ts.Name.Name; strings.HasSuffix(n, "Theme") || strings.HasSuffix(n, "Style") || n == "ColorScheme" {
							types = append(types, n)
						}
					}
				case *ast.FuncDecl:
					if d.Name.Name == "CopyWith" && d.Recv != nil {
						if id, ok := d.Recv.List[0].Type.(*ast.Ident); ok {
							methods = append(methods, id.Name)
						}
					}
				}
			}
		}
	}
	slices.Sort(types)
	slices.Sort(methods)
	if len(types) < 40 {
		t.Fatalf("found only %d theme types: %v", len(types), types)
	}
	if !slices.Equal(types, methods) {
		t.Fatalf("theme types and CopyWith methods differ (run go generate ./ui/material):\n types   %v\n methods %v", types, methods)
	}
}
