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
		fmt.Fprint(os.Stderr, `Usage: nectar dev [flags] [app dir] [-- app args]

Runs the app and restarts it whenever its code changes, keeping the window
size and the state registered with hotreload.Keep / mvvm Property.Keep.
Files mounted with ui.Config.WithDevResources reload without a restart.
A build error keeps the running app; fix it and save again.

`)
		fs.PrintDefaults()
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
	s := &devSession{o: o, tmp: tmp, state: filepath.Join(tmp, "state.json")}

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

	s.logf("building %s", o.Pkg)
	if bin, ok := s.build(); ok {
		s.start(bin)
	}
	for {
		var exited <-chan error
		if s.app != nil {
			exited = s.app.done
		}
		select {
		case changed := <-changes:
			s.logf("changed: %s; rebuilding", strings.Join(changed, ", "))
			start := time.Now()
			bin, ok := s.build()
			if !ok {
				continue // keep the running app
			}
			s.stop()
			s.start(bin)
			s.logf("restarted in %s", time.Since(start).Round(10*time.Millisecond))
		case err := <-exited:
			s.app = nil
			if err == nil {
				s.logf("app closed")
				return nil
			}
			s.logf("app exited: %v; waiting for changes", err)
		case <-interrupt:
			s.stop()
			return nil
		case <-o.Stop:
			s.stop()
			return nil
		}
	}
}

type devSession struct {
	o     DevOptions
	tmp   string
	state string
	n     int
	app   *devApp
}

type devApp struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	done  chan error
	bin   string
}

func (s *devSession) logf(format string, args ...any) {
	fmt.Fprintf(s.o.Log, "nectar dev: "+format+"\n", args...)
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
		s.logf("build failed:\n%s", strings.TrimSpace(string(out)))
		return "", false
	}
	return bin, true
}

func (s *devSession) start(bin string) {
	cmd := exec.Command(bin, s.o.Args...)
	cmd.Dir = s.o.Dir
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(),
		hotreload.EnvDev+"=1",
		hotreload.EnvState+"="+s.state,
		hotreload.EnvControl+"=stdin")
	stdin, err := cmd.StdinPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		s.logf("could not start the app: %v", err)
		return
	}
	app := &devApp{cmd: cmd, stdin: stdin, done: make(chan error, 1), bin: bin}
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
		s.logf("app didn't quit in time; killing it")
		_ = app.cmd.Process.Kill()
		<-app.done
	}
	_ = app.stdin.Close()
	if err := os.Remove(app.bin); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.logf("removing old build: %v", err)
	}
}
