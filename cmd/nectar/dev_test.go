package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// devApp is a stand-in for a Nectar app: it keeps a run counter with
// hotreload.Keep, writes it to out.txt, and quits on "reload" like ui.App.
const devAppSrc = `package main

import (
	"bufio"
	"os"
	"strconv"

	"github.com/minelifes/nectar_ui/ui/hotreload"
)

func main() {
	n := 0
	hotreload.Keep("runs", &n)
	n++
	os.WriteFile("out.txt", []byte(strconv.Itoa(n)), 0o644)
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		if sc.Text() == "reload" {
			break
		}
	}
	hotreload.Save()
}
`

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func TestDevRestartsOnChangeAndKeepsState(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	// Inside this module (testdata isn't part of ./...) so the app can
	// import the engine without fetching anything.
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("testdata", "devapp-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove("testdata") // only if empty
	defer os.RemoveAll(dir)
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte(devAppSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.txt")
	waitFor := func(want string) {
		t.Helper()
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) {
			if b, _ := os.ReadFile(out); string(b) == want {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		b, _ := os.ReadFile(out)
		t.Fatalf("out.txt = %q, want %q", b, want)
	}

	var log syncBuf
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- Dev(DevOptions{Dir: dir, Log: &log, Poll: 50 * time.Millisecond, Stop: stop}) }()
	defer func() {
		if t.Failed() {
			t.Log(log.String())
		}
	}()

	waitFor("1")
	// A build error keeps the running app.
	if err := os.WriteFile(src, []byte(devAppSrc+"\nfunc broken( {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for !strings.Contains(log.String(), "build failed") {
		time.Sleep(50 * time.Millisecond)
	}
	// Fixed: the app restarts and its counter survives.
	if err := os.WriteFile(src, []byte(devAppSrc+"\n// edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor("2")

	close(stop)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
