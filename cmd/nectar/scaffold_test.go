package main

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateAllTemplates(t *testing.T) {
	for _, tpl := range Templates {
		t.Run(tpl.Name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "my-cool_app")
			err := Generate(Options{Dir: dir, Template: tpl.Name, Module: "example.com/me/cool", NoTidy: true})
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range []string{"go.mod", "main.go", "app.go", "app_test.go", "README.md", ".gitignore"} {
				if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
					t.Errorf("missing %s", f)
				}
			}
			mod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
			if !strings.HasPrefix(string(mod), "module example.com/me/cool\n") || !strings.Contains(string(mod), "go "+GoVersion) {
				t.Errorf("go.mod:\n%s", mod)
			}
			// Every Go file parses and imports the engine by its GitHub path.
			files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
			usesEngine := false
			for _, f := range files {
				af, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
				if err != nil {
					t.Fatalf("%s: %v", filepath.Base(f), err)
				}
				for _, im := range af.Imports {
					if strings.HasPrefix(strings.Trim(im.Path.Value, `"`), EnginePath+"/") {
						usesEngine = true
					}
				}
			}
			if !usesEngine {
				t.Error("no file imports the engine")
			}
			main, _ := os.ReadFile(filepath.Join(dir, "main.go"))
			if !strings.Contains(string(main), `"My Cool App"`) {
				t.Errorf("title not derived from the folder name:\n%s", main)
			}
		})
	}
}

func TestGenerateRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("x"), 0o644)
	if err := Generate(Options{Dir: dir, NoTidy: true}); err == nil {
		t.Fatal("wrote into a non-empty folder")
	}
	if err := Generate(Options{Dir: dir, NoTidy: true, Force: true}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "keep.txt")); string(b) != "x" {
		t.Fatal("-force removed an existing file")
	}
}

func TestGenerateBadInput(t *testing.T) {
	if err := Generate(Options{Dir: t.TempDir(), Template: "nope", NoTidy: true}); err == nil {
		t.Error("unknown template accepted")
	}
	if err := Generate(Options{Dir: t.TempDir(), Module: "bad module", NoTidy: true}); err == nil {
		t.Error("bad module path accepted")
	}
}

func TestLocalReplace(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "app")
	engine, _ := filepath.Abs("../..")
	if err := Generate(Options{Dir: dir, Local: engine, NoTidy: true}); err != nil {
		t.Fatal(err)
	}
	mod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	if !strings.Contains(string(mod), "replace "+EnginePath+" => "+filepath.ToSlash(engine)) {
		t.Fatalf("go.mod:\n%s", mod)
	}
}

// TestGeneratedAppsBuild creates every template against this checkout and
// runs its tests. It needs the engine's dependencies (network or module
// cache), so it only runs with NECTAR_BUILD_TEST=1.
func TestGeneratedAppsBuild(t *testing.T) {
	if os.Getenv("NECTAR_BUILD_TEST") == "" {
		t.Skip("set NECTAR_BUILD_TEST=1 to build the generated apps")
	}
	engine, _ := filepath.Abs("../..")
	for _, tpl := range Templates {
		t.Run(tpl.Name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), tpl.Name+"app")
			var out strings.Builder
			if err := Generate(Options{Dir: dir, Template: tpl.Name, Local: engine, Out: &out}); err != nil {
				t.Fatalf("%v\n%s", err, out.String())
			}
			cmd := exec.Command("go", "test", "./...")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("go test: %v\n%s", err, b)
			}
		})
	}
}
