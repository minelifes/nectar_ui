package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"unicode"

	"github.com/minelifes/nectar_ui/ui/material/icons"
)

// Code generators for an existing app: `nectar add route` and
// `nectar create <kind> <Name>`. They follow the layout `nectar new`
// writes (internal/app, internal/pages, internal/components, ...).

// Project is the app the generators work on.
type Project struct {
	Root   string // the app folder (where go.mod is)
	Module string // its module path
	Out    io.Writer
	Force  bool // overwrite files that exist
}

// FindProject finds the app containing dir: the nearest folder with a
// go.mod, and its module path.
func FindProject(dir string) (Project, error) {
	d, err := filepath.Abs(dir)
	if err != nil {
		return Project{}, err
	}
	for {
		data, err := os.ReadFile(filepath.Join(d, "go.mod"))
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if f := strings.Fields(line); len(f) >= 2 && f[0] == "module" {
					return Project{Root: d, Module: strings.Trim(f[1], `"`)}, nil
				}
			}
			return Project{}, fmt.Errorf("%s has no module line", filepath.Join(d, "go.mod"))
		}
		parent := filepath.Dir(d)
		if parent == d {
			return Project{}, errors.New("not inside a Go module (no go.mod here or above); run it in your app's folder")
		}
		d = parent
	}
}

// --- names -------------------------------------------------------------------

// words splits a name into words, keeping their case: "user-profile" →
// user profile, "userProfile" → user Profile, "HTTPServer" → HTTP Server.
func words(s string) []string {
	var out []string
	rs := []rune(s)
	start := -1
	flush := func(i int) {
		if start >= 0 && i > start {
			out = append(out, string(rs[start:i]))
		}
		start = -1
	}
	for i, r := range rs {
		alnum := unicode.IsLetter(r) || unicode.IsDigit(r)
		if !alnum {
			flush(i)
			continue
		}
		if start < 0 {
			start = i
			continue
		}
		prev := rs[i-1]
		switch {
		case unicode.IsUpper(r) && unicode.IsLower(prev), unicode.IsUpper(r) && unicode.IsDigit(prev):
			flush(i) // userProfile → user|Profile
			start = i
		case unicode.IsUpper(prev) && unicode.IsUpper(r) && i+1 < len(rs) && unicode.IsLower(rs[i+1]):
			flush(i) // HTTPServer → HTTP|Server
			start = i
		}
	}
	flush(len(rs))
	return out
}

func title(w string) string {
	rs := []rune(w)
	rs[0] = unicode.ToUpper(rs[0])
	return string(rs)
}

// exportName is the exported Go name for s ("user-profile" → UserProfile).
func exportName(s string) string {
	var b strings.Builder
	for _, w := range words(s) {
		b.WriteString(title(w))
	}
	n := b.String()
	if n != "" && unicode.IsDigit(rune(n[0])) {
		n = "N" + n
	}
	return n
}

// lowerName is exportName with a lowercase first letter (for unexported
// types).
func lowerName(s string) string {
	ws := words(s)
	if len(ws) == 0 {
		return ""
	}
	n := strings.ToLower(ws[0])
	for _, w := range ws[1:] {
		n += title(w)
	}
	if unicode.IsDigit(rune(n[0])) {
		n = "n" + n
	}
	return n
}

func snakeName(s string) string { return strings.ToLower(strings.Join(words(s), "_")) }

func labelOf(s string) string {
	ws := words(s)
	for i, w := range ws {
		ws[i] = title(w)
	}
	return strings.Join(ws, " ")
}

// pkgName makes a package name from a path segment ("user-profile" →
// userprofile).
func pkgName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
		}
	}
	n := b.String()
	if n == "" || unicode.IsDigit(rune(n[0])) {
		n = "p" + n
	}
	if token.IsKeyword(n) {
		n += "pkg"
	}
	return n
}

// trimSuffixWord drops a trailing type word ("ProfilePage" → "Profile")
// for labels.
func trimSuffixWord(name string, suffixes ...string) string {
	for _, s := range suffixes {
		if t := strings.TrimSuffix(name, s); t != "" && t != name {
			return t
		}
	}
	return name
}

func checkName(kind, name string) (string, error) {
	n := exportName(name)
	if n == "" || !token.IsIdentifier(n) {
		return "", fmt.Errorf("%s name %q isn't a usable Go name", kind, name)
	}
	return n, nil
}

