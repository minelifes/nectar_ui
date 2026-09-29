package hotreload

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func devEnv(t *testing.T) string {
	t.Helper()
	state := filepath.Join(t.TempDir(), "state.json")
	t.Setenv(EnvDev, "1")
	t.Setenv(EnvState, state)
	reset()
	t.Cleanup(reset)
	return state
}

func TestKeepRoundTrip(t *testing.T) {
	devEnv(t)
	type form struct {
		Name string
		Tab  int
	}
	f := form{"ann", 2}
	Keep("form", &f)
	if err := Save(); err != nil {
		t.Fatal(err)
	}

	reset() // the next run
	var g form
	Keep("form", &g)
	if g != f {
		t.Fatalf("restored %+v, want %+v", g, f)
	}
	var h form
	Keep("form", &h) // restored once per run
	if h != (form{}) {
		t.Fatalf("second registration got stale state %+v", h)
	}
}

func TestDisabledIsNoOp(t *testing.T) {
	t.Setenv(EnvDev, "")
	t.Setenv(EnvState, filepath.Join(t.TempDir(), "state.json"))
	reset()
	n := 5
	Keep("n", &n)
	if err := Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(os.Getenv(EnvState)); err == nil {
		t.Fatal("Save wrote a state file outside dev mode")
	}
}

func TestWatchReportsChanges(t *testing.T) {
	dir := t.TempDir()
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "a")
	write("b.txt", "b")
	got := make(chan []string, 4)
	stop := Watch(dir, func(rel string) bool { return filepath.Ext(rel) == ".go" }, 20*time.Millisecond,
		func(c []string) { got <- c })
	defer stop()

	write("a.go", "changed")
	write("c.go", "new")
	write("b.txt", "ignored")
	select {
	case c := <-got:
		if !slices.Equal(c, []string{"a.go", "c.go"}) {
			t.Fatalf("changed = %v", c)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no change reported")
	}
}
