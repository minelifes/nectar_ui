package main

import (
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
	Dir     string // folder of theme_light.go / theme_dark.go; default: see defaultThemeDir
	Package string // default: the package already in Dir, else its name
	Prefix  string // func names: <Prefix>LightTheme, <Prefix>DarkTheme
	New     bool   // start new designs even if the files exist
}

func cmdTheme(args []string) error {
	var o ThemeOptions
	fs := flag.NewFlagSet("theme", flag.ContinueOnError)
	fs.StringVar(&o.Package, "pkg", "", "package of the generated files (default: the package already in the folder, else the folder name)")
	fs.StringVar(&o.Prefix, "prefix", "", "prefix of the generated funcs: <prefix>LightTheme and <prefix>DarkTheme")
	fs.BoolVar(&o.New, "new", false, "start new designs even if the files exist (they're overwritten on save)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: nectar theme [flags] [folder]

Opens the theme editor: design a light and a dark Material theme, each
with its own seed color, color scheme, type scale and component styles,
with a live preview. Save writes %s and %s into the folder
(default internal/theme when it exists, else the current folder), each a
func returning one material.Theme literal; running nectar theme on the
folder again reopens both designs.

`, themeeditor.LightFile, themeeditor.DarkFile)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		o.Dir = fs.Arg(0)
	}
	vm, err := prepareTheme(&o)
	if err != nil {
		return err
	}
	gen := vm.Options().Gen
	fmt.Printf("Editing %s and %s in %s (package %s). Save with the Save button or Ctrl/⌘+S.\n",
		themeeditor.LightFile, themeeditor.DarkFile, o.Dir, o.Package)
	if err := themeeditor.Run(vm); err != nil {
		return err
	}
	if strings.HasPrefix(vm.Status.Get(), "Saved ") && !vm.Dirty() {
		fmt.Printf("\nSaved. Use the themes in your app:\n\n\tmaterial.App{Theme: %s.%s(), DarkTheme: widgets.Ptr(%s.%s()), Dark: dark}\n",
			o.Package, gen.FuncName(false), o.Package, gen.FuncName(true))
	} else if vm.Dirty() {
		fmt.Println("\nClosed with unsaved changes.")
	}
	return nil
}

// prepareTheme fills in defaults and opens (or starts) the designs.
func prepareTheme(o *ThemeOptions) (*themeeditor.VM, error) {
	if o.Dir == "" {
		o.Dir = defaultThemeDir()
	}
	if st, err := os.Stat(o.Dir); err == nil && !st.IsDir() {
		return nil, fmt.Errorf("theme: %s is not a folder (the editor writes %s and %s into one)", o.Dir, themeeditor.LightFile, themeeditor.DarkFile)
	}
	if o.Package == "" {
		o.Package = packageFor(o.Dir)
	}
	if !token.IsIdentifier(o.Package) || token.IsKeyword(o.Package) || (o.Prefix != "" && !token.IsIdentifier(o.Prefix+"X")) {
		return nil, fmt.Errorf("theme: package %q / prefix %q: need Go identifiers", o.Package, o.Prefix)
	}
	project := themeeditor.NewProject()
	if !o.New {
		p, _, err := themeeditor.Load(o.Dir)
		if err != nil {
			return nil, fmt.Errorf("theme: %w (use -new to replace it)", err)
		}
		project = p
	}
	return themeeditor.NewVM(project, themeeditor.Options{Dir: o.Dir,
		Gen: themeeditor.GenOptions{Package: o.Package, Prefix: o.Prefix}}), nil
}

// defaultThemeDir is internal/theme in apps made by `nectar new -template
// material`, else the current folder.
func defaultThemeDir() string {
	if st, err := os.Stat(filepath.Join("internal", "theme")); err == nil && st.IsDir() {
		return filepath.Join("internal", "theme")
	}
	return "."
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
