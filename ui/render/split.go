package render

import (
	"math"

	"github.com/minelifes/nectar_ui/ui/geom"
)

// RenderSplit lays out panes along an axis with fixed-size dividers between
// them. Children alternate pane, divider, pane, divider, …, pane.
//
// Desired holds the wanted pane sizes (0 = no preference). Layout resolves
// them against the available space: panes keep their proportions when the
// space changes and always stay within [Mins[i], Maxs[i]] (Maxs 0 = no max).
type RenderSplit struct {
	Box
	MultiChild
	Vertical bool
	Gap      float32 // divider thickness
	Mins     []float32
	Maxs     []float32
	Desired  []float32

	// OnResolved receives the final pane sizes after every layout.
	OnResolved func(sizes []float32)

	resolved []float32
}

// Resolved returns the pane sizes from the last layout.
func (r *RenderSplit) Resolved() []float32 { return r.resolved }

func (r *RenderSplit) panes() int { return (len(r.Children()) + 1) / 2 }

func (r *RenderSplit) main(s geom.Size) float32 {
	if r.Vertical {
		return s.H
	}
	return s.W
}

func (r *RenderSplit) PerformLayout(c geom.Constraints) geom.Size {
	n := r.panes()
	kids := r.Children()
	maxMain, maxCross := c.MaxW, c.MaxH
	if r.Vertical {
		maxMain, maxCross = c.MaxH, c.MaxW
	}
	gaps := r.Gap * float32(max(n-1, 0))
	total := maxMain - gaps
	if geom.IsInf(maxMain) {
		// Unbounded: use the desired sizes (or the minimums) as they are.
		total = 0
		for i := 0; i < n; i++ {
			total += max(at(r.Desired, i), at(r.Mins, i))
		}
	}
	r.resolved = ResolveSplit(r.Desired, r.Mins, r.Maxs, n, max(total, 0))

	// Cross axis: fill bounded constraints, else the tallest pane.
	cross := maxCross
	boundedCross := !geom.IsInf(maxCross)
	if !boundedCross {
		cross = 0
		for i := 0; i < n; i++ {
			s := Layout(kids[2*i], r.cons(r.resolved[i], r.resolved[i], 0, geom.Inf))
			cross = max(cross, r.crossOf(s))
		}
	}
	pos := float32(0)
	for i, ch := range kids {
		var ext float32
		if i%2 == 0 {
			ext = r.resolved[i/2]
		} else {
			ext = r.Gap
		}
		Layout(ch, r.cons(ext, ext, cross, cross))
		if r.Vertical {
			SetOffset(ch, geom.Offset{Y: round(pos)})
		} else {
			SetOffset(ch, geom.Offset{X: round(pos)})
		}
		pos += ext
	}
	if r.OnResolved != nil {
		r.OnResolved(append([]float32(nil), r.resolved...))
	}
	if r.Vertical {
		return c.Constrain(geom.Size{W: cross, H: pos})
	}
	return c.Constrain(geom.Size{W: pos, H: cross})
}

func (r *RenderSplit) crossOf(s geom.Size) float32 {
	if r.Vertical {
		return s.W
	}
	return s.H
}

func (r *RenderSplit) cons(minMain, maxMain, minCross, maxCross float32) geom.Constraints {
	if r.Vertical {
		return geom.Constraints{MinW: minCross, MaxW: maxCross, MinH: minMain, MaxH: maxMain}
	}
	return geom.Constraints{MinW: minMain, MaxW: maxMain, MinH: minCross, MaxH: maxCross}
}

func (r *RenderSplit) Paint(ctx *PaintContext, o geom.Offset) {
	kids := r.Children()
	for i, ch := range kids {
		if i%2 == 0 {
			// Panes are clipped so content can't spill over neighbours; the
			// outer edges (not shared with another pane) let shadows bleed.
			b := ch.Base()
			clip := geom.RectFrom(o.Add(b.Offset()), b.Size())
			first, last := i == 0, i == len(kids)-1
			if r.Vertical {
				clip.X, clip.W = clip.X-PaintBleed, clip.W+2*PaintBleed
				if first {
					clip.Y, clip.H = clip.Y-PaintBleed, clip.H+PaintBleed
				}
				if last {
					clip.H += PaintBleed
				}
			} else {
				clip.Y, clip.H = clip.Y-PaintBleed, clip.H+2*PaintBleed
				if first {
					clip.X, clip.W = clip.X-PaintBleed, clip.W+PaintBleed
				}
				if last {
					clip.W += PaintBleed
				}
			}
			ctx.Canvas.PushClip(clip)
			ctx.PaintChild(ch, o)
			ctx.Canvas.PopClip()
		} else {
			ctx.PaintChild(ch, o)
		}
	}
}

