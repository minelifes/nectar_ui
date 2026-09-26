// Package geom holds the small value types shared by every layer of the
// engine: points, sizes, rectangles, insets, box constraints and colors.
//
// All coordinates are in logical pixels (DIP). The GPU backend multiplies by
// the window scale factor at the very end, so layout never has to care about
// HiDPI.
package geom

import "math"

// Inf is used for unbounded constraints.
var Inf = float32(math.Inf(1))

// IsInf reports whether v is +Inf.
func IsInf(v float32) bool { return math.IsInf(float64(v), 1) }

// Offset is a 2D point / vector.
type Offset struct{ X, Y float32 }

func (o Offset) Add(p Offset) Offset             { return Offset{o.X + p.X, o.Y + p.Y} }
func (o Offset) Sub(p Offset) Offset             { return Offset{o.X - p.X, o.Y - p.Y} }
func (o Offset) Scale(s float32) Offset          { return Offset{o.X * s, o.Y * s} }
func Pt(x, y float32) Offset                     { return Offset{x, y} }
func (o Offset) In(r Rect) bool                  { return r.Contains(o) }
func (o Offset) Translate(dx, dy float32) Offset { return Offset{o.X + dx, o.Y + dy} }

// Size is a width/height pair.
type Size struct{ W, H float32 }

func Sz(w, h float32) Size { return Size{w, h} }

// Rect is an axis-aligned rectangle (origin + size).
type Rect struct{ X, Y, W, H float32 }

func RectFrom(o Offset, s Size) Rect { return Rect{o.X, o.Y, s.W, s.H} }

func (r Rect) Right() float32  { return r.X + r.W }
func (r Rect) Bottom() float32 { return r.Y + r.H }
func (r Rect) Origin() Offset  { return Offset{r.X, r.Y} }
func (r Rect) Size() Size      { return Size{r.W, r.H} }
func (r Rect) Empty() bool     { return r.W <= 0 || r.H <= 0 }

func (r Rect) Contains(p Offset) bool {
	return p.X >= r.X && p.Y >= r.Y && p.X < r.Right() && p.Y < r.Bottom()
}

func (r Rect) Translate(o Offset) Rect { return Rect{r.X + o.X, r.Y + o.Y, r.W, r.H} }

// Intersect returns the overlap of r and o (possibly empty).
func (r Rect) Intersect(o Rect) Rect {
	x0 := max(r.X, o.X)
	y0 := max(r.Y, o.Y)
	x1 := min(r.Right(), o.Right())
	y1 := min(r.Bottom(), o.Bottom())
	if x1 <= x0 || y1 <= y0 {
		return Rect{x0, y0, 0, 0}
	}
	return Rect{x0, y0, x1 - x0, y1 - y0}
}

// Deflate shrinks the rectangle by the insets.
func (r Rect) Deflate(e EdgeInsets) Rect {
	return Rect{r.X + e.Left, r.Y + e.Top, max(0, r.W-e.Horizontal()), max(0, r.H-e.Vertical())}
}

// EdgeInsets are paddings on the four sides of a box.
type EdgeInsets struct{ Left, Top, Right, Bottom float32 }

func Insets(v float32) EdgeInsets              { return EdgeInsets{v, v, v, v} }
func InsetsHV(h, v float32) EdgeInsets         { return EdgeInsets{h, v, h, v} }
func InsetsLTRB(l, t, r, b float32) EdgeInsets { return EdgeInsets{l, t, r, b} }
func (e EdgeInsets) Horizontal() float32       { return e.Left + e.Right }
func (e EdgeInsets) Vertical() float32         { return e.Top + e.Bottom }
func (e EdgeInsets) IsZero() bool              { return e == EdgeInsets{} }

type Border struct {
	Radius float32
	Color  Color
	Width  float32
}

// Alignment is a point inside a box, (-1,-1) = top-left, (0,0) = center,
// (1,1) = bottom-right. Same convention as Flutter.
type Alignment struct{ X, Y float32 }

var (
	TopLeft      = Alignment{-1, -1}
	TopCenter    = Alignment{0, -1}
	TopRight     = Alignment{1, -1}
	CenterLeft   = Alignment{-1, 0}
	Center       = Alignment{0, 0}
	CenterRight  = Alignment{1, 0}
	BottomLeft   = Alignment{-1, 1}
	BottomCenter = Alignment{0, 1}
	BottomRight  = Alignment{1, 1}
)

// Inscribe returns the offset of a child of size child inside a parent of
// size parent according to the alignment.
func (a Alignment) Inscribe(parent, child Size) Offset {
	dx := (parent.W - child.W) / 2
	dy := (parent.H - child.H) / 2
	return Offset{dx + a.X*dx, dy + a.Y*dy}
}
