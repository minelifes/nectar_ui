package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/minelifes/nectar_ui/ui/fswatch"
	"github.com/minelifes/nectar_ui/ui/i18n"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/mvvm"
	"github.com/minelifes/nectar_ui/ui/settings"
	"github.com/minelifes/nectar_ui/ui/tasks"
	"github.com/minelifes/nectar_ui/ui/tester"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

func TestSettingsStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app", "settings.json")
	s, err := settings.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	size := settings.Define(s, "editor.fontSize", 14)
	theme := settings.Define(s, "theme", "light")
	type layout struct{ Panels []string }
	lay := settings.Define(s, "layout", layout{})
	if size.Get() != 14 || size.IsSet() {
		t.Fatal("default")
	}
	n := 0
	cancel := size.Subscribe(func() { n++ })
	size.Set(16)
	theme.Set("dark") // other keys don't notify size's subscribers
	lay.Set(layout{Panels: []string{"files", "editor"}})
	size.Set(16) // unchanged
	cancel()
	if n != 1 {
		t.Fatalf("%d notifications", n)
	}
	// A new store reads what was saved.
	s2, err := settings.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Get(s2, "editor.fontSize", 0) != 16 || settings.Get(s2, "theme", "") != "dark" ||
		strings.Join(settings.Get(s2, "layout", layout{}).Panels, ",") != "files,editor" {
		t.Fatalf("reloaded: %v", s2.Keys())
	}
	// A value of the wrong type reads as the default.
	if settings.Get(s2, "theme", 7) != 7 {
		t.Fatal("type mismatch should give the default")
	}
	size.Reset()
	if s3, _ := settings.OpenFile(path); s3.Has("editor.fontSize") {
		t.Fatal("reset not saved")
	}
	// No temp files left behind; a broken file is an error, not a wipe.
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("files: %v", entries)
	}
	os.WriteFile(path, []byte("{broken"), 0o644)
	if _, err := settings.OpenFile(path); err == nil {
		t.Fatal("broken settings accepted")
	}
	// A memory store never touches the disk.
	mem := settings.Memory()
	settings.Set(mem, "x", 1)
	if mem.Path() != "" || settings.Get(mem, "x", 0) != 1 {
		t.Fatal("memory store")
	}
}

func TestSettingDrivesWidgets(t *testing.T) {
	s := settings.Memory()
	label := settings.Define(s, "label", "one")
	builds := 0
	tt := tester.New(w.Align{Child: mvvm.Bind[string](label, func(_ w.BuildContext, v string) w.Widget {
		builds++
		return w.Text{Text: v}
	})}, 200, 100)
	label.Set("two")
	tt.Pump()
	if _, ok := tt.Find("two"); !ok || builds != 2 {
		t.Fatalf("binding: builds=%d %v", builds, tt.Texts())
	}
}

func TestTasks(t *testing.T) {
	var postMu sync.Mutex
	var posted []func()
	post := func(fn func()) { postMu.Lock(); posted = append(posted, fn); postMu.Unlock() }
	runPosted := func() {
		postMu.Lock()
		ps := posted
		posted = nil
		postMu.Unlock()
		for _, fn := range ps {
			fn()
		}
	}
	mgr := tasks.NewManager(post)
	step := make(chan struct{})
	var result error = errors.New("unset")
	task := mgr.Run("Indexing", func(ctx context.Context, p *tasks.Progress) error {
		p.Report(0.5, "half")
		<-step
		return nil
	}).Then(func(err error) { result = err })
	for task.Progress() != 0.5 {
		time.Sleep(time.Millisecond)
	}
	if task.Status() != "half" || task.State() != tasks.Running || len(mgr.Running()) != 1 {
		t.Fatalf("running: %v %v", task.Status(), task.State())
	}
	if p, n := mgr.Overall(); p != 0.5 || n != 1 {
		t.Fatalf("overall %v %d", p, n)
	}
	close(step)
	if err := task.Wait(); err != nil || task.State() != tasks.Done || task.Progress() != 1 {
		t.Fatalf("done: %v %v", err, task.State())
	}
	if result == nil {
		t.Fatal("Then ran off the UI goroutine")
	}
	runPosted()
	if result != nil {
		t.Fatalf("Then: %v", result)
	}

	// Cancel, failure, panic.
	c := mgr.Run("Long", func(ctx context.Context, p *tasks.Progress) error {
		<-ctx.Done()
		return ctx.Err()
	})
	c.Cancel()
	if c.Wait(); c.State() != tasks.Cancelled {
		t.Fatalf("cancelled: %v", c.State())
	}
	f := mgr.Run("Fail", func(context.Context, *tasks.Progress) error { return errors.New("boom") })
	if f.Wait(); f.State() != tasks.Failed || f.Err().Error() != "boom" {
		t.Fatalf("failed: %v %v", f.State(), f.Err())
	}
	pn := mgr.Run("Panic", func(context.Context, *tasks.Progress) error { panic("oops") })
	var pe *tasks.PanicError
	if err := pn.Wait(); !errors.As(err, &pe) || !strings.Contains(err.Error(), "oops") {
		t.Fatalf("panic: %v", err)
	}
	if len(mgr.Running()) != 0 || len(mgr.Recent()) != 4 {
		t.Fatalf("lists: %d running, %d recent", len(mgr.Running()), len(mgr.Recent()))
	}
	// Then after the end still runs (posted).
	late := false
	f.Then(func(error) { late = true })
	runPosted()
	if !late {
		t.Fatal("late Then")
	}
}

