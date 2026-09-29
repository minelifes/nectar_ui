package hotreload

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Watch polls the files under root and calls onChange with the paths
// (relative to root, slash-separated) that were added, changed or removed.
// Changes are reported once the tree has been quiet for one interval, so
// an editor saving several files at once gives a single call. match picks
// the files to watch (nil = all); directories starting with "." are
// skipped. onChange runs on the watcher's goroutine. stop ends the watch.
//
// Polling needs no platform support and is cheap for project-sized trees.
func Watch(root string, match func(rel string) bool, interval time.Duration, onChange func(changed []string)) (stop func()) {
	if interval <= 0 {
		interval = 300 * time.Millisecond
	}
	done := make(chan struct{})
	prev := scan(root, match)
	go func() {
		var pending []string
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
			}
			cur := scan(root, match)
			changed := diff(prev, cur)
			prev = cur
			if len(changed) > 0 {
				pending = append(pending, changed...)
				continue // wait for a quiet interval
			}
			if len(pending) > 0 {
				slices.Sort(pending)
				onChange(slices.Compact(pending))
				pending = nil
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}

type stamp struct {
	mod  time.Time
	size int64
}

func scan(root string, match func(string) bool) map[string]stamp {
	out := map[string]stamp{}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if match != nil && !match(rel) {
			return nil
		}
		if info, err := d.Info(); err == nil {
			out[rel] = stamp{info.ModTime(), info.Size()}
		}
		return nil
	})
	return out
}

func diff(a, b map[string]stamp) []string {
	var out []string
	for k, s := range b {
		if old, ok := a[k]; !ok || old != s {
			out = append(out, k)
		}
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			out = append(out, k)
		}
	}
	return out
}
