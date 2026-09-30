package themeeditor

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// Generated files, one per mode.
const (
	LightFile = "theme_light.go"
	DarkFile  = "theme_dark.go"
)

// FileName returns the generated file of a mode.
func FileName(dark bool) string {
	if dark {
		return DarkFile
	}
	return LightFile
}

// ---------------------------------------------------------------------------
// JSON: each generated file stores its mode's design in its last line
// (see Marker), so the files reopen in the editor.

// Marker starts the comment line holding the design in generated files.
const Marker = "//nectar:theme "

type designJSON struct {
	Seed  string         `json:"seed"`
	Edits map[string]any `json:"edits,omitempty"`
}

// MarshalJSON encodes the design with plain JSON values: colors as
// "#RRGGBBAA", insets as [left, top, right, bottom], durations in
// milliseconds, fonts by name.
func (d Design) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	for k, v := range d.Edits {
		switch x := v.(type) {
		case geom.Color:
			out[k] = ColorHex(x)
		case geom.EdgeInsets:
			out[k] = []float32{x.Left, x.Top, x.Right, x.Bottom}
		case time.Duration:
			out[k] = x.Milliseconds()
		default:
			out[k] = v
		}
	}
	return json.Marshal(designJSON{Seed: ColorHex(d.Seed), Edits: out})
}

// UnmarshalJSON decodes a design, checking every path and value against
// material.Theme.
func (d *Design) UnmarshalJSON(data []byte) error {
	var dj designJSON
	if err := json.Unmarshal(data, &dj); err != nil {
		return err
	}
	seed, err := ParseColor(dj.Seed)
	if err != nil {
		return fmt.Errorf("themeeditor: seed: %w", err)
	}
	nd := NewDesign()
	nd.Seed = seed
	p := &Project{Light: nd}
	for path, raw := range dj.Edits {
		v, err := decodeValue(path, raw)
		if err != nil {
			return err
		}
		if err := p.Set(path, false, v); err != nil {
			return err
		}
	}
	*d = p.Light
	return nil
}

func decodeValue(path string, raw any) (any, error) {
	k, err := KindOf(path)
	if err != nil {
		return nil, err
	}
	bad := func() (any, error) { return nil, fmt.Errorf("themeeditor: bad value %v for %s", raw, path) }
	num := func() (float64, bool) { f, ok := raw.(float64); return f, ok }
	switch k {
	case KindColor:
		s, ok := raw.(string)
		if !ok {
			return bad()
		}
		return ParseColor(s)
	case KindFloat:
		if f, ok := num(); ok {
			return float32(f), nil
		}
	case KindInt:
		if f, ok := num(); ok && f == math.Trunc(f) {
			return int(f), nil
		}
	case KindBool:
		if b, ok := raw.(bool); ok {
			return b, nil
		}
	case KindDuration:
		if f, ok := num(); ok {
			return time.Duration(f) * time.Millisecond, nil
		}
	case KindFont:
		if s, ok := raw.(string); ok {
			if _, err := fontByName(s); err == nil {
				return s, nil
			}
		}
	case KindInsets:
		if a, ok := raw.([]any); ok && len(a) == 4 {
			var e [4]float32
			for i, x := range a {
				f, ok := x.(float64)
				if !ok {
					return bad()
				}
				e[i] = float32(f)
			}
			return geom.InsetsLTRB(e[0], e[1], e[2], e[3]), nil
		}
	}
	return bad()
}

// ColorHex formats c as "#RRGGBBAA".
func ColorHex(c geom.Color) string {
	q := Quantize(c)
	b := func(v float32) int { return int(v*255 + 0.5) }
	return fmt.Sprintf("#%02X%02X%02X%02X", b(q.R), b(q.G), b(q.B), b(q.A))
}

// ParseColor reads "#RGB", "#RRGGBB" or "#RRGGBBAA" (the # is optional).
func ParseColor(s string) (geom.Color, error) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) == 6 {
		h += "FF"
	}
	if len(h) != 8 {
		return geom.Color{}, fmt.Errorf("themeeditor: %q is not a #RRGGBB or #RRGGBBAA color", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return geom.Color{}, fmt.Errorf("themeeditor: %q is not a hex color", s)
	}
	return geom.HexA(uint32(v)), nil
}

// ---------------------------------------------------------------------------
// Files

// Load reads the designs saved in dir (theme_light.go, theme_dark.go). A
// missing file gives a fresh design for that mode; found reports whether
// either existed. A file without the design line (not written by nectar
// theme) is an error, so it's never overwritten.
func Load(dir string) (p *Project, found bool, err error) {
	p = NewProject()
	for _, dark := range []bool{false, true} {
		path := filepath.Join(dir, FileName(dark))
		d, err := loadDesign(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, found, err
		}
		*p.Mode(dark) = d
		found = true
	}
	return p, found, nil
}

