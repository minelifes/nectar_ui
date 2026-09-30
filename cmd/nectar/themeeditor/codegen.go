package themeeditor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"math"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
)

// ---------------------------------------------------------------------------
// JSON: the design is saved inside the generated file (see Marker), so
// the file can be reopened in the editor.

// Marker starts the comment line holding the design in generated files.
const Marker = "//nectar:theme "

type projectJSON struct {
	Seed  string         `json:"seed"`
	Edits map[string]any `json:"edits,omitempty"`
	Light map[string]any `json:"light,omitempty"`
	Dark  map[string]any `json:"dark,omitempty"`
}

// MarshalJSON encodes the design with plain JSON values: colors as
// "#RRGGBBAA", insets as [left, top, right, bottom], durations in
// milliseconds, fonts by name.
func (p *Project) MarshalJSON() ([]byte, error) {
	enc := func(src map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range src {
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
		return out
	}
	return json.Marshal(projectJSON{Seed: ColorHex(p.Seed), Edits: enc(p.Edits), Light: enc(p.Light), Dark: enc(p.Dark)})
}

// UnmarshalJSON decodes a design, checking every path and value against
// material.Theme.
func (p *Project) UnmarshalJSON(data []byte) error {
	var pj projectJSON
	if err := json.Unmarshal(data, &pj); err != nil {
		return err
	}
	seed, err := ParseColor(pj.Seed)
	if err != nil {
		return fmt.Errorf("themeeditor: seed: %w", err)
	}
	*p = *NewProject()
	p.Seed = seed
	load := func(src map[string]any, dark bool, scheme bool) error {
		for path, raw := range src {
			if isScheme(path) != scheme {
				return fmt.Errorf("themeeditor: %s is in the wrong section", path)
			}
			v, err := decodeValue(path, raw)
			if err != nil {
				return err
			}
			if err := p.Set(path, dark, v); err != nil {
				return err
			}
		}
		return nil
	}
	if err := load(pj.Edits, false, false); err != nil {
		return err
	}
	if err := load(pj.Light, false, true); err != nil {
		return err
	}
	return load(pj.Dark, true, true)
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

// Load reads the design saved in a generated file (the Marker line).
func Load(path string) (*Project, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for sc.Scan() {
		if line, ok := strings.CutPrefix(sc.Text(), Marker); ok {
			p := NewProject()
			if err := json.Unmarshal([]byte(line), p); err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			return p, nil
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("%s: no %q line: not written by nectar theme", path, strings.TrimSpace(Marker))
}

// Save writes the generated code (with the design embedded) to path.
func (p *Project) Save(path string, o GenOptions) error {
	src, err := p.GoCode(o)
	if err != nil {
		return err
	}
	return os.WriteFile(path, src, 0o644)
}

// ---------------------------------------------------------------------------
// Go code

// GenOptions configure the generated file.
type GenOptions struct {
	Package string // default "theme"
	Func    string // default "Generated"
	Engine  string // engine module path; default github.com/minelifes/nectar_ui
}

// EngineModule is the default import path of the engine.
const EngineModule = "github.com/minelifes/nectar_ui"

// GoCode returns a gofmt-ed Go file with a func returning the designed
// theme for light or dark mode, ending with the design as a Marker line.
func (p *Project) GoCode(o GenOptions) ([]byte, error) {
	if o.Package == "" {
		o.Package = "theme"
	}
	if o.Func == "" {
		o.Func = "Generated"
	}
	if o.Engine == "" {
		o.Engine = EngineModule
	}
	imports := map[string]bool{}
	var body strings.Builder
	line := func(path string, v any) error {
		expr, err := goExpr(path, v, imports)
		if err != nil {
			return err
		}
		fmt.Fprintf(&body, "\tt.%s = %s\n", goPath(path), expr)
		return nil
	}
	imports["geom"] = true
	fmt.Fprintf(&body, "\tt := m.NewTheme(%s, dark)\n", colorExpr(p.Seed))
	if len(p.Light)+len(p.Dark) > 0 {
		body.WriteString("\tif dark {\n")
		for _, path := range sortedKeys(p.Dark) {
			body.WriteString("\t")
			if err := line(path, p.Dark[path]); err != nil {
				return nil, err
			}
		}
		body.WriteString("\t} else {\n")
		for _, path := range sortedKeys(p.Light) {
			body.WriteString("\t")
			if err := line(path, p.Light[path]); err != nil {
				return nil, err
			}
		}
		body.WriteString("\t}\n")
	}
	for _, path := range sortedKeys(p.Edits) {
		if err := line(path, p.Edits[path]); err != nil {
			return nil, err
		}
	}
	body.WriteString("\treturn t\n")

	design, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var src bytes.Buffer
	fmt.Fprintf(&src, `// Code generated by "nectar theme". Reopen this file in the editor to change
// the design (it is stored in the last line of this file):
//
//	nectar theme -o <this file>
//
// Use it in the app:
//
//	material.App{Theme: %s.%s(false), DarkTheme: widgets.Ptr(%s.%s(true)), Dark: dark}

package %s

import (
`, o.Package, o.Func, o.Package, o.Func, o.Package)
	if imports["time"] {
		src.WriteString("\t\"time\"\n\n")
	}
	fmt.Fprintf(&src, "\t%q\n", o.Engine+"/ui/geom")
	fmt.Fprintf(&src, "\tm %q\n", o.Engine+"/ui/material")
	if imports["w"] {
		fmt.Fprintf(&src, "\tw %q\n", o.Engine+"/ui/widgets")
	}
	if imports["text"] {
		fmt.Fprintf(&src, "\t%q\n", o.Engine+"/ui/widgets/text")
	}
	fmt.Fprintf(&src, ")\n\n// %s returns the light (dark = false) or dark theme designed in \"nectar theme\".\nfunc %s(dark bool) m.Theme {\n%s}\n\n%s%s\n",
		o.Func, o.Func, body.String(), Marker, design)
	out, err := format.Source(src.Bytes())
	if err != nil {
		return nil, fmt.Errorf("themeeditor: generated code doesn't parse: %w\n%s", err, src.Bytes())
	}
	return out, nil
}

func sortedKeys(mp map[string]any) []string {
	keys := make([]string, 0, len(mp))
	for k := range mp {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, comparePaths)
	return keys
}

// goPath turns "Carousel.ItemColors.0" into "Carousel.ItemColors[0]".
func goPath(path string) string {
	parts := strings.Split(path, ".")
	var b strings.Builder
	for i, part := range parts {
		if _, err := strconv.Atoi(part); err == nil {
			fmt.Fprintf(&b, "[%s]", part)
			continue
		}
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(part)
	}
	return b.String()
}

// goExpr is the Go expression assigning v to the field at path.
func goExpr(path string, v any, imports map[string]bool) (string, error) {
	if err := CheckValue(path, v); err != nil {
		return "", err
	}
	f, err := lookup(reflect.New(themeT).Elem(), path)
	if err != nil {
		return "", err
	}
	ptr := f.Kind() == reflect.Pointer
	switch x := v.(type) {
	case geom.Color:
		imports["geom"] = true
		return colorExpr(x), nil
	case float32:
		if ptr {
			return "m.Dp(" + num(x) + ")", nil
		}
		return num(x), nil
	case int:
		imports["w"] = true
		return fmt.Sprintf("w.Ptr(%d)", x), nil
	case bool:
		imports["w"] = true
		return fmt.Sprintf("w.Ptr(%t)", x), nil
	case geom.EdgeInsets:
		imports["w"], imports["geom"] = true, true
		if x.Left == x.Top && x.Top == x.Right && x.Right == x.Bottom {
			return "w.Ptr(geom.Insets(" + num(x.Left) + "))", nil
		}
		if x.Left == x.Right && x.Top == x.Bottom {
			return "w.Ptr(geom.InsetsHV(" + num(x.Left) + ", " + num(x.Top) + "))", nil
		}
		return fmt.Sprintf("w.Ptr(geom.InsetsLTRB(%s, %s, %s, %s))", num(x.Left), num(x.Top), num(x.Right), num(x.Bottom)), nil
	case time.Duration:
		imports["time"] = true
		return fmt.Sprintf("%d * time.Millisecond", x.Milliseconds()), nil
	case string:
		switch x {
		case FontRegular:
			imports["text"] = true
			return "text.DefaultFont()", nil
		case FontMedium:
			return "m.MediumFont()", nil
		case FontBold:
			imports["text"] = true
			return "text.DefaultBoldFont()", nil
		}
	}
	return "", fmt.Errorf("themeeditor: can't write %T %v for %s", v, v, path)
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
