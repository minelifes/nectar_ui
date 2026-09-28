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
	{"material", "Material 3 app: theme, dark mode, app bar, FAB counter page and a test (default)"},
	{"basic", "plain widgets, no Material: a minimal counter to build your own look from"},
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
	d := data{Name: name, Module: o.Module, Title: o.Title, Engine: EnginePath}

	fmt.Fprintf(o.Out, "Creating %s (%s template) in %s\n", o.Module, o.Template, dir)
	for _, src := range []string{"templates/common", "templates/" + o.Template} {
		if err := render(src, dir, d, o.Out); err != nil {
			return err
		}
	}
	if err := writeGoMod(dir, o); err != nil {
		return err
	}
	fmt.Fprintln(o.Out, "  create go.mod")

	if !o.NoTidy {
		if _, err := exec.LookPath("go"); err != nil {
			return errors.New("go isn't on PATH; install Go " + GoVersion + "+ or rerun with -no-tidy")
		}
		if o.Local == "" {
			if err := run(o.Out, dir, "go", "get", EnginePath+"@"+o.Version); err != nil {
				return fmt.Errorf("fetching the engine failed (is %s pushed to GitHub?): %w", EnginePath, err)
			}
		}
		if err := run(o.Out, dir, "go", "mod", "tidy"); err != nil {
			return err
		}
	}
	if o.Git {
		if err := run(o.Out, dir, "git", "init", "-q"); err != nil {
			return err
		}
	}
	rel := o.Dir
	fmt.Fprintf(o.Out, "\nDone. Next:\n\n  cd %s\n  CGO_ENABLED=0 go run .\n  go test ./...\n\n", rel)
	if o.NoTidy {
		fmt.Fprintln(o.Out, "Run `go mod tidy` first to fetch the engine (skipped with -no-tidy).")
	}
	return nil
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
func render(src, dir string, d data, out io.Writer) error {
	return fs.WalkDir(templateFS, src, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(p, src+"/")
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
		fmt.Fprintln(out, "  create", rel)
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

func run(out io.Writer, dir, name string, args ...string) error {
	fmt.Fprintf(out, "  run   %s %s\n", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
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
