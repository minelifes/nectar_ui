package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

func field(ctrl *w.TextEditingController, obscure bool) w.Widget {
	return w.Padding{Padding: geom.Insets(10), Child: w.Align{Alignment: geom.TopLeft,
		Child: w.SizedBox{Width: 200, Child: w.EditableText{Controller: ctrl, Obscure: obscure}}}}
}

func TestEditableTyping(t *testing.T) {
	ctrl := w.NewTextController("")
	tt := tester.New(field(ctrl, false), 300, 100)
	tt.Tap(20, 18) // focus
	tt.Type("hello wrld")
	for i := 0; i < 3; i++ {
		tt.Key(w.KeyLeft)
	}
	tt.Type("o")
	if got := ctrl.Text(); got != "hello world" {
		t.Fatalf("text = %q", got)
	}
	tt.Key(w.KeyEnd)
	tt.Key(w.KeyBackspace)
	tt.Key(w.KeyHome)
	tt.Key(w.KeyDelete)
	if got := ctrl.Text(); got != "ello worl" {
		t.Fatalf("after deletes: %q", got)
	}
	// Select all + copy + paste at end.
	tt.Key(w.KeyA, shortcut)
	tt.Key(w.KeyC, shortcut)
	if tt.Clipboard() != "ello worl" {
		t.Fatalf("clipboard %q", tt.Clipboard())
	}
	tt.Key(w.KeyEnd)
	tt.Key(w.KeyV, shortcut)
	if got := ctrl.Text(); got != "ello worlello worl" {
		t.Fatalf("after paste: %q", got)
	}
	// Shift-select two runes back and type over them.
	tt.Key(w.KeyLeft, w.ModShift)
	tt.Key(w.KeyLeft, w.ModShift)
	tt.Type("!")
	if got := ctrl.Text(); got != "ello worlello wo!" {
		t.Fatalf("replace selection: %q", got)
	}
	// Clicking elsewhere drops focus: typing does nothing.
	tt.Tap(290, 90)
	tt.Type("zzz")
	if got := ctrl.Text(); got != "ello worlello wo!" {
		t.Fatalf("typed while unfocused: %q", got)
	}
}

func TestEditableObscureAndTab(t *testing.T) {
	a, b := w.NewTextController("x"), w.NewTextController("")
	tt := tester.New(w.Column{Children: []w.Widget{field(a, false), field(b, true)}}, 300, 200)
	tt.Tap(20, 18)
	tt.Key(w.KeyTab) // to the second field
	tt.Type("pw")
	if b.Text() != "pw" || a.Text() != "x" {
		t.Fatalf("a=%q b=%q", a.Text(), b.Text())
	}
	found := false
	for _, s := range tt.Texts() {
		if s == "••" {
			found = true
		}
	}
	if !found {
		t.Fatalf("password not obscured: %v", tt.Texts())
	}
}

func TestScrollViewWheelAndLazyList(t *testing.T) {
	ctrl := w.NewScrollController()
	built := map[int]bool{}
	list := w.ListViewBuilder{Controller: ctrl, ItemCount: 1000, ItemExtent: 20,
		Builder: func(_ w.BuildContext, i int) w.Widget {
			built[i] = true
			return w.Text{Text: fmt.Sprintf("row %d", i)}
		}}
	tt := tester.New(w.Column{Children: []w.Widget{w.Expanded{Child: list}}}, 200, 100)
	if built[200] {
		t.Fatal("built far-away rows")
	}
	tt.Scroll(50, 50, 4000) // 200 rows down
	if ctrl.Offset() != 4000 {
		t.Fatalf("offset %v", ctrl.Offset())
	}
	if _, ok := tt.Find("row 200"); !ok {
		t.Fatalf("row 200 not built after scrolling; texts: %v", tt.Texts()[:3])
	}
	tt.Scroll(50, 50, 1e9) // clamps
	if max := ctrl.MaxOffset(); ctrl.Offset() != max || max != 1000*20-100 {
		t.Fatalf("offset %v max %v", ctrl.Offset(), max)
	}
	ctrl.AnimateTo(0, 300*time.Millisecond)
	tt.Advance(150 * time.Millisecond)
	mid := ctrl.Offset()
	tt.Settle()
	if !(mid > 0 && mid < 19900) || ctrl.Offset() != 0 {
		t.Fatalf("animateTo: mid %v end %v", mid, ctrl.Offset())
	}
}

