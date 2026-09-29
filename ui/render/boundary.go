package render

import "github.com/minelifes/nectar_ui/ui/geom"

// RenderRepaintBoundary records its subtree's display list once and replays
// it on later frames until something inside changes: a relayout, a widget
// update, MarkNeedsPaint. Moving the boundary (scrolling, animated
// translation) or changing opacity and clipping around it only replays the
// cached commands, shifted, faded and clipped. Use it around subtrees that
// are expensive to paint and change rarely, or that move as a whole.
type RenderRepaintBoundary struct {
	Box
	SingleChild

	cache      []Command
	recordedAt geom.Offset
	valid      bool
	rec        Canvas

	hits, misses int
}

// unclipped is the root clip while recording: the cached list must not
// depend on where the boundary happened to be clipped when recorded.
var unclipped = geom.Rect{X: -1e7, Y: -1e7, W: 2e7, H: 2e7}

func (r *RenderRepaintBoundary) PerformLayout(c geom.Constraints) geom.Size {
	return layoutProxy(r.child, c)
}

// CacheRepaintBoundaries turns display-list caching on (the default).
// Switch it off to check whether a drawing glitch comes from a stale cache
// (a render object that changes its looks without MarkNeedsPaint).
var CacheRepaintBoundaries = true

func (r *RenderRepaintBoundary) Paint(ctx *PaintContext, o geom.Offset) {
	if r.child == nil {
		return
	}
	if !CacheRepaintBoundaries {
		r.valid = false
		ctx.PaintChild(r.child, o)
		return
	}
	if !r.valid || r.paintDirty {
		r.rec.Reset(unclipped)
		(&PaintContext{Canvas: &r.rec}).PaintChild(r.child, o)
		r.cache = append(r.cache[:0], r.rec.Commands...)
		r.recordedAt, r.valid, r.paintDirty = o, true, false
		r.misses++
	} else {
		r.hits++
	}
	ctx.Canvas.replay(r.cache, o.Sub(r.recordedAt))
}

// Stats reports how often the boundary replayed its cache (hits) versus
// repainted its subtree (misses).
func (r *RenderRepaintBoundary) Stats() (hits, misses int) { return r.hits, r.misses }

// replay appends recorded commands shifted by delta, under the current
// clip and opacity.
func (c *Canvas) replay(cmds []Command, delta geom.Offset) {
	clip, alpha := c.Clip(), c.alpha()
	for _, cmd := range cmds {
		cmd.Rect = cmd.Rect.Translate(delta)
		cmd.Clip = cmd.Clip.Translate(delta).Intersect(clip)
		cmd.Center = cmd.Center.Add(delta)
		cmd.Color.A *= alpha
		if cmd.Color.A <= 0 || cmd.Clip.Empty() {
			continue
		}
		// Skip what's entirely clipped away (e.g. scrolled out of view).
		if b := cmd.bounds(); !b.Empty() && cmd.Clip.Intersect(b).Empty() {
			continue
		}
		c.Commands = append(c.Commands, cmd)
	}
}

// bounds is the area a command can touch (shadows spread past Rect).
func (cmd *Command) bounds() geom.Rect {
	r := cmd.Rect
	if cmd.Kind == CmdShadow {
		g := cmd.Blur * 3
		r = geom.Rect{X: r.X - g, Y: r.Y - g, W: r.W + 2*g, H: r.H + 2*g}
	}
	return r
}
