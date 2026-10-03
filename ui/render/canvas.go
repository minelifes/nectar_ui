package render

import (
	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/vector"
	"github.com/minelifes/nectar_ui/ui/widgets/text"
)

// CommandKind tells the GPU backend how to draw a Command.
type CommandKind uint8

const (
	CmdRect   CommandKind = iota // filled (rounded) rectangle
	CmdText                      // laid-out paragraph
	CmdShadow                    // blurred rounded-rect shadow
	CmdStroke                    // rounded-rect outline (stroke inside Rect)
	CmdArc                       // circular arc stroke inscribed in Rect
	CmdIcon                      // vector icon scaled into Rect
	CmdRipple                    // circle (Center, Blur=radius) clipped to a rounded rect
	CmdImage                     // Image's Src pixels into Rect (Radius corners, Width 1 = nearest)
)

// Command is one entry of the display list. Coordinates are logical pixels.
type Command struct {
	Kind   CommandKind
	Clip   geom.Rect // clip rect active when the command was recorded
	Rect   geom.Rect
	Color  geom.Color
	Radius float32         // corner radius for CmdRect / CmdShadow / CmdStroke
	Text   *text.Paragraph // for CmdText; drawn with its top-left at Rect.X/Y
	Blur   float32         // CmdShadow: blur radius
	Width  float32         // CmdStroke / CmdArc: line width
	Start  float32         // CmdArc: start angle, radians, clockwise from 3 o'clock
	Sweep  float32         // CmdArc: sweep, radians (clockwise)
	Icon   *vector.Icon    // CmdIcon
	Center geom.Offset     // CmdRipple: circle center (window coordinates)
	Image  *Image          // CmdImage
	Src    geom.Rect       // CmdImage: source rect in image pixels
}

// Canvas records drawing commands. It's a retained display list, not an
// immediate-mode API: the GPU backend consumes Commands once per frame.
type Canvas struct {
	Commands []Command
	clips    []geom.Rect
	alphas   []float32
}

// Reset clears the list and sets the root clip (the window).
func (c *Canvas) Reset(window geom.Rect) {
	c.Commands = c.Commands[:0]
	c.clips = append(c.clips[:0], window)
	c.alphas = append(c.alphas[:0], 1)
}

// PushOpacity multiplies the alpha of everything drawn until PopOpacity.
func (c *Canvas) PushOpacity(a float32) { c.alphas = append(c.alphas, c.alpha()*a) }

// PopOpacity restores the previous opacity.
func (c *Canvas) PopOpacity() {
	if len(c.alphas) > 1 {
		c.alphas = c.alphas[:len(c.alphas)-1]
	}
}

func (c *Canvas) alpha() float32 {
	if len(c.alphas) == 0 {
		return 1
	}
	return c.alphas[len(c.alphas)-1]
}

// add records cmd with the current clip and opacity applied.
func (c *Canvas) add(cmd Command) {
	cmd.Clip = c.Clip()
	cmd.Color.A *= c.alpha()
	if cmd.Color.A <= 0 || cmd.Clip.Empty() {
		return
	}
	c.Commands = append(c.Commands, cmd)
}

// Clip returns the current clip rectangle.
func (c *Canvas) Clip() geom.Rect { return c.clips[len(c.clips)-1] }

// PushClip intersects the current clip with r.
func (c *Canvas) PushClip(r geom.Rect) { c.clips = append(c.clips, c.Clip().Intersect(r)) }

// PopClip restores the previous clip.
func (c *Canvas) PopClip() {
	if len(c.clips) > 1 {
		c.clips = c.clips[:len(c.clips)-1]
	}
}

// FillRect fills r with color.
func (c *Canvas) FillRect(r geom.Rect, color geom.Color) { c.FillRoundRect(r, 0, color) }

// FillRoundRect fills r with rounded corners of the given radius.
func (c *Canvas) FillRoundRect(r geom.Rect, radius float32, color geom.Color) {
	if color.A <= 0 || r.Empty() {
		return
	}
	radius = min(radius, r.W/2, r.H/2)
	c.add(Command{Kind: CmdRect, Rect: r, Color: color, Radius: max(0, radius)})
}

