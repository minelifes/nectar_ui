package themeeditor

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
)

// sample returns a test value for a field kind; n varies it per field.
func sample(k Kind, n int) any {
	switch k {
	case KindColor:
		return geom.RGBA8(uint8(n*37), uint8(n*11), uint8(255-n), uint8(128+n%128))
	case KindFloat:
		return float32(n%40) + 0.5
	case KindInt:
		return n % 6
	case KindBool:
		return n%2 == 0
	case KindInsets:
		return geom.InsetsLTRB(1, float32(n%7), 3, 4)
	case KindDuration:
		return time.Duration(n) * time.Millisecond
	case KindFont:
		return []string{FontRegular, FontMedium, FontBold}[n%3]
	}
	panic("kind")
}

// fullProject edits every field the editor offers, with different values
// in the light and the dark design.
func fullProject(t *testing.T) *Project {
	p := NewProject()
	p.SetSeed(false, geom.Hex(0x0B57D0))
	p.SetSeed(true, geom.Hex(0x8B5000))
	n := 0
	for _, sec := range Sections() {
		for _, f := range sec.Fields {
			n++
			if k, err := KindOf(f.Path); err != nil || k != f.Kind {
				t.Fatalf("%s: KindOf = %v, %v; section says %v", f.Path, k, err, f.Kind)
			}
			must(t, p.Set(f.Path, false, sample(f.Kind, n)))
			must(t, p.Set(f.Path, true, sample(f.Kind, n+1)))
		}
	}
	must(t, p.Set("Card.Color", false, m.Transparent))
	if n < 300 {
		t.Fatalf("only %d editable fields", n)
	}
	return p
}

// setterLine matches field assignments like "\tt.Card.Radius = ...".
var setterLine = regexp.MustCompile(`(?m)^\s*t\.[A-Z]\w*(\.\w+)* = `)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestSectionsCoverTheTheme(t *testing.T) {
	names := map[string]bool{}
	for _, s := range Sections() {
		names[s.Name] = true
		if len(s.Fields) == 0 {
			t.Errorf("section %s has no fields", s.Name)
		}
	}
	for _, want := range []string{"General", "Scheme", "Text.BodyLarge", "FilledButton", "Card", "Input", "TreeView", "Carousel"} {
		if !names[want] {
			t.Errorf("no section %s", want)
		}
	}
}

func TestApplyAndValue(t *testing.T) {
	p := NewProject()
	p.Set("Card.Radius", false, float32(3))
	p.Set("Card.Elevation", false, 0)
	p.Set("Input.Outlined", false, true)
	p.Set("Text.BodyLarge.Size", false, float32(18))
	p.Set("Text.BodyLarge.Font", false, FontBold)
	p.Set("Scheme.Primary", false, geom.Hex(0xFF0000))
	p.Set("Scheme.Primary", true, geom.Hex(0x00FF00))
	p.Set("Carousel.ItemColors.1", false, geom.Hex(0x123456))
	light, dark := p.Theme(false), p.Theme(true)
	if *light.Card.Radius != 3 || *light.Card.Elevation != 0 || !*light.Input.Outlined || light.Text.BodyLarge.Size != 18 {
		t.Fatalf("edits not applied: %+v", light.Card)
	}
	if light.Scheme.Primary != geom.Hex(0xFF0000) || dark.Scheme.Primary != geom.Hex(0x00FF00) || !dark.Scheme.Dark {
		t.Fatal("scheme edits must be per mode")
	}
	// The dark design is independent: none of the light edits reach it.
	if dark.Card.Radius != nil || dark.Input.Outlined != nil || dark.Text.BodyLarge.Size == 18 {
		t.Fatal("light edits leaked into the dark design")
	}
	if !reflect.DeepEqual(p.Theme(true).Card, m.NewTheme(m.BaselineSeed, true).Card) {
		t.Fatal("dark card theme should be untouched")
	}
	p.SetSeed(true, geom.Hex(0x386A20))
	if p.Seed(false) != m.BaselineSeed || p.Theme(true).Seed != geom.Hex(0x386A20) {
		t.Fatal("seeds must be per mode")
	}
	if light.Carousel.ItemColors[1] != geom.Hex(0x123456) {
		t.Fatal("array item edit not applied")
	}
	if v, ok := Value(light, "Text.BodyLarge.Font"); !ok || v != FontBold {
		t.Fatalf("Value(font) = %v %v", v, ok)
	}
	if _, ok := Value(light, "Card.Margin"); ok {
		t.Fatal("unset pointer should report !ok")
	}
	if v, _ := Value(m.NewTheme(m.BaselineSeed, false), "Text.BodyLarge.Font"); v != FontRegular {
		t.Fatalf("default body font = %v", v)
	}
	p.Unset("Card.Radius", false)
	if p.Theme(false).Card.Radius != nil {
		t.Fatal("Unset didn't restore the default")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	p := fullProject(t)
	q := NewProject()
	for _, dark := range []bool{false, true} {
		data, err := json.Marshal(*p.Mode(dark))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, q.Mode(dark)); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(p.Theme(dark), q.Theme(dark)) {
			t.Fatalf("round trip changed the theme (dark=%v)", dark)
		}
	}
	var d Design
	if err := json.Unmarshal([]byte(`{"seed":"#6750A4","edits":{"Card.Nope":1}}`), &d); err == nil {
		t.Fatal("unknown path accepted")
	}
	if err := json.Unmarshal([]byte(`{"seed":"#6750A4","edits":{"Card.Radius":"big"}}`), &d); err == nil {
		t.Fatal("wrong value type accepted")
	}
	// The API checks types too, so generated code always compiles.
	r := NewProject()
	for path, v := range map[string]any{
		"Tooltip.WaitDuration": 700, "Card.Radius": 6, "Card.Elevation": float32(1),
		"Card.Color": "#fff", "Text.BodyLarge.Font": "comic", "Card.Nope": 1,
	} {
		if err := r.Set(path, false, v); err == nil {
			t.Errorf("Set(%s, %T) accepted", path, v)
		}
	}
	if r.Count(false)+r.Count(true) != 0 {
		t.Fatal("rejected values were stored")
	}
}