// --- files -------------------------------------------------------------------

// genData is what the templates in templates/gen see.
type genData struct {
	Engine, Module string
	Pkg            string // package name
	Name           string // the type
	Lower          string // lowerCamel of Name
	Recv           string // receiver name
	Snake          string // snake_case of the base name
	Label          string // "User Profile"
	Path           string // route path, for pages
	File           string // the file's path in the app
	Components     bool   // internal/components has Page (material apps)
}

func newGenData(p Project, pkg, name, base string) genData {
	return genData{Engine: EnginePath, Module: p.Module, Pkg: pkg, Name: name,
		Lower: lowerName(name), Recv: strings.ToLower(name[:1]), Snake: snakeName(base),
		Label: labelOf(base), Components: hasType(filepath.Join(p.Root, "internal", "components"), "Page")}
}

// writeGen renders templates/gen/<tmpl> to rel (inside the app), gofmt'ed.
func (p Project) writeGen(tmpl, rel string, d genData) error {
	d.File = filepath.ToSlash(rel)
	raw, err := templateFS.ReadFile("templates/gen/" + tmpl)
	if err != nil {
		return err
	}
	t, err := template.New(tmpl).Delims("[[", "]]").Parse(string(raw))
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, d); err != nil {
		return err
	}
	return p.writeGo(rel, buf.Bytes())
}

// writeGo formats src and writes it to rel, refusing to overwrite unless
// Force.
func (p Project) writeGo(rel string, src []byte) error {
	out, err := format.Source(src)
	if err != nil {
		return fmt.Errorf("generating %s: %w", rel, err)
	}
	path := filepath.Join(p.Root, rel)
	if _, err := os.Stat(path); err == nil && !p.Force {
		return fmt.Errorf("%s exists (use -force to overwrite)", filepath.ToSlash(rel))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return err
	}
	newPrinter(p.Out).step("create", filepath.ToSlash(rel))
	return nil
}

// packageOf returns the package name used in dir (from a .go file there),
// or def when the folder has none yet.
func packageOf(dir, def string) string {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, n), nil, parser.PackageClauseOnly)
		if err == nil {
			return f.Name.Name
		}
	}
	return def
}

// hasType reports whether a non-test file in dir declares type name.
func hasType(dir, name string) bool {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, n), nil, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok {
				for _, s := range g.Specs {
					if ts, ok := s.(*ast.TypeSpec); ok && ts.Name.Name == name {
						return true
					}
				}
			}
		}
	}
	return false
}

// --- create ------------------------------------------------------------------

// CreateOptions configures Create.
type CreateOptions struct {
	Dir   string // output folder inside the app (default: per kind)
	Route string // page / mvvm: also register at this route path
	Icon  string // with Route: the rail icon
}

// Kinds are what `nectar create` makes, with their aliases.
var Kinds = []struct {
	Name, Aliases, Description string
}{
	{"stateless", "statelesswidget widget", "a stateless widget (internal/components)"},
	{"stateful", "statefulwidget statefullwidget", "a stateful widget with its State (internal/components)"},
	{"page", "screen", "a page package (internal/pages/<name>); -route also registers it"},
	{"component", "", "a reusable stateless widget (internal/components)"},
	{"model", "", "a data model (internal/models)"},
	{"mvvm", "feature viewmodel", "a feature: model, view model, view and test (internal/features/<name>); -route registers the view"},
	{"dialog", "", "a dialog with a Show function (internal/components)"},
	{"service", "", "a service interface + implementation, provided to widgets (internal/services)"},
}

func kindOf(s string) (string, bool) {
	s = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(s, "-", ""), "_", ""))
	for _, k := range Kinds {
		if s == k.Name || slices.Contains(strings.Fields(k.Aliases), s) {
			return k.Name, true
		}
	}
	return "", false
}

