package main

import (
	"errors"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/minelifes/nectar_ui/cmd/nectar/themeeditor"
)

// ThemeOptions configure `nectar theme`.
type ThemeOptions struct {
	Out     string // generated file; default: see defaultThemeOut
	Package string // default: the package of Out's folder, else its name
	Func    string // default "Generated"
	New     bool   // start a new design even if Out exists
}

func cmdTheme(args []string) error {
	var o ThemeOptions
	fs := flag.NewFlagSet("theme", flag.ContinueOnError)
	fs.StringVar(&o.Out, "o", "", "Go file to write (default internal/theme/theme_gen.go when that folder exists, else theme_gen.go)")
	fs.StringVar(&o.Package, "pkg", "", "package of the generated file (default: the package already in that folder, else the folder name)")
	fs.StringVar(&o.Func, "func", "Generated", "name of the generated func(dark bool) material.Theme")
	fs.BoolVar(&o.New, "new", false, "start a new design even if the output file exists (it's overwritten on save)")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `Usage: nectar theme [flags] [file.go]

Opens the theme editor: pick a seed color, adjust the color scheme, the
type scale and any Material component, with a live preview. Save writes Go
code (a func returning material.Theme for light or dark mode); running
nectar theme on that file again reopens the design.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		o.Out = fs.Arg(0)
	}
	vm, err := prepareTheme(&o)
	if err != nil {
		return err
	}
	fmt.Printf("Editing %s (package %s, func %s). Save with the Save button or Ctrl/⌘+S.\n", o.Out, o.Package, o.Func)
	if err := themeeditor.Run(vm); err != nil {
		return err
	}
	if strings.HasPrefix(vm.Status.Get(), "Saved ") && !vm.Dirty() {
		fmt.Printf("\nSaved %s. Use it in your app:\n\n\tmaterial.App{Theme: %s.%s(false), DarkTheme: widgets.Ptr(%s.%s(true)), Dark: dark}\n",
			o.Out, o.Package, o.Func, o.Package, o.Func)
	} else if vm.Dirty() {
		fmt.Println("\nClosed with unsaved changes.")
	}
	return nil
}

// prepareTheme fills in defaults and opens (or starts) the design.
func prepareTheme(o *ThemeOptions) (*themeeditor.VM, error) {
	if o.Out == "" {
		o.Out = defaultThemeOut()
	}
	if !strings.HasSuffix(o.Out, ".go") {
		return nil, fmt.Errorf("theme: %s: the output must be a .go file", o.Out)
	}
	if o.Package == "" {
		o.Package = packageFor(filepath.Dir(o.Out))
	}
	if o.Func == "" {
		o.Func = "Generated"
	}
	if !token.IsIdentifier(o.Package) || !token.IsIdentifier(o.Func) || !unicode.IsUpper(rune(o.Func[0])) {
		return nil, fmt.Errorf("theme: package %q / func %q: need Go identifiers, the func exported", o.Package, o.Func)
	}
	var project *themeeditor.Project
	if _, err := os.Stat(o.Out); err == nil && !o.New {
		p, err := themeeditor.Load(o.Out)
		if err != nil {
			return nil, fmt.Errorf("theme: %w (use -new to replace it)", err)
		}
		project = p
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return themeeditor.NewVM(project, themeeditor.Options{Out: o.Out,
		Gen: themeeditor.GenOptions{Package: o.Package, Func: o.Func}}), nil
}

// defaultThemeOut is internal/theme/theme_gen.go in apps made by `nectar
// new -template material`, else theme_gen.go here.
func defaultThemeOut() string {
	if st, err := os.Stat(filepath.Join("internal", "theme")); err == nil && st.IsDir() {
		return filepath.Join("internal", "theme", "theme_gen.go")
	}
	return "theme_gen.go"
}

// packageFor returns the package the Go files in dir already use, else a
// name made from the folder's name ("theme" if that isn't usable).
func packageFor(dir string) string {
	if pkgs, err := parser.ParseDir(token.NewFileSet(), dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.PackageClauseOnly); err == nil {
		for name := range pkgs {
			return name
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "theme"
	}
	name := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			return unicode.ToLower(r)
		}
		return -1
	}, filepath.Base(abs))
	if !token.IsIdentifier(name) || token.IsKeyword(name) {
		return "theme"
	}
	return name
}
