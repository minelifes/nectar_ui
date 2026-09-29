package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"unicode"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// loadIcon reads a PNG (or JPEG) app icon. It should be square, ideally
// 1024×1024; smaller sizes are scaled down from it.
func loadIcon(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	b := img.Bounds()
	if b.Dx() != b.Dy() {
		return nil, fmt.Errorf("%s is %d×%d; the icon must be square (1024×1024 recommended)", path, b.Dx(), b.Dy())
	}
	if b.Dx() < 256 {
		return nil, fmt.Errorf("%s is %d×%d; use at least 256×256 (1024×1024 recommended)", path, b.Dx(), b.Dy())
	}
	return img, nil
}

// scaled returns img resized to n×n.
func scaled(img image.Image, n int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, n, n))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Src, nil)
	return dst
}

func pngBytes(img image.Image) []byte {
	var b bytes.Buffer
	(&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&b, img)
	return b.Bytes()
}

// ---------------------------------------------------------------------------
// macOS .icns: a list of PNG-compressed entries.

func icnsBytes(img image.Image) []byte {
	entries := []struct {
		typ  string
		size int
	}{
		{"icp4", 16}, {"icp5", 32}, {"icp6", 64}, {"ic07", 128}, {"ic08", 256}, {"ic09", 512},
		{"ic10", 1024}, {"ic11", 32}, {"ic12", 64}, {"ic13", 256}, {"ic14", 512},
	}
	cache := map[int][]byte{}
	var body bytes.Buffer
	for _, e := range entries {
		data, ok := cache[e.size]
		if !ok {
			data = pngBytes(scaled(img, e.size))
			cache[e.size] = data
		}
		body.WriteString(e.typ)
		binary.Write(&body, binary.BigEndian, uint32(8+len(data)))
		body.Write(data)
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes()
}

// ---------------------------------------------------------------------------
// Windows: icon images in ICO/resource form, and a COFF object (.syso)
// with RT_ICON, RT_GROUP_ICON and RT_MANIFEST that the Go linker embeds
// into the .exe.

var winIconSizes = []int{16, 24, 32, 48, 64, 128, 256}

// iconImage encodes one icon image: PNG for 256 (Vista+), a 32-bit DIB
// for the smaller sizes (what Explorer handles everywhere).
func iconImage(img image.Image, n int) []byte {
	s := scaled(img, n)
	if n >= 256 {
		return pngBytes(s)
	}
	var b bytes.Buffer
	le := binary.LittleEndian
	maskRow := ((n + 31) / 32) * 4
	// BITMAPINFOHEADER; height counts the XOR image and the AND mask.
	binary.Write(&b, le, uint32(40))
	binary.Write(&b, le, int32(n))
	binary.Write(&b, le, int32(2*n))
	binary.Write(&b, le, uint16(1))  // planes
	binary.Write(&b, le, uint16(32)) // bpp
	binary.Write(&b, le, uint32(0))  // BI_RGB
	binary.Write(&b, le, uint32(n*n*4+maskRow*n))
	binary.Write(&b, le, [4]uint32{})
	for y := n - 1; y >= 0; y-- { // bottom-up BGRA
		for x := 0; x < n; x++ {
			c := s.NRGBAAt(x, y)
			b.Write([]byte{c.B, c.G, c.R, c.A})
		}
	}
	b.Write(make([]byte, maskRow*n)) // AND mask unused: alpha wins
	return b.Bytes()
}

// groupIcon is the GRPICONDIR pointing at RT_ICON ids 1..len(images).
func groupIcon(images [][]byte) []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	binary.Write(&b, le, [3]uint16{0, 1, uint16(len(images))})
	for i, data := range images {
		n := winIconSizes[i]
		dim := byte(n)
		if n >= 256 {
			dim = 0
		}
		b.Write([]byte{dim, dim, 0, 0})
		binary.Write(&b, le, uint16(1))  // planes
		binary.Write(&b, le, uint16(32)) // bpp
		binary.Write(&b, le, uint32(len(data)))
		binary.Write(&b, le, uint16(i+1))
	}
	return b.Bytes()
}

