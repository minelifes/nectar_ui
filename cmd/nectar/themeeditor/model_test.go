package themeeditor

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

// fullProject edits every field the editor offers.
func fullProject(t *testing.T) *Project {
	p := NewProject()
	p.Seed = geom.Hex(0x0B57D0)
	n := 0
	for _, sec := range Sections() {
		for _, f := range sec.Fields {
			n++
			if k, err := KindOf(f.Path); err != nil || k != f.Kind {
				t.Fatalf("%s: KindOf = %v, %v; section says %v", f.Path, k, err, f.Kind)
			}
			p.Set(f.Path, false, sample(f.Kind, n))
			if isScheme(f.Path) {
				p.Set(f.Path, true, sample(f.Kind, n+1))
			}
		}
	}
	p.Set("Card.Color", false, m.Transparent)
	if n < 300 {
		t.Fatalf("only %d editable fields", n)
	}
	return p
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
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	q := NewProject()
	if err := json.Unmarshal(data, q); err != nil {
		t.Fatal(err)
	}
	for _, dark := range []bool{false, true} {
		if !reflect.DeepEqual(p.Theme(dark), q.Theme(dark)) {
			t.Fatalf("round trip changed the theme (dark=%v)", dark)
		}
	}
	if err := json.Unmarshal([]byte(`{"seed":"#6750A4","edits":{"Card.Nope":1}}`), q); err == nil {
		t.Fatal("unknown path accepted")
	}
	if err := json.Unmarshal([]byte(`{"seed":"#6750A4","edits":{"Card.Radius":"big"}}`), q); err == nil {
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
	if r.Count() != 0 {
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
			os.MkdirAll(pkg, 0o755)
			file := filepath.Join(pkg, "theme_gen.go")
			if err := p.Save(file, GenOptions{Package: "theme", Func: "Brand"}); err != nil {
				t.Fatal(err)
			}
			// Reopening the file gives the same design.
			q, err := Load(file)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(p.Theme(false), q.Theme(false)) || !reflect.DeepEqual(p.Theme(true), q.Theme(true)) {
				t.Fatal("Load(Save(p)) differs from p")
			}
			abs, _ := filepath.Abs(file)
			mainSrc := `package main

import (
	"fmt"
	"os"
	"reflect"

	"github.com/minelifes/nectar_ui/cmd/nectar/themeeditor"
	"github.com/minelifes/nectar_ui/cmd/nectar/themeeditor/` + filepath.ToSlash(filepath.Join(filepath.Base("testdata"), filepath.Base(dir), "theme")) + `"
)

func main() {
	p, err := themeeditor.Load(` + "`" + abs + "`" + `)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	for _, dark := range []bool{false, true} {
		if !reflect.DeepEqual(theme.Brand(dark), p.Theme(dark)) {
			fmt.Println("MISMATCH dark =", dark)
			os.Exit(1)
		}
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
				src, _ := os.ReadFile(file)
				t.Fatalf("generated code: %v\n%s\n--- code ---\n%s", err, out, src)
			}
		})
	}
}
