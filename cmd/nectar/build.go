package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Manifest is nectar.json in the app root: what `nectar build` needs to
// package the app.
type Manifest struct {
	Name       string `json:"name"`       // display name ("My App")
	ID         string `json:"id"`         // reverse-DNS id ("com.example.myapp")
	Version    string `json:"version"`    // "1.2.3"
	Executable string `json:"executable"` // binary name ("myapp")
	Icon       string `json:"icon"`       // square PNG, 1024×1024 recommended
	Copyright  string `json:"copyright,omitempty"`
	// Category: macOS LSApplicationCategoryType (e.g.
	// "public.app-category.developer-tools"); on Linux the last part is
	// mapped to a freedesktop category when it matches one.
	Category string `json:"category,omitempty"`
}

var idPartRE = regexp.MustCompile(`[^A-Za-z0-9-]+`)

// bundleID derives a reverse-DNS id from a module path:
// github.com/me/shop → com.github.me.shop, shop → com.example.shop.
func bundleID(module string) string {
	parts := strings.Split(module, "/")
	var out []string
	if host := parts[0]; strings.Contains(host, ".") {
		hp := strings.Split(host, ".")
		for i := len(hp) - 1; i >= 0; i-- {
			out = append(out, hp[i])
		}
		parts = parts[1:]
	} else {
		out = []string{"com", "example"}
	}
	for _, p := range parts {
		if p = strings.Trim(idPartRE.ReplaceAllString(p, "-"), "-"); p != "" {
			out = append(out, strings.ToLower(p))
		}
	}
	return strings.Join(out, ".")
}

