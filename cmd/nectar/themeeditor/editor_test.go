package themeeditor

import (
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/render"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

func openEditor(t *testing.T, dir string) (*tester.Tester, *VM) {
	t.Helper()
	vm := NewVM(nil, Options{Dir: dir})
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

// The light and dark themes are separate designs: switching to dark shows
// a fresh design to configure, and switching back shows each one's edits.
func TestLightAndDarkAreSeparateDesigns(t *testing.T) {
	tt, vm := openEditor(t, "")
	// Configure the light theme: seed, a card radius, a scheme role.
	vm.SetSeed(geom.Hex(0x006A6A))
	vm.Set("Scheme.Surface", geom.Hex(0xF4FBFA))
	selectSection(t, tt, "Card")
	r, _ := tt.Find("Radius")
	tt.Drag(geom.Pt(r.X+30, r.Y+r.H+24), geom.Pt(r.X+340, r.Y+r.H+24))
	lightRadius, ok := vm.Edit("Card.Radius")
	if !ok {
		t.Fatal("light radius not set")
	}

	// Dark: a fresh design, nothing carried over.
	tapText(t, tt, "Dark")
	if !vm.Dark.Get() {
		t.Fatal("Dark button didn't switch")
	}
	if _, ok := vm.Edit("Card.Radius"); ok || vm.Seed() != m.BaselineSeed || vm.EditsIn(true) != 0 {
		t.Fatalf("dark design isn't fresh: seed %v, %d edits", vm.Seed(), vm.EditsIn(true))
	}
	if _, ok := tt.Find("Dark theme"); !ok {
		t.Fatal("the editor doesn't say it edits the dark theme")
	}
	if !vm.Theme(true).Scheme.Dark || vm.Theme(true).Card.Radius != nil {
		t.Fatal("dark preview should be the untouched dark baseline")
	}
	// Configure dark differently.
	vm.SetSeed(geom.Hex(0x8B5000))
	vm.Set("Card.Radius", float32(2))
	vm.Set("Chip.Radius", float32(12))

	// Back to light: exactly the light configuration.
	tapText(t, tt, "Light")
	if vm.Dark.Get() || vm.Seed() != geom.Hex(0x006A6A) {
		t.Fatalf("light seed = %v", vm.Seed())
	}
	if v, _ := vm.Edit("Card.Radius"); v != lightRadius {
		t.Fatalf("light radius = %v, want %v", v, lightRadius)
	}
	if _, ok := vm.Edit("Chip.Radius"); ok {
		t.Fatal("a dark edit shows up in light")
	}
	if vm.Theme(false).Scheme.Surface != geom.Hex(0xF4FBFA) {
		t.Fatal("light scheme edit lost")
	}
	// And dark again: the dark configuration.
	tapText(t, tt, "Dark")
	if v, _ := vm.Edit("Card.Radius"); v != float32(2) || vm.Seed() != geom.Hex(0x8B5000) {
		t.Fatalf("dark radius %v seed %v", v, vm.Seed())
	}
	if vm.Theme(true).Scheme.Surface == geom.Hex(0xF4FBFA) {
		t.Fatal("light scheme edit leaked into dark")
	}

	// Copy from light makes dark start from the light design.
	tapText(t, tt, "Copy from light")
	if v, _ := vm.Edit("Card.Radius"); v != lightRadius || vm.Seed() != geom.Hex(0x006A6A) {
		t.Fatal("copy from light")
	}
	// Reset only resets the edited mode.
	vm.ResetMode()
	if vm.EditsIn(true) != 0 || vm.EditsIn(false) == 0 {
		t.Fatalf("reset: light %d dark %d", vm.EditsIn(false), vm.EditsIn(true))
	}
}

func TestSaveAndCodeView(t *testing.T) {
	dir := t.TempDir()
	tt, vm := openEditor(t, dir)
	vm.Set("Card.Radius", float32(6))
	vm.SetSeed(geom.Hex(0x0B57D0))
	vm.Dark.Set(true)
	vm.Set("Card.Radius", float32(9))
	vm.Dark.Set(false)
	tt.Pump()

	tapText(t, tt, "Code")
	tt.Settle()
	texts := strings.Join(tt.Texts(), "\n")
	for _, want := range []string{"func LightTheme() m.Theme {", "return m.Theme{", "Seed: geom.Hex(0x0B57D0),", "Radius: m.Dp(6),"} {
		if !strings.Contains(texts, want) {
			t.Fatalf("light code lacks %q:\n%s", want, texts)
		}
	}
	tapText(t, tt, "Copy")
	if !strings.Contains(tt.Clipboard(), "func LightTheme() m.Theme {") {
		t.Fatal("Copy didn't put the code on the clipboard")
	}
	// The other file.
	tapText(t, tt, DarkFile)
	texts = strings.Join(tt.Texts(), "\n")
	if !strings.Contains(texts, "func DarkTheme() m.Theme {") || !strings.Contains(texts, "Radius: m.Dp(9),") {
		t.Fatalf("dark code:\n%s", texts)
	}
	tt.Key(w.KeyEscape)
	tt.Settle()

	// Ctrl+S (the platform shortcut) saves both files.
	mod := w.ModControl
	if runtime.GOOS == "darwin" {
		mod = w.ModSuper
	}
	tt.Key(w.KeyS, mod)
	if vm.Dirty() {
		t.Fatalf("not saved: %q", vm.Status.Get())
	}
	p, found, err := Load(dir)
	if err != nil || !found {
		t.Fatal(err, found)
	}
	for _, dark := range []bool{false, true} {
		if !reflect.DeepEqual(p.Theme(dark), vm.Theme(dark)) {
			t.Fatalf("saved design differs (dark=%v)", dark)
		}
	}

	// The Save button saves further edits.
	vm.Set("Card.Radius", float32(10))
	tt.Pump()
	tapText(t, tt, "Save")
	if p, _, _ := Load(dir); *p.Theme(false).Card.Radius != 10 || *p.Theme(true).Card.Radius != 9 {
		t.Fatal("Save button didn't save")
	}
}

func TestReopenSavedDesign(t *testing.T) {
	dir := t.TempDir()
	p := NewProject()
	must(t, p.Set("Chip.Radius", false, float32(4)))
	must(t, p.Set("Chip.Radius", true, float32(7)))
	if err := p.Save(dir, GenOptions{Package: "brand"}); err != nil {
		t.Fatal(err)
	}
	q, _, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	vm := NewVM(q, Options{Dir: dir})
	tt := tester.New(Editor{VM: vm}, 1440, 900)
	vm.Section.Set("Chip")
	tt.Pump()
	if v, ok := vm.Edit("Chip.Radius"); !ok || v != float32(4) {
		t.Fatalf("reopened light edit = %v %v", v, ok)
	}
	if !slices.Contains(tt.Texts(), "4") {
		t.Fatal("the slider value isn't shown")
	}
	vm.Dark.Set(true)
	tt.Pump()
	if v, _ := vm.Edit("Chip.Radius"); v != float32(7) || !slices.Contains(tt.Texts(), "7") {
		t.Fatalf("reopened dark edit = %v", v)
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