// Create generates a kind of code named name in the app.
func Create(p Project, kind, name string, o CreateOptions) error {
	k, ok := kindOf(kind)
	if !ok {
		return fmt.Errorf("unknown kind %q (nectar create -h lists them)", kind)
	}
	n, err := checkName(k, name)
	if err != nil {
		return err
	}
	dir := func(def string) string {
		if o.Dir != "" {
			return filepath.Clean(o.Dir)
		}
		return def
	}
	pr := newPrinter(p.Out)
	pr.header("create", pr.bold(k)+" "+pr.cyan(n))
	switch k {
	case "stateless", "component", "stateful":
		d := dir(filepath.Join("internal", "components"))
		pkg := packageOf(filepath.Join(p.Root, d), pkgName(filepath.Base(d)))
		tmpl := "stateless.go.tmpl"
		if k == "stateful" {
			tmpl = "stateful.go.tmpl"
		}
		err = p.writeGen(tmpl, filepath.Join(d, snakeName(n)+".go"), newGenData(p, pkg, n, n))
	case "page":
		if o.Route != "" {
			err = AddRoute(p, RouteOptions{Path: o.Route, Name: n, Icon: o.Icon, quiet: true})
			break
		}
		base := trimSuffixWord(n, "Page", "Screen")
		d := dir(filepath.Join("internal", "pages", snakeName(base)))
		pkg := packageOf(filepath.Join(p.Root, d), pkgName(filepath.Base(d)))
		gd := newGenData(p, pkg, n, base)
		err = p.writeGen("page.go.tmpl", filepath.Join(d, pkg+".go"), gd)
	case "model":
		d := dir(filepath.Join("internal", "models"))
		pkg := packageOf(filepath.Join(p.Root, d), pkgName(filepath.Base(d)))
		err = p.writeGen("model.go.tmpl", filepath.Join(d, snakeName(n)+".go"), newGenData(p, pkg, n, n))
	case "mvvm":
		base := trimSuffixWord(n, "ViewModel", "VM", "Feature")
		d := dir(filepath.Join("internal", "features", snakeName(base)))
		pkg := packageOf(filepath.Join(p.Root, d), pkgName(strings.ReplaceAll(snakeName(base), "_", "")))
		gd := newGenData(p, pkg, exportName(base), base)
		for _, f := range [][2]string{
			{"mvvm_model.go.tmpl", "model.go"},
			{"mvvm_viewmodel.go.tmpl", "viewmodel.go"},
			{"mvvm_view.go.tmpl", "view.go"},
			{"mvvm_viewmodel_test.go.tmpl", "viewmodel_test.go"},
		} {
			if err = p.writeGen(f[0], filepath.Join(d, f[1]), gd); err != nil {
				return err
			}
		}
		if o.Route != "" {
			err = AddRoute(p, RouteOptions{Path: o.Route, Name: "View", Label: labelOf(base), Icon: o.Icon,
				PageDir: d, quiet: true})
		}
	case "dialog":
		if !strings.HasSuffix(n, "Dialog") {
			n += "Dialog"
		}
		d := dir(filepath.Join("internal", "components"))
		pkg := packageOf(filepath.Join(p.Root, d), pkgName(filepath.Base(d)))
		gd := newGenData(p, pkg, n, trimSuffixWord(n, "Dialog"))
		err = p.writeGen("dialog.go.tmpl", filepath.Join(d, snakeName(n)+".go"), gd)
	case "service":
		if !strings.HasSuffix(n, "Service") {
			n += "Service"
		}
		d := dir(filepath.Join("internal", "services"))
		pkg := packageOf(filepath.Join(p.Root, d), pkgName(filepath.Base(d)))
		gd := newGenData(p, pkg, n, n)
		err = p.writeGen("service.go.tmpl", filepath.Join(d, snakeName(n)+".go"), gd)
	}
	if err != nil {
		return err
	}
	pr.printf("\n")
	pr.success("Created %s %s", k, n)
	pr.printf("\n")
	return nil
}

// --- add route -------------------------------------------------------------

// RouteOptions configures AddRoute.
type RouteOptions struct {
	Path   string // "/settings/profile"
	Name   string // the page type ("Profile")
	Label  string // rail label (default: from Name)
	Icon   string // Material icon name (default: article)
	Hidden bool   // no rail entry
	// RoutesFile is the file with the Routes list (default
	// internal/app/routes.go).
	RoutesFile string
	// PageDir uses an existing package instead of creating a page in
	// internal/pages/<path> (Name is then its widget type).
	PageDir string

	quiet bool // part of another command: no header
}

var segmentRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// cleanRoutePath normalizes a route path: one leading "/", no trailing one.
func cleanRoutePath(p string) string { return "/" + strings.Trim(strings.TrimSpace(p), "/") }

