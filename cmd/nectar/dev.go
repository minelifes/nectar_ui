package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/minelifes/nectar_ui/ui/hotreload"
)

// DevOptions configures Dev.
type DevOptions struct {
	Dir  string   // app folder: the module root, watched and run from here
	Pkg  string   // package to build, relative to Dir (default ".")
	Args []string // passed to the app
	// Ext lists the file extensions whose changes trigger a rebuild
	// (default .go, .mod, .sum).
	Ext []string
	// Poll is how often files are checked (default 300ms).
	Poll time.Duration
	Log  io.Writer
	// Stop ends the session (tests); Ctrl+C does too.
	Stop <-chan struct{}
}

func cmdDev(args []string) error {
	var o DevOptions
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	fs.StringVar(&o.Pkg, "pkg", ".", "package to run, relative to the app folder")
	ext := fs.String("ext", ".go,.mod,.sum", "comma-separated extensions whose changes rebuild the app")
	fs.Usage = func() {
		p := newPrinter(os.Stderr)
		p.printf("\n%s %s %s\n\n", p.bold("Usage:"), p.green("nectar dev"), p.dim("[flags] [app dir] [-- app args]"))
		p.printf(`Runs the app and restarts it whenever its code changes, keeping the window
size and the state registered with %s / %s.
Files mounted with %s reload without a restart.
A build error keeps the running app; fix it and save again.

`, p.cyan("hotreload.Keep"), p.cyan("mvvm Property.Keep"), p.cyan("ui.Config.WithDevResources"))
		p.flags(fs)
		p.printf("\n")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	o.Dir = "."
	if len(rest) > 0 && rest[0] != "--" {
		o.Dir, rest = rest[0], rest[1:]
	}
	if len(rest) > 0 && rest[0] == "--" {
		rest = rest[1:]
	}
	o.Args = rest
	for _, e := range strings.Split(*ext, ",") {
		if e = strings.TrimSpace(e); e != "" {
			o.Ext = append(o.Ext, e)
		}
	}
	o.Log = os.Stderr
	return Dev(o)
}

// Dev builds and runs the app, rebuilding and restarting it on changes.
// It returns when the app exits by itself (the window was closed), on
// Ctrl+C, or when o.Stop closes.
func Dev(o DevOptions) error {
	if o.Pkg == "" {
		o.Pkg = "."
	}
	if len(o.Ext) == 0 {
		o.Ext = []string{".go", ".mod", ".sum"}
	}
	if o.Log == nil {
		o.Log = io.Discard
	}
	dir, err := filepath.Abs(o.Dir)
	if err != nil {
		return err
	}
	o.Dir = dir
	tmp, err := os.MkdirTemp("", "nectar-dev-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	s := &devSession{o: o, tmp: tmp, state: filepath.Join(tmp, "state.json"), p: newPrinter(o.Log)}

	changes := make(chan []string, 1)
	done := make(chan struct{})
	defer close(done) // after stopWatch (defers run in reverse): unblocks a pending send
	stopWatch := hotreload.Watch(o.Dir, func(rel string) bool {
		return slices.Contains(o.Ext, filepath.Ext(rel))
	}, o.Poll, func(changed []string) {
		select {
		case changes <- changed:
		case <-done:
		}
	})
	defer stopWatch()
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)

	s.banner()
	s.event(evBuild, "building %s", s.p.bold(o.Pkg))
	start := time.Now()
	if bin, ok := s.build(); ok {
		s.start(bin)
		if s.app != nil {
			s.event(evOK, "running %s", s.p.dim("(built in "+since(start)+")"))
		}
	}
	for {
		var exited <-chan error
		if s.app != nil {
			exited = s.app.done
		}
		select {
		case changed := <-changes:
			s.event(evChange, "changed %s", s.files(changed))
			start := time.Now()
			bin, ok := s.build()
			if !ok {
				continue // keep the running app
			}
			s.stop()
			s.start(bin)
			if s.app != nil {
				s.event(evOK, "restarted in %s", s.p.bold(since(start)))
			}
		case err := <-exited:
			s.app.flush()
			s.app = nil
			if err == nil {
				s.event(evStop, "app closed")
				return nil
			}
			s.event(evFail, "app exited: %v %s", err, s.p.dim("— waiting for changes"))
		case <-interrupt:
			s.stop()
			s.p.printf("\n")
			s.event(evStop, "stopped")
			return nil
		case <-o.Stop:
			s.stop()
			return nil
		}
	}
}

// since formats the time since start for log lines.
func since(start time.Time) string {
	d := time.Since(start)
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	return d.Round(10 * time.Millisecond).String()
}

type devSession struct {
	p     *printer
	o     DevOptions
	tmp   string
	state string
	n     int
	app   *devApp
}

type devApp struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	done     chan error
	bin      string
	out, err *gutter
}

// flush writes out a last partial line of the app's output.
func (a *devApp) flush() {
	a.out.Flush()
	a.err.Flush()
}