func loadDesign(path string) (Design, error) {
	f, err := os.Open(path)
	if err != nil {
		return Design{}, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for sc.Scan() {
		if line, ok := strings.CutPrefix(sc.Text(), Marker); ok {
			var d Design
			if err := json.Unmarshal([]byte(line), &d); err != nil {
				return Design{}, fmt.Errorf("%s: %w", path, err)
			}
			return d, nil
		}
	}
	if err := sc.Err(); err != nil {
		return Design{}, err
	}
	return Design{}, fmt.Errorf("%s: no %q line: not written by nectar theme", path, strings.TrimSpace(Marker))
}

// Save writes theme_light.go and theme_dark.go into dir (created if
// needed).
func (p *Project) Save(dir string, o GenOptions) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, dark := range []bool{false, true} {
		src, err := p.GoCode(dark, o)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, FileName(dark)), src, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Go code

// GenOptions configure the generated files.
type GenOptions struct {
	Package string // default "theme"
	Prefix  string // func names: <Prefix>LightTheme, <Prefix>DarkTheme
	Engine  string // engine module path; default github.com/minelifes/nectar_ui
}

// EngineModule is the default import path of the engine.
const EngineModule = "github.com/minelifes/nectar_ui"

// FuncName returns the generated func of a mode.
func (o GenOptions) FuncName(dark bool) string {
	if dark {
		return o.Prefix + "DarkTheme"
	}
	return o.Prefix + "LightTheme"
}

func (o GenOptions) withDefaults() GenOptions {
	if o.Package == "" {
		o.Package = "theme"
	}
	if o.Engine == "" {
		o.Engine = EngineModule
	}
	return o
}

// GoCode returns a mode's gofmt-ed Go file: a func returning the theme as
// one material.Theme struct literal (the full color scheme and type scale,
// and the component theme fields that were set), ending with the design
// as a Marker line.
func (p *Project) GoCode(dark bool, o GenOptions) ([]byte, error) {
	o = o.withDefaults()
	g := &litWriter{imports: map[string]bool{}}
	lit, err := g.value(reflect.ValueOf(p.Theme(dark)), 1)
	if err != nil {
		return nil, err
	}
	design, err := json.Marshal(*p.Mode(dark))
	if err != nil {
		return nil, err
	}
	mode, other := "light", "dark"
	if dark {
		mode, other = "dark", "light"
	}
	var src strings.Builder
	fmt.Fprintf(&src, `// Code generated by "nectar theme": the %s theme (the %s one is in %s).
// Reopen the design in the editor with:
//
//	nectar theme <this folder>
//
// Use both in the app:
//
//	material.App{Theme: %s.%s(), DarkTheme: widgets.Ptr(%s.%s()), Dark: dark}

package %s

import (
`, mode, other, FileName(!dark), o.Package, o.FuncName(false), o.Package, o.FuncName(true), o.Package)
	if g.imports["time"] {
		src.WriteString("\t\"time\"\n\n")
	}
	fmt.Fprintf(&src, "\t%q\n", o.Engine+"/ui/geom")
	fmt.Fprintf(&src, "\tm %q\n", o.Engine+"/ui/material")
	if g.imports["w"] {
		fmt.Fprintf(&src, "\tw %q\n", o.Engine+"/ui/widgets")
	}
	if g.imports["text"] {
		fmt.Fprintf(&src, "\t%q\n", o.Engine+"/ui/widgets/text")
	}
	fmt.Fprintf(&src, ")\n\n// %s returns the %s theme designed in \"nectar theme\".\nfunc %s() m.Theme {\n\treturn %s\n}\n\n%s%s\n",
		o.FuncName(dark), mode, o.FuncName(dark), lit, Marker, design)
	out, err := format.Source([]byte(src.String()))
	if err != nil {
		return nil, fmt.Errorf("themeeditor: generated code doesn't parse: %w\n%s", err, src.String())
	}
	return out, nil
}

// litWriter writes Go expressions for theme values.
type litWriter struct{ imports map[string]bool }

var materialPkg = themeT.PkgPath()

// value returns the Go expression for v; depth is the indentation of the
// line it starts on.
func (g *litWriter) value(v reflect.Value, depth int) (string, error) {
	t := v.Type()
	switch {
	case t == colorT:
		g.imports["geom"] = true
		return colorExpr(v.Interface().(geom.Color)), nil
	case t == durationT:
		g.imports["time"] = true
		return fmt.Sprintf("%d * time.Millisecond", v.Interface().(time.Duration).Milliseconds()), nil
	case t == fontPtrT:
		return g.font(v.Interface().(*text.Font))
	case t.Kind() == reflect.Float32:
		return num(float32(v.Float())), nil
	case t.Kind() == reflect.Bool:
		return strconv.FormatBool(v.Bool()), nil
	case t.Kind() == reflect.Pointer:
		return g.pointer(v)
	case t.Kind() == reflect.Array:
		items := make([]string, v.Len())
		for i := range items {
			s, err := g.value(v.Index(i), depth)
			if err != nil {
				return "", err
			}
			items[i] = s
		}
		elem, err := g.typeName(t.Elem())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("[%d]%s{%s}", v.Len(), elem, strings.Join(items, ", ")), nil
	case t.Kind() == reflect.Struct:
		return g.structLit(v, depth)
	}
	return "", fmt.Errorf("themeeditor: can't write %s", t)
}

// structLit writes T{ Field: value, ... } with the non-zero fields, one per
// line.
func (g *litWriter) structLit(v reflect.Value, depth int) (string, error) {
	name, err := g.typeName(v.Type())
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(name + "{\n")
	in := strings.Repeat("\t", depth+1)
	for i := range v.NumField() {
		f, sf := v.Field(i), v.Type().Field(i)
		if !sf.IsExported() || f.IsZero() {
			continue
		}
		s, err := g.value(f, depth+1)
		if err != nil {
			return "", fmt.Errorf("%s.%s: %w", name, sf.Name, err)
		}
		fmt.Fprintf(&b, "%s%s: %s,\n", in, sf.Name, s)
	}
	b.WriteString(strings.Repeat("\t", depth) + "}")
	return b.String(), nil
}

func (g *litWriter) pointer(v reflect.Value) (string, error) {
	e := v.Elem()
	switch {
	case e.Kind() == reflect.Float32:
		return "m.Dp(" + num(float32(e.Float())) + ")", nil
	case e.Kind() == reflect.Int:
		g.imports["w"] = true
		return fmt.Sprintf("w.Ptr(%d)", e.Int()), nil
	case e.Kind() == reflect.Bool:
		g.imports["w"] = true
		return fmt.Sprintf("w.Ptr(%t)", e.Bool()), nil
	case e.Type() == insetsT:
		g.imports["w"], g.imports["geom"] = true, true
		x := e.Interface().(geom.EdgeInsets)
		switch {
		case x.Left == x.Top && x.Top == x.Right && x.Right == x.Bottom:
			return "w.Ptr(geom.Insets(" + num(x.Left) + "))", nil
		case x.Left == x.Right && x.Top == x.Bottom:
			return "w.Ptr(geom.InsetsHV(" + num(x.Left) + ", " + num(x.Top) + "))", nil
		}
		return fmt.Sprintf("w.Ptr(geom.InsetsLTRB(%s, %s, %s, %s))", num(x.Left), num(x.Top), num(x.Right), num(x.Bottom)), nil
	}
	return "", fmt.Errorf("themeeditor: can't write %s", v.Type())
}

func (g *litWriter) font(f *text.Font) (string, error) {
	switch FontName(f) {
	case FontRegular:
		g.imports["text"] = true
		return "text.DefaultFont()", nil
	case FontMedium:
		return "m.MediumFont()", nil
	case FontBold:
		g.imports["text"] = true
		return "text.DefaultBoldFont()", nil
	}
	return "", fmt.Errorf("themeeditor: a font Nectar doesn't ship can't be written")
}

func (g *litWriter) typeName(t reflect.Type) (string, error) {
	switch t.PkgPath() {
	case materialPkg:
		return "m." + t.Name(), nil
	case styleT.PkgPath():
		g.imports["text"] = true
		return "text." + t.Name(), nil
	case colorT.PkgPath():
		g.imports["geom"] = true
		return "geom." + t.Name(), nil
	}
	return "", fmt.Errorf("themeeditor: can't write type %s", t)
}

func num(f float32) string { return strconv.FormatFloat(float64(f), 'g', -1, 32) }

// colorExpr writes c as geom.Hex / geom.HexA, or m.Transparent.
func colorExpr(c geom.Color) string {
	if c == m.Transparent {
		return "m.Transparent"
	}
	h := strings.TrimPrefix(ColorHex(c), "#")
	if strings.HasSuffix(h, "FF") {
		return "geom.Hex(0x" + h[:6] + ")"
	}
	return "geom.HexA(0x" + h + ")"
}