// AddRoute creates a page (unless it exists) and registers it in Routes.
func AddRoute(p Project, o RouteOptions) error {
	pr := newPrinter(p.Out)
	path := cleanRoutePath(o.Path)
	var segs []string
	if path != "/" {
		segs = strings.Split(strings.TrimPrefix(path, "/"), "/")
	}
	for _, s := range segs {
		if !segmentRE.MatchString(s) {
			return fmt.Errorf("route path %q: segment %q may only use letters, digits, - and _", o.Path, s)
		}
	}
	name, err := checkName("page", o.Name)
	if err != nil {
		return err
	}
	if !o.quiet {
		pr.header("add route", pr.cyan(path)+" "+pr.bold(name))
	}
	routesRel := o.RoutesFile
	if routesRel == "" {
		routesRel = filepath.Join("internal", "app", "routes.go")
	}
	routesPath := filepath.Join(p.Root, routesRel)
	src, err := os.ReadFile(routesPath)
	if err != nil {
		return fmt.Errorf("no routes file %s (apps made with `nectar new -template material` have one; pass -routes for another file): %w", filepath.ToSlash(routesRel), err)
	}
	ed, err := parseRoutes(routesPath, src)
	if err != nil {
		return err
	}
	ed.fillPaths() // older apps: give the existing routes paths first
	if slices.Contains(ed.paths, path) {
		return fmt.Errorf("route %s exists in %s", path, filepath.ToSlash(routesRel))
	}

	// The page: an existing package, or a new one under internal/pages.
	pageDir := o.PageDir
	if pageDir == "" {
		if len(segs) == 0 {
			pageDir = filepath.Join("internal", "pages", snakeName(trimSuffixWord(name, "Page", "Screen")))
		} else {
			pageDir = filepath.Join(append([]string{"internal", "pages"}, segs...)...)
		}
		abs := filepath.Join(p.Root, pageDir)
		pkg := packageOf(abs, pkgName(filepath.Base(pageDir)))
		if !hasType(abs, name) {
			label := o.Label
			if label == "" {
				label = labelOf(trimSuffixWord(name, "Page", "Screen"))
			}
			gd := newGenData(p, pkg, name, trimSuffixWord(name, "Page", "Screen"))
			gd.Label, gd.Path = label, path
			file := pkg + ".go"
			if _, err := os.Stat(filepath.Join(abs, file)); err == nil {
				file = snakeName(name) + ".go"
			}
			if err := p.writeGen("page.go.tmpl", filepath.Join(pageDir, file), gd); err != nil {
				return err
			}
		}
	}
	pkg := packageOf(filepath.Join(p.Root, pageDir), pkgName(filepath.Base(pageDir)))
	importPath := p.Module + "/" + filepath.ToSlash(pageDir)

	iconsName := ed.importName(EnginePath + "/ui/material/icons")
	icon, err := iconIdent(o.Icon)
	if err != nil {
		return err
	}
	if iconsName == "" {
		iconsName = ed.addImport("icons", EnginePath+"/ui/material/icons")
	}
	iconExpr := iconsName + "." + icon
	pkgRef := ed.importName(importPath)
	if pkgRef == "" {
		alias := pkg
		if ed.names[alias] {
			alias = pkgName(strings.Join(segs, ""))
			if ed.names[alias] || alias == pkg {
				alias = pkg + "page"
			}
		}
		pkgRef = ed.addImport(alias, importPath)
	}
	widgets := ed.importName(EnginePath + "/ui/widgets")
	if widgets == "" {
		widgets = ed.addImport("w", EnginePath+"/ui/widgets")
	}
	if !ed.fields["Path"] {
		ed.addField("Path string // \"/settings/profile\"; app.Go(ctx, path) shows it")
	}
	if o.Hidden && !ed.fields["Hidden"] {
		ed.addField("Hidden bool // reachable with app.Go, no navigation rail entry")
	}
	label := o.Label
	if label == "" {
		label = labelOf(trimSuffixWord(name, "Page", "Screen"))
	}
	entry := fmt.Sprintf("{Path: %q, Label: %q, Icon: %s", path, label, iconExpr)
	if o.Hidden {
		entry += ", Hidden: true"
	}
	entry += fmt.Sprintf(", Page: func() %s.Widget { return %s.%s{} }},", widgets, pkgRef, name)
	ed.addEntry(entry)
	out, err := ed.apply()
	if err != nil {
		return fmt.Errorf("updating %s: %w", filepath.ToSlash(routesRel), err)
	}
	if err := os.WriteFile(routesPath, out, 0o644); err != nil {
		return err
	}
	pr.step("update", filepath.ToSlash(routesRel)+pr.dim("  + "+path))

	// app.Go: apps made before routes had paths have no router yet.
	appDir := filepath.Dir(routesPath)
	if !hasFunc(appDir, "Go") {
		rel := filepath.Join(filepath.Dir(routesRel), "router.go")
		gd := newGenData(p, packageOf(appDir, "app"), "Router", "router")
		if err := p.writeRouter(rel, gd); err != nil {
			return err
		}
		pr.step("note", "the Shell must show Routes by path for app.Go to work: see the router in a new `nectar new` app (internal/app/shell.go)")
	}
	if !o.quiet {
		pr.printf("\n")
		pr.success("Added route %s → %s.%s", path, pkgRef, name)
		pr.printf("  %s %s\n\n", pr.dim("open it from any widget:"), pr.cyan(fmt.Sprintf("app.Go(ctx, %q)", path)))
	}
	return nil
}

