package main

import (
	"bytes"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundleIDAndExe(t *testing.T) {
	for in, want := range map[string]string{
		"github.com/me/shop": "com.github.me.shop", "shop": "com.example.shop",
		"example.com/My_App": "com.example.my-app", "gitlab.acme.io/x/y": "io.acme.gitlab.x.y",
	} {
		if got := bundleID(in); got != want {
			t.Errorf("bundleID(%q) = %q, want %q", in, got, want)
		}
	}
	if got := exeName("My Cool_App"); got != "my-cool-app" {
		t.Errorf("exeName = %q", got)
	}
}

func TestDefaultIcon(t *testing.T) {
	img := defaultIcon(0x0B57D0, "shop")
	if b := img.Bounds(); b.Dx() != 1024 || b.Dy() != 1024 {
		t.Fatalf("size %v", b)
	}
	// Corners transparent, center-ish opaque, and the letter is white.
	if _, _, _, a := img.At(5, 5).RGBA(); a != 0 {
		t.Error("corner not transparent")
	}
	if _, _, _, a := img.At(512, 150).RGBA(); a != 0xFFFF {
		t.Error("body not opaque")
	}
	white := 0
	for x := 300; x < 724; x++ {
		if r, g, b, _ := img.At(x, 512).RGBA(); r > 0xF000 && g > 0xF000 && b > 0xF000 {
			white++
		}
	}
	if white == 0 {
		t.Error("no letter drawn")
	}
}

func TestICNS(t *testing.T) {
	data := icnsBytes(defaultIcon(0x6750A4, "A"))
	if string(data[:4]) != "icns" || int(binary.BigEndian.Uint32(data[4:])) != len(data) {
		t.Fatal("bad header")
	}
	types := map[string]bool{}
	for off := 8; off < len(data); {
		typ, n := string(data[off:off+4]), int(binary.BigEndian.Uint32(data[off+4:]))
		if !bytes.HasPrefix(data[off+8:], []byte("\x89PNG")) {
			t.Errorf("%s isn't PNG", typ)
		}
		types[typ] = true
		off += n
	}
	for _, want := range []string{"icp4", "ic07", "ic10", "ic14"} {
		if !types[want] {
			t.Errorf("missing %s", want)
		}
	}
}

func TestICO(t *testing.T) {
	data := icoBytes(defaultIcon(0x6750A4, "A"))
	if binary.LittleEndian.Uint16(data[2:]) != 1 || int(binary.LittleEndian.Uint16(data[4:])) != len(winIconSizes) {
		t.Fatal("bad ICONDIR")
	}
}

// writeHello makes a stdlib-only main package to link resources into.
func writeHello(t *testing.T) string {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module hello\n\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() { println(\"hi\") }\n"), 0o644)
	return dir
}

func goBuildTo(t *testing.T, dir, goos, goarch, out string, ldflags string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-ldflags", ldflags, "-o", out, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+goarch, "GOFLAGS=-mod=mod", "GOWORK=off")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s/%s: %v\n%s", goos, goarch, err, b)
	}
}

