package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Terminal output: colors, symbols and layout shared by every command.
//
// Colors are used only when the output is a terminal, NO_COLOR is unset
// and TERM isn't "dumb"; FORCE_COLOR (or CLICOLOR_FORCE) turns them on
// anyway, e.g. under a CI log viewer that renders ANSI.

// colorFor reports whether ANSI colors should be written to w.
func colorFor(w io.Writer) bool {
	if v, ok := os.LookupEnv("FORCE_COLOR"); ok && v != "0" && v != "false" {
		return true
	}
	if v := os.Getenv("CLICOLOR_FORCE"); v != "" && v != "0" {
		return true
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok || os.Getenv("TERM") == "dumb" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil || st.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	return enableVT(f)
}

// palette is a set of styles; the zero value (no color) returns text as is.
type palette struct{ on bool }

func (p palette) sgr(code, s string) string {
	if !p.on || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (p palette) bold(s string) string    { return p.sgr("1", s) }
func (p palette) dim(s string) string     { return p.sgr("2", s) }
func (p palette) red(s string) string     { return p.sgr("31", s) }
func (p palette) green(s string) string   { return p.sgr("32", s) }
func (p palette) yellow(s string) string  { return p.sgr("33", s) }
func (p palette) blue(s string) string    { return p.sgr("34", s) }
func (p palette) magenta(s string) string { return p.sgr("35", s) }
func (p palette) cyan(s string) string    { return p.sgr("36", s) }

// honey is the nectar brand color (256-color amber).
func (p palette) honey(s string) string { return p.sgr("1;38;5;214", s) }

// badge is white-on-honey, for the command name at the top of a run.
func (p palette) badge(s string) string {
	if !p.on {
		return s
	}
	return p.sgr("1;38;5;232;48;5;214", " "+s+" ")
}

// Symbols, with ASCII fallbacks when there's no color (logs, old consoles).
func (p palette) sym(fancy, plain string) string {
	if p.on {
		return fancy
	}
	return plain
}

// printer writes styled lines to a writer.
type printer struct {
	w io.Writer
	palette
}

func newPrinter(w io.Writer) *printer {
	if w == nil {
		w = io.Discard
	}
	return &printer{w: w, palette: palette{on: colorFor(w)}}
}

func (p *printer) printf(format string, args ...any) { fmt.Fprintf(p.w, format, args...) }

// header starts a command: "▌ nectar new  notes (material template)".
func (p *printer) header(cmd, detail string) {
	p.printf("\n%s %s\n\n", p.badge("nectar "+cmd), detail)
}

// step is one indented action line: "  create  internal/app/app.go".
// The verb picks its color.
func (p *printer) step(verb, detail string) {
	v := fmt.Sprintf("%-7s", verb)
	switch verb {
	case "create", "write":
		v = p.green(v)
		if i := strings.LastIndexByte(detail, '/'); i >= 0 {
			detail = p.dim(detail[:i+1]) + detail[i+1:]
		}
	case "run":
		v = p.cyan(v)
		detail = p.dim(detail)
	case "note", "skip":
		v = p.yellow(v)
	default:
		v = p.magenta(v)
	}
	p.printf("  %s %s\n", v, detail)
}

func (p *printer) success(format string, args ...any) {
	p.printf("%s %s\n", p.green(p.sym("✓", "ok")), p.bold(fmt.Sprintf(format, args...)))
}

func (p *printer) warn(format string, args ...any) {
	p.printf("%s %s\n", p.yellow(p.sym("!", "warning:")), fmt.Sprintf(format, args...))
}

func (p *printer) fail(format string, args ...any) {
	p.printf("%s %s\n", p.red(p.sym("✗", "error:")), p.red(fmt.Sprintf(format, args...)))
}

// commands prints "next steps": each line a command and a dim comment.
func (p *printer) commands(title string, lines [][2]string) {
	p.printf("\n%s\n\n", p.bold(title))
	width := 0
	for _, l := range lines {
		width = max(width, len(l[0]))
	}
	for _, l := range lines {
		p.printf("  %s%s  %s\n", p.cyan(l[0]), strings.Repeat(" ", width-len(l[0])), p.dim("# "+l[1]))
	}
	p.printf("\n")
}

// flags prints a FlagSet's flags: name and type in color, default dim.
func (p *printer) flags(fs *flag.FlagSet) {
	fs.VisitAll(func(f *flag.Flag) {
		name, usage := flag.UnquoteUsage(f)
		head := "  " + p.cyan("-"+f.Name)
		if name != "" {
			head += " " + p.dim(name)
		}
		p.printf("%s\n      %s", head, usage)
		if f.DefValue != "" && f.DefValue != "false" {
			p.printf(" %s", p.dim("(default "+f.DefValue+")"))
		}
		p.printf("\n")
	})
}

// --- Go compiler output ------------------------------------------------------

var goErrRE = regexp.MustCompile(`^(\s*)(\S+?\.go):(\d+)(:\d+)?:(.*)$`)

// goErrors styles `go build` output: "file.go:12:3: message" gets the
// location in cyan/yellow, "# package" headers dim, the rest as is.
func (p palette) goErrors(out string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		switch m := goErrRE.FindStringSubmatch(line); {
		case m != nil:
			b.WriteString(m[1] + p.cyan(m[2]) + p.dim(":") + p.yellow(m[3]+m[4]) + p.dim(":") + p.red(m[5]))
		case strings.HasPrefix(line, "#"):
			b.WriteString(p.dim(line))
		default:
			b.WriteString(line)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// --- gutter writer -------------------------------------------------------------

// gutter prefixes every line written through it (the running app's own
// output under nectar dev), so it stands apart from nectar's lines. Safe
// for concurrent use; a partial line is held until its newline arrives.
type gutter struct {
	mu     sync.Mutex
	w      io.Writer
	prefix string
	style  func(string) string // applied to each line's text (optional)
	buf    []byte
}

func (g *gutter) line(s string) string {
	if g.style != nil {
		s = g.style(s)
	}
	return g.prefix + s + "\n"
}

func (g *gutter) Write(b []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.buf = append(g.buf, b...)
	for {
		i := bytes.IndexByte(g.buf, '\n')
		if i < 0 {
			break
		}
		if _, err := io.WriteString(g.w, g.line(strings.TrimSuffix(string(g.buf[:i]), "\r"))); err != nil {
			return len(b), err
		}
		g.buf = g.buf[i+1:]
	}
	return len(b), nil
}

// Flush writes a trailing partial line (the app exited mid-line).
func (g *gutter) Flush() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.buf) > 0 {
		io.WriteString(g.w, g.line(string(g.buf)))
		g.buf = nil
	}
}

// clock is the time shown before dev log lines (a var for tests).
var clock = func() string { return time.Now().Format("15:04:05") }