func at(s []float32, i int) float32 {
	if i < len(s) {
		return s[i]
	}
	return 0
}

// maxOf returns pane i's maximum (Inf when unset).
func maxOf(maxs []float32, i int) float32 {
	if v := at(maxs, i); v > 0 {
		return v
	}
	return geom.Inf
}

// ResolveSplit turns desired pane sizes into sizes that add up to total
// while respecting mins/maxs. Panes without a desired size share the space
// left over by the others equally. When the sum differs from total, the
// difference is spread in proportion to the current sizes (so the layout
// scales with the window). If the minimums don't fit, panes get their
// minimums (and overflow).
func ResolveSplit(desired, mins, maxs []float32, n int, total float32) []float32 {
	out := make([]float32, n)
	if n == 0 {
		return out
	}
	// Start: desired sizes, equal shares for unset panes.
	var set float32
	unset := 0
	for i := 0; i < n; i++ {
		if d := at(desired, i); d > 0 {
			out[i] = d
			set += d
		} else {
			unset++
		}
	}
	if unset > 0 {
		share := max(total-set, 0) / float32(unset)
		if total-set <= 0 {
			share = total / float32(n)
		}
		for i := 0; i < n; i++ {
			if at(desired, i) <= 0 {
				out[i] = share
			}
		}
	}
	clampAll := func() {
		for i := range out {
			out[i] = min(max(out[i], at(mins, i)), maxOf(maxs, i))
		}
	}
	clampAll()
	// Distribute the remaining difference among panes that can still move,
	// proportionally to their size; repeat as panes hit their limits.
	for iter := 0; iter < 16; iter++ {
		var sum float32
		for _, v := range out {
			sum += v
		}
		diff := total - sum
		if float32(math.Abs(float64(diff))) < 0.01 {
			break
		}
		var weight float32
		for i, v := range out {
			if (diff > 0 && v < maxOf(maxs, i)) || (diff < 0 && v > at(mins, i)) {
				weight += max(v, 1)
			}
		}
		if weight == 0 {
			break // everything is at a limit
		}
		for i, v := range out {
			if (diff > 0 && v < maxOf(maxs, i)) || (diff < 0 && v > at(mins, i)) {
				out[i] = v + diff*max(v, 1)/weight
			}
		}
		clampAll()
	}
	return out
}

// DragSplit returns new pane sizes after moving divider i (between panes i
// and i+1) by delta. The panes on the side the divider moves towards
// shrink, nearest first, down to their minimums; the panes on the other
// side grow, nearest first, up to their maximums. The move is limited so
// the total stays the same.
func DragSplit(sizes, mins, maxs []float32, i int, delta float32) []float32 {
	out := append([]float32(nil), sizes...)
	n := len(out)
	if i < 0 || i >= n-1 || delta == 0 {
		return out
	}
	// growIdx / shrinkIdx list panes nearest-first on each side.
	var growIdx, shrinkIdx []int
	left := make([]int, 0, i+1)
	for j := i; j >= 0; j-- {
		left = append(left, j)
	}
	right := make([]int, 0, n-i-1)
	for j := i + 1; j < n; j++ {
		right = append(right, j)
	}
	amount := delta
	if delta > 0 {
		growIdx, shrinkIdx = left, right
	} else {
		growIdx, shrinkIdx = right, left
		amount = -delta
	}
	var canGrow, canShrink float32
	for _, j := range growIdx {
		canGrow += maxOf(maxs, j) - out[j]
	}
	for _, j := range shrinkIdx {
		canShrink += out[j] - at(mins, j)
	}
	amount = min(amount, max(canGrow, 0), max(canShrink, 0))
	rem := amount
	for _, j := range growIdx {
		take := min(rem, maxOf(maxs, j)-out[j])
		if take > 0 {
			out[j] += take
			rem -= take
		}
	}
	rem = amount
	for _, j := range shrinkIdx {
		take := min(rem, out[j]-at(mins, j))
		if take > 0 {
			out[j] -= take
			rem -= take
		}
	}
	return out
}
