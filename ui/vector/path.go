// Package vector holds resolution-independent shapes: an SVG path-data
// parser and Icons built from it. Shapes are rasterized on demand into the
// GPU glyph atlas at the exact physical pixel size they're drawn at.
package vector

import (
	"fmt"
	"math"
	"strconv"
	"sync"
	"sync/atomic"

	xvector "golang.org/x/image/vector"
)

// Op is a path segment kind.
type Op uint8

const (
	MoveTo Op = iota
	LineTo
	QuadTo
	CubeTo
	Close
)

// Seg is one path segment; P holds up to 3 points (control points first).
type Seg struct {
	Op Op
	P  [3][2]float32
}

// Path is a list of absolute segments in its own coordinate space.
type Path struct {
	Segs []Seg
}

// Rasterize adds the path to z, scaled by s and offset by (dx, dy).
func (p *Path) Rasterize(z *xvector.Rasterizer, s, dx, dy float32) {
	tx := func(q [2]float32) (float32, float32) { return q[0]*s + dx, q[1]*s + dy }
	open := false
	for _, sg := range p.Segs {
		switch sg.Op {
		case MoveTo:
			if open {
				z.ClosePath()
			}
			z.MoveTo(tx(sg.P[0]))
			open = true
		case LineTo:
			z.LineTo(tx(sg.P[0]))
		case QuadTo:
			bx, by := tx(sg.P[0])
			cx, cy := tx(sg.P[1])
			z.QuadTo(bx, by, cx, cy)
		case CubeTo:
			bx, by := tx(sg.P[0])
			cx, cy := tx(sg.P[1])
			ex, ey := tx(sg.P[2])
			z.CubeTo(bx, by, cx, cy, ex, ey)
		case Close:
			z.ClosePath()
			open = false
		}
	}
	if open {
		z.ClosePath()
	}
}

// ParseSVG parses SVG path data ("M3 18h18v-2H3v2z ..."). All commands are
// supported, including arcs (converted to cubics).
func ParseSVG(d string) (*Path, error) {
	p := &Path{}
	sc := scanner{s: d}
	var cmd byte
	var cx, cy, sx, sy float32 // current point, subpath start
	var lcx, lcy float32       // last control point (for S/T)
	var lastOp byte
	for {
		sc.skipSep()
		if sc.eof() {
			break
		}
		if c := sc.peek(); isCmd(c) {
			cmd = c
			sc.i++
		} else if cmd == 0 {
			return nil, fmt.Errorf("vector: path must start with a command at %d", sc.i)
		}
		rel := cmd >= 'a'
		up := cmd &^ 0x20
		num := func() (float32, error) { return sc.number() }
		pt := func() (float32, float32, error) {
			x, err := num()
			if err != nil {
				return 0, 0, err
			}
			y, err := num()
			if err != nil {
				return 0, 0, err
			}
			if rel {
				x, y = x+cx, y+cy
			}
			return x, y, nil
		}
		switch up {
		case 'M':
			x, y, err := pt()
			if err != nil {
				return nil, err
			}
			p.add(MoveTo, x, y)
			cx, cy, sx, sy = x, y, x, y
			// Subsequent pairs are implicit LineTo.
			if rel {
				cmd = 'l'
			} else {
				cmd = 'L'
			}
		case 'L':
			x, y, err := pt()
			if err != nil {
				return nil, err
			}
			p.add(LineTo, x, y)
			cx, cy = x, y
		case 'H':
			x, err := num()
			if err != nil {
				return nil, err
			}
			if rel {
				x += cx
			}
			p.add(LineTo, x, cy)
			cx = x
		case 'V':
			y, err := num()
			if err != nil {
				return nil, err
			}
			if rel {
				y += cy
			}
			p.add(LineTo, cx, y)
			cy = y
		case 'C', 'S':
			var x1, y1 float32
			if up == 'C' {
				var err error
				if x1, y1, err = pt(); err != nil {
					return nil, err
				}
			} else if lastOp == 'C' || lastOp == 'S' {
				x1, y1 = 2*cx-lcx, 2*cy-lcy
			} else {
				x1, y1 = cx, cy
			}
			x2, y2, err := pt()
			if err != nil {
				return nil, err
			}
			x, y, err := pt()
			if err != nil {
				return nil, err
			}
			p.Segs = append(p.Segs, Seg{Op: CubeTo, P: [3][2]float32{{x1, y1}, {x2, y2}, {x, y}}})
			lcx, lcy, cx, cy = x2, y2, x, y
		case 'Q', 'T':
			var x1, y1 float32
			if up == 'Q' {
				var err error
				if x1, y1, err = pt(); err != nil {
					return nil, err
				}
			} else if lastOp == 'Q' || lastOp == 'T' {
				x1, y1 = 2*cx-lcx, 2*cy-lcy
			} else {
				x1, y1 = cx, cy
			}
			x, y, err := pt()
			if err != nil {
				return nil, err
			}
			p.Segs = append(p.Segs, Seg{Op: QuadTo, P: [3][2]float32{{x1, y1}, {x, y}}})
			lcx, lcy, cx, cy = x1, y1, x, y
		case 'A':
			rx, err := num()
			if err != nil {
				return nil, err
			}
			ry, err := num()
			if err != nil {
				return nil, err
			}
			rot, err := num()
			if err != nil {
				return nil, err
			}
			large, err := sc.flag()
			if err != nil {
				return nil, err
			}
			sweep, err := sc.flag()
			if err != nil {
				return nil, err
			}
			x, y, err := pt()
			if err != nil {
				return nil, err
			}
			p.arc(cx, cy, rx, ry, rot, large, sweep, x, y)
			cx, cy = x, y
		case 'Z':
			p.Segs = append(p.Segs, Seg{Op: Close})
			cx, cy = sx, sy
		default:
			return nil, fmt.Errorf("vector: unknown command %q", cmd)
		}
		lastOp = up
	}
	return p, nil
}

