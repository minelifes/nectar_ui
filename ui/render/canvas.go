package render

import (
	"nectar_ui/ui/geom"
	"nectar_ui/ui/text"
)

// CommandKind tells the GPU backend how to draw a Command.
type CommandKind uint8

const (
	CmdRect CommandKind = iota // filled (rounded) rectangle
	CmdText                    // laid-out paragraph
)

// Command is one entry of the display list. Coordinates are logical pixels.
type Command struct {
	Kind   CommandKind
	Clip   geom.Rect // clip rect active when the command was recorded
	Rect   geom.Rect
	Color  geom.Color
	Radius float32         // corner radius for CmdRect
	Text   *text.Paragraph // for CmdText; drawn with its top-left at Rect.X/Y
}

// Canvas records drawing commands. It's a retained display list, not an
// immediate-mode API: the GPU backend consumes Commands once per frame.
type Canvas struct {
	Commands []Command
	clips    []geom.Rect
}

// Reset clears the list and sets the root clip (the window).
func (c *Canvas) Reset(window geom.Rect) {
	c.Commands = c.Commands[:0]
	c.clips = append(c.clips[:0], window)
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
	c.Commands = append(c.Commands, Command{Kind: CmdRect, Clip: c.Clip(), Rect: r, Color: color, Radius: max(0, radius)})
}

// DrawParagraph draws laid-out text with its top-left corner at origin.
func (c *Canvas) DrawParagraph(p *text.Paragraph, origin geom.Offset) {
	if p == nil || len(p.Glyphs) == 0 {
		return
	}
	c.Commands = append(c.Commands, Command{
		Kind: CmdText, Clip: c.Clip(),
		Rect:  geom.Rect{X: origin.X, Y: origin.Y, W: p.Width, H: p.Height},
		Color: p.Style.Color, Text: p,
	})
}
