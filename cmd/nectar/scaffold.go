package main

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"go/format"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"unicode"
)

// EnginePath is the engine's module path on GitHub.
const EnginePath = "github.com/minelifes/nectar_ui"

// GoVersion is the minimum Go version the engine needs.
const GoVersion = "1.26.0"

//go:embed all:templates
var templateFS embed.FS

// Template is one kind of app `nectar new` can create.
type Template struct {
	Name        string
	Description string
}

// Templates lists the app templates (folders under templates/, plus the
// shared files in templates/common).
var Templates = []Template{
	{"material", "structured Material 3 app: internal/{app,theme,state,components,pages}, light/dark theme, navigation rail, settings page, tests (default)"},
	{"basic", "core widgets only, no Material: a counter with a custom button, to build your own look from"},
	{"explorer", "Material file browser: split view with a file tree and a file preview"},
}

func templateNames() string {
	var names []string
	for _, t := range Templates {
		names = append(names, t.Name)
	}
	return strings.Join(names, ", ")
}

// Options configure Generate.
type Options struct {
	Dir      string // folder to create
	Module   string // module path; default: folder name
	Template string // default: material
	Title    string // window title; default: from folder name
	Version  string // engine version for go get; default: latest
	Local    string // engine checkout to use via a replace directive
	Seed     string // brand color "#RRGGBB" for the theme; default M3 baseline purple
	NoTidy   bool   // skip go get / go mod tidy
	Git      bool   // git init
	Force    bool   // allow a non-empty folder
	Out      io.Writer
}

// data is what templates see.
type data struct {
	Name   string // folder name
	Module string
	Title  string
	Engine string // engine import path
	Seed   string // brand color as a Go literal, e.g. 0x6750A4
	Exe    string // binary name
	ID     string // reverse-DNS bundle id
}

var modulePathRE = regexp.MustCompile(`^[A-Za-z0-9._~\-]+(/[A-Za-z0-9._~\-]+)*$`)

// Generate creates an app from a template.
func Generate(o Options) error {
	if o.Out == nil {
		o.Out = io.Discard
	}
	if o.Template == "" {
		o.Template = "material"
	}
	if o.Version == "" {
		o.Version = "latest"
	}
	if !knownTemplate(o.Template) {
		return fmt.Errorf("unknown template %q (have: %s)", o.Template, templateNames())
	}
	dir, err := filepath.Abs(o.Dir)
	if err != nil {
		return err
	}
	name := filepath.Base(dir)
	if o.Module == "" {
		o.Module = name
	}
	if !modulePathRE.MatchString(o.Module) {
		return fmt.Errorf("invalid module path %q", o.Module)
	}
	if o.Title == "" {
		o.Title = titleFrom(name)
	}
	if err := checkDir(dir, o.Force); err != nil {
		return err
	}
	seed, err := parseSeed(o.Seed)
	if err != nil {
		return err
	}
	d := data{Name: name, Module: o.Module, Title: o.Title, Engine: EnginePath, Seed: seed,
		Exe: exeName(name), ID: bundleID(o.Module)}

	p := newPrinter(o.Out)
	p.header("new", p.bold(o.Module)+" "+p.dim("("+o.Template+" template) in "+dir))
	// Template files win over common ones with the same name.
	own := map[string]bool{}
	fs.WalkDir(templateFS, "templates/"+o.Template, func(p string, e fs.DirEntry, err error) error {
		if err == nil && !e.IsDir() {
			own[strings.TrimPrefix(p, "templates/"+o.Template+"/")] = true
		}
		return nil
	})
	if err := render("templates/common", dir, d, p, own); err != nil {
		return err
	}
	if err := render("templates/"+o.Template, dir, d, p, nil); err != nil {
		return err
	}
	var seedRGB uint32
	fmt.Sscanf(seed, "0x%X", &seedRGB)
	if err := os.MkdirAll(filepath.Join(dir, "assets", "images"), 0o755); err != nil {
		return err
	}
	if err := writeIconPNG(filepath.Join(dir, "assets", "icon.png"), defaultIcon(seedRGB, o.Title)); err != nil {
		return err
	}
	p.step("create", "assets/icon.png")
	if err := writeIconPNG(filepath.Join(dir, "assets", "images", "welcome.png"), welcomeImage(seedRGB, 1200, 480)); err != nil {
		return err
	}
	p.step("create", "assets/images/welcome.png")
	if err := writeGoMod(dir, o); err != nil {
		return err
	}
	p.step("create", "go.mod")

	if !o.NoTidy {
		if _, err := exec.LookPath("go"); err != nil {
			return errors.New("go isn't on PATH; install Go " + GoVersion + "+ or rerun with -no-tidy")
		}
		if o.Local == "" {
			if err := run(p, dir, "go", "get", EnginePath+"@"+o.Version); err != nil {
				return fmt.Errorf("fetching the engine failed (is %s pushed to GitHub?): %w", EnginePath, err)
			}
		}
		if err := run(p, dir, "go", "mod", "tidy"); err != nil {
			return err
		}
	}
	if o.Git {
		if err := run(p, dir, "git", "init", "-q"); err != nil {
			return err
		}
	}
	p.printf("\n")
	p.success("Created %s", o.Title)
	next := [][2]string{
		{"cd " + o.Dir, "go to the app"},
		{"nectar dev", "run it; restarts on save, state kept"},
		{"go test ./...", "test"},
		{"nectar build", "package as a desktop app with its icon"},
	}
	if o.NoTidy {
		next = append([][2]string{next[0], {"go mod tidy", "fetch the engine (skipped with -no-tidy)"}}, next[1:]...)
	}
	p.commands("Next", next)
	return nil
}