// exeName turns a folder name into a binary name.
func exeName(name string) string {
	n := strings.Trim(idPartRE.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if n == "" {
		return "app"
	}
	return n
}

func readManifest(dir string) (Manifest, error) {
	abs, _ := filepath.Abs(dir)
	m := Manifest{}
	data, err := os.ReadFile(filepath.Join(dir, "nectar.json"))
	switch {
	case errors.Is(err, os.ErrNotExist):
		// Older apps: fall back to defaults from the folder name.
	case err != nil:
		return m, err
	default:
		if err := json.Unmarshal(data, &m); err != nil {
			return m, fmt.Errorf("nectar.json: %w", err)
		}
	}
	base := filepath.Base(abs)
	if m.Name == "" {
		m.Name = titleFrom(base)
	}
	if m.Executable == "" {
		m.Executable = exeName(base)
	}
	if m.ID == "" {
		m.ID = bundleID(base)
	}
	if m.Version == "" {
		m.Version = "0.1.0"
	}
	if m.Icon == "" {
		m.Icon = "assets/icon.png"
	}
	return m, nil
}

// BuildOptions configure Build.
type BuildOptions struct {
	Dir     string // app root (with go.mod and nectar.json)
	OS      string // target GOOS; default: this machine
	Arch    string // GOARCH, or "universal" for macOS (amd64 + arm64)
	Out     string // output folder; default <Dir>/dist
	Console bool   // Windows: keep the console window
	Log     io.Writer
}

func cmdBuild(args []string) error {
	var o BuildOptions
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.StringVar(&o.OS, "os", runtime.GOOS, "target OS: darwin, windows, linux")
	fs.StringVar(&o.Arch, "arch", "", "target arch: amd64, arm64 (macOS also: universal; default: universal on macOS, this machine's arch elsewhere)")
	fs.StringVar(&o.Out, "o", "", "output folder (default: dist inside the app)")
	fs.BoolVar(&o.Console, "console", false, "windows: show a console window (for log output)")
	fs.Usage = func() {
		p := newPrinter(os.Stderr)
		p.printf("\n%s %s %s\n\n", p.bold("Usage:"), p.green("nectar build"), p.dim("[flags] [app dir]"))
		p.printf("Packages the app with its icon: macOS .app, Windows .exe, Linux folder with .desktop.\n\n")
		p.flags(fs)
		p.printf("\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	o.Dir = "."
	if fs.NArg() > 0 {
		o.Dir = fs.Arg(0)
	}
	o.Log = os.Stdout
	path, err := Build(o)
	if err != nil {
		return err
	}
	p := newPrinter(os.Stdout)
	p.printf("\n")
	p.success("Built %s", relPath(".", path))
	p.printf("\n")
	return nil
}

// Build packages the app and returns the path of the result.
func Build(o BuildOptions) (string, error) {
	if o.Log == nil {
		o.Log = io.Discard
	}
	if o.OS == "" {
		o.OS = runtime.GOOS
	}
	if o.Arch == "" {
		o.Arch = runtime.GOARCH
		if o.OS == "darwin" {
			o.Arch = "universal"
		}
	}
	if _, err := os.Stat(filepath.Join(o.Dir, "go.mod")); err != nil {
		return "", fmt.Errorf("%s has no go.mod; run nectar build in the app folder", o.Dir)
	}
	m, err := readManifest(o.Dir)
	if err != nil {
		return "", err
	}
	if o.Out == "" {
		o.Out = filepath.Join(o.Dir, "dist")
	}
	if err := os.MkdirAll(o.Out, 0o755); err != nil {
		return "", err
	}
	p := newPrinter(o.Log)
	p.header("build", p.bold(m.Name)+" "+p.dim(m.Version+" for ")+p.cyan(o.OS+"/"+o.Arch))
	icon, err := loadIcon(filepath.Join(o.Dir, m.Icon))
	if errors.Is(err, os.ErrNotExist) {
		p.step("note", m.Icon+" not found; using a generated icon (nectar icon writes one)")
		icon, err = defaultIcon(0x6750A4, m.Name), nil
	}
	if err != nil {
		return "", err
	}
	switch o.OS {
	case "darwin":
		return buildMac(o, m, icon)
	case "windows":
		return buildWindows(o, m, icon)
	case "linux":
		return buildLinux(o, m, icon)
	}
	out := filepath.Join(o.Out, m.Executable)
	return out, goBuild(o, o.OS, o.Arch, out, "")
}

func goBuild(o BuildOptions, goos, goarch, out, ldflags string) error {
	abs, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	args := []string{"build", "-trimpath", "-ldflags", strings.TrimSpace("-s -w " + ldflags), "-o", abs, "."}
	p := newPrinter(o.Log)
	p.step("go", "build "+p.cyan(goos+"/"+goarch)+p.dim(" → "+relPath(o.Dir, abs)))
	cmd := exec.Command("go", args...)
	cmd.Dir = o.Dir
	var log bytes.Buffer
	cmd.Stdout, cmd.Stderr = &log, &log
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+goarch)
	err = cmd.Run()
	if log.Len() > 0 {
		g := &gutter{w: p.w, prefix: "    "}
		io.WriteString(g, p.goErrors(log.String()))
		g.Flush()
	}
	if err != nil {
		return fmt.Errorf("go build for %s/%s failed", goos, goarch)
	}
	return nil
}

// ---------------------------------------------------------------------------
// macOS

func buildMac(o BuildOptions, m Manifest, icon image.Image) (string, error) {
	app := filepath.Join(o.Out, m.Name+".app")
	if err := os.RemoveAll(app); err != nil {
		return "", err
	}
	macos := filepath.Join(app, "Contents", "MacOS")
	res := filepath.Join(app, "Contents", "Resources")
	for _, d := range []string{macos, res} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", err
		}
	}
	bin := filepath.Join(macos, m.Executable)
	if o.Arch == "universal" {
		tmp, err := os.MkdirTemp("", "nectar-build")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(tmp)
		var slices []string
		for _, arch := range []string{"arm64", "amd64"} {
			p := filepath.Join(tmp, arch)
			if err := goBuild(o, "darwin", arch, p, ""); err != nil {
				return "", err
			}
			slices = append(slices, p)
		}
		if err := writeFat(bin, slices); err != nil {
			return "", err
		}
		newPrinter(o.Log).step("lipo", "arm64 + amd64 → universal binary")
	} else if err := goBuild(o, "darwin", o.Arch, bin, ""); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(res, "icon.icns"), icnsBytes(icon), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(infoPlist(m)), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "PkgInfo"), []byte("APPL????"), 0o644); err != nil {
		return "", err
	}
	// Ad-hoc sign so the bundle launches on Apple silicon; use your own
	// identity (codesign -s "Developer ID …") and notarize to distribute.
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("codesign"); err == nil {
			newPrinter(o.Log).step("sign", "codesign --force --deep --sign - "+relPath(o.Dir, app)+" (ad hoc)")
			cmd := exec.Command("codesign", "--force", "--deep", "--sign", "-", app)
			cmd.Stdout, cmd.Stderr = o.Log, o.Log
			if err := cmd.Run(); err != nil {
				return "", fmt.Errorf("codesign: %w", err)
			}
		}
	}
	return app, nil
}