func (p *Path) add(op Op, x, y float32) {
	p.Segs = append(p.Segs, Seg{Op: op, P: [3][2]float32{{x, y}}})
}

// arc converts an SVG elliptical arc to cubic Béziers (SVG spec F.6).
func (p *Path) arc(x1, y1, rx, ry, rotDeg float32, large, sweep bool, x2, y2 float32) {
	if rx == 0 || ry == 0 || (x1 == x2 && y1 == y2) {
		p.add(LineTo, x2, y2)
		return
	}
	fx1, fy1, fx2, fy2 := float64(x1), float64(y1), float64(x2), float64(y2)
	frx, fry := math.Abs(float64(rx)), math.Abs(float64(ry))
	phi := float64(rotDeg) * math.Pi / 180
	cosP, sinP := math.Cos(phi), math.Sin(phi)
	dx, dy := (fx1-fx2)/2, (fy1-fy2)/2
	x1p := cosP*dx + sinP*dy
	y1p := -sinP*dx + cosP*dy
	lambda := x1p*x1p/(frx*frx) + y1p*y1p/(fry*fry)
	if lambda > 1 {
		s := math.Sqrt(lambda)
		frx, fry = frx*s, fry*s
	}
	num := frx*frx*fry*fry - frx*frx*y1p*y1p - fry*fry*x1p*x1p
	den := frx*frx*y1p*y1p + fry*fry*x1p*x1p
	coef := 0.0
	if den != 0 && num > 0 {
		coef = math.Sqrt(num / den)
	}
	if large == sweep {
		coef = -coef
	}
	cxp := coef * frx * y1p / fry
	cyp := -coef * fry * x1p / frx
	cxc := cosP*cxp - sinP*cyp + (fx1+fx2)/2
	cyc := sinP*cxp + cosP*cyp + (fy1+fy2)/2
	ang := func(ux, uy, vx, vy float64) float64 {
		a := math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy)
		return a
	}
	t1 := ang(1, 0, (x1p-cxp)/frx, (y1p-cyp)/fry)
	dt := ang((x1p-cxp)/frx, (y1p-cyp)/fry, (-x1p-cxp)/frx, (-y1p-cyp)/fry)
	if !sweep && dt > 0 {
		dt -= 2 * math.Pi
	} else if sweep && dt < 0 {
		dt += 2 * math.Pi
	}
	n := int(math.Ceil(math.Abs(dt) / (math.Pi / 2)))
	step := dt / float64(n)
	k := 4.0 / 3.0 * math.Tan(step/4)
	pt := func(t float64) (float64, float64, float64, float64) {
		c, s := math.Cos(t), math.Sin(t)
		x := cxc + frx*c*cosP - fry*s*sinP
		y := cyc + frx*c*sinP + fry*s*cosP
		// derivative
		ddx := -frx*s*cosP - fry*c*sinP
		ddy := -frx*s*sinP + fry*c*cosP
		return x, y, ddx, ddy
	}
	t := t1
	for i := 0; i < n; i++ {
		ax, ay, adx, ady := pt(t)
		bx, by, bdx, bdy := pt(t + step)
		if i == n-1 {
			bx, by = fx2, fy2
		}
		p.Segs = append(p.Segs, Seg{Op: CubeTo, P: [3][2]float32{
			{float32(ax + k*adx), float32(ay + k*ady)},
			{float32(bx - k*bdx), float32(by - k*bdy)},
			{float32(bx), float32(by)},
		}})
		t += step
	}
}