func TestTaskToast(t *testing.T) {
	tt, ctx := ctxApp(w.SizedBox{}, 800, 600)
	mgr := tasks.ManagerFor(*ctx)
	if tasks.ManagerFor(*ctx) != mgr {
		t.Fatal("one manager per tree")
	}
	step := make(chan struct{})
	task := mgr.Run("Building", func(ctx context.Context, p *tasks.Progress) error {
		p.Report(0.25, "compiling")
		<-step
		return nil
	})
	m.ShowTaskToast(*ctx, task)
	waitFor(t, tt, func() bool { _, ok := tt.Find("compiling"); return ok })
	close(step)
	task.Wait()
	waitFor(t, tt, func() bool { _, ok := tt.Find("Done"); return ok })
	// Cancel from the toast.
	c := mgr.Run("Long", func(ctx context.Context, p *tasks.Progress) error { <-ctx.Done(); return ctx.Err() })
	m.ShowTaskToast(*ctx, c)
	tt.Pump()
	tt.TapText("Cancel")
	c.Wait()
	if c.State() != tasks.Cancelled {
		t.Fatalf("toast cancel: %v", c.State())
	}
}

func waitFor(t *testing.T, tt *tester.Tester, cond func() bool) {
	t.Helper()
	if !tt.PumpUntil(cond, 2*time.Second) {
		t.Fatalf("timed out; showing %v", tt.Texts())
	}
}

func TestFileWatcher(t *testing.T) {
	for _, poll := range []bool{false, true} {
		t.Run(fmt.Sprint("poll=", poll), func(t *testing.T) {
			dir := t.TempDir()
			os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
			os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
			batches := make(chan []fswatch.Event, 16)
			stop, err := fswatch.Watch(dir, fswatch.Options{Poll: poll, PollInterval: 30 * time.Millisecond, Debounce: 60 * time.Millisecond},
				func(evs []fswatch.Event) { batches <- evs })
			if err != nil {
				t.Fatal(err)
			}
			defer stop()
			next := func() string {
				select {
				case evs := <-batches:
					var parts []string
					for _, e := range evs {
						parts = append(parts, e.Op.String()+":"+e.Path)
					}
					return strings.Join(parts, " ")
				case <-time.After(3 * time.Second):
					return "timeout"
				}
			}
			time.Sleep(50 * time.Millisecond)
			os.WriteFile(filepath.Join(dir, "a.txt"), []byte("changed!"), 0o644)
			os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644)
			os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("x"), 0o644) // ignored
			if got := next(); got != "write:a.txt create:b.txt" {
				t.Fatalf("batch 1: %s", got)
			}
			// A new folder with a file in it, then changes inside it.
			os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o755)
			os.WriteFile(filepath.Join(dir, "sub", "deep", "c.go"), []byte("c"), 0o644)
			got := next()
			if !strings.Contains(got, "create:sub/deep/c.go") {
				t.Fatalf("batch 2: %s", got)
			}
			time.Sleep(50 * time.Millisecond)
			os.WriteFile(filepath.Join(dir, "sub", "deep", "c.go"), []byte("cc"), 0o644)
			if got := next(); got != "write:sub/deep/c.go" {
				t.Fatalf("batch 3: %s", got)
			}
			os.Rename(filepath.Join(dir, "b.txt"), filepath.Join(dir, "sub", "b2.txt"))
			os.Remove(filepath.Join(dir, "a.txt"))
			if got := next(); got != "remove:a.txt remove:b.txt create:sub/b2.txt" {
				t.Fatalf("batch 4: %s", got)
			}
		})
	}
}