// Kinds of dev log lines: each has its symbol and color.
type devEvent uint8

const (
	evBuild devEvent = iota
	evOK
	evChange
	evFail
	evWarn
	evStop
)

// event prints a timestamped status line: "19:32:07 ✓ restarted in 840ms".
func (s *devSession) event(kind devEvent, format string, args ...any) {
	p := s.p
	var sym string
	switch kind {
	case evBuild:
		sym = p.cyan(p.sym("◆", "*"))
	case evOK:
		sym = p.green(p.sym("✓", "+"))
	case evChange:
		sym = p.blue(p.sym("↻", "~"))
	case evFail:
		sym = p.red(p.sym("✗", "x"))
	case evWarn:
		sym = p.yellow(p.sym("!", "!"))
	case evStop:
		sym = p.dim(p.sym("■", "-"))
	}
	msg := fmt.Sprintf(format, args...)
	if kind == evFail {
		msg = p.red(msg)
	}
	p.printf("%s %s %s\n", p.dim(clock()), sym, msg)
}

// banner opens the session: what runs, what's watched, how to stop.
func (s *devSession) banner() {
	p := s.p
	exts := make([]string, len(s.o.Ext))
	for i, e := range s.o.Ext {
		exts[i] = p.yellow(e)
	}
	p.header("dev", p.bold(filepath.Base(s.o.Dir))+" "+p.dim(tildePath(s.o.Dir)))
	p.printf("  %s %s\n", p.dim("watching"), strings.Join(exts, p.dim(", ")))
	p.printf("  %s %s\n", p.dim("restart "), "on save, keeping window size and hotreload.Keep state")
	p.printf("  %s %s\n\n", p.dim("stop    "), p.cyan("Ctrl+C")+p.dim(" or close the window"))
}

// files lists changed files, at most three by name.
func (s *devSession) files(changed []string) string {
	shown := changed
	if len(shown) > 3 {
		shown = shown[:3]
	}
	names := make([]string, len(shown))
	for i, f := range shown {
		names[i] = s.p.bold(filepath.ToSlash(f))
	}
	out := strings.Join(names, s.p.dim(", "))
	if n := len(changed) - len(shown); n > 0 {
		out += s.p.dim(fmt.Sprintf(" +%d more", n))
	}
	return out
}

// tildePath shortens a path under the home folder to ~/…
func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if r, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(r, "..") {
			return filepath.Join("~", r)
		}
	}
	return p
}

// build compiles the app to a new binary (a fresh name each time: Windows
// can't overwrite a running .exe).
func (s *devSession) build() (string, bool) {
	s.n++
	bin := filepath.Join(s.tmp, fmt.Sprintf("app-%d", s.n))
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, s.o.Pkg)
	cmd.Dir = s.o.Dir
	cmd.Env = os.Environ()
	if os.Getenv("CGO_ENABLED") == "" {
		cmd.Env = append(cmd.Env, "CGO_ENABLED=0")
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		s.event(evFail, "build failed %s", s.p.dim("— the running app keeps going; fix and save"))
		g := &gutter{w: s.o.Log, prefix: "         " + s.p.red(s.p.sym("│", "|")) + " "}
		io.WriteString(g, s.p.goErrors(strings.TrimSpace(string(out))))
		g.Flush()
		return "", false
	}
	return bin, true
}

func (s *devSession) start(bin string) {
	cmd := exec.Command(bin, s.o.Args...)
	cmd.Dir = s.o.Dir
	// The app's own output, indented under our status symbols behind a
	// bar, so it stands apart from nectar's lines. stdout and stderr look
	// the same: Go's log and slog write ordinary messages to stderr.
	bar := "         " + s.p.dim(s.p.sym("│", "|")) + " "
	out := &gutter{w: os.Stdout, prefix: bar}
	errOut := &gutter{w: os.Stderr, prefix: bar}
	cmd.Stdout, cmd.Stderr = out, errOut
	cmd.Env = append(os.Environ(),
		hotreload.EnvDev+"=1",
		hotreload.EnvState+"="+s.state,
		hotreload.EnvControl+"=stdin")
	stdin, err := cmd.StdinPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		s.event(evFail, "could not start the app: %v", err)
		return
	}
	app := &devApp{cmd: cmd, stdin: stdin, done: make(chan error, 1), bin: bin, out: out, err: errOut}
	go func() { app.done <- cmd.Wait() }()
	s.app = app
}

// stop asks the app to save its state and quit, and kills it if it
// doesn't within a few seconds.
func (s *devSession) stop() {
	app := s.app
	if app == nil {
		return
	}
	s.app = nil
	_, _ = io.WriteString(app.stdin, "reload\n")
	select {
	case <-app.done:
	case <-time.After(5 * time.Second):
		s.event(evWarn, "app didn't quit in time; killing it")
		_ = app.cmd.Process.Kill()
		<-app.done
	}
	_ = app.stdin.Close()
	app.flush()
	if err := os.Remove(app.bin); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.event(evWarn, "removing old build: %v", err)
	}
}
