// Package fswatch reports file changes under a folder as they happen,
// using the OS's change notifications (inotify, kqueue, FSEvents via
// kqueue, ReadDirectoryChangesW) through fsnotify, and falls back to
// polling where those aren't available. Subfolders are watched too,
// including ones created later. Bursts of changes (a save that writes a
// temp file and renames it, a git checkout) arrive as one batch.
//
//	stop, err := fswatch.Watch("project", fswatch.Options{}, func(evs []fswatch.Event) {
//	    app.Post(func() { tree.Reload("") })
//	})
package fswatch

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Op is what happened to a path.
type Op uint8

const (
	Create Op = iota + 1
	Write
	Remove
)

func (o Op) String() string {
	switch o {
	case Create:
		return "create"
	case Write:
		return "write"
	case Remove:
		return "remove"
	}
	return "?"
}

// Event is one changed path, relative to the watched root and
// slash-separated. A rename is a Remove of the old path and a Create of
// the new one.
type Event struct {
	Path string
	Op   Op
	Dir  bool // the path is (or was) a folder
}

// Options configure Watch.
type Options struct {
	// Match picks what to watch by relative path (nil = everything except
	// folders whose name starts with "."). Return false for a folder to
	// skip its whole subtree.
	Match func(rel string, dir bool) bool
	// Debounce is how long changes must pause before a batch is
	// delivered (default 100ms).
	Debounce time.Duration
	// Poll forces polling (network drives, or for tests); PollInterval
	// sets its period (default 500ms).
	Poll         bool
	PollInterval time.Duration
}

func (o Options) match(rel string, dir bool) bool {
	if o.Match != nil {
		return o.Match(rel, dir)
	}
	if dir && rel != "." && strings.HasPrefix(filepath.Base(rel), ".") {
		return false
	}
	return true
}

// Watch starts watching root and calls onChange (on the watcher's own
// goroutine) with each batch of changes, sorted by path. stop ends it.
func Watch(root string, opt Options, onChange func([]Event)) (stop func(), err error) {
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(root); err != nil {
		return nil, err
	}
	if opt.Debounce <= 0 {
		opt.Debounce = 100 * time.Millisecond
	}
	if !opt.Poll {
		if stop, err := watchNotify(root, opt, onChange); err == nil {
			return stop, nil
		}
	}
	return watchPoll(root, opt, onChange), nil
}

// batcher collects changed paths and delivers them after a quiet period.
type batcher struct {
	root     string
	opt      Options
	onChange func([]Event)

	mu      sync.Mutex
	pending map[string]bool // rel path → "created during this batch"
	dirs    map[string]bool // rel paths known to be folders
	timer   *time.Timer
	stopped bool
}

func newBatcher(root string, opt Options, onChange func([]Event)) *batcher {
	return &batcher{root: root, opt: opt, onChange: onChange, pending: map[string]bool{}, dirs: map[string]bool{}}
}

func (b *batcher) add(rel string, created, dir bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return
	}
	b.pending[rel] = b.pending[rel] || created
	if dir {
		b.dirs[rel] = true
	}
	if b.timer == nil {
		b.timer = time.AfterFunc(b.opt.Debounce, b.flush)
	} else {
		b.timer.Reset(b.opt.Debounce)
	}
}

// flush classifies each path by what's on disk now.
func (b *batcher) flush() {
	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		return
	}
	pend := b.pending
	b.pending, b.timer = map[string]bool{}, nil
	b.mu.Unlock()
	evs := make([]Event, 0, len(pend))
	for rel, created := range pend {
		info, err := os.Lstat(filepath.Join(b.root, filepath.FromSlash(rel)))
		b.mu.Lock()
		wasDir := b.dirs[rel]
		b.mu.Unlock()
		switch {
		case err != nil:
			evs = append(evs, Event{Path: rel, Op: Remove, Dir: wasDir})
			b.mu.Lock()
			delete(b.dirs, rel)
			b.mu.Unlock()
		case created:
			evs = append(evs, Event{Path: rel, Op: Create, Dir: info.IsDir()})
		case info.IsDir():
			// A folder's own mtime changes with its entries: those
			// are reported themselves.
		default:
			evs = append(evs, Event{Path: rel, Op: Write})
		}
	}
	if len(evs) == 0 {
		return
	}
	sort.Slice(evs, func(i, j int) bool { return evs[i].Path < evs[j].Path })
	b.onChange(evs)
}

func (b *batcher) stop() {
	b.mu.Lock()
	b.stopped = true
	if b.timer != nil {
		b.timer.Stop()
	}
	b.mu.Unlock()
}

func rel(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(r)
}

func watchNotify(root string, opt Options, onChange func([]Event)) (func(), error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	b := newBatcher(root, opt, onChange)
	// addTree watches dir and its subfolders; with report, the files and
	// folders found are reported as created (they appeared before the
	// watch on the new folder started).
	var addTree func(dir string, report bool) error
	addTree = func(dir string, report bool) error {
		return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			r := rel(root, p)
			if !opt.match(r, d.IsDir()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if report && p != dir {
				b.add(r, true, d.IsDir())
			}
			if d.IsDir() {
				b.mu.Lock()
				b.dirs[r] = true
				b.mu.Unlock()
				if err := w.Add(p); err != nil {
					return err
				}
			}
			return nil
		})
	}
	if err := addTree(root, false); err != nil {
		w.Close()
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				r := rel(root, ev.Name)
				info, statErr := os.Lstat(ev.Name)
				isDir := statErr == nil && info.IsDir()
				if !opt.match(r, isDir) {
					continue
				}
				switch {
				case ev.Has(fsnotify.Create):
					b.add(r, true, isDir)
					if isDir {
						_ = addTree(ev.Name, true)
					}
				case ev.Has(fsnotify.Remove), ev.Has(fsnotify.Rename):
					b.add(r, false, false)
				case ev.Has(fsnotify.Write), ev.Has(fsnotify.Chmod):
					if !isDir {
						b.add(r, false, false)
					}
				}
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			w.Close()
			b.stop()
		})
	}, nil
}

type stamp struct {
	mod  time.Time
	size int64
	dir  bool
}

func scan(root string, opt Options) map[string]stamp {
	out := map[string]stamp{}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == root {
			return nil
		}
		r := rel(root, p)
		if !opt.match(r, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info, err := d.Info(); err == nil {
			out[r] = stamp{info.ModTime(), info.Size(), d.IsDir()}
		}
		return nil
	})
	return out
}

func watchPoll(root string, opt Options, onChange func([]Event)) func() {
	iv := opt.PollInterval
	if iv <= 0 {
		iv = 500 * time.Millisecond
	}
	b := newBatcher(root, opt, onChange)
	prev := scan(root, opt)
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(iv)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
			}
			cur := scan(root, opt)
			for p, s := range cur {
				old, ok := prev[p]
				switch {
				case !ok:
					b.add(p, true, s.dir)
				case !s.dir && (old.mod != s.mod || old.size != s.size):
					b.add(p, false, false)
				}
			}
			for p, s := range prev {
				if _, ok := cur[p]; !ok {
					b.add(p, false, s.dir)
				}
			}
			prev = cur
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			b.stop()
		})
	}
}
