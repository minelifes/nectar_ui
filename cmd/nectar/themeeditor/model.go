// Package themeeditor is the `nectar theme` tool: a Nectar UI app for
// designing a Material theme (seed color, color scheme, type scale and
// every component theme) with a live preview, which writes the result as
// Go code.
//
// The design is a Project: a seed color plus a set of edits, each a path
// into material.Theme ("Card.Radius", "Text.BodyLarge.Size",
// "FilledButton.BackgroundColor") and a value. Everything else keeps the
// value material.NewTheme gives it. Color scheme edits ("Scheme.Primary")
// are kept separately for the light and the dark theme, since the two
// schemes differ; all other edits apply to both.
package themeeditor

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// Project is a theme design: what the editor shows, saves and turns into
// code.
type Project struct {
	Seed  geom.Color
	Edits map[string]any // path → value, shared by light and dark
	Light map[string]any // Scheme.* edits of the light theme
	Dark  map[string]any // Scheme.* edits of the dark theme
}

// NewProject starts a design from the M3 baseline seed.
func NewProject() *Project {
	return &Project{Seed: m.BaselineSeed, Edits: map[string]any{}, Light: map[string]any{}, Dark: map[string]any{}}
}

// Clone returns a deep copy.
func (p *Project) Clone() *Project {
	c := &Project{Seed: p.Seed, Edits: map[string]any{}, Light: map[string]any{}, Dark: map[string]any{}}
	for k, v := range p.Edits {
		c.Edits[k] = v
	}
	for k, v := range p.Light {
		c.Light[k] = v
	}
	for k, v := range p.Dark {
		c.Dark[k] = v
	}
	return c
}

// edits returns the map a path belongs to for the given mode.
func (p *Project) edits(path string, dark bool) map[string]any {
	if !isScheme(path) {
		return p.Edits
	}
	if dark {
		return p.Dark
	}
	return p.Light
}

func isScheme(path string) bool { return strings.HasPrefix(path, "Scheme.") }

// Get returns the edit at path (for the given mode) and whether there is one.
func (p *Project) Get(path string, dark bool) (any, bool) {
	v, ok := p.edits(path, dark)[path]
	return v, ok
}

// Set records an edit. The value must have the path's kind (see KindOf
// and CheckValue); colors are rounded to 8 bits per channel so generated
// code reproduces them exactly.
func (p *Project) Set(path string, dark bool, v any) error {
	if err := CheckValue(path, v); err != nil {
		return err
	}
	if c, ok := v.(geom.Color); ok {
		v = Quantize(c)
	}
	p.edits(path, dark)[path] = v
	return nil
}

// CheckValue reports whether v can be stored at path: a geom.Color for
// colors, float32 for sizes, int for elevations, bool for flags,
// geom.EdgeInsets, time.Duration, or a font name.
func CheckValue(path string, v any) error {
	k, err := KindOf(path)
	if err != nil {
		return err
	}
	ok := false
	switch x := v.(type) {
	case geom.Color:
		ok = k == KindColor
	case float32:
		ok = k == KindFloat
	case int:
		ok = k == KindInt
	case bool:
		ok = k == KindBool
	case geom.EdgeInsets:
		ok = k == KindInsets
	case time.Duration:
		ok = k == KindDuration
	case string:
		_, ferr := fontByName(x)
		ok = k == KindFont && ferr == nil
	}
	if !ok {
		return fmt.Errorf("themeeditor: %T %v can't be stored at %s", v, v, path)
	}
	return nil
}

// Unset removes the edit at path: back to the default.
func (p *Project) Unset(path string, dark bool) { delete(p.edits(path, dark), path) }

// Count returns the number of edits.
func (p *Project) Count() int { return len(p.Edits) + len(p.Light) + len(p.Dark) }

// Theme builds the light or dark theme the project describes: NewTheme
// from the seed, then the scheme edits of that mode, then every other edit
// (text styles first, so component edits see the final type scale).
func (p *Project) Theme(dark bool) m.Theme {
	t := m.NewTheme(p.Seed, dark)
	for _, path := range p.Paths(dark) {
		v, _ := p.Get(path, dark)
		if err := setPath(&t, path, v); err != nil {
			panic(err) // paths are validated when edits are made or loaded
		}
	}
	return t
}

// Paths returns the edited paths of a mode in application order.
func (p *Project) Paths(dark bool) []string {
	var out []string
	for k := range p.edits("Scheme.", dark) {
		out = append(out, k)
	}
	for k := range p.Edits {
		out = append(out, k)
	}
	slices.SortFunc(out, comparePaths)
	return out
}

// comparePaths orders paths: Scheme, then Text, then the rest, each
// alphabetically.
func comparePaths(a, b string) int {
	rank := func(p string) int {
		switch {
		case isScheme(p):
			return 0
		case strings.HasPrefix(p, "Text."):
			return 1
		}
		return 2
	}
	if ra, rb := rank(a), rank(b); ra != rb {
		return ra - rb
	}
	return strings.Compare(a, b)
}