func TestI18n(t *testing.T) {
	b := i18n.NewBundle("en")
	err := b.LoadFS(fstest.MapFS{
		"locales/en.json":    {Data: []byte(`{"hello": "Hello, {name}!", "files": {"one": "{n} file", "other": "{n} files", "zero": "No files"}, "only.en": "English"}`)},
		"locales/ru.json":    {Data: []byte(`{"hello": "Привет, {name}!", "files": {"one": "{n} файл", "few": "{n} файла", "many": "{n} файлов", "other": "{n} файла"}}`)},
		"locales/pt-BR.json": {Data: []byte(`{"hello": "Olá, {name}!"}`)},
		"locales/pt.json":    {Data: []byte(`{"bye": "Tchau"}`)},
	}, "locales/*.json")
	if err != nil {
		t.Fatal(err)
	}
	ru := b.Localizer("ru_RU.UTF-8")
	if ru.Language() != "ru-RU" || ru.T("hello", "name", "Аня") != "Привет, Аня!" {
		t.Fatalf("ru: %q %q", ru.Language(), ru.T("hello", "name", "Аня"))
	}
	for n, want := range map[int]string{1: "1 файл", 3: "3 файла", 5: "5 файлов", 11: "11 файлов", 21: "21 файл", 22: "22 файла"} {
		if got := ru.N("files", n); got != want {
			t.Errorf("ru N(%d) = %q, want %q", n, got, want)
		}
	}
	en := b.Localizer("en-US")
	if en.N("files", 1) != "1 file" || en.N("files", 7) != "7 files" || en.N("files", 0) != "No files" {
		t.Fatalf("en plurals: %q %q %q", en.N("files", 1), en.N("files", 7), en.N("files", 0))
	}
	// pt-BR falls back to pt, then en; unknown keys come back as themselves.
	pt := b.Localizer("pt-BR")
	if pt.T("hello", "name", "Zé") != "Olá, Zé!" || pt.T("bye") != "Tchau" || pt.T("only.en") != "English" || pt.T("nope") != "nope" {
		t.Fatalf("fallback: %q %q %q", pt.T("hello", "name", "Zé"), pt.T("bye"), pt.T("only.en"))
	}
	if i18n.PluralForm("pl", 22) != "few" || i18n.PluralForm("pl", 25) != "many" || i18n.PluralForm("ja", 1) != "other" || i18n.PluralForm("fr", 0) != "one" {
		t.Fatal("plural rules")
	}
	t.Setenv("LANGUAGE", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "de_AT.UTF-8")
	if got := i18n.Detect(); len(got) != 1 || got[0] != "de-AT" {
		t.Fatalf("detect: %v", got)
	}

	// Widgets: switching the Localizer rebuilds what uses it.
	loc := mvvm.NewProperty(en)
	tt := tester.New(mvvm.Bind[*i18n.Localizer](loc, func(_ w.BuildContext, l *i18n.Localizer) w.Widget {
		return i18n.Localizations{Localizer: l, Child: w.Align{Child: w.Builder{Builder: func(ctx w.BuildContext) w.Widget {
			return w.Text{Text: i18n.T(ctx, "hello", "name", "Bo") + " / " + i18n.N(ctx, "files", 2)}
		}}}}
	}), 400, 100)
	if _, ok := tt.Find("Hello, Bo! / 2 files"); !ok {
		t.Fatalf("en UI: %v", tt.Texts())
	}
	loc.Set(ru)
	tt.Pump()
	if _, ok := tt.Find("Привет, Bo! / 2 файла"); !ok {
		t.Fatalf("ru UI: %v", tt.Texts())
	}
}

func TestOpenWindowWithoutApp(t *testing.T) {
	_, ctx := ctxApp(w.SizedBox{}, 100, 100)
	var got error
	w.OpenWindow(*ctx, w.WindowOptions{Title: "x"}, w.SizedBox{}, func(_ w.Window, err error) { got = err })
	if !errors.Is(got, w.ErrNoWindows) {
		t.Fatalf("got %v", got)
	}
}
