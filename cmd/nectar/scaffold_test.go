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

// Files each template must produce (on top of go.mod, README.md, .gitignore).
var wantFiles = map[string][]string{
	"material": {"main.go", "internal/app/app.go", "internal/app/shell.go", "internal/app/routes.go",
		"internal/theme/theme.go", "internal/theme/tokens.go", "internal/state/settings.go", "internal/components/page.go",
		"internal/pages/home/home.go", "internal/pages/settings/settings.go", "tests/app_test.go"},
	"basic":    {"main.go", "internal/app/app.go", "internal/app/button.go", "tests/app_test.go"},
	"explorer": {"main.go", "internal/app/app.go", "internal/app/explorer.go", "tests/app_test.go"},
}

func TestGenerateAllTemplates(t *testing.T) {
	for _, tpl := range Templates {
		t.Run(tpl.Name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "my-cool_app")
			err := Generate(Options{Dir: dir, Template: tpl.Name, Module: "example.com/me/cool", NoTidy: true, Seed: "#0b57d0"})
			if err != nil {
				t.Fatal(err)
			}
			if len(wantFiles[tpl.Name]) == 0 {
				t.Fatalf("no wantFiles for %s", tpl.Name)
			}
			for _, f := range append([]string{"go.mod", "README.md", ".gitignore", "nectar.json", "assets/icon.png", "assets/images/welcome.png", "assets/assets.go"}, wantFiles[tpl.Name]...) {
				if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
					t.Errorf("missing %s", f)
				}
			}
			mod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
			if !strings.HasPrefix(string(mod), "module example.com/me/cool\n") || !strings.Contains(string(mod), "go "+GoVersion) {
				t.Errorf("go.mod:\n%s", mod)
			}
			// Every Go file parses; imports are the engine's GitHub path or
			// the app's own module.
			usesEngine, all := false, ""
			filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
					return err
				}
				src, _ := os.ReadFile(p)
				all += string(src)
				af, err := parser.ParseFile(token.NewFileSet(), p, src, parser.ImportsOnly)
				if err != nil {
					t.Fatalf("%s: %v", p, err)
				}
				for _, im := range af.Imports {
					path := strings.Trim(im.Path.Value, `"`)
					if strings.HasPrefix(path, EnginePath+"/") {
						usesEngine = true
					}
					thirdParty := strings.Contains(strings.Split(path, "/")[0], ".") // stdlib has no dot
					if thirdParty && !strings.HasPrefix(path, EnginePath) && !strings.HasPrefix(path, "example.com/me/cool/") {
						t.Errorf("%s imports %s", p, path)
					}
				}
				return nil
			})
			if !usesEngine {
				t.Error("no file imports the engine")
			}
			tests, _ := filepath.Glob(filepath.Join(dir, "*", "*_test.go"))
			for _, f := range tests {
				if filepath.Base(filepath.Dir(f)) != "tests" {
					t.Errorf("test outside tests/: %s", f)
				}
			}
			if !strings.Contains(all, `"My Cool App"`) {
				t.Error("title not derived from the folder name")
			}
			m, err := readManifest(dir)
			if err != nil || m.ID != "com.example.me.cool" || m.Executable != "my-cool-app" || m.Name != "My Cool App" {
				t.Errorf("nectar.json: %+v %v", m, err)
			}
			if _, err := loadIcon(filepath.Join(dir, m.Icon)); err != nil {
				t.Error(err)
			}
			if tpl.Name == "material" && !strings.Contains(all, "geom.Hex(0x0B57D0)") {
				t.Error("-seed not applied to the theme")
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

func TestParseSeed(t *testing.T) {
	for in, want := range map[string]string{"": "0x6750A4", "#0b57d0": "0x0B57D0", "0x006A6A": "0x006A6A", "386a20": "0x386A20"} {
		if got, err := parseSeed(in); err != nil || got != want {
			t.Errorf("parseSeed(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"#fff", "blue", "#12345G"} {
		if _, err := parseSeed(bad); err == nil {
			t.Errorf("parseSeed(%q) accepted", bad)
		}
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