func infoPlist(m Manifest) string {
	esc := html.EscapeString
	var extra strings.Builder
	if m.Copyright != "" {
		fmt.Fprintf(&extra, "\t<key>NSHumanReadableCopyright</key>\n\t<string>%s</string>\n", esc(m.Copyright))
	}
	if m.Category != "" && strings.HasPrefix(m.Category, "public.app-category.") {
		fmt.Fprintf(&extra, "\t<key>LSApplicationCategoryType</key>\n\t<string>%s</string>\n", esc(m.Category))
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>%[1]s</string>
	<key>CFBundleDisplayName</key>
	<string>%[1]s</string>
	<key>CFBundleIdentifier</key>
	<string>%[2]s</string>
	<key>CFBundleVersion</key>
	<string>%[3]s</string>
	<key>CFBundleShortVersionString</key>
	<string>%[3]s</string>
	<key>CFBundleExecutable</key>
	<string>%[4]s</string>
	<key>CFBundleIconFile</key>
	<string>icon</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>LSMinimumSystemVersion</key>
	<string>11.0</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>NSSupportsAutomaticGraphicsSwitching</key>
	<true/>
%[5]s</dict>
</plist>
`, esc(m.Name), esc(m.ID), esc(m.Version), esc(m.Executable), extra.String())
}

// writeFat joins thin Mach-O binaries into a universal ("fat") binary,
// like lipo -create.
func writeFat(out string, thin []string) error {
	type slice struct {
		cpu, sub uint32
		data     []byte
	}
	var sl []slice
	for _, p := range thin {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if len(data) < 12 || binary.LittleEndian.Uint32(data) != 0xFEEDFACF {
			return fmt.Errorf("%s isn't a 64-bit Mach-O binary", p)
		}
		sl = append(sl, slice{binary.LittleEndian.Uint32(data[4:]), binary.LittleEndian.Uint32(data[8:]), data})
	}
	const align = 14 // 2^14 = 16 KiB, what lipo uses for arm64
	var b bytes.Buffer
	be := binary.BigEndian
	binary.Write(&b, be, [2]uint32{0xCAFEBABE, uint32(len(sl))})
	off := uint32(8 + 20*len(sl))
	offsets := make([]uint32, len(sl))
	for i, s := range sl {
		off = (off + (1<<align - 1)) &^ (1<<align - 1)
		offsets[i] = off
		binary.Write(&b, be, [5]uint32{s.cpu, s.sub, off, uint32(len(s.data)), align})
		off += uint32(len(s.data))
	}
	for i, s := range sl {
		b.Write(make([]byte, int(offsets[i])-b.Len()))
		b.Write(s.data)
	}
	return os.WriteFile(out, b.Bytes(), 0o755)
}

// ---------------------------------------------------------------------------
// Windows

func buildWindows(o BuildOptions, m Manifest, icon image.Image) (string, error) {
	syso, err := sysoBytes(icon, m.ID, m.Version, o.Arch)
	if err != nil {
		return "", err
	}
	// The Go linker picks up *_windows_<arch>.syso files in the package;
	// write it only for this build.
	sysoPath := filepath.Join(o.Dir, "nectar_rsrc_windows_"+o.Arch+".syso")
	if err := os.WriteFile(sysoPath, syso, 0o644); err != nil {
		return "", err
	}
	defer os.Remove(sysoPath)
	ld := "-H windowsgui"
	if o.Console {
		ld = ""
	}
	exe := filepath.Join(o.Out, m.Executable+".exe")
	if err := goBuild(o, "windows", o.Arch, exe, ld); err != nil {
		return "", err
	}
	return exe, nil
}

// ---------------------------------------------------------------------------
// Linux

var freedesktopCategories = map[string]string{
	"developer-tools": "Development", "graphics-design": "Graphics", "productivity": "Office",
	"utilities": "Utility", "games": "Game", "education": "Education", "music": "Audio",
	"video": "Video", "photography": "Graphics", "business": "Office", "finance": "Office",
}

func buildLinux(o BuildOptions, m Manifest, icon image.Image) (string, error) {
	dir := filepath.Join(o.Out, m.Executable+"-linux-"+o.Arch)
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := goBuild(o, "linux", o.Arch, filepath.Join(dir, m.Executable), ""); err != nil {
		return "", err
	}
	if err := writeIconPNG(filepath.Join(dir, m.Executable+".png"), scaled(icon, 512)); err != nil {
		return "", err
	}
	cat := "Utility"
	if c, ok := freedesktopCategories[strings.TrimPrefix(m.Category, "public.app-category.")]; ok {
		cat = c
	}
	desktop := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=%s\nExec=%s\nIcon=%s\nCategories=%s;\nTerminal=false\nStartupWMClass=%s\n",
		m.Name, m.Executable, m.Executable, cat, m.Executable)
	if err := os.WriteFile(filepath.Join(dir, m.Executable+".desktop"), []byte(desktop), 0o644); err != nil {
		return "", err
	}
	install := fmt.Sprintf(`#!/bin/sh
# Installs %[1]s for the current user (menu entry + icon).
set -e
cd "$(dirname "$0")"
mkdir -p ~/.local/bin ~/.local/share/applications ~/.local/share/icons/hicolor/512x512/apps
cp %[2]s ~/.local/bin/
cp %[2]s.png ~/.local/share/icons/hicolor/512x512/apps/
sed "s|^Exec=.*|Exec=$HOME/.local/bin/%[2]s|" %[2]s.desktop > ~/.local/share/applications/%[3]s.desktop
echo "Installed %[1]s. It shows up in your app menu."
`, m.Name, m.Executable, m.ID)
	if err := os.WriteFile(filepath.Join(dir, "install.sh"), []byte(install), 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// relPath shows path relative to base when it's inside it.
func relPath(base, path string) string {
	b, err1 := filepath.Abs(base)
	a, err2 := filepath.Abs(path)
	if err1 != nil || err2 != nil {
		return path
	}
	if r, err := filepath.Rel(b, a); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return path
}
