package themeeditor

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/mvvm"
)

// Options configure the editor.
type Options struct {
	// Dir is the folder Save writes theme_light.go and theme_dark.go to
	// (and the designs were loaded from, if they existed). Empty: no
	// saving, the code can still be copied.
	Dir string
	Gen GenOptions
}

// VM is the editor's view model. The project is changed only on the UI
// goroutine, through its methods; Rev counts the changes so the views
// bound to it rebuild. Edits go to the design of the previewed mode
// (Dark): the light and dark designs are independent.
type VM struct {
	mvvm.ViewModel
	Rev     *mvvm.Property[int]
	Dark    *mvvm.Property[bool]
	Section *mvvm.Property[string] // selected section name
	Filter  *mvvm.Property[string] // section search
	Status  *mvvm.Property[string] // last save result
	Saved   *mvvm.Property[int]    // Rev at the last save (-1: never)

	opts     Options
	project  *Project
	sections []Section
	cache    [2]*m.Theme // light, dark for the current Rev
	cacheRev int
}

// NewVM opens project p (a new one if nil).
func NewVM(p *Project, o Options) *VM {
	if p == nil {
		p = NewProject()
	}
	vm := &VM{
		Rev: mvvm.NewProperty(0), Dark: mvvm.NewProperty(false), Section: mvvm.NewProperty("General"),
		Filter: mvvm.NewProperty(""), Status: mvvm.NewProperty(""), Saved: mvvm.NewProperty(0),
		opts: o, project: p, sections: Sections(), cacheRev: -1,
	}
	return vm
}

// Project returns a copy of the current design.
func (vm *VM) Project() *Project { return vm.project.Clone() }

// Options returns the editor options.
func (vm *VM) Options() Options { return vm.opts }

// Sections returns every section.
func (vm *VM) Sections() []Section { return vm.sections }

// VisibleSections returns the sections matching the search filter (by
// title or by a field name).
func (vm *VM) VisibleSections() []Section {
	q := strings.ToLower(strings.TrimSpace(vm.Filter.Get()))
	if q == "" {
		return vm.sections
	}
	var out []Section
	for _, s := range vm.sections {
		if strings.Contains(strings.ToLower(s.Title), q) {
			out = append(out, s)
			continue
		}
		for _, f := range s.Fields {
			if strings.Contains(strings.ToLower(f.Name), q) {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

// Current returns the selected section.
func (vm *VM) Current() Section {
	name := vm.Section.Get()
	for _, s := range vm.sections {
		if s.Name == name {
			return s
		}
	}
	return vm.sections[0]
}

// Theme returns the designed theme for the given mode (cached per Rev).
func (vm *VM) Theme(dark bool) m.Theme {
	if vm.cacheRev != vm.Rev.Get() {
		vm.cache, vm.cacheRev = [2]*m.Theme{}, vm.Rev.Get()
	}
	i := 0
	if dark {
		i = 1
	}
	if vm.cache[i] == nil {
		t := vm.project.Theme(dark)
		vm.cache[i] = &t
	}
	return *vm.cache[i]
}

// Seed returns the previewed mode's seed color.
func (vm *VM) Seed() geom.Color { return vm.project.Seed(vm.Dark.Get()) }

// SetSeed changes the previewed mode's seed color (its edits stay on top).
func (vm *VM) SetSeed(c geom.Color) {
	c = Quantize(c)
	if c.A == 0 || c == vm.Seed() {
		return
	}
	vm.project.SetSeed(vm.Dark.Get(), c)
	vm.changed()
}

// CopyFromOtherMode replaces the previewed mode's design with a copy of
// the other mode's (a starting point: the scheme is still generated for
// this mode from the seed, unless scheme roles were edited).
func (vm *VM) CopyFromOtherMode() {
	dark := vm.Dark.Get()
	*vm.project.Mode(dark) = vm.project.Mode(!dark).clone()
	vm.changed()
}

// EditsIn returns the number of edits of a mode.
func (vm *VM) EditsIn(dark bool) int { return vm.project.Count(dark) }

// Edit returns the edit at path for the previewed mode.
func (vm *VM) Edit(path string) (any, bool) { return vm.project.Get(path, vm.Dark.Get()) }

// Set records an edit (scheme edits go to the previewed mode).
func (vm *VM) Set(path string, v any) {
	if old, ok := vm.Edit(path); ok && old == v {
		return
	}
	if err := vm.project.Set(path, vm.Dark.Get(), v); err != nil {
		vm.Status.Set(err.Error())
		return
	}
	vm.changed()
}

// Unset reverts path to its default.
func (vm *VM) Unset(path string) {
	if _, ok := vm.Edit(path); !ok {
		return
	}
	vm.project.Unset(path, vm.Dark.Get())
	vm.changed()
}

// ResetSection reverts every field of a section in the previewed mode.
func (vm *VM) ResetSection(s Section) {
	for _, f := range s.Fields {
		vm.project.Unset(f.Path, vm.Dark.Get())
	}
	vm.changed()
}

// ResetMode starts the previewed mode's design over from the baseline.
func (vm *VM) ResetMode() {
	*vm.project.Mode(vm.Dark.Get()) = NewDesign()
	vm.changed()
}

// EditCount returns how many fields of s are edited in the previewed mode.
func (vm *VM) EditCount(s Section) int {
	n := 0
	for _, f := range s.Fields {
		if _, ok := vm.Edit(f.Path); ok {
			n++
		}
	}
	return n
}

// Code returns the generated Go file of a mode.
func (vm *VM) Code(dark bool) (string, error) {
	src, err := vm.project.GoCode(dark, vm.opts.Gen)
	return string(src), err
}

// Dirty reports whether there are unsaved changes.
func (vm *VM) Dirty() bool { return vm.Saved.Get() != vm.Rev.Get() }

// Save writes theme_light.go and theme_dark.go to Options.Dir.
func (vm *VM) Save() error {
	if vm.opts.Dir == "" {
		err := fmt.Errorf("no output folder: start with nectar theme <folder>")
		vm.Status.Set(err.Error())
		return err
	}
	if err := vm.project.Save(vm.opts.Dir, vm.opts.Gen); err != nil {
		vm.Status.Set("Save failed: " + err.Error())
		return err
	}
	vm.Saved.Set(vm.Rev.Get())
	vm.Status.Set("Saved " + filepath.Join(vm.opts.Dir, LightFile) + " and " + DarkFile)
	return nil
}

func (vm *VM) changed() {
	vm.Rev.Update(func(n int) int { return n + 1 })
	vm.Notify()
}