// Quantize rounds a color to 8 bits per channel (what geom.HexA stores).
func Quantize(c geom.Color) geom.Color {
	q := func(v float32) uint8 { return uint8(min(max(v, 0), 1)*255 + 0.5) }
	return geom.RGBA8(q(c.R), q(c.G), q(c.B), q(c.A))
}

// ---------------------------------------------------------------------------
// Paths and kinds

// Kind is the type of value an edit holds.
type Kind int

const (
	KindColor    Kind = iota // geom.Color
	KindFloat                // float32 (*float32 and text.Style sizes)
	KindInt                  // int (*int: elevation)
	KindBool                 // bool (*bool)
	KindInsets               // geom.EdgeInsets (*geom.EdgeInsets)
	KindDuration             // time.Duration
	KindFont                 // string: FontRegular / FontMedium / FontBold
)

// Font names for text styles.
const (
	FontRegular = "regular"
	FontMedium  = "medium"
	FontBold    = "bold"
)

var (
	colorT    = reflect.TypeFor[geom.Color]()
	insetsT   = reflect.TypeFor[geom.EdgeInsets]()
	durationT = reflect.TypeFor[time.Duration]()
	styleT    = reflect.TypeFor[text.Style]()
	fontPtrT  = reflect.TypeFor[*text.Font]()
	float32T  = reflect.TypeFor[float32]()
	themeT    = reflect.TypeFor[m.Theme]()
)

// Field is one editable leaf of material.Theme.
type Field struct {
	Path string // "Card.Radius"
	Name string // "Radius"
	Kind Kind
}

// Section is a group of fields: the scheme, one text style, one component.
// Group says where the sidebar lists it: "Theme", "Typography" or
// "Components".
type Section struct {
	Name   string // "Card", "Scheme", "Text.BodyLarge", "General"
	Title  string // shown in the editor
	Fields []Field
}

// Group returns the sidebar group of the section.
func (s Section) Group() string {
	switch {
	case s.Name == "General" || s.Name == "Scheme":
		return "Theme"
	case strings.HasPrefix(s.Name, "Text."):
		return "Typography"
	}
	return "Components"
}

// Sections lists everything the editor can change, in display order:
// general theme colors, the color scheme, each text style, then each
// component theme.
func Sections() []Section {
	general := Section{Name: "General", Title: "General"}
	var comps []Section
	for i := range themeT.NumField() {
		f := themeT.Field(i)
		switch {
		case f.Name == "Seed":
		case f.Name == "Scheme":
			comps = append(comps, Section{Name: "Scheme", Title: "Color scheme", Fields: fieldsOf(f.Type, "Scheme.", "")})
		case f.Name == "Text":
			for j := range f.Type.NumField() {
				sf := f.Type.Field(j)
				comps = append(comps, Section{Name: "Text." + sf.Name, Title: sf.Name, Fields: fieldsOf(sf.Type, "Text."+sf.Name+".", "")})
			}
		case f.Type == colorT:
			general.Fields = append(general.Fields, Field{Path: f.Name, Name: f.Name, Kind: KindColor})
		case f.Type.Kind() == reflect.Struct:
			comps = append(comps, Section{Name: f.Name, Title: f.Name, Fields: fieldsOf(f.Type, f.Name+".", "")})
		}
	}
	return append([]Section{general}, comps...)
}

// fieldsOf flattens the editable leaves of struct type t. label prefixes
// the names of nested fields ("TextStyle Size").
func fieldsOf(t reflect.Type, prefix, label string) []Field {
	var out []Field
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() || f.Type.Kind() == reflect.Bool {
			continue // ColorScheme.Dark
		}
		path := prefix + f.Name
		switch {
		case f.Type == styleT:
			out = append(out, fieldsOf(f.Type, path+".", label+f.Name+" ")...)
		case f.Type.Kind() == reflect.Array:
			for k := range f.Type.Len() {
				out = append(out, Field{Path: fmt.Sprintf("%s.%d", path, k), Name: fmt.Sprintf("%s%s %d", label, f.Name, k+1), Kind: KindColor})
			}
		default:
			if k, ok := kindOfType(f.Type); ok {
				out = append(out, Field{Path: path, Name: label + f.Name, Kind: k})
			}
		}
	}
	return out
}

func kindOfType(t reflect.Type) (Kind, bool) {
	switch {
	case t == colorT:
		return KindColor, true
	case t == durationT:
		return KindDuration, true
	case t == fontPtrT:
		return KindFont, true
	case t == float32T:
		return KindFloat, true
	case t.Kind() == reflect.Pointer:
		switch t.Elem().Kind() {
		case reflect.Float32:
			return KindFloat, true
		case reflect.Int:
			return KindInt, true
		case reflect.Bool:
			return KindBool, true
		}
		if t.Elem() == insetsT {
			return KindInsets, true
		}
	}
	return 0, false
}