// writeRouter writes the material template's router into an older app.
func (p Project) writeRouter(rel string, gd genData) error {
	raw, err := templateFS.ReadFile("templates/material/internal/app/router.go.tmpl")
	if err != nil {
		return err
	}
	t, err := template.New("router").Delims("[[", "]]").Parse(string(raw))
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data{Engine: EnginePath, Module: p.Module}); err != nil {
		return err
	}
	src := bytes.Replace(buf.Bytes(), []byte("package app\n"), []byte("package "+gd.Pkg+"\n"), 1)
	return p.writeGo(rel, src)
}

// hasFunc reports whether a non-test file in dir declares func name.
func hasFunc(dir, name string) bool {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, n), nil, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == name {
				return true
			}
		}
	}
	return false
}

// iconIdent turns "person", "Person" or "arrow_back" into the icons
// package identifier, checking it exists (default: Article).
func iconIdent(s string) (string, error) {
	if s == "" {
		return "Article", nil
	}
	ident := func(snake string) string {
		var b strings.Builder
		for _, part := range strings.Split(snake, "_") {
			if part != "" {
				b.WriteString(strings.ToUpper(part[:1]) + part[1:])
			}
		}
		id := b.String()
		if id != "" && unicode.IsDigit(rune(id[0])) {
			id = "N" + id
		}
		return id
	}
	if _, ok := icons.ByName[strings.ToLower(s)]; ok {
		return ident(strings.ToLower(s)), nil
	}
	for snake := range icons.ByName {
		if id := ident(snake); id == s || strings.EqualFold(id, s) {
			return id, nil
		}
	}
	return "", fmt.Errorf("no Material icon %q (use its name, e.g. person, arrow_back, settings)", s)
}

// --- editing routes.go -------------------------------------------------------

// routesEdit is a set of text insertions into routes.go, found with the
// parser so the rest of the file (comments, layout) stays as it is.
type routesEdit struct {
	src     []byte
	imports map[string]string // import path → local name
	names   map[string]bool   // local names taken
	fields  map[string]bool   // Route's fields
	paths   []string          // existing route paths
	noPath  []pathless        // entries without a Path (apps made before paths)

	importAt     int  // offset to insert import lines at
	importParen  bool // the import decl has parentheses
	structAt     int  // offset right after Route's "{"
	entriesAt    int  // offset of the Routes literal's closing "}"
	entriesTrail bool // the last entry has no trailing comma
	inserts      []insert
}

type pathless struct {
	at    int // offset right after the entry's "{"
	label string
	first bool
}

type insert struct {
	at   int
	text string
}