// fader animates its opacity when toggled.
type fader struct{}

func (fader) CreateState() w.State { return &faderState{} }

type faderState struct {
	w.StateBase
	a *w.Animated
}

func (s *faderState) InitState() { s.a = w.NewAnimated(s, 200*time.Millisecond, w.Linear, 0) }
func (s *faderState) Build(w.BuildContext) w.Widget {
	return w.GestureDetector{OnTap: func() {
		s.SetState(func() { s.a.Set(1 - s.a.Target()) })
	}, Child: w.SizedBox{Width: 100, Height: 100, Child: w.Text{Text: fmt.Sprintf("%.2f", s.a.Value())}}}
}

func TestAnimatedFollowsTarget(t *testing.T) {
	tt := tester.New(fader{}, 200, 200)
	if tt.Texts()[0] != "0.00" {
		t.Fatalf("start %v", tt.Texts())
	}
	tt.Tap(50, 50)
	tt.Advance(100 * time.Millisecond)
	mid := tt.Texts()[0]
	tt.Settle()
	if mid == "0.00" || mid == "1.00" || tt.Texts()[0] != "1.00" {
		t.Fatalf("mid %v end %v", mid, tt.Texts())
	}
	if tt.Build.HasActiveTickers() {
		t.Fatal("ticker still running after the animation finished")
	}
}

func TestNavigatorPushPopEscape(t *testing.T) {
	var popped any
	home := w.Builder{Builder: func(ctx w.BuildContext) w.Widget {
		return w.GestureDetector{OnTap: func() {
			w.NavigatorOf(ctx).Push(&w.Route{
				Barrier: geom.Black.WithAlpha(0.3), BarrierDismissible: true,
				OnPop: func(r any) { popped = r },
				Builder: func(ctx w.BuildContext) w.Widget {
					return w.Center{Child: w.GestureDetector{
						OnTap: func() { w.NavigatorOf(ctx).Pop("ok") },
						Child: w.Text{Text: "dialog"}}}
				}})
		}, Child: w.SizedBox{Width: 400, Height: 300, Child: w.Text{Text: "home"}}}
	}}
	tt := tester.New(w.Overlay{Child: w.Navigator{Home: home}}, 400, 300)
	tt.Tap(10, 10)
	tt.Settle()
	if _, ok := tt.Find("dialog"); !ok {
		t.Fatal("dialog not shown")
	}
	if err := tt.TapText("dialog"); err != nil {
		t.Fatal(err)
	}
	tt.Settle()
	if _, ok := tt.Find("dialog"); ok || popped != "ok" {
		t.Fatalf("dialog still shown or wrong result %v", popped)
	}
	// Escape dismisses a barrier-dismissible route; the barrier tap too.
	tt.Tap(10, 10)
	tt.Settle()
	tt.Key(w.KeyEscape)
	tt.Settle()
	if _, ok := tt.Find("dialog"); ok {
		t.Fatal("escape did not dismiss")
	}
	tt.Tap(10, 10)
	tt.Settle()
	tt.Tap(5, 290) // barrier
	tt.Settle()
	if _, ok := tt.Find("dialog"); ok {
		t.Fatal("barrier tap did not dismiss")
	}
}

var shortcut = func() w.Modifiers {
	if (w.Modifiers(w.ModSuper)).Shortcut() {
		return w.ModSuper
	}
	return w.ModControl
}()
