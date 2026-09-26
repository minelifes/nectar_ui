package render

import "nectar_ui/ui/geom"

// Axis is a layout direction.
type Axis uint8

const (
	Horizontal Axis = iota
	Vertical
)

// MainAxisAlignment distributes free space along the main axis.
type MainAxisAlignment uint8

const (
	MainStart MainAxisAlignment = iota
	MainCenter
	MainEnd
	MainSpaceBetween
	MainSpaceAround
	MainSpaceEvenly
)

// CrossAxisAlignment positions children on the cross axis.
type CrossAxisAlignment uint8

const (
	CrossStart CrossAxisAlignment = iota
	CrossCenter
	CrossEnd
	CrossStretch
)

// RenderFlex lays children out in a row or column (Flutter's Flex).
// Children wrapped in RenderFlexible share the remaining main-axis space.
type RenderFlex struct {
	Box
	MultiChild
	Direction Axis
	Main      MainAxisAlignment
	Cross     CrossAxisAlignment
	Spacing   float32 // gap between children
	// ShrinkMain makes the flex as small as its children on the main axis
	// (Flutter's MainAxisSize.min). Default is to fill the main axis.
	ShrinkMain bool
}

// helpers to treat width/height generically
func (r *RenderFlex) main(s geom.Size) float32 {
	if r.Direction == Horizontal {
		return s.W
	}
	return s.H
}

func (r *RenderFlex) cross(s geom.Size) float32 {
	if r.Direction == Horizontal {
		return s.H
	}
	return s.W
}

func (r *RenderFlex) size(main, cross float32) geom.Size {
	if r.Direction == Horizontal {
		return geom.Size{W: main, H: cross}
	}
	return geom.Size{W: cross, H: main}
}

func (r *RenderFlex) constraints(minMain, maxMain, minCross, maxCross float32) geom.Constraints {
	if r.Direction == Horizontal {
		return geom.Constraints{MinW: minMain, MaxW: maxMain, MinH: minCross, MaxH: maxCross}
	}
	return geom.Constraints{MinW: minCross, MaxW: maxCross, MinH: minMain, MaxH: maxMain}
}

func (r *RenderFlex) PerformLayout(c geom.Constraints) geom.Size {
	maxMain, maxCross := c.MaxW, c.MaxH
	if r.Direction == Vertical {
		maxMain, maxCross = c.MaxH, c.MaxW
	}
	minCross := float32(0)
	if r.Cross == CrossStretch {
		minCross = maxCross
		if geom.IsInf(maxCross) {
			minCross = 0
		}
	}

	n := len(r.children)
	gaps := r.Spacing * float32(max(0, n-1))

	// Pass 1: inflexible children get unbounded main-axis constraints.
	var used, crossSize float32
	totalFlex := 0
	for _, ch := range r.children {
		if f, ok := ch.(*RenderFlexible); ok && f.Flex > 0 {
			totalFlex += f.Flex
			continue
		}
		s := Layout(ch, r.constraints(0, geom.Inf, minCross, maxCross))
		used += r.main(s)
		crossSize = max(crossSize, r.cross(s))
	}

	// Pass 2: flexible children split what's left.
	free := maxMain - used - gaps
	if totalFlex > 0 {
		if geom.IsInf(maxMain) {
			free = 0 // can't share infinity; flex children get min size
		}
		per := max(0, free) / float32(totalFlex)
		for _, ch := range r.children {
			f, ok := ch.(*RenderFlexible)
			if !ok || f.Flex <= 0 {
				continue
			}
			share := per * float32(f.Flex)
			minMain := float32(0)
			if f.Fit {
				minMain = share
			}
			s := Layout(ch, r.constraints(minMain, share, minCross, maxCross))
			used += r.main(s)
			crossSize = max(crossSize, r.cross(s))
		}
	}

	content := used + gaps
	mainSize := maxMain
	if r.ShrinkMain || geom.IsInf(maxMain) {
		mainSize = content
	}
	if r.Cross == CrossStretch && !geom.IsInf(maxCross) {
		crossSize = maxCross
	}
	size := c.Constrain(r.size(mainSize, crossSize))
	mainSize, crossSize = r.main(size), r.cross(size)

	// Position children.
	remaining := max(0, mainSize-content)
	lead, between := float32(0), r.Spacing
	switch r.Main {
	case MainCenter:
		lead = remaining / 2
	case MainEnd:
		lead = remaining
	case MainSpaceBetween:
		if n > 1 {
			between += remaining / float32(n-1)
		}
	case MainSpaceAround:
		if n > 0 {
			between += remaining / float32(n)
			lead = remaining / float32(n) / 2
		}
	case MainSpaceEvenly:
		if n > 0 {
			between += remaining / float32(n+1)
			lead = remaining / float32(n+1)
		}
	}
	pos := lead
	for _, ch := range r.children {
		s := ch.Base().size
		var cpos float32
		switch r.Cross {
		case CrossCenter:
			cpos = (crossSize - r.cross(s)) / 2
		case CrossEnd:
			cpos = crossSize - r.cross(s)
		}
		p, cp := round(pos), round(cpos)
		if r.Direction == Horizontal {
			SetOffset(ch, geom.Offset{X: p, Y: cp})
		} else {
			SetOffset(ch, geom.Offset{X: cp, Y: p})
		}
		pos += r.main(s) + between
	}
	return size
}

func (r *RenderFlex) Paint(ctx *PaintContext, o geom.Offset) {
	for _, ch := range r.children {
		ctx.PaintChild(ch, o)
	}
}
