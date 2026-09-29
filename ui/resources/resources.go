// Package resources gives the app read access to files shipped inside it:
// images, fonts, data. Embed a folder into the binary with go:embed and
// mount it once at startup; widgets then load by name (e.g. an
// widgets.AssetImage) no matter where the app is installed or run from.
//
//	//go:embed images fonts
//	var files embed.FS
//
//	resources.Mount("", files)                // at the root
//	resources.Mount("plugin", pluginFiles)    // under plugin/ (returns an unmount func)
//	data, err := resources.ReadFile("images/logo.png")
//
// While developing you can mount the folder on disk instead (edits show up
// without rebuilding): resources.Mount("", os.DirFS("assets")).
//
// Mounts stack: a file in a later mount hides the same name in earlier
// ones, so an app can override a library's defaults. Names use forward
// slashes and no leading "/", as in io/fs. Everything here is safe for
// concurrent use.
package resources

import (
	"errors"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"
	"time"
)

type mount struct {
	prefix string // "" or "a/b"
	fsys   fs.FS
}

var (
	mu     sync.RWMutex
	mounts []*mount
)

// Mount adds fsys to the resources under prefix ("" = the root). The
// returned func removes it again.
func Mount(prefix string, fsys fs.FS) (unmount func()) {
	prefix = strings.Trim(path.Clean("/"+prefix), "/")
	m := &mount{prefix, fsys}
	mu.Lock()
	mounts = append(mounts, m)
	mu.Unlock()
	return func() {
		mu.Lock()
		mounts = slices.DeleteFunc(mounts, func(x *mount) bool { return x == m })
		mu.Unlock()
	}
}

// Reset removes all mounts (mainly for tests).
func Reset() {
	mu.Lock()
	mounts = nil
	mu.Unlock()
}

// FS returns the merged view of all mounts as an fs.FS (it also
// implements fs.ReadFileFS, fs.ReadDirFS and fs.StatFS), e.g. to pass to
// a FileTree or template.ParseFS.
func FS() fs.FS { return merged{} }

// Open opens a resource.
func Open(name string) (fs.File, error) { return merged{}.Open(name) }

// ReadFile reads a whole resource.
func ReadFile(name string) ([]byte, error) { return merged{}.ReadFile(name) }

// ReadDir lists a folder across all mounts, sorted by name.
func ReadDir(name string) ([]fs.DirEntry, error) { return merged{}.ReadDir(name) }

// Stat describes a resource.
func Stat(name string) (fs.FileInfo, error) { return merged{}.Stat(name) }

// Exists reports whether a resource (file or folder) exists.
func Exists(name string) bool {
	_, err := Stat(name)
	return err == nil
}

type merged struct{}

func snapshot() []*mount {
	mu.RLock()
	defer mu.RUnlock()
	return slices.Clone(mounts)
}

// inner maps name to the path inside m, or ok=false if m doesn't cover it.
func (m *mount) inner(name string) (string, bool) {
	switch {
	case m.prefix == "":
		return name, true
	case name == m.prefix:
		return ".", true
	case strings.HasPrefix(name, m.prefix+"/"):
		return name[len(m.prefix)+1:], true
	}
	return "", false
}

func check(op, name string) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
	}
	return nil
}

func (merged) Open(name string) (fs.File, error) {
	if err := check("open", name); err != nil {
		return nil, err
	}
	ms := snapshot()
	for i := len(ms) - 1; i >= 0; i-- {
		if p, ok := ms[i].inner(name); ok {
			f, err := ms[i].fsys.Open(p)
			if err == nil {
				if st, _ := f.Stat(); st != nil && st.IsDir() {
					f.Close()
					return openDir(name)
				}
				return f, nil
			}
			if !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
		}
	}
	if isVirtualDir(ms, name) {
		return openDir(name)
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (merged) ReadFile(name string) ([]byte, error) {
	if err := check("read", name); err != nil {
		return nil, err
	}
	ms := snapshot()
	for i := len(ms) - 1; i >= 0; i-- {
		if p, ok := ms[i].inner(name); ok {
			data, err := fs.ReadFile(ms[i].fsys, p)
			if err == nil {
				return data, nil
			}
			if !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
		}
	}
	return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
}

func (merged) Stat(name string) (fs.FileInfo, error) {
	if err := check("stat", name); err != nil {
		return nil, err
	}
	ms := snapshot()
	for i := len(ms) - 1; i >= 0; i-- {
		if p, ok := ms[i].inner(name); ok {
			st, err := fs.Stat(ms[i].fsys, p)
			if err == nil {
				if st.IsDir() {
					return dirInfo(name), nil
				}
				return st, nil
			}
			if !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
		}
	}
	if isVirtualDir(ms, name) {
		return dirInfo(name), nil
	}
	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

// ReadDir merges the folder's entries from every mount; later mounts win
// on name clashes. Mount prefixes appear as folders.
func (merged) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := check("readdir", name); err != nil {
		return nil, err
	}
	ms := snapshot()
	byName := map[string]fs.DirEntry{}
	found := false
	for _, m := range ms {
		if p, ok := m.inner(name); ok {
			entries, err := fs.ReadDir(m.fsys, p)
			if err == nil {
				found = true
				for _, e := range entries {
					byName[e.Name()] = e
				}
			} else if !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
		}
		// A mount at a/b/c shows up as folder "b" inside "a".
		if child, ok := childOfPrefix(m.prefix, name); ok {
			found = true
			if _, dup := byName[child]; !dup {
				byName[child] = fs.FileInfoToDirEntry(dirInfo(child))
			}
		}
	}
	if !found {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	out := make([]fs.DirEntry, 0, len(byName))
	for _, e := range byName {
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return out, nil
}

// childOfPrefix: for prefix "a/b/c" and dir "a" returns "b".
func childOfPrefix(prefix, dir string) (string, bool) {
	if prefix == "" {
		return "", false
	}
	rest := prefix
	if dir != "." {
		if !strings.HasPrefix(prefix, dir+"/") {
			return "", false
		}
		rest = prefix[len(dir)+1:]
	}
	first, _, _ := strings.Cut(rest, "/")
	return first, first != ""
}

func isVirtualDir(ms []*mount, name string) bool {
	if name == "." {
		return true
	}
	for _, m := range ms {
		if m.prefix == name || strings.HasPrefix(m.prefix, name+"/") {
			return true
		}
	}
	return false
}

// --- directories -----------------------------------------------------------

type dirInfo string

func (d dirInfo) Name() string     { return path.Base(string(d)) }
func (dirInfo) Size() int64        { return 0 }
func (dirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o555 }
func (dirInfo) ModTime() time.Time { return time.Time{} }
func (dirInfo) IsDir() bool        { return true }
func (dirInfo) Sys() any           { return nil }
func (d dirInfo) String() string   { return fs.FormatFileInfo(d) }

type dirFile struct {
	name    string
	entries []fs.DirEntry
	pos     int
}

func openDir(name string) (fs.File, error) {
	entries, err := merged{}.ReadDir(name)
	if err != nil {
		return nil, err
	}
	return &dirFile{name: name, entries: entries}, nil
}

func (d *dirFile) Stat() (fs.FileInfo, error) { return dirInfo(d.name), nil }
func (d *dirFile) Close() error               { return nil }
func (d *dirFile) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.name, Err: errors.New("is a directory")}
}

func (d *dirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	rest := d.entries[d.pos:]
	if n <= 0 {
		d.pos = len(d.entries)
		return rest, nil
	}
	if len(rest) == 0 {
		return nil, io.EOF
	}
	n = min(n, len(rest))
	d.pos += n
	return rest[:n], nil
}
