package text

import (
	"fmt"
	"sync"
)

// FontWeight is a font's thickness, 100 (thin) to 900 (black), as in CSS
// and Flutter's FontWeight. 0 means "not set": a Style inherits it, and
// it's drawn as Normal.
type FontWeight int

const (
	Thin       FontWeight = 100
	ExtraLight FontWeight = 200
	Light      FontWeight = 300
	Normal     FontWeight = 400 // regular
	Medium     FontWeight = 500
	SemiBold   FontWeight = 600
	Bold       FontWeight = 700
	ExtraBold  FontWeight = 800
	Black      FontWeight = 900
)

// W100 … W900 are the numeric names (Flutter's FontWeight.w100 …).
const (
	W100 = Thin
	W200 = ExtraLight
	W300 = Light
	W400 = Normal
	W500 = Medium
	W600 = SemiBold
	W700 = Bold
	W800 = ExtraBold
	W900 = Black
)

func (w FontWeight) String() string {
	switch w {
	case 0:
		return "unset"
	case Thin:
		return "thin"
	case ExtraLight:
		return "extra-light"
	case Light:
		return "light"
	case Normal:
		return "normal"
	case Medium:
		return "medium"
	case SemiBold:
		return "semi-bold"
	case Bold:
		return "bold"
	case ExtraBold:
		return "extra-bold"
	case Black:
		return "black"
	}
	return fmt.Sprintf("w%d", int(w))
}

// orNormal is w, or Normal when unset.
func (w FontWeight) orNormal() FontWeight {
	if w <= 0 {
		return Normal
	}
	return w
}

// Family is a typeface in several weights (one font file per weight):
//
//	comfortaa := text.NewFamily("Comfortaa").
//	    Add(text.Light, light).
//	    Add(text.Normal, regular).
//	    Add(text.Bold, bold)
//	style := text.Style{Font: comfortaa.Font(), Weight: text.Bold}
//
// A Style picks the face closest to its Weight the way CSS does. A bold
// weight (600+) that's 200 or more above the boldest face is drawn with a
// synthetic bold of that face, so text.Style{Weight: text.Bold} always
// looks bold, even with a single-weight font.
type Family struct {
	name  string
	mu    sync.Mutex
	faces map[FontWeight]*Font
}

// NewFamily returns an empty family.
func NewFamily(name string) *Family {
	return &Family{name: name, faces: map[FontWeight]*Font{}}
}

// Name returns the family name.
func (fam *Family) Name() string { return fam.name }

// Add registers f as the family's face for weight w and returns fam. A
// font belongs to one family: adding it to another moves it.
func (fam *Family) Add(w FontWeight, f *Font) *Family {
	if f == nil {
		return fam
	}
	f = f.plain()
	fam.mu.Lock()
	fam.faces[w.orNormal()] = f
	fam.mu.Unlock()
	f.family, f.weight = fam, w.orNormal()
	return fam
}

// Font returns the face for Normal (or the closest one): the font to put
// in a Style. nil if the family is empty.
func (fam *Family) Font() *Font { return fam.Match(Normal) }

// Match returns the face closest to w (CSS font matching): for 400, 500
// then lighter then bolder; for 500, 400 then lighter then bolder; below
// 400, lighter first, then bolder; above 500, bolder first, then lighter.
func (fam *Family) Match(w FontWeight) *Font {
	w = w.orNormal()
	fam.mu.Lock()
	defer fam.mu.Unlock()
	if f, ok := fam.faces[w]; ok {
		return f
	}
	var best *Font
	bestScore := 1 << 30
	for fw, f := range fam.faces {
		if s := matchScore(w, fw); s < bestScore || (s == bestScore && fw < best.weight) {
			best, bestScore = f, s
		}
	}
	return best
}

// matchScore ranks face weight fw for a wanted weight w (lower is better).
func matchScore(w, fw FontWeight) int {
	d := int(fw - w)
	ad := max(d, -d)
	switch {
	case w == Normal && fw == Medium, w == Medium && fw == Normal:
		return 0
	case w <= Medium: // lighter first
		if d < 0 {
			return ad
		}
		return 1000 + ad
	default: // bolder first
		if d > 0 {
			return ad
		}
		return 1000 + ad
	}
}

// Family returns the family f was added to, or nil.
func (f *Font) Family() *Family { return f.plain().family }

// Weight returns the weight f was registered with (Family.Add), Normal for
// a font outside a family; a synthetic bold reports the weight it draws.
func (f *Font) Weight() FontWeight {
	if f.synthetic > 0 {
		return f.synthetic
	}
	return f.plain().weight.orNormal()
}

// plain returns the real face behind f: f itself, the font a WithFallback
// font was made from, or the font a synthetic bold was made from.
func (f *Font) plain() *Font {
	if f.orig != nil {
		return f.orig
	}
	return f.Face()
}

// weightKey caches WithWeight results so styles keep comparing equal
// (fonts compare by pointer).
type weightKey struct {
	f *Font
	w FontWeight
}

var (
	weightMu    sync.Mutex
	weightCache = map[weightKey]*Font{}
)

// WithWeight returns the font to draw f's text at weight w: the closest
// face of f's family, made synthetically bold when the family has nothing
// bold enough (see Family). f's fallback chain is kept. Results are
// cached, so the same (f, w) always gives the same *Font.
func (f *Font) WithWeight(w FontWeight) *Font {
	if f == nil || w <= 0 {
		return f
	}
	key := weightKey{f, w}
	weightMu.Lock()
	if r, ok := weightCache[key]; ok {
		weightMu.Unlock()
		return r
	}
	weightMu.Unlock()

	base := f.plain()
	best := base
	if fam := base.family; fam != nil {
		if m := fam.Match(w); m != nil {
			best = m
		}
	}
	out := best
	if bw := best.weight.orNormal(); w >= SemiBold && w-bw >= 200 {
		out = best.emboldened(w)
	}
	if len(f.fallback) > 0 && out != f {
		out = out.WithFallback(f.fallback...)
	}
	weightMu.Lock()
	if r, ok := weightCache[key]; ok { // lost a race: keep the first
		out = r
	} else {
		weightCache[key] = out
	}
	weightMu.Unlock()
	return out
}

// emboldened returns f drawn synthetically at weight w: each glyph is
// thickened by a fraction of the em and advances that much further.
// Cached per (f, w).
func (f *Font) emboldened(w FontWeight) *Font {
	key := weightKey{f, -w}
	weightMu.Lock()
	defer weightMu.Unlock()
	if r, ok := weightCache[key]; ok {
		return r
	}
	c := *f
	c.id = nextFontID.Add(1) // its own atlas entries
	c.orig = f
	c.synthetic = w
	// Normal → Bold thickens by 1/24 em, about what a real bold adds.
	c.embolden = float32(w-f.weight.orNormal()) / 300 / 24
	c.fallback = nil
	c.face = &c
	weightCache[key] = &c
	return &c
}
