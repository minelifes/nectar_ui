package widgets

import (
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/minelifes/nectar_ui/ui/vector"
)

// FileTreeOptions configures FileNodes.
type FileTreeOptions struct {
	ShowHidden bool // include names starting with "."
	FilesFirst bool // list files before folders (default: folders first)
	// Filter, if set, drops entries it returns false for.
	Filter func(p string, d fs.DirEntry) bool
	// Icon picks an entry's icon (open = the expanded variant of a folder).
	Icon func(p string, dir, open bool) *vector.Icon
}

// FileNodes lists dir of fsys (use os.DirFS for the real disk) as tree
// nodes: folders are loaded lazily when expanded. Keys are slash paths
// relative to the fsys root, and Data holds the fs.DirEntry. Entries are
// sorted by name, ignoring case. A folder that can't be read is empty.
func FileNodes(fsys fs.FS, dir string, opt FileTreeOptions) []TreeNode {
	if dir == "" {
		dir = "."
	}
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil
	}
	slices.SortStableFunc(entries, func(a, b fs.DirEntry) int {
		if a.IsDir() != b.IsDir() {
			if a.IsDir() != opt.FilesFirst {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name()), strings.ToLower(b.Name()))
	})
	var out []TreeNode
	for _, e := range entries {
		name := e.Name()
		if !opt.ShowHidden && strings.HasPrefix(name, ".") {
			continue
		}
		p := path.Join(dir, name)
		if opt.Filter != nil && !opt.Filter(p, e) {
			continue
		}
		n := TreeNode{Key: p, Label: name, Data: e}
		if e.IsDir() {
			n.Branch = true
			n.Load = func() []TreeNode { return FileNodes(fsys, p, opt) }
		}
		if opt.Icon != nil {
			n.Icon = opt.Icon(p, e.IsDir(), false)
			if e.IsDir() {
				n.ExpandedIcon = opt.Icon(p, true, true)
			}
		}
		out = append(out, n)
	}
	return out
}
