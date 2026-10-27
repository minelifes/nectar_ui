package tests

import (
	"testing"

	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

func TestComponentThemeCopyWith(t *testing.T) {
	base := m.CardTheme{Color: tRed, Radius: m.Dp(12), Elevation: w.Ptr(1)}
	got := base.CopyWith(m.CardTheme{Color: tBlue, BorderWidth: m.Dp(2)})
	if got.Color != tBlue || *got.Radius != 12 || *got.Elevation != 1 || *got.BorderWidth != 2 {
		t.Fatalf("CopyWith = %+v", got)
	}
	if base.Color != tRed || base.BorderWidth != nil {
		t.Fatal("CopyWith changed its receiver")
	}
	// A set zero (pointer to 0) is a value, not "unset".
	if sq := base.CopyWith(m.CardTheme{Radius: m.Dp(0)}); *sq.Radius != 0 {
		t.Fatalf("radius 0 not applied: %v", *sq.Radius)
	}
	if same := base.CopyWith(m.CardTheme{}); same.Color != tRed || *same.Radius != 12 {
		t.Fatal("an empty CopyWith changed fields")
	}
}

func TestThemeCopyWithMergesNested(t *testing.T) {
	th := m.NewTheme(m.BaselineSeed, false)
	th.Card.Radius = m.Dp(4)
	got := th.CopyWith(m.Theme{
		Card:   m.CardTheme{Color: tGreen},
		Scheme: m.ColorScheme{Primary: tBlue},
		Text:   m.TextTheme{BodyMedium: text.Style{Size: 20}},
	})
	if got.Card.Color != tGreen || *got.Card.Radius != 4 {
		t.Fatalf("card: %+v (the Radius set before must stay)", got.Card)
	}
	if got.Scheme.Primary != tBlue || got.Scheme.Surface != th.Scheme.Surface {
		t.Fatal("scheme: Primary not replaced or the rest lost")
	}
	if got.Text.BodyMedium.Size != 20 || got.Text.BodyMedium.Font != th.Text.BodyMedium.Font ||
		got.Text.BodyMedium.LineHeight != th.Text.BodyMedium.LineHeight {
		t.Fatalf("text style must merge field by field: %+v", got.Text.BodyMedium)
	}
	if got.FilledButton.Radius != th.FilledButton.Radius || got.Seed != th.Seed {
		t.Fatal("untouched fields changed")
	}
}

func TestButtonStyleCopyWithKeepsTextStyle(t *testing.T) {
	base := m.ButtonStyle{BackgroundColor: tRed, TextStyle: text.Style{Size: 14, LetterSpacing: 0.5}}
	got := base.CopyWith(m.ButtonStyle{TextStyle: text.Style{Size: 18}})
	if got.BackgroundColor != tRed || got.TextStyle.Size != 18 || got.TextStyle.LetterSpacing != 0.5 {
		t.Fatalf("CopyWith = %+v", got)
	}
}

func TestTextStyleCopyWith(t *testing.T) {
	base := text.Style{Size: 14, Color: tRed, LetterSpacing: 0.25, TabSize: 4}
	got := base.CopyWith(text.Style{Color: tBlue, Decoration: text.Underline})
	if got.Size != 14 || got.Color != tBlue || got.LetterSpacing != 0.25 || got.TabSize != 4 || got.Decoration != text.Underline {
		t.Fatalf("CopyWith = %+v", got)
	}
}

// The way it's meant to be used: a card restyled from the theme.
type copyWithCard struct{}

func (copyWithCard) Build(ctx w.BuildContext) w.Widget {
	return m.Card{Style: m.ThemeOf(ctx).Card.CopyWith(m.CardTheme{Color: tGreen}),
		Child: w.SizedBox{Width: 80, Height: 40}}
}

func TestCopyWithFromThemeOf(t *testing.T) {
	tt := tester.New(themed(m.Theme{Card: m.CardTheme{Color: tRed}}, copyWithCard{}), 200, 100)
	cv := tt.Pump()
	if len(rectsIn(cv, tGreen)) == 0 || len(rectsIn(cv, tRed)) != 0 {
		t.Fatalf("card drawn with %d green / %d red rects, want the CopyWith color", len(rectsIn(cv, tGreen)), len(rectsIn(cv, tRed)))
	}
}