func parseRoutes(path string, src []byte) (*routesEdit, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	off := func(p token.Pos) int { return fset.Position(p).Offset }
	ed := &routesEdit{src: src, imports: map[string]string{}, names: map[string]bool{}, fields: map[string]bool{}, entriesAt: -1, structAt: -1}
	for _, im := range f.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		name := filepath.Base(p)
		if im.Name != nil {
			name = im.Name.Name
		}
		ed.imports[p] = name
		ed.names[name] = true
	}
	ed.importAt = -1
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok {
			continue
		}
		switch g.Tok {
		case token.IMPORT:
			if g.Lparen.IsValid() {
				ed.importAt, ed.importParen = off(g.Rparen), true
			} else {
				ed.importAt = off(g.End())
			}
		case token.TYPE:
			for _, s := range g.Specs {
				ts := s.(*ast.TypeSpec)
				if st, ok := ts.Type.(*ast.StructType); ok && ts.Name.Name == "Route" {
					ed.structAt = off(st.Fields.Opening) + 1
					for _, fl := range st.Fields.List {
						for _, n := range fl.Names {
							ed.fields[n.Name] = true
						}
					}
				}
			}
		case token.VAR:
			for _, s := range g.Specs {
				vs := s.(*ast.ValueSpec)
				for i, n := range vs.Names {
					if n.Name != "Routes" || i >= len(vs.Values) {
						continue
					}
					cl, ok := vs.Values[i].(*ast.CompositeLit)
					if !ok {
						return nil, errors.New("Routes isn't a []Route{...} literal")
					}
					ed.entriesAt = off(cl.Rbrace)
					if len(cl.Elts) > 0 {
						last := off(cl.Elts[len(cl.Elts)-1].End())
						ed.entriesTrail = !bytes.Contains(src[last:ed.entriesAt], []byte(","))
					}
					for ei, e := range cl.Elts {
						lit, ok := e.(*ast.CompositeLit)
						if !ok {
							continue
						}
						hasPath, label := false, ""
						for _, kv := range lit.Elts {
							kv, ok := kv.(*ast.KeyValueExpr)
							if !ok {
								continue
							}
							k, _ := kv.Key.(*ast.Ident)
							bl, _ := kv.Value.(*ast.BasicLit)
							if k == nil {
								continue
							}
							switch k.Name {
							case "Path":
								hasPath = true
								if bl != nil {
									if v, err := strconv.Unquote(bl.Value); err == nil {
										ed.paths = append(ed.paths, cleanRoutePath(v))
									}
								}
							case "Label":
								if bl != nil {
									label, _ = strconv.Unquote(bl.Value)
								}
							}
						}
						if !hasPath {
							ed.noPath = append(ed.noPath, pathless{off(lit.Lbrace) + 1, label, ei == 0})
						}
					}
				}
			}
		}
	}
	switch {
	case ed.entriesAt < 0:
		return nil, errors.New("no `var Routes = []Route{...}` in the routes file")
	case ed.structAt < 0:
		return nil, errors.New("no `type Route struct` in the routes file")
	case ed.importAt < 0:
		return nil, errors.New("the routes file has no imports")
	}
	return ed, nil
}

func (ed *routesEdit) importName(path string) string { return ed.imports[path] }

// addImport imports path as name (no alias when it's the default) and
// returns the name.
func (ed *routesEdit) addImport(name, path string) string {
	spec := strconv.Quote(path)
	if name != filepath.Base(path) {
		spec = name + " " + spec
	}
	if ed.importParen {
		ed.inserts = append(ed.inserts, insert{ed.importAt, "\t" + spec + "\n"})
	} else {
		ed.inserts = append(ed.inserts, insert{ed.importAt, "\nimport " + spec + "\n"})
	}
	ed.imports[path], ed.names[name] = name, true
	return name
}

// fillPaths gives routes without a Path one: "/" for the first (the start
// page), "/" + the label in snake case for the rest.
func (ed *routesEdit) fillPaths() {
	for _, e := range ed.noPath {
		p := "/"
		if !e.first || slices.Contains(ed.paths, "/") {
			base := "/" + snakeName(e.label)
			if base == "/" {
				base = "/page"
			}
			p = base
			for n := 2; slices.Contains(ed.paths, p); n++ {
				p = fmt.Sprintf("%s_%d", base, n)
			}
		}
		ed.paths = append(ed.paths, p)
		ed.inserts = append(ed.inserts, insert{e.at, fmt.Sprintf("Path: %q, ", p)})
	}
	ed.noPath = nil
}

func (ed *routesEdit) addField(line string) {
	ed.inserts = append(ed.inserts, insert{ed.structAt, "\n\t" + line})
	ed.fields[strings.Fields(line)[0]] = true
}

func (ed *routesEdit) addEntry(entry string) {
	text := "\t" + entry + "\n"
	if ed.entriesTrail {
		text = ",\n" + text
	}
	ed.inserts = append(ed.inserts, insert{ed.entriesAt, text})
}