// TestWindowsResourcesLink links the .syso into a real Windows binary and
// reads the icon and manifest back out of the .exe's resource section.
func TestWindowsResourcesLink(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go")
	}
	for _, arch := range []string{"amd64", "arm64", "386"} {
		t.Run(arch, func(t *testing.T) {
			dir := writeHello(t)
			syso, err := sysoBytes(defaultIcon(0x006A6A, "T"), "com.example.hello", "1.2.3", arch)
			if err != nil {
				t.Fatal(err)
			}
			// The .syso itself is a valid COFF object.
			if f, err := pe.NewFile(bytes.NewReader(syso)); err != nil || f.Section(".rsrc") == nil {
				t.Fatalf("COFF: %v", err)
			}
			os.WriteFile(filepath.Join(dir, "rsrc_windows_"+arch+".syso"), syso, 0o644)
			exe := filepath.Join(dir, "hello.exe")
			goBuildTo(t, dir, "windows", arch, exe, "-H windowsgui")

			f, err := pe.Open(exe)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			rs := f.Section(".rsrc")
			if rs == nil {
				t.Fatal("no .rsrc in the exe")
			}
			raw, _ := rs.Data()
			le := binary.LittleEndian
			// Walk type → id → lang → data entry; return the resource bytes.
			find := func(typ, id uint32) []byte {
				lookup := func(dirOff int, want uint32) (uint32, bool) {
					n := int(le.Uint16(raw[dirOff+12:])) + int(le.Uint16(raw[dirOff+14:]))
					for i := 0; i < n; i++ {
						e := dirOff + 16 + 8*i
						if want == 0 || le.Uint32(raw[e:]) == want {
							return le.Uint32(raw[e+4:]), true
						}
					}
					return 0, false
				}
				o1, ok := lookup(0, typ)
				if !ok {
					return nil
				}
				o2, ok := lookup(int(o1&0x7FFFFFFF), id)
				if !ok {
					return nil
				}
				o3, ok := lookup(int(o2&0x7FFFFFFF), 0)
				if !ok {
					return nil
				}
				rva, size := le.Uint32(raw[o3:]), le.Uint32(raw[o3+4:])
				start := int(rva - rs.VirtualAddress)
				if start < 0 || start+int(size) > len(raw) {
					t.Fatalf("resource %d/%d RVA %#x outside .rsrc (not relocated?)", typ, id, rva)
				}
				return raw[start : start+int(size)]
			}
			grp := find(14, 1)
			if grp == nil || le.Uint16(grp[4:]) != uint16(len(winIconSizes)) {
				t.Fatalf("RT_GROUP_ICON: %x", grp)
			}
			big := find(3, uint32(len(winIconSizes))) // the 256px image
			if !bytes.HasPrefix(big, []byte("\x89PNG")) {
				t.Fatal("256px icon isn't PNG")
			}
			if _, err := png.Decode(bytes.NewReader(big)); err != nil {
				t.Fatal(err)
			}
			small := find(3, 1)
			if len(small) < 40 || le.Uint32(small) != 40 || int32(le.Uint32(small[4:])) != 16 {
				t.Fatal("16px icon isn't a 16×16 DIB")
			}
			if man := find(24, 1); !strings.Contains(string(man), "PerMonitorV2") || !strings.Contains(string(man), `version="1.2.3.0"`) {
				t.Fatalf("manifest: %s", man)
			}
		})
	}
}

func TestUniversalMachO(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go")
	}
	dir := writeHello(t)
	var thin []string
	for _, arch := range []string{"arm64", "amd64"} {
		p := filepath.Join(dir, "hello-"+arch)
		goBuildTo(t, dir, "darwin", arch, p, "-s -w")
		thin = append(thin, p)
	}
	out := filepath.Join(dir, "hello")
	if err := writeFat(out, thin); err != nil {
		t.Fatal(err)
	}
	ff, err := macho.OpenFat(out)
	if err != nil {
		t.Fatal(err)
	}
	defer ff.Close()
	cpus := map[macho.Cpu]bool{}
	for _, a := range ff.Arches {
		cpus[a.Cpu] = true
		if a.Offset%(1<<14) != 0 {
			t.Errorf("%v slice not 16K aligned", a.Cpu)
		}
	}
	if !cpus[macho.CpuArm64] || !cpus[macho.CpuAmd64] {
		t.Fatalf("arches %v", cpus)
	}
}

func TestInfoPlist(t *testing.T) {
	p := infoPlist(Manifest{Name: "R&D <Tool>", ID: "com.x.y", Version: "2.0.1", Executable: "rd", Category: "public.app-category.utilities"})
	for _, want := range []string{"<string>R&amp;D &lt;Tool&gt;</string>", "<string>com.x.y</string>", "<string>rd</string>", "LSApplicationCategoryType", "<string>icon</string>"} {
		if !strings.Contains(p, want) {
			t.Errorf("Info.plist missing %s", want)
		}
	}
}

func TestReadManifestDefaults(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "note_pad")
	os.MkdirAll(dir, 0o755)
	m, err := readManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "Note Pad" || m.Executable != "note-pad" || m.Version != "0.1.0" || m.Icon != "assets/icon.png" {
		t.Fatalf("%+v", m)
	}
	os.WriteFile(filepath.Join(dir, "nectar.json"), []byte(`{"name":"Pad","version":"3.1.0"}`), 0o644)
	if m, _ = readManifest(dir); m.Name != "Pad" || m.Version != "3.1.0" || m.Executable != "note-pad" {
		t.Fatalf("%+v", m)
	}
}
