package material

import (
	"io/fs"
	"path"
	"strings"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/material/icons"
	"github.com/minelifes/nectar_ui/ui/vector"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// TreeView is widgets.TreeView with M3 styling: pill-shaped rows, a
// secondary-container selection, state-layer hover and a focus outline.
type TreeView struct {
	Roots      []w.TreeNode
	Controller *w.TreeController

	OnSelect   func(node w.TreeNode)
	OnActivate func(node w.TreeNode)
	OnToggle   func(node w.TreeNode, expanded bool)

	RowHeight      float32 // default 36
	Indent         float32 // default 20
	ShowGuides     bool    // vertical indent guide lines
	TapSelectsOnly bool
	ShrinkWrap     bool
	Row            func(ctx w.BuildContext, row w.TreeRow) w.Widget
}

func (t TreeView) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	sc := th.Scheme
	h, indent := t.RowHeight, t.Indent
	if h == 0 {
		h = 36
	}
	if indent == 0 {
		indent = 20
	}
	st := w.TreeStyle{
		Text:          Styled(th.Text.LabelLarge, sc.OnSurfaceVariant),
		SelectedText:  sc.OnSecondaryContainer,
		IconColor:     sc.OnSurfaceVariant,
		SelectedColor: sc.SecondaryContainer,
		HoverColor:    sc.OnSurface.WithAlpha(HoverOpacity),
		FocusColor:    sc.Secondary,
		Radius:        CornerFull,
		Inset:         4,
	}
	if t.ShowGuides {
		st.GuideColor = sc.OutlineVariant
	}
	return w.TreeView{
		Roots: t.Roots, Controller: t.Controller,
		OnSelect: t.OnSelect, OnActivate: t.OnActivate, OnToggle: t.OnToggle,
		RowHeight: h, Indent: indent, TapSelectsOnly: t.TapSelectsOnly, ShrinkWrap: t.ShrinkWrap,
		Style: st, ThumbColor: sc.OnSurface.WithAlpha(0.3), Row: t.Row,
	}
}

// FileTree browses a file system (os.DirFS(dir) for a real folder, or an
// embed.FS / fstest.MapFS) as a TreeView: folders first, lazily loaded,
// with icons by file type. Node keys are slash paths relative to the FS
// root, so Controller.Expand("src/ui") and Controller.Reload("src") work
// with paths.
type FileTree struct {
	FS   fs.FS
	Root string // folder inside FS to show; default "."
	// ShowRoot adds the root folder itself as a single expanded top node
	// (labelled RootLabel, default the folder's name).
	ShowRoot   bool
	RootLabel  string
	ShowHidden bool
	FilesFirst bool
	Filter     func(p string, d fs.DirEntry) bool

	Controller *w.TreeController
	OnSelect   func(p string, d fs.DirEntry) // d is nil for the ShowRoot node
	OnOpen     func(p string)                // double-click / Enter on a file

	RowHeight  float32
	Indent     float32
	ShowGuides bool
	ShrinkWrap bool
}

func (f FileTree) Build(ctx w.BuildContext) w.Widget {
	sc := ThemeOf(ctx).Scheme
	root := f.Root
	if root == "" {
		root = "."
	}
	opt := w.FileTreeOptions{ShowHidden: f.ShowHidden, FilesFirst: f.FilesFirst, Filter: f.Filter, Icon: FileIcon}
	var roots []w.TreeNode
	if f.FS != nil {
		if f.ShowRoot {
			label := f.RootLabel
			if label == "" {
				label = path.Base(root)
			}
			roots = []w.TreeNode{{Key: root, Label: label, Branch: true, Expanded: true,
				Icon: FileIcon(root, true, false), ExpandedIcon: FileIcon(root, true, true),
				Load: func() []w.TreeNode { return colorFolders(w.FileNodes(f.FS, root, opt), sc.Primary) }}}
			roots[0].IconColor = sc.Primary
		} else {
			roots = colorFolders(w.FileNodes(f.FS, root, opt), sc.Primary)
		}
	}
	entry := func(n w.TreeNode) fs.DirEntry {
		d, _ := n.Data.(fs.DirEntry)
		return d
	}
	return TreeView{
		Roots: roots, Controller: f.Controller,
		RowHeight: f.RowHeight, Indent: f.Indent, ShowGuides: f.ShowGuides, ShrinkWrap: f.ShrinkWrap,
		OnSelect: func(n w.TreeNode) {
			if f.OnSelect != nil {
				f.OnSelect(n.Key, entry(n))
			}
		},
		OnActivate: func(n w.TreeNode) {
			if f.OnOpen != nil && !n.Expandable() {
				f.OnOpen(n.Key)
			}
		},
	}
}

// colorFolders tints folder icons (and those of lazily loaded subfolders).
func colorFolders(nodes []w.TreeNode, c geom.Color) []w.TreeNode {
	for i := range nodes {
		if nodes[i].Branch {
			nodes[i].IconColor = c
			if load := nodes[i].Load; load != nil {
				nodes[i].Load = func() []w.TreeNode { return colorFolders(load(), c) }
			}
		}
	}
	return nodes
}

// FileIcon picks a Material icon for a path by its extension (open = an
// expanded folder).
func FileIcon(p string, dir, open bool) *vector.Icon {
	if dir {
		if open {
			return icons.FolderOpen
		}
		return icons.Folder
	}
	switch strings.ToLower(path.Ext(p)) {
	case ".go", ".c", ".h", ".cpp", ".hpp", ".cc", ".rs", ".py", ".java", ".kt", ".swift", ".rb", ".php",
		".ts", ".tsx", ".jsx", ".wgsl", ".glsl", ".hlsl", ".metal", ".lua", ".zig", ".dart", ".cs", ".mod", ".sum":
		return icons.Code
	case ".js", ".mjs":
		return icons.Javascript
	case ".html", ".htm":
		return icons.Html
	case ".css", ".scss":
		return icons.Css
	case ".json", ".yaml", ".yml", ".toml", ".xml", ".ini":
		return icons.DataObject
	case ".sh", ".bash", ".zsh", ".fish", ".bat", ".ps1":
		return icons.Terminal
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".bmp", ".ico", ".tiff":
		return icons.Image
	case ".mp4", ".mov", ".mkv", ".webm", ".avi":
		return icons.Movie
	case ".mp3", ".wav", ".flac", ".ogg", ".m4a":
		return icons.AudioFile
	case ".pdf":
		return icons.PictureAsPdf
	case ".md", ".txt", ".rst", ".doc", ".docx", ".rtf":
		return icons.Description
	case ".zip", ".tar", ".gz", ".tgz", ".7z", ".rar", ".xz":
		return icons.FolderZip
	}
	return icons.InsertDriveFile
}
