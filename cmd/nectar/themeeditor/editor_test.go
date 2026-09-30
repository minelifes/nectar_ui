package themeeditor

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

func openEditor(t *testing.T, out string) (*tester.Tester, *VM) {
	t.Helper()
	vm := NewVM(nil, Options{Out: out})
	return tester.New(Editor{VM: vm}, 1440, 900), vm
}

// tapText taps the visible text s, failing the test if it's not there.
func tapText(t *testing.T, tt *tester.Tester, s string) geom.Rect {
	t.Helper()
	r, ok := tt.Find(s)
	if !ok {
		t.Fatalf("%q not on screen; texts: %v", s, tt.Texts())
	}
	tt.Tap(r.X+r.W/2, r.Y+r.H/2)
	return r
}

// selectSection finds a section with the search box and opens it, like a
// user would.
func selectSection(t *testing.T, tt *tester.Tester, title string) {
	t.Helper()
	tapText(t, tt, "Search components")
	tt.Type(title)
	tapText(t, tt, title)
}

// hasRect reports whether a filled rect of color c was drawn.
func hasRect(cv *render.Canvas, c geom.Color) bool {
	for _, cmd := range cv.Commands {
		if cmd.Kind == render.CmdRect && cmd.Color == c {
			return true
		}
	}
	return false
}

func TestSelectAndSearchSections(t *testing.T) {
	tt, vm := openEditor(t, "")
	selectSection(t, tt, "Card")
	if vm.Section.Get() != "Card" {
		t.Fatalf("section = %q", vm.Section.Get())
	}
	if _, ok := tt.Find("Preview · Card"); !ok {
		t.Fatal("preview doesn't follow the selected section")
	}
	vm.Filter.Set("slid")
	tt.Pump()
	vis := vm.VisibleSections()
	if len(vis) != 1 || vis[0].Name != "Slider" {
		t.Fatalf("search 'slid' = %v", vis)
	}
	if _, ok := tt.Find("Checkbox"); ok {
		t.Fatal("filtered-out section still listed")
	}
}

func TestTypeHexColorUpdatesPreview(t *testing.T) {
	tt, vm := openEditor(t, "")
	selectSection(t, tt, "FilledButton")
	label := tapText(t, tt, "BackgroundColor")
	// The hex field sits below the label, right of the swatch.
	tt.Tap(label.X+150, label.Y+label.H+30)
	tt.Type("#E53935")
	if v, ok := vm.Edit("FilledButton.BackgroundColor"); !ok || v != geom.Hex(0xE53935) {
		t.Fatalf("edit = %v %v", v, ok)
	}
	if !hasRect(tt.Pump(), geom.Hex(0xE53935)) {
		t.Fatal("preview buttons didn't turn red")
	}
	// Reset (the undo button next to the label) restores the default.
	lab, _ := tt.Find("BackgroundColor")
	tt.Tap(lab.X+375, lab.Y+lab.H/2)
	if _, ok := vm.Edit("FilledButton.BackgroundColor"); ok {
		t.Fatal("reset didn't remove the edit")
	}
	if hasRect(tt.Pump(), geom.Hex(0xE53935)) {
		t.Fatal("preview kept the reset color")
	}
}

func TestSliderAndChoiceEditors(t *testing.T) {
	tt, vm := openEditor(t, "")
	selectSection(t, tt, "Card")
	r, _ := tt.Find("Radius")
	// Drag the Radius slider thumb to the far right: 48.
	y := r.Y + r.H + 24
	tt.Drag(geom.Pt(r.X+30, y), geom.Pt(r.X+340, y))
	if v, ok := vm.Edit("Card.Radius"); !ok || v.(float32) < 40 {
		t.Fatalf("Card.Radius = %v %v", v, ok)
	}
	// Elevation is a segmented 0..5 choice.
	el, _ := tt.Find("Elevation")
	tt.Tap(el.X+20, el.Y+el.H+22) // first segment: "0"
	if v, ok := vm.Edit("Card.Elevation"); !ok || v != 0 {
		t.Fatalf("Card.Elevation = %v %v", v, ok)
	}
	if !vm.Dirty() {
		t.Fatal("edits should mark the design unsaved")
	}
}

