// Package material implements Material Design 3 widgets and theming on top
// of the widgets package: a seed-based color scheme (light and dark), the
// M3 type scale, shapes, elevation, state layers and the component set.
//
//	material.App{
//	    Theme: material.NewTheme(geom.Hex(0x6750A4), false),
//	    Home:  MyPage{},
//	}
package material

import (
	"sync"

	"golang.org/x/image/font/gofont/gomedium"

	"github.com/minelifes/nectar_ui/ui/geom"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// ColorScheme holds every M3 color role.
type ColorScheme struct {
	Dark bool

	Primary, OnPrimary, PrimaryContainer, OnPrimaryContainer         geom.Color
	Secondary, OnSecondary, SecondaryContainer, OnSecondaryContainer geom.Color
	Tertiary, OnTertiary, TertiaryContainer, OnTertiaryContainer     geom.Color
	Error, OnError, ErrorContainer, OnErrorContainer                 geom.Color

	Surface, OnSurface, OnSurfaceVariant geom.Color
	SurfaceDim, SurfaceBright            geom.Color
	SurfaceContainerLowest               geom.Color
	SurfaceContainerLow                  geom.Color
	SurfaceContainer                     geom.Color
	SurfaceContainerHigh                 geom.Color
	SurfaceContainerHighest              geom.Color
	SurfaceVariant                       geom.Color

	Outline, OutlineVariant                          geom.Color
	InverseSurface, InverseOnSurface, InversePrimary geom.Color
	Shadow, Scrim                                    geom.Color
}

// SchemeFromSeed generates a "tonal spot" scheme (the M3 default) from one
// seed color.
func SchemeFromSeed(seed geom.Color, dark bool) ColorScheme {
	s := PaletteOf(seed)
	p := TonalPalette{Hue: s.Hue, Chroma: max(s.Chroma, 36)}
	// Chroma/hue offsets are calibrated so the baseline seed reproduces the
	// published M3 baseline scheme (tertiary #7D5260, error #B3261E, ...).
	sec := TonalPalette{Hue: s.Hue, Chroma: 14}
	ter := TonalPalette{Hue: s.Hue + 53, Chroma: 21}
	n := TonalPalette{Hue: s.Hue, Chroma: 4}
	nv := TonalPalette{Hue: s.Hue, Chroma: 7}
	e := TonalPalette{Hue: 36.4, Chroma: 68}
	return SchemeFromPalettes(p, sec, ter, n, nv, e, dark)
}

// SchemeFromPalettes maps palettes to roles with the M3 tone table.
func SchemeFromPalettes(p, s, t, n, nv, e TonalPalette, dark bool) ColorScheme {
	if !dark {
		return ColorScheme{
			Primary: p.Tone(40), OnPrimary: p.Tone(100), PrimaryContainer: p.Tone(90), OnPrimaryContainer: p.Tone(10),
			Secondary: s.Tone(40), OnSecondary: s.Tone(100), SecondaryContainer: s.Tone(90), OnSecondaryContainer: s.Tone(10),
			Tertiary: t.Tone(40), OnTertiary: t.Tone(100), TertiaryContainer: t.Tone(90), OnTertiaryContainer: t.Tone(10),
			Error: e.Tone(40), OnError: e.Tone(100), ErrorContainer: e.Tone(90), OnErrorContainer: e.Tone(10),
			Surface: n.Tone(98), OnSurface: n.Tone(10), OnSurfaceVariant: nv.Tone(30),
			SurfaceDim: n.Tone(87), SurfaceBright: n.Tone(98),
			SurfaceContainerLowest: n.Tone(100), SurfaceContainerLow: n.Tone(96), SurfaceContainer: n.Tone(94),
			SurfaceContainerHigh: n.Tone(92), SurfaceContainerHighest: n.Tone(90), SurfaceVariant: nv.Tone(90),
			Outline: nv.Tone(50), OutlineVariant: nv.Tone(80),
			InverseSurface: n.Tone(20), InverseOnSurface: n.Tone(95), InversePrimary: p.Tone(80),
			Shadow: n.Tone(0), Scrim: n.Tone(0),
		}
	}
	return ColorScheme{
		Dark:    true,
		Primary: p.Tone(80), OnPrimary: p.Tone(20), PrimaryContainer: p.Tone(30), OnPrimaryContainer: p.Tone(90),
		Secondary: s.Tone(80), OnSecondary: s.Tone(20), SecondaryContainer: s.Tone(30), OnSecondaryContainer: s.Tone(90),
		Tertiary: t.Tone(80), OnTertiary: t.Tone(20), TertiaryContainer: t.Tone(30), OnTertiaryContainer: t.Tone(90),
		Error: e.Tone(80), OnError: e.Tone(20), ErrorContainer: e.Tone(30), OnErrorContainer: e.Tone(90),
		Surface: n.Tone(6), OnSurface: n.Tone(90), OnSurfaceVariant: nv.Tone(80),
		SurfaceDim: n.Tone(6), SurfaceBright: n.Tone(24),
		SurfaceContainerLowest: n.Tone(4), SurfaceContainerLow: n.Tone(10), SurfaceContainer: n.Tone(12),
		SurfaceContainerHigh: n.Tone(17), SurfaceContainerHighest: n.Tone(22), SurfaceVariant: nv.Tone(30),
		Outline: nv.Tone(60), OutlineVariant: nv.Tone(30),
		InverseSurface: n.Tone(90), InverseOnSurface: n.Tone(20), InversePrimary: p.Tone(40),
		Shadow: n.Tone(0), Scrim: n.Tone(0),
	}
}

// ---------------------------------------------------------------------------
// Typography

var (
	fontOnce   sync.Once
	fontMedium *text.Font
)

// MediumFont returns the medium-weight (500) font used for titles/labels.
func MediumFont() *text.Font {
	fontOnce.Do(func() {
		f, err := text.ParseFont("Go Medium", gomedium.TTF)
		if err != nil {
			f = text.DefaultBoldFont()
		}
		fontMedium = f
	})
	return fontMedium
}

// TextTheme is the M3 type scale.
type TextTheme struct {
	DisplayLarge, DisplayMedium, DisplaySmall    text.Style
	HeadlineLarge, HeadlineMedium, HeadlineSmall text.Style
	TitleLarge, TitleMedium, TitleSmall          text.Style
	BodyLarge, BodyMedium, BodySmall             text.Style
	LabelLarge, LabelMedium, LabelSmall          text.Style
}

// DefaultTextTheme returns the M3 type scale (sizes / line heights /
// tracking from the spec) colored with c.
func DefaultTextTheme(c geom.Color) TextTheme {
	reg, med := text.DefaultFont(), MediumFont()
	st := func(f *text.Font, size, line, track float32) text.Style {
		return text.Style{Font: f, Size: size, LineHeight: line / size, LetterSpacing: track, Color: c}
	}
	return TextTheme{
		DisplayLarge: st(reg, 57, 64, -0.25), DisplayMedium: st(reg, 45, 52, 0), DisplaySmall: st(reg, 36, 44, 0),
		HeadlineLarge: st(reg, 32, 40, 0), HeadlineMedium: st(reg, 28, 36, 0), HeadlineSmall: st(reg, 24, 32, 0),
		TitleLarge: st(reg, 22, 28, 0), TitleMedium: st(med, 16, 24, 0.15), TitleSmall: st(med, 14, 20, 0.1),
		BodyLarge: st(reg, 16, 24, 0.5), BodyMedium: st(reg, 14, 20, 0.25), BodySmall: st(reg, 12, 16, 0.4),
		LabelLarge: st(med, 14, 20, 0.1), LabelMedium: st(med, 12, 16, 0.5), LabelSmall: st(med, 11, 16, 0.5),
	}
}

// Styled returns s in color c.
func Styled(s text.Style, c geom.Color) text.Style { s.Color = c; return s }

// ---------------------------------------------------------------------------
// Shapes, elevation, state layers

// M3 corner radii.
const (
	CornerNone       float32 = 0
	CornerExtraSmall float32 = 4
	CornerSmall      float32 = 8
	CornerMedium     float32 = 12
	CornerLarge      float32 = 16
	CornerExtraLarge float32 = 28
	CornerFull       float32 = 9999 // clamped to half the shortest side
)

// State-layer opacities.
const (
	HoverOpacity   float32 = 0.08
	FocusOpacity   float32 = 0.10
	PressedOpacity float32 = 0.10
	DraggedOpacity float32 = 0.16

	DisabledContentOpacity   float32 = 0.38
	DisabledContainerOpacity float32 = 0.12
)

// Elevation levels 0–5 in dp (M3).
var elevationDP = [6]float32{0, 1, 3, 6, 8, 12}

// ---------------------------------------------------------------------------
// Theme

// Theme bundles everything components need.
type Theme struct {
	Seed   geom.Color // the seed the scheme was generated from (if any)
	Scheme ColorScheme
	Text   TextTheme
}

// NewTheme builds a theme from a seed color.
func NewTheme(seed geom.Color, dark bool) Theme {
	s := SchemeFromSeed(seed, dark)
	return Theme{Seed: seed, Scheme: s, Text: DefaultTextTheme(s.OnSurface)}
}

// Baseline M3 seed (purple, #6750A4).
var BaselineSeed = geom.Hex(0x6750A4)

// ThemeOf returns the nearest theme (the baseline light theme if none).
func ThemeOf(ctx w.BuildContext) Theme {
	if t, ok := w.Of[Theme](ctx); ok {
		return t
	}
	return defaultTheme()
}

var (
	defThemeOnce sync.Once
	defTheme     Theme
)

func defaultTheme() Theme {
	defThemeOnce.Do(func() { defTheme = NewTheme(BaselineSeed, false) })
	return defTheme
}

// ThemeScope provides a theme to its subtree (and matching default text
// and icon styles).
type ThemeScope struct {
	Theme Theme
	Child w.Widget
}

func (t ThemeScope) Build(w.BuildContext) w.Widget {
	s := t.Theme.Scheme
	return w.Provider[Theme]{Value: t.Theme, Child: w.DefaultTextStyle{
		Style: t.Theme.Text.BodyMedium,
		Child: w.IconTheme{Size: 24, Color: s.OnSurfaceVariant, Child: t.Child},
	}}
}