// --- scanner ---------------------------------------------------------------

type scanner struct {
	s string
	i int
}

func (s *scanner) eof() bool  { return s.i >= len(s.s) }
func (s *scanner) peek() byte { return s.s[s.i] }

func (s *scanner) skipSep() {
	for !s.eof() {
		switch s.s[s.i] {
		case ' ', ',', '\t', '\n', '\r':
			s.i++
		default:
			return
		}
	}
}

func isCmd(c byte) bool {
	switch c | 0x20 {
	case 'm', 'l', 'h', 'v', 'c', 's', 'q', 't', 'a', 'z':
		return true
	}
	return false
}

func (s *scanner) number() (float32, error) {
	s.skipSep()
	start := s.i
	if !s.eof() && (s.s[s.i] == '-' || s.s[s.i] == '+') {
		s.i++
	}
	dot, exp := false, false
	for !s.eof() {
		c := s.s[s.i]
		switch {
		case c >= '0' && c <= '9':
		case c == '.' && !dot && !exp:
			dot = true
		case (c == 'e' || c == 'E') && !exp:
			exp = true
			if s.i+1 < len(s.s) && (s.s[s.i+1] == '-' || s.s[s.i+1] == '+') {
				s.i++
			}
		default:
			goto done
		}
		s.i++
	}
done:
	if start == s.i {
		return 0, fmt.Errorf("vector: expected number at %d", start)
	}
	v, err := strconv.ParseFloat(s.s[start:s.i], 32)
	return float32(v), err
}

// flag reads an arc flag, which may be written without separators ("a1 1 0 011 1").
func (s *scanner) flag() (bool, error) {
	s.skipSep()
	if s.eof() {
		return false, fmt.Errorf("vector: expected flag")
	}
	c := s.s[s.i]
	s.i++
	switch c {
	case '0':
		return false, nil
	case '1':
		return true, nil
	}
	return false, fmt.Errorf("vector: bad arc flag %q", c)
}

// --- Icons -----------------------------------------------------------------

var nextIconID atomic.Uint32

// Icon is a named vector glyph in a square view box (24×24 for Material).
// Path data is parsed lazily on first draw.
type Icon struct {
	Name string
	View float32 // view box edge length

	id   uint32
	data string
	once sync.Once
	path *Path
}

// NewIcon creates an icon from SVG path data in a view×view box.
func NewIcon(name string, view float32, pathData string) *Icon {
	return &Icon{Name: name, View: view, data: pathData, id: nextIconID.Add(1)}
}

// ID uniquely identifies the icon (atlas cache key).
func (ic *Icon) ID() uint32 { return ic.id }

// Path returns the parsed path (nil if the data was invalid).
func (ic *Icon) Path() *Path {
	ic.once.Do(func() {
		p, err := ParseSVG(ic.data)
		if err == nil {
			ic.path = p
		}
	})
	return ic.path
}
