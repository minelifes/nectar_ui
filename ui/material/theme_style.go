package material

import (
	"reflect"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// How component themes and styles work
//
// Every component reads its look from three places, the first set value
// winning (Flutter's order):
//
//  1. the widget's own Style field,
//  2. the matching field of the Theme (Theme.Card, Theme.FilledButton, ...),
//  3. the M3 default, derived from the color scheme and type scale.
//
// "Set" means:
//   - colors: not the zero geom.Color. Use Transparent for an explicit
//     "no color" (it's invisible, but not zero);
//   - text styles: field by field, like Text.Style: the non-zero fields
//     (Font, Size, Color, ...) replace the default's;
//   - numbers, insets, elevation and flags: not nil. They are pointers
//     because 0 is a meaningful value (square corners, no shadow). Write
//     Dp(12) for a *float32 and w.Ptr(v) for any other type.
//
// Theme is a plain value: to restyle a subtree, copy it, change fields and
// provide it with ThemeScope:
//
//	t := material.ThemeOf(ctx)
//	t.Card.Radius = material.Dp(4)
//	return material.ThemeScope{Theme: t, Child: panel}

// Transparent is an explicit "no color" for theme and style fields, where
// the zero geom.Color means "use the default". It paints nothing.
var Transparent = geom.Color{R: 1, G: 1, B: 1, A: 0}

// Dp returns a pointer to v, for optional sizes in themes and styles
// (Radius: material.Dp(8)).
func Dp(v float32) *float32 { return &v }

// pick returns c, or def when c is unset (zero).
func pick(c, def geom.Color) geom.Color {
	if c == (geom.Color{}) {
		return def
	}
	return c
}

// pickF returns *p, or def when p is nil.
func pickF(p *float32, def float32) float32 {
	if p == nil {
		return def
	}
	return *p
}

// pickI returns *p, or def when p is nil.
func pickI(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

// pickB returns *p, or def when p is nil.
func pickB(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// pickE returns *p, or def when p is nil.
func pickE(p *geom.EdgeInsets, def geom.EdgeInsets) geom.EdgeInsets {
	if p == nil {
		return def
	}
	return *p
}

// pickT returns def with the set fields of s on top.
func pickT(s, def text.Style) text.Style {
	if s.Font != nil {
		def.Font = s.Font
	}
	if s.Size != 0 {
		def.Size = s.Size
	}
	if s.Color != (geom.Color{}) {
		def.Color = s.Color
	}
	if s.LineHeight != 0 {
		def.LineHeight = s.LineHeight
	}
	if s.LetterSpacing != 0 {
		def.LetterSpacing = s.LetterSpacing
	}
	return def
}

// side resolves an outline from a style's color and width: a color alone
// gets a 1px line, a width alone the default color, neither the defaults.
func side(c geom.Color, width *float32, defC geom.Color, defW float32) (geom.Color, float32) {
	switch {
	case c != (geom.Color{}) && width == nil:
		return c, max(defW, 1)
	case c == (geom.Color{}) && width != nil:
		return defC, *width
	}
	return pick(c, defC), pickF(width, defW)
}

// pickTC is pickT for a text style whose default color is c.
func pickTC(s, def text.Style, c geom.Color) text.Style { return pickT(s, Styled(def, c)) }

var textStyleType = reflect.TypeFor[text.Style]()

// merge returns base with every set field of over on top (see the rules
// above). Nested structs of this package (a ButtonStyle inside another
// theme) merge field by field too; other structs (colors, insets) are
// replaced whole when set.
func merge[T any](base, over T) T {
	b := reflect.ValueOf(&base).Elem()
	mergeValue(b, reflect.ValueOf(over))
	return base
}

func mergeValue(dst, src reflect.Value) {
	for i := range src.NumField() {
		d, s := dst.Field(i), src.Field(i)
		switch {
		case s.Kind() == reflect.Struct && s.Type() == textStyleType:
			d.Set(reflect.ValueOf(pickT(s.Interface().(text.Style), d.Interface().(text.Style))))
		case s.Kind() == reflect.Struct && s.Type().PkgPath() == reflect.TypeFor[Theme]().PkgPath():
			mergeValue(d, s)
		case !s.IsZero():
			d.Set(s)
		}
	}
}