func TestColorPickerDialog(t *testing.T) {
	tt, vm := openEditor(t, "")
	selectSection(t, tt, "Card")
	lab, _ := tt.Find("Color")
	tt.Tap(lab.X+20, lab.Y+lab.H+30) // the swatch
	tt.Settle()
	if _, ok := tt.Find("Pick a color"); !ok {
		t.Fatal("color picker didn't open")
	}
	tapText(t, tt, "Transparent")
	tt.Settle()
	if v, ok := vm.Edit("Card.Color"); !ok || v != Quantize(geom.Color{R: 1, G: 1, B: 1}) {
		t.Fatalf("Card.Color = %v %v", v, ok)
	}
	if _, ok := tt.Find("Pick a color"); ok {
		t.Fatal("dialog should close")
	}
	// Open again and apply the current color: Apply sets an edit.
	vm.Unset("Card.Color")
	tt.Pump()
	lab, _ = tt.Find("Color")
	tt.Tap(lab.X+20, lab.Y+lab.H+30)
	tt.Settle()
	tapText(t, tt, "Apply")
	if _, ok := vm.Edit("Card.Color"); !ok {
		t.Fatal("Apply didn't set the color")
	}
}

func TestDarkModeEditsTheDarkScheme(t *testing.T) {
	tt, vm := openEditor(t, "")
	vm.Dark.Set(true)
	vm.Section.Set("Scheme")
	tt.Pump()
	if _, ok := tt.Find("Editing the dark scheme: switch the preview mode (top right) to edit the other one. Unedited roles follow the seed."); !ok {
		t.Fatal("no dark-scheme note")
	}
	vm.Set("Scheme.Primary", geom.Hex(0x00FF00))
	p := vm.Project()
	if _, ok := p.Dark["Scheme.Primary"]; !ok || len(p.Light) != 0 {
		t.Fatalf("scheme edit landed in the wrong mode: light=%v dark=%v", p.Light, p.Dark)
	}
	if !vm.Theme(true).Scheme.Dark || vm.Theme(true).Scheme.Primary != geom.Hex(0x00FF00) {
		t.Fatal("dark preview theme")
	}
}

func TestSaveAndCodeView(t *testing.T) {
	out := filepath.Join(t.TempDir(), "theme_gen.go")
	tt, vm := openEditor(t, out)
	vm.Set("Card.Radius", float32(6))
	vm.SetSeed(geom.Hex(0x0B57D0))
	tt.Pump()

	tapText(t, tt, "Code")
	tt.Settle()
	texts := strings.Join(tt.Texts(), "\n")
	if !strings.Contains(texts, "func Generated(dark bool) m.Theme") || !strings.Contains(texts, "t.Card.Radius = m.Dp(6)") {
		t.Fatalf("code view doesn't show the code:\n%s", texts)
	}
	tapText(t, tt, "Copy")
	if !strings.Contains(tt.Clipboard(), "t.Card.Radius = m.Dp(6)") {
		t.Fatal("Copy didn't put the code on the clipboard")
	}
	tt.Key(w.KeyEscape)
	tt.Settle()

	// Ctrl+S (the platform shortcut) saves.
	mod := w.ModControl
	if runtime.GOOS == "darwin" {
		mod = w.ModSuper
	}
	tt.Key(w.KeyS, mod)
	if vm.Dirty() {
		t.Fatalf("not saved: %q", vm.Status.Get())
	}
	p, err := Load(out)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Theme(false), vm.Theme(false)) {
		t.Fatal("saved design differs")
	}
	src, _ := os.ReadFile(out)
	if !strings.Contains(string(src), "geom.Hex(0x0B57D0)") {
		t.Fatal("seed not in the code")
	}

	// The Save button saves further edits.
	vm.Set("Card.Radius", float32(10))
	tt.Pump()
	tapText(t, tt, "Save")
	if p, _ := Load(out); *p.Theme(false).Card.Radius != 10 {
		t.Fatal("Save button didn't save")
	}
}

func TestReopenSavedDesign(t *testing.T) {
	out := filepath.Join(t.TempDir(), "brand.go")
	p := NewProject()
	p.Set("Chip.Radius", false, float32(4))
	if err := p.Save(out, GenOptions{Package: "brand"}); err != nil {
		t.Fatal(err)
	}
	q, err := Load(out)
	if err != nil {
		t.Fatal(err)
	}
	vm := NewVM(q, Options{Out: out})
	tt := tester.New(Editor{VM: vm}, 1440, 900)
	vm.Section.Set("Chip")
	tt.Pump()
	if v, ok := vm.Edit("Chip.Radius"); !ok || v != float32(4) {
		t.Fatalf("reopened edit = %v %v", v, ok)
	}
	if !slices.Contains(tt.Texts(), "4") {
		t.Fatal("the slider value isn't shown")
	}
	if vm.Dirty() {
		t.Fatal("a freshly opened design isn't unsaved")
	}
}

func TestEverySectionRenders(t *testing.T) {
	tt, vm := openEditor(t, "")
	for _, s := range vm.Sections() {
		vm.Section.Set(s.Name)
		tt.Pump()
		if _, ok := tt.Find(s.Title); !ok {
			t.Errorf("section %s: title not shown", s.Name)
		}
	}
	// Dark preview too.
	vm.Dark.Set(true)
	tt.Pump()
}
