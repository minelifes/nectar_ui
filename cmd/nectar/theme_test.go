package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/minelifes/nectar_ui/cmd/nectar/themeeditor"
)

func TestThemeDefaultsInMaterialApp(t *testing.T) {
	t.Chdir(t.TempDir())
	os.MkdirAll(filepath.Join("internal", "theme"), 0o755)
	os.WriteFile(filepath.Join("internal", "theme", "theme.go"), []byte("package apptheme\n"), 0o644)
	o := ThemeOptions{}
	vm, err := prepareTheme(&o)
	if err != nil {
		t.Fatal(err)
	}
	if o.Dir != filepath.Join("internal", "theme") || o.Package != "apptheme" {
		t.Fatalf("defaults: %+v", o)
	}
	if g := vm.Options().Gen; g.FuncName(false) != "LightTheme" || g.FuncName(true) != "DarkTheme" {
		t.Fatalf("func names: %s %s", g.FuncName(false), g.FuncName(true))
	}
	if vm.Dirty() || vm.EditsIn(false)+vm.EditsIn(true) != 0 {
		t.Fatal("new designs should be empty and saved-state clean")
	}
}

func TestThemeDefaultsElsewhere(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-brand")
	os.MkdirAll(dir, 0o755)
	t.Chdir(dir)
	o := ThemeOptions{Prefix: "Brand"}
	vm, err := prepareTheme(&o)
	if err != nil {
		t.Fatal(err)
	}
	if o.Dir != "." || o.Package != "mybrand" || vm.Options().Gen.FuncName(true) != "BrandDarkTheme" {
		t.Fatalf("defaults: %+v", o)
	}
}

func TestThemeReopensAndProtectsFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	p := themeeditor.NewProject()
	p.Set("Card.Radius", false, float32(5))
	p.Set("Card.Radius", true, float32(8))
	if err := p.Save("brand", themeeditor.GenOptions{Package: "brand"}); err != nil {
		t.Fatal(err)
	}
	o := ThemeOptions{Dir: "brand"}
	vm, err := prepareTheme(&o)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := vm.Edit("Card.Radius"); !ok || v != float32(5) {
		t.Fatalf("light design not reopened: %v %v", v, ok)
	}
	vm.Dark.Set(true)
	if v, ok := vm.Edit("Card.Radius"); !ok || v != float32(8) {
		t.Fatalf("dark design not reopened: %v %v", v, ok)
	}
	if o.Package != "brand" {
		t.Fatalf("package = %q, want the files' own", o.Package)
	}
	// -new starts over.
	o = ThemeOptions{Dir: "brand", New: true}
	if vm, err = prepareTheme(&o); err != nil || vm.EditsIn(false)+vm.EditsIn(true) != 0 {
		t.Fatalf("-new: %v", err)
	}
	// A hand-written theme_light.go is never loaded (and so never overwritten).
	os.MkdirAll("mine", 0o755)
	os.WriteFile(filepath.Join("mine", themeeditor.LightFile), []byte("package mine\n\nfunc Mine() {}\n"), 0o644)
	if _, err := prepareTheme(&ThemeOptions{Dir: "mine"}); err == nil || !strings.Contains(err.Error(), "not written by nectar theme") {
		t.Fatalf("hand-written file: %v", err)
	}
	os.WriteFile("file.go", []byte("package x\n"), 0o644)
	for _, bad := range []ThemeOptions{{Dir: "file.go"}, {Dir: "x", Package: "1pkg"}, {Dir: "x", Package: "func"}, {Dir: "x", Prefix: "no-dash"}} {
		if _, err := prepareTheme(&bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}
