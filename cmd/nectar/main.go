// Command nectar creates Nectar UI apps.
//
//	go install github.com/minelifes/nectar_ui/cmd/nectar@latest
//
//	nectar new myapp                      # Material 3 app in ./myapp
//	nectar new -template explorer tools/browser
//	nectar new -module github.com/me/notes -title "Notes" -seed "#0B57D0" notes
//	nectar templates                      # list templates
//	nectar build                          # package ./ as a desktop app (+ icon)
//	nectar build -os windows -arch amd64  # cross-build a Windows .exe
//	nectar icon -seed "#0B57D0"           # regenerate assets/icon.png
//
// The new app depends on the engine fetched from GitHub
// (github.com/minelifes/nectar_ui). Use -local to point it at a checkout
// on disk instead, e.g. while working on the engine itself.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "new", "create", "init":
		err = cmdNew(args)
	case "build":
		err = cmdBuild(args)
	case "icon":
		err = cmdIcon(args)
	case "templates", "list":
		for _, t := range Templates {
			fmt.Printf("  %-10s %s\n", t.Name, t.Description)
		}
	case "version", "-v", "--version":
		fmt.Println("nectar", toolVersion())
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "nectar: unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "nectar:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `nectar creates Nectar UI apps.

Usage:
  nectar new [flags] <dir>   create an app in <dir>
  nectar build [flags] [dir] package an app: macOS .app, Windows .exe, Linux + .desktop
  nectar icon [flags]        generate a default app icon (assets/icon.png)
  nectar templates           list the app templates
  nectar version             print the version

Flags for new:
`)
	newFlags(&Options{}).PrintDefaults()
}

func newFlags(o *Options) *flag.FlagSet {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.StringVar(&o.Module, "module", "", "module path of the new app (default: the folder name)")
	fs.StringVar(&o.Template, "template", "material", "app template: "+templateNames())
	fs.StringVar(&o.Title, "title", "", "window title (default: from the folder name)")
	fs.StringVar(&o.Seed, "seed", "", "brand color the theme is generated from, e.g. #0B57D0 (default: M3 baseline #6750A4)")
	fs.StringVar(&o.Version, "version", defaultEngineVersion(), "engine version to fetch: latest, a tag (v0.2.0), a branch or a commit")
	fs.StringVar(&o.Local, "local", "", "use the engine checkout in this folder (adds a replace directive) instead of GitHub")
	fs.BoolVar(&o.NoTidy, "no-tidy", false, "only write files; don't run go get / go mod tidy")
	fs.BoolVar(&o.Git, "git", false, "run git init in the new folder")
	fs.BoolVar(&o.Force, "force", false, "write into a folder that isn't empty (existing files are kept unless the template has them)")
	return fs
}

func cmdIcon(args []string) error {
	fs := flag.NewFlagSet("icon", flag.ContinueOnError)
	seed := fs.String("seed", "", "background color, e.g. #0B57D0 (default: M3 baseline #6750A4)")
	name := fs.String("name", "", "app name; its first letter goes on the icon (default: from nectar.json)")
	out := fs.String("o", "assets/icon.png", "output PNG (1024×1024)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	lit, err := parseSeed(*seed)
	if err != nil {
		return err
	}
	if *name == "" {
		m, err := readManifest(".")
		if err != nil {
			return err
		}
		*name = m.Name
	}
	var c uint32
	fmt.Sscanf(lit, "0x%X", &c)
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	if err := writeIconPNG(*out, defaultIcon(c, *name)); err != nil {
		return err
	}
	fmt.Println("Wrote", *out, "(replace it with your own 1024×1024 PNG any time)")
	return nil
}

func cmdNew(args []string) error {
	var o Options
	fs := newFlags(&o)
	fs.Usage = usage
	// Accept flags before and after the folder name.
	var rest []string
	for {
		if err := fs.Parse(args); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		rest = append(rest, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if len(rest) != 1 {
		return fmt.Errorf("new needs exactly one folder name, got %d (%s)", len(rest), strings.Join(rest, " "))
	}
	o.Dir = rest[0]
	o.Out = os.Stdout
	return Generate(o)
}

// toolVersion is the version this binary was installed at.
func toolVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "(devel)"
}

// defaultEngineVersion matches the engine to the CLI when the CLI was
// installed at a release (go install …/cmd/nectar@v0.3.0 → engine v0.3.0).
func defaultEngineVersion() string {
	if v := toolVersion(); strings.HasPrefix(v, "v") && !strings.Contains(v, "+dirty") {
		return v
	}
	return "latest"
}
