package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// genApp writes a material app into testdata inside this module, so it
// builds against this engine checkout without fetching anything (its
// go.mod is removed: the engine's module covers it).
func genApp(t *testing.T) Project {
	t.Helper()
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("testdata", "genapp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir); os.Remove("testdata") })
	module := EnginePath + "/cmd/nectar/" + filepath.ToSlash(dir)
	if err := Generate(Options{Dir: dir, Module: module, Template: "material", NoTidy: true, Force: true}); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(dir, "go.mod"))
	abs, _ := filepath.Abs(dir)
	return Project{Root: abs, Module: module, Out: &bytes.Buffer{}}
}

func goIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func read(t *testing.T, p Project, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(p.Root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestGeneratorsBuildAndPass(t *testing.T) {
	if testing.Short() {
		t.Skip("builds an app")
	}
	p := genApp(t)
	steps := []func() error{
		func() error {
			return AddRoute(p, RouteOptions{Path: "/settings/profile", Name: "Profile", Icon: "person"})
		},
		func() error { return AddRoute(p, RouteOptions{Path: "about/", Name: "AboutPage", Hidden: true}) },
		func() error { return Create(p, "statelessWidget", "status-badge", CreateOptions{}) },
		func() error { return Create(p, "statefullWidget", "Counter", CreateOptions{}) },
		func() error { return Create(p, "model", "user", CreateOptions{}) },
		func() error { return Create(p, "model", "Type", CreateOptions{}) }, // a keyword as name
		func() error { return Create(p, "mvvm", "Todo", CreateOptions{Route: "/todos", Icon: "checklist"}) },
		func() error { return Create(p, "page", "Reports", CreateOptions{}) },
		func() error { return Create(p, "page", "Stats", CreateOptions{Route: "/reports/stats"}) },
		func() error { return Create(p, "dialog", "Confirm", CreateOptions{}) },
		func() error { return Create(p, "service", "storage", CreateOptions{}) },
		func() error { return Create(p, "component", "Avatar", CreateOptions{Dir: "internal/ui/parts"}) },
	}
	for i, s := range steps {
		if err := s(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	routes := read(t, p, "internal/app/routes.go")
	for _, want := range []string{
		`{Path: "/settings/profile", Label: "Profile", Icon: icons.Person, Page: func() w.Widget { return profile.Profile{} }}`,
		`{Path: "/about", Label: "About", Icon: icons.Article, Hidden: true, Page: func() w.Widget { return about.AboutPage{} }}`,
		`{Path: "/todos", Label: "Todo", Icon: icons.Checklist, Page: func() w.Widget { return todo.View{} }}`,
		`"` + p.Module + `/internal/pages/settings/profile"`,
		`"` + p.Module + `/internal/features/todo"`,
		"// Route is a page of the app", // comments survive
	} {
		if !strings.Contains(routes, want) {
			t.Errorf("routes.go lacks %s\n%s", want, routes)
		}
	}
	for _, f := range []string{
		"internal/pages/settings/profile/profile.go", "internal/pages/about/about.go",
		"internal/components/status_badge.go", "internal/components/counter.go",
		"internal/models/user.go", "internal/models/type.go",
		"internal/features/todo/model.go", "internal/features/todo/viewmodel.go",
		"internal/features/todo/view.go", "internal/features/todo/viewmodel_test.go",
		"internal/pages/reports/reports.go", "internal/pages/reports/stats/stats.go",
		"internal/components/confirm_dialog.go", "internal/services/storage_service.go",
		"internal/ui/parts/avatar.go",
	} {
		if _, err := os.Stat(filepath.Join(p.Root, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	// A navigation test using the generated routes, next to the app's own.
	nav := `package tests

import (
	"testing"

	"` + EnginePath + `/ui/tester"

	"` + p.Module + `/internal/app"
)

func TestGeneratedRoutes(t *testing.T) {
	tt := tester.New(app.New(), 1100, 720)
	tt.Settle()
	for _, label := range []string{"Profile", "Todo"} {
		if err := tt.TapText(label); err != nil {
			t.Fatalf("rail %s: %v (%v)", label, err, tt.Texts())
		}
		tt.Settle()
	}
	tt.TapText("New item")
	tt.Type("Buy milk")
	tt.TapText("Add")
	tt.Settle()
	if _, ok := tt.Find("Buy milk"); !ok {
		t.Fatalf("mvvm page didn't add the item: %v", tt.Texts())
	}
	if _, ok := tt.Find("About"); ok {
		t.Fatal("hidden route shown in the rail")
	}
	if _, ok := app.RouteFor("/settings/profile/"); !ok {
		t.Fatal("RouteFor")
	}
}
`
	if err := os.WriteFile(filepath.Join(p.Root, "tests", "routes_test.go"), []byte(nav), 0o644); err != nil {
		t.Fatal(err)
	}
	goIn(t, p.Root, "vet", "./...")
	goIn(t, p.Root, "test", "./...")
}

// An app made before routes had paths: the Path field, the import and a
// router are added.
func TestAddRouteToOldApp(t *testing.T) {
	if testing.Short() {
		t.Skip("builds an app")
	}
	p := genApp(t)
	old := `package app

import (
	"` + EnginePath + `/ui/material/icons"
	"` + EnginePath + `/ui/vector"
	w "` + EnginePath + `/ui/widgets"

	"` + p.Module + `/internal/pages/home"
	"` + p.Module + `/internal/pages/settings"
)

// Route is a top-level destination in the navigation rail.
type Route struct {
	Label string
	Icon  *vector.Icon
	Page  func() w.Widget
}

var Routes = []Route{
	{Label: "Home", Icon: icons.Home, Page: func() w.Widget { return home.Page{} }},
	{Label: "Settings", Icon: icons.Settings, Page: func() w.Widget { return settings.Page{} }}}
`
	os.WriteFile(filepath.Join(p.Root, "internal/app/routes.go"), []byte(old), 0o644)
	os.Remove(filepath.Join(p.Root, "internal/app/router.go"))
	// The old shell picks pages by index.
	shell := read(t, p, "internal/app/shell.go")
	shell = shell[:strings.Index(shell, "type shellState struct")] + `type shellState struct {
	w.StateBase
	page int
}

func (s *shellState) Build(ctx w.BuildContext) w.Widget {
	dests := make([]m.NavigationDestination, len(Routes))
	for i, r := range Routes {
		dests[i] = m.NavigationDestination{Icon: r.Icon, Label: r.Label}
	}
	_ = icons.Home
	_ = state.Of(ctx)
	return m.Scaffold{
		Rail: m.NavigationRail{Destinations: dests, Selected: s.page,
			OnSelected: func(i int) { s.SetState(func() { s.page = i }) }},
		Body: w.KeyedSubtree{ID: s.page, Child: Routes[s.page].Page()},
	}
}
`
	os.WriteFile(filepath.Join(p.Root, "internal/app/shell.go"), []byte(shell), 0o644)
	if err := AddRoute(p, RouteOptions{Path: "/help", Name: "Help", Hidden: true}); err != nil {
		t.Fatal(err)
	}
	routes := read(t, p, "internal/app/routes.go")
	for _, want := range []string{"Path   string", "Hidden bool", `{Path: "/", Label: "Home"`, `{Path: "/settings", Label: "Settings"`, `{Path: "/help", Label: "Help", Icon: icons.Article, Hidden: true, Page: func() w.Widget { return help.Help{} }},`} {
		if !strings.Contains(routes, want) {
			t.Errorf("routes.go lacks %q:\n%s", want, routes)
		}
	}
	if _, err := os.Stat(filepath.Join(p.Root, "internal/app/router.go")); err != nil {
		t.Fatal("router.go not written for an old app")
	}
	goIn(t, p.Root, "vet", "./...")
}

func TestGeneratorErrors(t *testing.T) {
	p := genApp(t)
	if err := AddRoute(p, RouteOptions{Path: "/settings", Name: "Again"}); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Errorf("duplicate route: %v", err)
	}
	if err := AddRoute(p, RouteOptions{Path: "/bad path", Name: "X"}); err == nil {
		t.Error("a path with a space was accepted")
	}
	if err := AddRoute(p, RouteOptions{Path: "/x", Name: "X", Icon: "no_such_icon_at_all"}); err == nil {
		t.Error("unknown icon accepted")
	}
	if err := Create(p, "gizmo", "X", CreateOptions{}); err == nil {
		t.Error("unknown kind accepted")
	}
	if err := Create(p, "model", "123", CreateOptions{}); err != nil {
		t.Errorf("digit name should become N123: %v", err)
	}
	if err := Create(p, "model", "User", CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := Create(p, "model", "User", CreateOptions{}); err == nil || !strings.Contains(err.Error(), "-force") {
		t.Errorf("overwrite without -force: %v", err)
	}
	p.Force = true
	if err := Create(p, "model", "User", CreateOptions{}); err != nil {
		t.Errorf("-force: %v", err)
	}
}

func TestNames(t *testing.T) {
	for in, want := range map[string][4]string{
		"user-profile": {"UserProfile", "userProfile", "user_profile", "User Profile"},
		"userProfile":  {"UserProfile", "userProfile", "user_profile", "User Profile"},
		"HTTPServer":   {"HTTPServer", "httpServer", "http_server", "HTTP Server"},
		"todo":         {"Todo", "todo", "todo", "Todo"},
	} {
		got := [4]string{exportName(in), lowerName(in), snakeName(in), labelOf(in)}
		if got != want {
			t.Errorf("%s: %v, want %v", in, got, want)
		}
	}
	if pkgName("user-profile") != "userprofile" || pkgName("type") != "typepkg" || pkgName("2fa") != "p2fa" {
		t.Error("pkgName")
	}
	for in, want := range map[string]string{"person": "Person", "Person": "Person", "arrow_back": "ArrowBack", "ArrowBack": "ArrowBack", "": "Article"} {
		if got, err := iconIdent(in); err != nil || got != want {
			t.Errorf("iconIdent(%q) = %q, %v", in, got, err)
		}
	}
}
