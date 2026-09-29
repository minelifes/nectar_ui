package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/minelifes/nectar_ui/ui/hotreload"
	"github.com/minelifes/nectar_ui/ui/mvvm"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// fileLabel shows a resource file and a counter kept in its State.
type fileLabel struct{ bump *func() }

func (fileLabel) CreateState() w.State { return &fileLabelState{} }

type fileLabelState struct {
	w.StateBase
	n, reassembled int
}

func (s *fileLabelState) Reassemble() { s.reassembled++ }

var lastFileLabel *fileLabelState

func (s *fileLabelState) Build(ctx w.BuildContext) w.Widget {
	lastFileLabel = s
	*w.WidgetOf[fileLabel](s).bump = func() { s.SetState(func() { s.n++ }) }
	data, _ := w.ResourcesOf(ctx).ReadFile("msg.txt")
	return w.Text{Text: fmt.Sprintf("%s %d", data, s.n)}
}

func TestReassembleRebuildsEverythingAndKeepsState(t *testing.T) {
	files := fstest.MapFS{"msg.txt": {Data: []byte("hello")}}
	var bump func()
	tt := tester.New(w.Center{Child: fileLabel{bump: &bump}}, 200, 100, tester.WithResources("", files))
	bump()
	tt.Pump()
	if got := tt.Texts()[0]; got != "hello 1" {
		t.Fatalf("got %q", got)
	}
	files["msg.txt"] = &fstest.MapFile{Data: []byte("bye")}
	tt.Pump()
	if got := tt.Texts()[0]; got != "hello 1" {
		t.Fatalf("changed without a reassemble: %q", got)
	}
	tt.Root.Reassemble()
	tt.Pump()
	if got := tt.Texts()[0]; got != "bye 1" {
		t.Fatalf("after reassemble got %q, want the new file with the state kept", got)
	}
	if lastFileLabel.reassembled != 1 {
		t.Fatalf("State.Reassemble called %d times, want 1", lastFileLabel.reassembled)
	}
}

// TestPropertyKeepSurvivesRestart runs itself again as "the next process"
// with the state file the first run saved.
func TestPropertyKeepSurvivesRestart(t *testing.T) {
	child := os.Getenv("NECTAR_KEEP_CHILD") == "1"
	if !child {
		t.Setenv(hotreload.EnvDev, "1")
		t.Setenv(hotreload.EnvState, filepath.Join(t.TempDir(), "state.json"))
	}
	p := mvvm.NewProperty(0).Keep("test.count")
	l := mvvm.NewList[string]().Keep("test.items")
	if child {
		if p.Get() != 42 || !slices.Equal(l.Get(), []string{"a", "b"}) {
			t.Fatalf("restored %d %v", p.Get(), l.Get())
		}
		return
	}
	p.Set(42)
	l.Append("a", "b")
	if err := hotreload.Save(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPropertyKeepSurvivesRestart$")
	cmd.Env = append(os.Environ(), "NECTAR_KEEP_CHILD=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("next run: %v\n%s", err, out)
	}
}