// KindOf returns the kind of the field at path.
func KindOf(path string) (Kind, error) {
	v, err := lookup(reflect.New(themeT).Elem(), path)
	if err != nil {
		return 0, err
	}
	if v.Kind() == reflect.Array {
		return 0, fmt.Errorf("themeeditor: %s is an array, edit its items", path)
	}
	k, ok := kindOfType(v.Type())
	if !ok {
		return 0, fmt.Errorf("themeeditor: %s (%s) can't be edited", path, v.Type())
	}
	return k, nil
}

// lookup finds the field at path inside v (a settable Theme value).
func lookup(v reflect.Value, path string) (reflect.Value, error) {
	for _, part := range strings.Split(path, ".") {
		switch v.Kind() {
		case reflect.Struct:
			f := v.FieldByName(part)
			if !f.IsValid() {
				return reflect.Value{}, fmt.Errorf("themeeditor: no field %q in %s (path %s)", part, v.Type(), path)
			}
			v = f
		case reflect.Array:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= v.Len() {
				return reflect.Value{}, fmt.Errorf("themeeditor: bad index %q in %s", part, path)
			}
			v = v.Index(i)
		default:
			return reflect.Value{}, fmt.Errorf("themeeditor: %s goes past a leaf", path)
		}
	}
	return v, nil
}

// fonts maps font names to the fonts Nectar ships.
func fontByName(name string) (*text.Font, error) {
	switch name {
	case FontRegular:
		return text.DefaultFont(), nil
	case FontMedium:
		return m.MediumFont(), nil
	case FontBold:
		return text.DefaultBoldFont(), nil
	}
	return nil, fmt.Errorf("themeeditor: unknown font %q", name)
}

// FontName is the name of a font Nectar ships ("" for others and nil).
func FontName(f *text.Font) string {
	switch f {
	case nil:
		return ""
	case text.DefaultFont():
		return FontRegular
	case m.MediumFont():
		return FontMedium
	case text.DefaultBoldFont():
		return FontBold
	}
	return ""
}

// setPath stores v at path inside theme t.
func setPath(t *m.Theme, path string, v any) error {
	f, err := lookup(reflect.ValueOf(t).Elem(), path)
	if err != nil {
		return err
	}
	switch x := v.(type) {
	case geom.Color:
		if f.Type() != colorT {
			return fmt.Errorf("themeeditor: %s is not a color", path)
		}
		f.Set(reflect.ValueOf(x))
	case float32:
		switch {
		case f.Type() == float32T:
			f.SetFloat(float64(x))
		case f.Kind() == reflect.Pointer && f.Type().Elem().Kind() == reflect.Float32:
			f.Set(reflect.ValueOf(&x))
		default:
			return fmt.Errorf("themeeditor: %s is not a number", path)
		}
	case int:
		if f.Kind() != reflect.Pointer || f.Type().Elem().Kind() != reflect.Int {
			return fmt.Errorf("themeeditor: %s is not an integer", path)
		}
		f.Set(reflect.ValueOf(&x))
	case bool:
		if f.Kind() != reflect.Pointer || f.Type().Elem().Kind() != reflect.Bool {
			return fmt.Errorf("themeeditor: %s is not a flag", path)
		}
		f.Set(reflect.ValueOf(&x))
	case geom.EdgeInsets:
		if f.Kind() != reflect.Pointer || f.Type().Elem() != insetsT {
			return fmt.Errorf("themeeditor: %s is not insets", path)
		}
		f.Set(reflect.ValueOf(&x))
	case time.Duration:
		if f.Type() != durationT {
			return fmt.Errorf("themeeditor: %s is not a duration", path)
		}
		f.SetInt(int64(x))
	case string:
		if f.Type() != fontPtrT {
			return fmt.Errorf("themeeditor: %s is not a font", path)
		}
		font, err := fontByName(x)
		if err != nil {
			return err
		}
		f.Set(reflect.ValueOf(font))
	default:
		return fmt.Errorf("themeeditor: unsupported value %T for %s", v, path)
	}
	return nil
}

// Value returns the effective value at path in theme t (edited or not) in
// the editor's representation: pointers dereferenced, nil pointers and
// zero values reported as unset (ok false).
func Value(t m.Theme, path string) (v any, ok bool) {
	f, err := lookup(reflect.ValueOf(&t).Elem(), path)
	if err != nil {
		return nil, false
	}
	switch {
	case f.Type() == fontPtrT:
		name := FontName(f.Interface().(*text.Font))
		return name, name != ""
	case f.Kind() == reflect.Pointer:
		if f.IsNil() {
			return nil, false
		}
		return f.Elem().Interface(), true
	}
	return f.Interface(), !f.IsZero()
}