func TestParseColor(t *testing.T) {
	for in, want := range map[string]geom.Color{
		"#FF0000": geom.Hex(0xFF0000), "00ff0080": geom.HexA(0x00FF0080), "#abc": geom.Hex(0xAABBCC),
	} {
		if got, err := ParseColor(in); err != nil || got != want {
			t.Errorf("ParseColor(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseColor("#12"); err == nil {
		t.Error("short color accepted")
	}
	if ColorHex(geom.HexA(0x11223344)) != "#11223344" {
		t.Error("ColorHex")
	}
}

// TestGeneratedCodeMatchesPreview writes the code for a design that edits
// every field, compiles it with a program comparing it to Project.Theme,
// and runs it: the generated theme must equal what the editor shows.
func TestGeneratedCodeMatchesPreview(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a program")
	}
	for name, p := range map[string]*Project{"full": fullProject(t), "empty": NewProject()} {
		t.Run(name, func(t *testing.T) {
			if err := os.MkdirAll("testdata", 0o755); err != nil {
				t.Fatal(err)
			}
			dir, err := os.MkdirTemp("testdata", "gen-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove("testdata")
			defer os.RemoveAll(dir)
			pkg := filepath.Join(dir, "theme")
			if err := p.Save(pkg, GenOptions{Package: "theme", Prefix: "Brand"}); err != nil {
				t.Fatal(err)
			}
			// Two files, each one struct literal: no field-by-field setters.
			for _, dark := range []bool{false, true} {
				src, err := os.ReadFile(filepath.Join(pkg, FileName(dark)))
				if err != nil {
					t.Fatal(err)
				}
				code := string(src)
				fn := GenOptions{Prefix: "Brand"}.FuncName(dark)
				if !strings.Contains(code, "func "+fn+"() m.Theme {\n\treturn m.Theme{\n") || setterLine.MatchString(code) {
					t.Fatalf("%s isn't a single literal:\n%.600s", FileName(dark), code)
				}
			}
			// Reopening the folder gives the same designs.
			q, found, err := Load(pkg)
			if err != nil || !found {
				t.Fatal(err, found)
			}
			if !reflect.DeepEqual(p.Theme(false), q.Theme(false)) || !reflect.DeepEqual(p.Theme(true), q.Theme(true)) {
				t.Fatal("Load(Save(p)) differs from p")
			}
			abs, _ := filepath.Abs(pkg)
			mainSrc := `package main

import (
	"fmt"
	"os"
	"reflect"

	"github.com/minelifes/nectar_ui/cmd/nectar/themeeditor"
	"github.com/minelifes/nectar_ui/cmd/nectar/themeeditor/` + filepath.ToSlash(filepath.Join(filepath.Base("testdata"), filepath.Base(dir), "theme")) + `"
)

func main() {
	p, _, err := themeeditor.Load(` + "`" + abs + "`" + `)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if !reflect.DeepEqual(theme.BrandLightTheme(), p.Theme(false)) {
		fmt.Println("MISMATCH light")
		os.Exit(1)
	}
	if !reflect.DeepEqual(theme.BrandDarkTheme(), p.Theme(true)) {
		fmt.Println("MISMATCH dark")
		os.Exit(1)
	}
	fmt.Println("OK")
}
`
			os.MkdirAll(filepath.Join(dir, "cmd"), 0o755)
			if err := os.WriteFile(filepath.Join(dir, "cmd", "main.go"), []byte(mainSrc), 0o644); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command("go", "run", "./"+filepath.ToSlash(filepath.Join(dir, "cmd"))).CombinedOutput()
			if err != nil || strings.TrimSpace(string(out)) != "OK" {
				src, _ := os.ReadFile(filepath.Join(pkg, LightFile))
				t.Fatalf("generated code: %v\n%s\n--- code ---\n%s", err, out, src)
			}
		})
	}
}
