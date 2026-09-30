package main

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"
)

func TestColorOnlyForTerminals(t *testing.T) {
	t.Setenv("FORCE_COLOR", "")
	os.Unsetenv("FORCE_COLOR")
	t.Setenv("CLICOLOR_FORCE", "")
	var buf bytes.Buffer
	if colorFor(&buf) {
		t.Error("colors into a buffer")
	}
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if colorFor(f) {
		t.Error("colors into a regular file")
	}
	t.Setenv("FORCE_COLOR", "1")
	if !colorFor(&buf) {
		t.Error("FORCE_COLOR ignored")
	}
	t.Setenv("FORCE_COLOR", "0")
	if colorFor(&buf) {
		t.Error("FORCE_COLOR=0 forces colors")
	}
}

func TestNoColorWins(t *testing.T) {
	os.Unsetenv("FORCE_COLOR")
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("NO_COLOR", "1")
	if colorFor(os.Stdout) {
		t.Error("NO_COLOR ignored")
	}
}

func TestPaletteOffIsPlain(t *testing.T) {
	var p palette
	for _, s := range []string{p.bold("x"), p.red("x"), p.honey("x"), p.dim("x")} {
		if s != "x" {
			t.Errorf("styled without color: %q", s)
		}
	}
	if p.badge("nectar dev") != "nectar dev" || p.sym("✓", "ok") != "ok" {
		t.Error("badge / symbol not plain")
	}
	on := palette{on: true}
	if got := on.green("ok"); got != "\x1b[32mok\x1b[0m" {
		t.Errorf("green = %q", got)
	}
	if on.red("") != "" {
		t.Error("empty text got escapes")
	}
}

func TestGoErrorsHighlighting(t *testing.T) {
	out := "# example.com/app\n./main.go:12:3: undefined: foo\ninternal/app/app.go:7: syntax error\nnote: module requires go 1.26"
	plain := palette{}.goErrors(out)
	if plain != out+"\n" {
		t.Errorf("plain output changed:\n%s", plain)
	}
	c := palette{on: true}.goErrors(out)
	for _, want := range []string{
		"\x1b[2m# example.com/app\x1b[0m",
		"\x1b[36m./main.go\x1b[0m", "\x1b[33m12:3\x1b[0m", "\x1b[31m undefined: foo\x1b[0m",
		"\x1b[36minternal/app/app.go\x1b[0m", "\x1b[33m7\x1b[0m",
		"note: module requires go 1.26\n",
	} {
		if !strings.Contains(c, want) {
			t.Errorf("missing %q in\n%q", want, c)
		}
	}
}

func TestGutterPrefixesWholeLines(t *testing.T) {
	var buf bytes.Buffer
	g := &gutter{w: &buf, prefix: "| "}
	g.Write([]byte("hel"))
	if buf.Len() != 0 {
		t.Fatal("partial line written early")
	}
	g.Write([]byte("lo\r\nworld\nlast"))
	g.Flush()
	if got := buf.String(); got != "| hello\n| world\n| last\n" {
		t.Fatalf("gutter output %q", got)
	}
	buf.Reset()
	g = &gutter{w: &buf, prefix: "> ", style: strings.ToUpper}
	g.Write([]byte("go: downloading x\n"))
	if buf.String() != "> GO: DOWNLOADING X\n" {
		t.Fatalf("styled gutter %q", buf.String())
	}
}

func TestPrinterPlainOutput(t *testing.T) {
	var buf bytes.Buffer
	p := newPrinter(&buf)
	p.header("new", "notes")
	p.step("create", "go.mod")
	p.step("run", "go mod tidy")
	p.success("Created %s", "Notes")
	p.commands("Next", [][2]string{{"cd notes", "go to the app"}, {"nectar dev", "run it"}})
	want := "\nnectar new notes\n\n" +
		"  create  go.mod\n" +
		"  run     go mod tidy\n" +
		"ok Created Notes\n" +
		"\nNext\n\n" +
		"  cd notes    # go to the app\n" +
		"  nectar dev  # run it\n\n"
	if buf.String() != want {
		t.Fatalf("got\n%q\nwant\n%q", buf.String(), want)
	}
}

func TestFlagsListing(t *testing.T) {
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	fs.String("seed", "", "brand `color`")
	fs.Bool("git", false, "run git init")
	fs.String("template", "material", "app template")
	var buf bytes.Buffer
	(&printer{w: &buf}).flags(fs)
	want := "  -git\n      run git init\n" +
		"  -seed color\n      brand color\n" +
		"  -template string\n      app template (default material)\n"
	if buf.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", buf.String(), want)
	}
}

func TestDevEventsAndFiles(t *testing.T) {
	old := clock
	clock = func() string { return "12:00:00" }
	defer func() { clock = old }()
	var buf bytes.Buffer
	s := &devSession{p: newPrinter(&buf)}
	s.event(evOK, "restarted in %s", "840ms")
	s.event(evFail, "build failed")
	s.event(evChange, "changed %s", s.files([]string{"a.go", "b.go", "c.go", "d.go", "e.go"}))
	want := "12:00:00 + restarted in 840ms\n" +
		"12:00:00 x build failed\n" +
		"12:00:00 ~ changed a.go, b.go, c.go +2 more\n"
	if buf.String() != want {
		t.Fatalf("got\n%q\nwant\n%q", buf.String(), want)
	}
}