// FillCircle fills a circle.
func (c *Canvas) FillCircle(center geom.Offset, radius float32, color geom.Color) {
	c.FillRoundRect(geom.Rect{X: center.X - radius, Y: center.Y - radius, W: 2 * radius, H: 2 * radius}, radius, color)
}

// StrokeRoundRect draws a rounded-rect outline of the given width, inside r.
func (c *Canvas) StrokeRoundRect(r geom.Rect, radius, width float32, color geom.Color) {
	if r.Empty() || width <= 0 {
		return
	}
	radius = min(radius, r.W/2, r.H/2)
	c.add(Command{Kind: CmdStroke, Rect: r, Color: color, Radius: max(0, radius), Width: width})
}

// StrokeCircle draws a circle outline.
func (c *Canvas) StrokeCircle(center geom.Offset, radius, width float32, color geom.Color) {
	c.StrokeRoundRect(geom.Rect{X: center.X - radius, Y: center.Y - radius, W: 2 * radius, H: 2 * radius}, radius, width, color)
}

// DrawShadow draws a soft shadow for a rounded rect (paint it before the
// shape). blur is roughly the shadow's spread in pixels.
func (c *Canvas) DrawShadow(r geom.Rect, radius, blur float32, color geom.Color) {
	if r.Empty() {
		return
	}
	radius = min(radius, r.W/2, r.H/2)
	c.add(Command{Kind: CmdShadow, Rect: r, Color: color, Radius: max(0, radius), Blur: max(blur, 0.5)})
}

// StrokeArc draws a circular arc inscribed in the square r. Angles are in
// radians, clockwise, 0 = 3 o'clock.
func (c *Canvas) StrokeArc(r geom.Rect, start, sweep, width float32, color geom.Color) {
	if r.Empty() || width <= 0 || sweep == 0 {
		return
	}
	if sweep < 0 {
		start, sweep = start+sweep, -sweep
	}
	c.add(Command{Kind: CmdArc, Rect: r, Color: color, Start: start, Sweep: min(sweep, 6.2831855), Width: width})
}

// FillRipple fills the part of a circle (center, circleR) that lies inside
// the rounded rect r: the Material ink ripple.
func (c *Canvas) FillRipple(r geom.Rect, radius float32, center geom.Offset, circleR float32, color geom.Color) {
	if r.Empty() || circleR <= 0 {
		return
	}
	radius = min(radius, r.W/2, r.H/2)
	c.add(Command{Kind: CmdRipple, Rect: r, Color: color, Radius: max(0, radius), Center: center, Blur: circleR})
}

// DrawIcon draws a vector icon scaled to fill the square r.
func (c *Canvas) DrawIcon(ic *vector.Icon, r geom.Rect, color geom.Color) {
	if ic == nil || r.Empty() {
		return
	}
	c.add(Command{Kind: CmdIcon, Rect: r, Color: color, Icon: ic})
}

// DrawParagraph draws laid-out text with its top-left corner at origin.
//
// Span backgrounds are drawn under the glyphs and decoration lines
// (underline, strikethrough) over them.
func (c *Canvas) DrawParagraph(p *text.Paragraph, origin geom.Offset) {
	if p == nil {
		return
	}
	c.drawDecorations(p, origin, true)
	if len(p.Glyphs) > 0 {
		color := p.Style.Color
		if p.Rich {
			// Glyphs carry their own colors; the command's alpha is the
			// opacity applied to them all.
			color = geom.Color{R: 1, G: 1, B: 1, A: 1}
		}
		c.add(Command{
			Kind:  CmdText,
			Rect:  geom.Rect{X: origin.X, Y: origin.Y, W: p.Width, H: p.Height},
			Color: color, Text: p,
		})
	}
	c.drawDecorations(p, origin, false)
}

func (c *Canvas) drawDecorations(p *text.Paragraph, origin geom.Offset, behind bool) {
	for _, d := range p.Decorations {
		if d.Behind != behind || d.Line >= len(p.Lines) {
			continue
		}
		r := d.Rect
		r.X += origin.X + p.Lines[d.Line].X
		r.Y += origin.Y
		c.FillRect(r, d.Color)
	}
}