var hexColorRE = regexp.MustCompile(`^[0-9A-Fa-f]{6}$`)

// parseSeed turns "#0B57D0", "0x0B57D0" or "0B57D0" into "0x0B57D0".
func parseSeed(s string) (string, error) {
	if s == "" {
		return "0x6750A4", nil
	}
	h := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(s, "#"), "0x"), "0X")
	if !hexColorRE.MatchString(h) {
		return "", fmt.Errorf("invalid -seed %q: want a hex color like #0B57D0", s)
	}
	return "0x" + strings.ToUpper(h), nil
}

func knownTemplate(n string) bool {
	for _, t := range Templates {
		if t.Name == n {
			return true
		}
	}
	return false
}

// checkDir makes sure dir is missing or empty (unless force).
func checkDir(dir string, force bool) error {
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return os.MkdirAll(dir, 0o755)
	case err != nil:
		return err
	case len(entries) > 0 && !force:
		return fmt.Errorf("%s isn't empty (use -force to write into it anyway)", dir)
	}
	return nil
}

// render executes every *.tmpl under src into dir. A leading "_" in a
// file name becomes "." (embed skips dotfiles), and .go files are gofmt'ed.
func render(src, dir string, d data, pr *printer, skip map[string]bool) error {
	return fs.WalkDir(templateFS, src, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(p, src+"/")
		if skip[rel] {
			return nil
		}
		rel = strings.TrimSuffix(rel, ".tmpl")
		if base := path.Base(rel); strings.HasPrefix(base, "_") {
			rel = path.Join(path.Dir(rel), "."+base[1:])
		}
		raw, err := templateFS.ReadFile(p)
		if err != nil {
			return err
		}
		t, err := template.New(p).Delims("[[", "]]").Parse(string(raw))
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, d); err != nil {
			return err
		}
		content := buf.Bytes()
		if strings.HasSuffix(rel, ".go") {
			if content, err = format.Source(content); err != nil {
				return fmt.Errorf("template %s: %w", p, err)
			}
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return err
		}
		pr.step("create", rel)
		return nil
	})
}

func writeGoMod(dir string, o Options) error {
	var b strings.Builder
	fmt.Fprintf(&b, "module %s\n\ngo %s\n", o.Module, GoVersion)
	if o.Local != "" {
		local, err := filepath.Abs(o.Local)
		if err != nil {
			return err
		}
		if _, err := os.Stat(filepath.Join(local, "go.mod")); err != nil {
			return fmt.Errorf("-local %s: no go.mod there", o.Local)
		}
		// The zero pseudo-version is what go uses for a replaced module
		// that has no real version.
		fmt.Fprintf(&b, "\nrequire %s v0.0.0-00010101000000-000000000000\n\nreplace %s => %s\n",
			EnginePath, EnginePath, filepath.ToSlash(local))
	}
	return os.WriteFile(filepath.Join(dir, "go.mod"), []byte(b.String()), 0o644)
}

func run(p *printer, dir, name string, args ...string) error {
	p.step("run", name+" "+strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	// The tool's own output (go: downloading …), indented under the step.
	out := &gutter{w: p.w, prefix: "          ", style: p.dim}
	defer out.Flush()
	cmd.Stdout, cmd.Stderr = out, out
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	return cmd.Run()
}

// titleFrom turns "my-cool_app" into "My Cool App".
func titleFrom(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' || r == '.' || r == ' ' })
	for i, w := range words {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	if len(words) == 0 {
		return name
	}
	return strings.Join(words, " ")
}