// icoBytes writes a standalone .ico file (same images as the .exe).
func icoBytes(img image.Image) []byte {
	var images [][]byte
	for _, n := range winIconSizes {
		images = append(images, iconImage(img, n))
	}
	var b bytes.Buffer
	le := binary.LittleEndian
	binary.Write(&b, le, [3]uint16{0, 1, uint16(len(images))})
	off := 6 + 16*len(images)
	for i, data := range images {
		dim := byte(winIconSizes[i])
		if winIconSizes[i] >= 256 {
			dim = 0
		}
		b.Write([]byte{dim, dim, 0, 0})
		binary.Write(&b, le, [2]uint16{1, 32})
		binary.Write(&b, le, [2]uint32{uint32(len(data)), uint32(off)})
		off += len(data)
	}
	for _, data := range images {
		b.Write(data)
	}
	return b.Bytes()
}

const winManifest = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity type="win32" name="%s" version="%s"/>
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware>
      <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2</dpiAwareness>
    </windowsSettings>
  </application>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1">
    <application>
      <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
    </application>
  </compatibility>
</assembly>
`

// winVersion4 turns "1.2.3" into the four-part "1.2.3.0" manifests need.
func winVersion4(v string) string {
	var p [4]int
	fmt.Sscanf(v, "%d.%d.%d.%d", &p[0], &p[1], &p[2], &p[3])
	return fmt.Sprintf("%d.%d.%d.%d", p[0], p[1], p[2], p[3])
}

type winRes struct {
	typ, id uint32
	data    []byte
}

// sysoBytes builds a COFF object holding the resources for goarch.
func sysoBytes(img image.Image, id, version, goarch string) ([]byte, error) {
	var machine, relType uint16
	chars := uint16(0x0004) // IMAGE_FILE_LINE_NUMS_STRIPPED
	switch goarch {
	case "amd64":
		machine, relType = 0x8664, 3 // IMAGE_REL_AMD64_ADDR32NB
	case "arm64":
		machine, relType = 0xAA64, 2 // IMAGE_REL_ARM64_ADDR32NB
	case "386":
		machine, relType = 0x014C, 7 // IMAGE_REL_I386_DIR32NB
		chars |= 0x0100              // 32-bit machine
	default:
		return nil, fmt.Errorf("windows/%s isn't supported for icons", goarch)
	}
	const rtIcon, rtGroupIcon, rtManifest = 3, 14, 24
	var res []winRes
	var images [][]byte
	for i, n := range winIconSizes {
		data := iconImage(img, n)
		images = append(images, data)
		res = append(res, winRes{rtIcon, uint32(i + 1), data})
	}
	res = append(res, winRes{rtGroupIcon, 1, groupIcon(images)})
	res = append(res, winRes{rtManifest, 1, []byte(fmt.Sprintf(winManifest, id, winVersion4(version)))})
	section, relocs := rsrcSection(res)

	le := binary.LittleEndian
	var b bytes.Buffer
	const headerSize, sectionHeaderSize, relocSize = 20, 40, 10
	rawOff := headerSize + sectionHeaderSize
	relocOff := rawOff + len(section)
	symOff := relocOff + relocSize*len(relocs)
	if len(relocs) > 0xFFFF {
		return nil, errors.New("too many resources")
	}
	// IMAGE_FILE_HEADER
	binary.Write(&b, le, machine)
	binary.Write(&b, le, uint16(1)) // sections
	binary.Write(&b, le, uint32(0)) // timestamp
	binary.Write(&b, le, uint32(symOff))
	binary.Write(&b, le, uint32(1)) // symbols
	binary.Write(&b, le, uint16(0)) // optional header
	binary.Write(&b, le, chars)
	// IMAGE_SECTION_HEADER
	b.WriteString(".rsrc\x00\x00\x00")
	binary.Write(&b, le, [2]uint32{0, 0}) // virtual size, address
	binary.Write(&b, le, uint32(len(section)))
	binary.Write(&b, le, uint32(rawOff))
	binary.Write(&b, le, uint32(relocOff))
	binary.Write(&b, le, uint32(0)) // line numbers
	binary.Write(&b, le, uint16(len(relocs)))
	binary.Write(&b, le, uint16(0))
	binary.Write(&b, le, uint32(0x40000040)) // initialized data | read
	b.Write(section)
	for _, off := range relocs {
		binary.Write(&b, le, uint32(off))
		binary.Write(&b, le, uint32(0)) // symbol 0: the section
		binary.Write(&b, le, relType)
	}
	// Symbol table: .rsrc (static, section 1), then an empty string table.
	b.WriteString(".rsrc\x00\x00\x00")
	binary.Write(&b, le, uint32(0))
	binary.Write(&b, le, int16(1))
	binary.Write(&b, le, uint16(0))
	b.Write([]byte{3, 0}) // IMAGE_SYM_CLASS_STATIC, no aux
	binary.Write(&b, le, uint32(4))
	return b.Bytes(), nil
}

// rsrcSection lays out the three-level resource directory (type → id →
// language) followed by the data. It returns the section and the offsets
// of the data-entry RVAs, which need relocating.
func rsrcSection(res []winRes) ([]byte, []int) {
	// Group by type; res is already ordered by type then id, which is
	// the ascending order the directory needs.
	var types []uint32
	byType := map[uint32][]int{} // type → indexes into res
	for i, r := range res {
		if _, ok := byType[r.typ]; !ok {
			types = append(types, r.typ)
		}
		byType[r.typ] = append(byType[r.typ], i)
	}
	const dirSize, entrySize, dataEntrySize = 16, 8, 16
	const lang = 0x0409
	// Sizes of each table so we can compute offsets up front.
	rootSize := dirSize + entrySize*len(types)
	off := rootSize
	typeDirOff := map[uint32]int{}
	for _, t := range types {
		typeDirOff[t] = off
		off += dirSize + entrySize*len(byType[t])
	}
	langDirOff := make([]int, len(res))
	for i := range res {
		langDirOff[i] = off
		off += dirSize + entrySize
	}
	dataEntryOff := make([]int, len(res))
	for i := range res {
		dataEntryOff[i] = off
		off += dataEntrySize
	}
	dataOff := make([]int, len(res))
	for i, r := range res {
		off = (off + 7) &^ 7
		dataOff[i] = off
		off += len(r.data)
	}
	buf := make([]byte, off)
	le := binary.LittleEndian
	dir := func(at, n int) {
		le.PutUint16(buf[at+14:], uint16(n)) // id entries
	}
	entry := func(at int, id uint32, target int, subdir bool) {
		le.PutUint32(buf[at:], id)
		v := uint32(target)
		if subdir {
			v |= 0x80000000
		}
		le.PutUint32(buf[at+4:], v)
	}
	dir(0, len(types))
	for ti, t := range types {
		entry(dirSize+ti*entrySize, t, typeDirOff[t], true)
		list := byType[t]
		dir(typeDirOff[t], len(list))
		for j, i := range list {
			entry(typeDirOff[t]+dirSize+j*entrySize, res[i].id, langDirOff[i], true)
		}
	}
	var relocs []int
	for i, r := range res {
		dir(langDirOff[i], 1)
		entry(langDirOff[i]+dirSize, lang, dataEntryOff[i], false)
		le.PutUint32(buf[dataEntryOff[i]:], uint32(dataOff[i])) // RVA, relocated
		le.PutUint32(buf[dataEntryOff[i]+4:], uint32(len(r.data)))
		relocs = append(relocs, dataEntryOff[i])
		copy(buf[dataOff[i]:], r.data)
	}
	return buf, relocs
}

// ---------------------------------------------------------------------------
// Default icon: a rounded square in the brand color with the app's
// initial.

func defaultIcon(seed uint32, name string) image.Image {
	const n = 1024
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	base := color.NRGBA{uint8(seed >> 16), uint8(seed >> 8), uint8(seed), 255}
	light := mix(base, color.NRGBA{255, 255, 255, 255}, 0.28)
	// macOS-style icon grid: 824px body with ~185px corners, centered.
	const inset, radius = 100.0, 185.0
	for y := 0; y < n; y++ {
		t := float64(y) / n
		c := mix(light, base, t)
		for x := 0; x < n; x++ {
			a := roundRectCoverage(float64(x)+0.5, float64(y)+0.5, inset, inset, n-inset, n-inset, radius)
			if a > 0 {
				c2 := c
				c2.A = uint8(a * 255)
				img.SetNRGBA(x, y, c2)
			}
		}
	}
	letter := initial(name)
	if letter == "" {
		return img
	}
	f, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return img
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 520, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return img
	}
	defer face.Close()
	d := &font.Drawer{Dst: img, Src: image.NewUniform(color.NRGBA{255, 255, 255, 245}), Face: face}
	bounds, _ := d.BoundString(letter)
	w := (bounds.Max.X - bounds.Min.X).Ceil()
	h := (bounds.Max.Y - bounds.Min.Y).Ceil()
	x := (n-w)/2 - bounds.Min.X.Floor()
	y := (n-h)/2 - bounds.Min.Y.Floor()
	d.Dot = fixed.P(x, y)
	d.DrawString(letter)
	return img
}

func initial(name string) string {
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return string(unicode.ToUpper(r))
		}
	}
	return ""
}

func mix(a, b color.NRGBA, t float64) color.NRGBA {
	l := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return color.NRGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 255}
}

// roundRectCoverage is an anti-aliased inside test for a rounded rect.
func roundRectCoverage(px, py, x0, y0, x1, y1, r float64) float64 {
	cx := math.Max(x0+r, math.Min(px, x1-r))
	cy := math.Max(y0+r, math.Min(py, y1-r))
	d := math.Hypot(px-cx, py-cy) - r
	if px < x0 || px > x1 || py < y0 || py > y1 {
		d = math.Max(d, 1)
	}
	return math.Max(0, math.Min(1, 0.5-d))
}

// writeIconPNG writes img as a PNG file.
func writeIconPNG(path string, img image.Image) error {
	return os.WriteFile(path, pngBytes(img), 0o644)
}

// welcomeImage draws a simple landscape in the brand color (sky, sun,
// three mountain ridges) as a sample picture for new apps.
func welcomeImage(seed uint32, w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	base := color.NRGBA{uint8(seed >> 16), uint8(seed >> 8), uint8(seed), 255}
	white := color.NRGBA{255, 255, 255, 255}
	skyTop, skyLow := mix(base, white, 0.55), mix(base, white, 0.9)
	sunC := mix(color.NRGBA{255, 214, 120, 255}, white, 0.2)
	sx, sy, sr := float64(w)*0.72, float64(h)*0.38, float64(h)*0.16
	ridges := []struct {
		base, amp, freq, phase float64
		c                      color.NRGBA
	}{
		{0.62, 0.10, 2.1, 0.3, mix(base, white, 0.45)},
		{0.74, 0.08, 3.3, 1.7, mix(base, white, 0.2)},
		{0.86, 0.06, 4.7, 4.1, mix(base, color.NRGBA{0, 0, 0, 255}, 0.25)},
	}
	for y := 0; y < h; y++ {
		fy := float64(y) + 0.5
		sky := mix(skyTop, skyLow, fy/float64(h))
		for x := 0; x < w; x++ {
			fx := float64(x) + 0.5
			c := sky
			// Sun with a soft glow.
			if d := math.Hypot(fx-sx, fy-sy); d < sr*2.2 {
				if d < sr {
					c = mix(c, sunC, math.Min(1, sr-d))
				} else {
					c = mix(c, sunC, 0.35*(1-(d-sr)/(sr*1.2))*0.6)
				}
			}
			for _, r := range ridges {
				t := fx / float64(w)
				top := (r.base - r.amp*(math.Sin(t*r.freq*math.Pi+r.phase)*0.6+math.Sin(t*r.freq*2.3*math.Pi+r.phase*1.7)*0.4)) * float64(h)
				if a := fy - top + 0.5; a > 0 {
					c = mix(c, r.c, math.Min(1, a))
				}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}