// apply returns the source with the insertions, gofmt'ed.
func (ed *routesEdit) apply() ([]byte, error) {
	// Back to front, and at one offset the last added first, so text
	// inserted at the same place keeps the order it was added in.
	ins := slices.Clone(ed.inserts)
	slices.Reverse(ins)
	sort.SliceStable(ins, func(i, j int) bool { return ins[i].at > ins[j].at })
	out := slices.Clone(ed.src)
	for _, in := range ins {
		out = slices.Insert(out, in.at, []byte(in.text)...)
	}
	return format.Source(out)
}

// --- commands --------------------------------------------------------------

func cmdAdd(args []string) error {
	if len(args) == 0 || args[0] != "route" {
		return errors.New("usage: nectar add route <path> <PageName> [flags] (only routes can be added)")
	}
	var o RouteOptions
	var force bool
	fs := flag.NewFlagSet("add route", flag.ContinueOnError)
	fs.StringVar(&o.Label, "label", "", "navigation label (default: from the page name)")
	fs.StringVar(&o.Icon, "icon", "", "Material icon for the rail, e.g. person, settings (default: article)")
	fs.BoolVar(&o.Hidden, "hidden", false, "no navigation rail entry; open it with app.Go(ctx, path)")
	fs.StringVar(&o.RoutesFile, "routes", "", "routes file (default: internal/app/routes.go)")
	fs.BoolVar(&force, "force", false, "overwrite a page file that exists")
	fs.Usage = func() {
		p := newPrinter(os.Stderr)
		p.printf("\n%s %s %s\n\n", p.bold("Usage:"), p.green("nectar add route"), p.dim("<path> <PageName> [flags]"))
		p.printf("Creates the page in internal/pages/<path> (unless it exists) and adds it to Routes:\n\n")
		p.printf("  %s\n\n", p.cyan(`nectar add route /settings/profile Profile -icon person`))
		p.flags(fs)
		p.printf("\n")
	}
	rest, err := parseInterleaved(fs, args[1:])
	if err != nil {
		return err
	}
	if len(rest) != 2 {
		fs.Usage()
		return fmt.Errorf("add route needs a path and a page name, got %d arguments", len(rest))
	}
	o.Path, o.Name = rest[0], rest[1]
	p, err := FindProject(".")
	if err != nil {
		return err
	}
	p.Out, p.Force = os.Stdout, force
	return AddRoute(p, o)
}

// cmdGenerate is `nectar create <kind> <Name>`.
func cmdGenerate(kind string, args []string) error {
	var o CreateOptions
	var force bool
	fs := createFlags(&o, &force)
	rest, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		fs.Usage()
		return fmt.Errorf("create %s needs one name, got %d", kind, len(rest))
	}
	p, err := FindProject(".")
	if err != nil {
		return err
	}
	p.Out, p.Force = os.Stdout, force
	return Create(p, kind, rest[0], o)
}

func createFlags(o *CreateOptions, force *bool) *flag.FlagSet {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	fs.StringVar(&o.Dir, "dir", "", "output folder inside the app (default: per kind)")
	fs.StringVar(&o.Route, "route", "", "page / mvvm: also add it to Routes at this path")
	fs.StringVar(&o.Icon, "icon", "", "with -route: Material icon for the rail")
	fs.BoolVar(force, "force", false, "overwrite files that exist")
	fs.Usage = func() { createUsage(fs) }
	return fs
}

func createUsage(fs *flag.FlagSet) {
	if fs == nil {
		fs = createFlags(&CreateOptions{}, new(bool))
	}
	p := newPrinter(os.Stderr)
	p.printf("\n%s %s %s\n\n", p.bold("Usage:"), p.green("nectar create"), p.dim("<kind> <Name> [flags]"))
	p.printf("%s\n", p.bold("Kinds"))
	for _, k := range Kinds {
		alias := ""
		if k.Aliases != "" {
			alias = p.dim(" (" + strings.ReplaceAll(k.Aliases, " ", ", ") + ")")
		}
		p.printf("  %s %s%s\n", p.green(pad(k.Name, 10)), k.Description, alias)
	}
	p.printf("\n%s\n", p.bold("Flags"))
	p.flags(fs)
	p.printf("\n%s %s\n\n", p.dim("New app:"), p.cyan("nectar new <dir>"))
}

// parseInterleaved parses flags given before, between or after the
// positional arguments and returns the positionals.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var rest []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return rest, nil
		}
		rest = append(rest, fs.Arg(0))
		args = fs.Args()[1:]
	}
}
