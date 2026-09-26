package geom

// Constraints are box constraints passed down the render tree during layout
// ("constraints go down, sizes go up, parent sets position").
type Constraints struct {
	MinW, MaxW float32
	MinH, MaxH float32
}

// Tight constraints force exactly the given size.
func Tight(s Size) Constraints { return Constraints{s.W, s.W, s.H, s.H} }

// Loose constraints allow anything from zero up to the given size.
func Loose(s Size) Constraints { return Constraints{0, s.W, 0, s.H} }

// Unbounded allows any size.
func Unbounded() Constraints { return Constraints{0, Inf, 0, Inf} }

func (c Constraints) IsTight() bool          { return c.MinW >= c.MaxW && c.MinH >= c.MaxH }
func (c Constraints) HasBoundedWidth() bool  { return !IsInf(c.MaxW) }
func (c Constraints) HasBoundedHeight() bool { return !IsInf(c.MaxH) }

// Biggest is the largest size that satisfies the constraints. Unbounded axes
// fall back to the minimum.
func (c Constraints) Biggest() Size {
	w, h := c.MaxW, c.MaxH
	if IsInf(w) {
		w = c.MinW
	}
	if IsInf(h) {
		h = c.MinH
	}
	return Size{w, h}
}

// Smallest is the smallest size that satisfies the constraints.
func (c Constraints) Smallest() Size { return Size{c.MinW, c.MinH} }

// Constrain clamps a size into the constraints.
func (c Constraints) Constrain(s Size) Size {
	return Size{clamp(s.W, c.MinW, c.MaxW), clamp(s.H, c.MinH, c.MaxH)}
}

// Loosen drops the minimums.
func (c Constraints) Loosen() Constraints { return Constraints{0, c.MaxW, 0, c.MaxH} }

// Deflate shrinks the constraints by insets (used by padding).
func (c Constraints) Deflate(e EdgeInsets) Constraints {
	h, v := e.Horizontal(), e.Vertical()
	return Constraints{
		MinW: max(0, c.MinW-h), MaxW: max(0, c.MaxW-h),
		MinH: max(0, c.MinH-v), MaxH: max(0, c.MaxH-v),
	}
}

// Enforce clamps these constraints so they also satisfy parent.
func (c Constraints) Enforce(p Constraints) Constraints {
	return Constraints{
		MinW: clamp(c.MinW, p.MinW, p.MaxW), MaxW: clamp(c.MaxW, p.MinW, p.MaxW),
		MinH: clamp(c.MinH, p.MinH, p.MaxH), MaxH: clamp(c.MaxH, p.MinH, p.MaxH),
	}
}

// WithTightWidth / WithTightHeight fix one axis.
func (c Constraints) WithTightWidth(w float32) Constraints  { c.MinW, c.MaxW = w, w; return c }
func (c Constraints) WithTightHeight(h float32) Constraints { c.MinH, c.MaxH = h, h; return c }

func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
