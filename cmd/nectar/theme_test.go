package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/minelifes/nectar_ui/cmd/nectar/themeeditor"
)

func TestThemeDefaultsInMaterialApp(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	os.MkdirAll(filepath.Join("internal", "theme"), 0o755)
	os.WriteFile(filepath.Join("internal", "theme", "theme.go"), []byte("package apptheme\n"), 0o644)
	o := ThemeOptions{}
	vm, err := prepareTheme(&o)
	if err != nil {
		t.Fatal(err)
	}
	if o.Out != filepath.Join("internal", "theme", "theme_gen.go") || o.Package != "apptheme" || o.Func != "Generated" {
		t.Fatalf("defaults: %+v", o)
	}
	if vm.Dirty() || vm.Project().Count() != 0 {
		t.Fatal("new design should be empty and saved-state clean")
	}
}

func TestThemeDefaultsElsewhere(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-brand")
	os.MkdirAll(dir, 0o755)
	t.Chdir(dir)
	o := ThemeOptions{}
	if _, err := prepareTheme(&o); err != nil {
		t.Fatal(err)
	}
	if o.Out != "theme_gen.go" || o.Package != "mybrand" {
		t.Fatalf("defaults: %+v", o)
	}
}

func TestThemeReopensAndProtectsFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	p := themeeditor.NewProject()
	p.Set("Card.Radius", false, float32(5))
	if err := p.Save("brand.go", themeeditor.GenOptions{Package: "brand"}); err != nil {
		t.Fatal(err)
	}
	o := ThemeOptions{Out: "brand.go"}
	vm, err := prepareTheme(&o)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := vm.Edit("Card.Radius"); !ok || v != float32(5) {
		t.Fatalf("existing design not reopened: %v %v", v, ok)
	}
	if o.Package != "brand" {
		t.Fatalf("package = %q, want the file's own", o.Package)
	}
	// -new starts over.
	o = ThemeOptions{Out: "brand.go", New: true}
	if vm, err = prepareTheme(&o); err != nil || vm.Project().Count() != 0 {
		t.Fatalf("-new: %v %v", err, vm.Project().Count())
	}
	// A hand-written file is never loaded (and so never overwritten).
	os.WriteFile("mine.go", []byte("package brand\n\nfunc Mine() {}\n"), 0o644)
	if _, err := prepareTheme(&ThemeOptions{Out: "mine.go"}); err == nil || !strings.Contains(err.Error(), "not written by nectar theme") {
		t.Fatalf("hand-written file: %v", err)
	}
	for _, bad := range []ThemeOptions{{Out: "x.txt"}, {Out: "x.go", Func: "lower"}, {Out: "x.go", Package: "1pkg"}} {
		if _, err := prepareTheme(&bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}
