# Nectar UI

A retained-mode UI engine in Go on top of wgpu (`gogpu/wgpu`, plus a fork of `gogpu/gogpu` in `internal/gogpu` for windowing; see [its FORK.md](internal/gogpu/FORK.md) for what it adds).
Architecture follows Flutter: **Widget → Element → RenderObject → display list → GPU**.

```
CGO_ENABLED=0 go run .
go test ./...          # GPU test falls back to the CPU adapter when no GPU is present
```

## Create an app

```sh
go install github.com/minelifes/nectar_ui/cmd/nectar@latest
nectar new myapp                     # Material 3 starter in ./myapp
cd myapp && CGO_ENABLED=0 go run .
nectar dev                           # or run it with hot reload (see below)
nectar theme                         # design its light and dark themes in a live editor (see below)
```

`nectar new` writes a ready-to-run module that imports the engine from GitHub (`go get github.com/minelifes/nectar_ui@<version>` + `go mod tidy`).

| Flag | Meaning |
|---|---|
| `-template` | `material` (default: structured app with `internal/{app,theme,state,components,pages}`, light/dark theme, navigation rail, settings page, tests), `basic` (core widgets only, custom button), `explorer` (file tree + preview in a split view) |
| `-seed` | brand color the theme is generated from, e.g. `#0B57D0` (default: M3 baseline `#6750A4`) |
| `-module` | module path of the new app (default: the folder name) |
| `-title` | window title (default: from the folder name, `my-app` → "My App") |
| `-version` | engine version: `latest`, a tag, branch or commit (default: the CLI's own release, else `latest`) |
| `-local` | use an engine checkout on disk (adds a `replace`), e.g. `-local ~/Projects/Go/nectar_ui` before pushing |
| `-no-tidy`, `-git`, `-force` | skip fetching / run `git init` / write into a non-empty folder |

### Package as a desktop app

Run this in the app folder (pure Go, cross-compiles from any OS):

```sh
nectar build                            # this OS; on macOS a universal (arm64+amd64) .app
nectar build -os windows -arch amd64    # dist/<exe>.exe with icon + DPI manifest, no console
nectar build -os linux -arch arm64      # dist/<exe>-linux-arm64/ with .desktop, icon, install.sh
```

Name, bundle id, version and icon come from the app's `nectar.json`. The icon is `assets/icon.png`, a square PNG at 1024×1024. `nectar new` generates a default one in the seed color, and `nectar icon -seed "#0B57D0"` regenerates it.

- **macOS:** you get `Info.plist`, an `.icns` icon and an ad-hoc signature.
- **Windows:** the icon and manifest are written to a temporary `.syso` resource that the Go linker embeds in the `.exe`.
- **Linux:** the window icon is set at runtime via `ui.Config.WithIcon`.

**Terminal output:** the CLI uses colors and symbols when it writes to a terminal (`nectar dev` shows timestamped status lines, highlighted build errors, and the app's own output indented behind a `│` bar). Piped or redirected output is plain text. Set `NO_COLOR=1` to turn colors off, or `FORCE_COLOR=1` to keep them in a CI log. Windows 10+ consoles are switched to ANSI mode automatically.

`nectar templates` lists the templates. Their sources live in `cmd/nectar/templates` (`[[.Engine]]`-style placeholders, `_name` → `.name`). `NECTAR_BUILD_TEST=1 go test ./cmd/nectar` generates each one against this checkout and runs its tests.

## Packages (dependencies point downwards only)

```
ui            App: windows + frame loop (build → layout → paint → GPU), native menu, file drops
├── material  Material 3 components and themes
├── devtools  widget inspector, performance overlay, tree dumps
├── extension extension points, manifests, lazy activation (extension/wasm: sandboxed plugins)
├── menu      menus as data (in-app menu bar, context menus, native macOS menu)
├── commands  commands, rebindable key sequences, scopes, the command palette's data
├── mvvm      view models: observable properties, lists, bindings
├── settings  typed preferences saved in the user's config folder
├── tasks     background work with progress and cancellation
├── i18n      message catalogs, plurals, language fallback
├── fswatch   file change notifications (fsnotify, polling fallback)
├── undo      undo/redo history with groups and merging
├── fuzzy     fuzzy matching for pickers and palettes
├── hotreload `nectar dev` support: state kept across restarts, file watching
├── widgets   Widget / Element tree, State, BuildOwner, basic widgets
├── gpu       wgpu pipeline, WGSL uber-shader, batching, atlas upload
├── render    RenderObject tree: layout (constraints), paint → Canvas display list
├── text      Font, rich paragraph layout (spans, fallback, wrap / align / ellipsis), glyph atlas
└── geom      Offset, Size, Rect, EdgeInsets, Constraints, Color
```

## Frame pipeline

1. `BuildOwner.FlushBuild` — runs `Post`ed callbacks, rebuilds dirty elements (shallowest first).
2. `PipelineOwner.FlushLayout` — constraints go down, sizes go up. Unchanged subtrees reuse cached sizes.
3. `PipelineOwner.FlushPaint` — render objects record `CmdRect` / `CmdText` into a `Canvas`.
4. `gpu.Renderer.Draw` — the display list becomes one vertex/index buffer. There's one draw call per clip rect.

Frames are on demand: `SetState`, `Post` and `MarkNeedsLayout/Paint` call `RequestRedraw`, and nothing renders while the UI is idle.

**Repaint boundaries.** `widgets.RepaintBoundary{Child: …}` records its subtree's display list once. It replays that list until something inside changes: a relayout, a widget update, or `MarkNeedsPaint`.

- **What's free:** moving the boundary (scroll, translation), fading it (`Opacity`), or clipping it just replays the cached commands, shifted, faded and clipped. Commands scrolled out of view are skipped.
- **Scroll views** wrap their content in one automatically, so scrolling doesn't repaint the content.
- **Where to use one:** around subtrees that are costly to paint and change rarely, for example a static panel next to an animation.
- **Checking it works:** `RenderRepaintBoundary.Stats()` reports hits (replays) and misses (repaints).
- **For custom render objects:** call `MarkNeedsPaint` (or `render.InvalidatePaint` inside a frame) when something changes only its appearance.
- **Suspect a stale cache?** Set `render.CacheRepaintBoundaries = false` to paint everything every frame. If a glitch disappears, some render object changes its looks without `MarkNeedsPaint`. `tests/boundary_diff_test.go` checks the Material widgets this way, comparing cached and uncached output step by step.

## Text

- **Fonts:** `golang.org/x/image/font/sfnt` parses TTF/OTF. The Go fonts are embedded, so there's no setup (Latin, Cyrillic, Greek).
- **Shaping:** Latin, Cyrillic and Greek in fonts without ligature tables take the fast path (cmap lookup plus kerning). Everything else goes through `go-text/typesetting`, a pure-Go HarfBuzz port:
  - Complex scripts: Arabic joining forms, Indic conjuncts and reordering, combining marks, and so on.
  - Ligatures ("fi", "ffi", programming-font arrows) in fonts that have them. `Style.NoLigatures` turns them off, e.g. for code where every character should stay visible.
  - Bidirectional text (Unicode bidi algorithm): Arabic and Hebrew run right to left, and mixed lines are reordered per run. An RTL paragraph aligns `AlignStart` to the right.
  - The caret stops before every character, inside ligatures too, and moves in visual order. Selections over mixed-direction text are drawn as one rectangle per visual run (`Paragraph.SelectionRects`).
- **Layout:** greedy word wrap at spaces and hyphens, mid-word break for long words, `\n` hard breaks, `LineHeight`, `LetterSpacing`, alignment, and `MaxLines` + ellipsis.
- **Atlas:** glyphs are rasterized on demand at *physical* pixel size with 4 horizontal subpixel positions, so text stays crisp at any DPI. They're stored in an RGBA atlas (1024², shelf packer). Only dirty rows are uploaded, and the atlas resets itself when it fills up.
- **Editing:** multiline fields (`EditableText{Multiline: true}`, `material.TextField{Multiline: true}`) soft-wrap at the field width using the same line breaking as `Text`. `NoWrap` turns that off.
  - Caret movement works on visual lines: ↑/↓, Home/End and clicks.
  - Selection spans wrapped lines.
- **IME** (CJK and other input methods):
  - While the user composes, the preedit shows inline in the field, underlined, with the IME's caret inside it. The committed text then replaces it, and cancelling leaves the field unchanged.
  - Editing keys belong to the input method while it's composing.
  - Password and read-only fields don't take IME input.
  - Fields report their caret rectangle (`FocusNode.IMERect`) so the platform can place the candidate window.
  - Your own widgets can take part via `FocusNode.OnIME` (`IMEStart` / `IMEUpdate` / `IMEEnd`).
  - In tests: `tt.Compose("ni", -1)`, `tt.Commit("你")`, `tt.CancelIME()`, `tt.IMERect()`.
  - Platform side: gogpu declares the composition callbacks, and `ui.App` subscribes to them, but its macOS/Windows/Linux backends don't emit them yet. Inline composition turns on as soon as they do. Until then, Windows delivers the committed text as ordinary typing, while macOS and Linux get no IME input.
- **Rich text:** `widgets.RichText{Spans: []text.Span{...}}` lays out runs with their own font, size, color, background and `Underline` / `Strikethrough` / `Overline` in one paragraph (`text.LayoutSpans` underneath). A single span lays out exactly like `Text`.
- **Font fallback:** `font.WithFallback(cjk, emoji)` draws the runes a font lacks from the next font that has them; `text.SetGlobalFallback(...)` sets fallbacks for every font. The built-in Go fonts have no CJK glyphs: load one (e.g. Noto Sans CJK) with `text.ParseFont` and add it as a fallback.
- **Tabs:** `Style.TabSize` sets tab stops in spaces (0 keeps a tab one space wide).
- **Selection:** `widgets.SelectableText` (read-only): drag, double-click a word, triple-click all, Shift+arrows, Ctrl/⌘+A and Ctrl/⌘+C. Paragraphs expose `OffsetAt`, `CaretAt`, `SelectionRects` and `text.WordAt` for your own text widgets.
- **Not yet:** vertical text, and emoji color fonts (COLR/CBDT).

## Writing widgets

```go
// Stateless
type Greeting struct{ Name string }
func (g Greeting) Build(ctx widgets.BuildContext) widgets.Widget {
    return widgets.Text{Text: "Hi " + g.Name}
}

// Stateful
type Counter struct{}
func (Counter) CreateState() widgets.State { return &counterState{} }
type counterState struct{ widgets.StateBase; n int }
func (s *counterState) Build(ctx widgets.BuildContext) widgets.Widget { ... }
// s.SetState(func(){ s.n++ })  on the UI goroutine
// s.Post(func(){ s.n++ })      from any goroutine
```

Custom render object: write a struct embedding `render.Box` (plus `render.SingleChild` / `render.MultiChild` if it has children). Implement `PerformLayout` and `Paint`, plus `VisitChildren` for leaves. Then add a widget that implements `CreateRenderObject` / `UpdateRenderObject` (+ `ChildWidget()` / `ChildWidgets()`). `render.RenderParagraph` and `widgets.paragraph` are the smallest complete example.

## MVVM: view models and bindings (`ui/mvvm`)

A second way to drive the UI, next to `StatefulWidget` + `SetState`: keep the state in a view model made of observable values, and bind widgets to exactly the values they show. A change rebuilds only those bindings, never the page around them.

```go
type CounterVM struct {
    mvvm.ViewModel                 // Notifier + Dispose/Own for cleanup
    Count *mvvm.Property[int]
}
func NewCounterVM() *CounterVM { return &CounterVM{Count: mvvm.NewProperty(0)} }
func (vm *CounterVM) Increment() { vm.Count.Update(func(n int) int { return n + 1 }) }

// the view: built once, only the Bind re-runs on changes
mvvm.Provide[*CounterVM]{Create: NewCounterVM, Child: CounterPage{}}

func (CounterPage) Build(ctx w.BuildContext) w.Widget {
    vm := mvvm.Use[*CounterVM](ctx)
    return w.Column{Children: []w.Widget{
        mvvm.Bind(vm.Count, func(ctx w.BuildContext, n int) w.Widget { return w.Text{Text: strconv.Itoa(n)} }),
        m.FilledButton{Label: "+1", OnPressed: vm.Increment},
    }}
}
```

- **Observables:** `Property[T]` (notifies only when the value changes; `Set` fits `OnChanged` callbacks), `Computed[T]` (derived from other observables, notifies only when its result changes), `List[T]` (copy-on-write slice: `Get` returns a snapshot that later edits never touch), and `Notifier` / `ViewModel` for models that notify as a whole. Anything with `Subscribe(func()) (cancel func())` can be bound.
- **Bindings:** `Bind(o, builder)` rebuilds with o's value; `Watch(ctx, o)` (or `o.Watch(ctx)` on a `Property`, `Computed` or `List`) inside any `Build` subscribes that widget; `Select(model, pick, builder)` rebuilds only when the picked value changes; `Observer{Sources, Builder}` watches several.
- **Lifetime:** `Provide[VM]` creates the view model when it enters the tree and calls its `Dispose` when it leaves; `Use[VM](ctx)` finds it (a plain `widgets.Provider[VM]` works too). Subscriptions end with the widget, and a build that stops watching something drops that subscription.
- **Threads:** observables are safe for concurrent use, so a view model may update them from any goroutine. Bindings coalesce notifications and rebuild once, on the UI goroutine, before the next frame.
- **Lower level:** `widgets.Listen(ctx, listenable)` is what `Watch` uses; call it from your own widgets or controllers.

`CGO_ENABLED=0 go run ./examples/mvvm` is a todo list built this way.

**What a rebuild touches.** Whether from `SetState` or a binding, a rebuild updates only what changed below it:

- A child whose new widget equals the old one is skipped with its whole subtree. Widgets are compared by value: fields, nested widgets and slices (so `Row{Children: …}` with the same children is skipped), pointers by identity. Funcs and maps always count as changed, as does a slice that reuses the old one's backing array (it may have been edited in place), so build new slices instead of mutating old ones.
- Built-in render widgets mark their render object only when a property really changes, so a rebuild that changes nothing visible (say, a new `OnTap` closure) doesn't make the surrounding repaint boundary repaint. Custom render widgets that do the same (call `MarkNeedsPaint` / `MarkNeedsLayout` themselves) can say so with a `MarksOwnPaint()` method; without it the engine repaints the boundary after each update to be safe.

## Hot reload: `nectar dev`

Go can't swap code into a running process, so `nectar dev` does the next best thing: it rebuilds and restarts the app whenever a `.go` file changes (usually a second or two), and carries its state across the restart.

```sh
nectar dev                  # in the app folder; or: nectar dev path/to/app -- app args
nectar dev -pkg ./cmd/app   # the main package, if it isn't the folder itself
```

- **Build errors** are printed and the running app stays up; fix the code, save, and it restarts.
- **The window** keeps its size (or maximized / fullscreen).
- **State you opt in** survives: `mvvm.NewProperty(0).Keep("counter")`, `mvvm.NewList[T]().Keep("todos")`, or `hotreload.Keep("page.tab", &s.tab)` for any JSON-serializable variable (e.g. in `InitState`). The old process saves these before quitting; the new one restores each when it registers the same key. Everything else starts fresh, like a normal launch.
- **Assets reload live, without a restart:** `cfg.WithResources("", assets.FS).WithDevResources("", "assets")` mounts the `assets` folder from disk under `nectar dev` (release builds keep using the embedded copy). Saving a file there evicts its images from the cache and rebuilds the whole tree, keeping all state (`Root.Reassemble`; a `State` can implement `Reassemble()` to drop its own caches). Apps made with `nectar new` are set up this way.
- Outside `nectar dev` all of this is a no-op, so the calls stay in release code. The app learns it runs under `nectar dev` from `NECTAR_DEV=1` (see package `hotreload`), and the tool asks it to save and quit over stdin.

## Input

Pointer events from gogpu are queued and processed at the start of the next frame, on the UI thread, before build.
So handlers can call `SetState` directly.

- `GestureDetector{OnTap, OnTapDown, OnTapUp, OnTapCancel, OnPanStart, OnPanUpdate, OnPanEnd}`: taps and drags.
  When detectors are nested, the innermost one that handles the gesture wins and the others get `OnTapCancel`.
  Moving past `render.TouchSlop` (8px) turns a tap into a drag. A pressed pointer is captured, so drags keep
  working outside the widget.
- `MouseRegion{Cursor, OnEnter, OnExit, OnHover}`: hover and the mouse cursor.
- `Listener{OnEvent}`: raw events, including scroll wheel (`PointerScroll`, `e.Scroll`).
- `Builder{Builder: func(ctx) Widget}`: an inline widget made from a closure.

`CGO_ENABLED=0 go run ./examples/input` shows buttons with hover/pressed states, a counter and a draggable box.

## Images and resources

**Resources** (`ui/resources`) are files shipped inside the app. Embed a folder and mount it when the window opens; widgets then read by name through their context, wherever the app is installed:

```go
//go:embed images fonts
var files embed.FS

cfg := ui.DefaultConfig().
    WithResources("", files).            // at the root
    WithResources("charts", chartFiles)  // under charts/; later mounts win

// in any widget
data, err := widgets.ResourcesOf(ctx).ReadFile("charts/bar.json")
w.Image{Source: w.AssetImage{Name: "images/logo.png"}}
```

`ResourcesOf(ctx)` returns a `*resources.Set`. A Set works as an `io/fs` filesystem, and has `ReadFile`, `ReadDir`, `Stat`, `Exists` and `Mount`.

There are three layers, and each sees the files of the ones above it:

1. **Global:** `resources.Mount(…)`, for libraries that register files in `init()`.
2. **The app:** `cfg.WithResources(…)`. These files are mounted when the window opens, go away when it closes, and are also available as `app.Resources()`.
3. **A subtree:** `widgets.Resources{Prefix: "charts", FS: chartFiles, Child: …}` adds files for its children only, and removes them when it leaves the tree.

`AssetImage` resolves through this chain, so the same name can be a different file in different subtrees without the image cache mixing them up.

**While developing**, add `WithDevResources("", "assets")`: under `nectar dev` the folder is read from disk and edits show up right away (see Hot reload). Apps made with `nectar new` embed `assets/` and mount it both ways in `main.go`.

**Tests** mount the same way: `tester.New(app, w, h, tester.WithResources("", files))`; the resources are then available as `tt.Resources`.

**`widgets.Image`** loads and decodes in the background, with the result cached and shared by every widget showing the same source:

```go
w.Image{Source: w.AssetImage{Name: "images/logo.png"}, Width: 120}
w.Image{Source: w.FileImage{Path: "/Users/me/photo.jpg"}, Fit: w.FitCover, Radius: 12}
w.Image{Source: w.NetworkImage{
    URL:     "https://api.example.com/render",
    Method:  "POST",                                   // default GET (POST if Body set)
    Headers: map[string]string{"Authorization": "Bearer " + token},
    Body:    payload,
    // AllowHTTP: true  permits plain http:// (https only by default, redirects included)
}, Width: 64, Height: 64, Placeholder: m.CircularProgressIndicator{},
   Error: func(err error) w.Widget { return w.Text{Text: "offline"} }, FadeIn: 200 * time.Millisecond}
w.Image{Source: w.MemoryImage{Data: pngBytes}, Pixelated: true}
```

- **Formats:** PNG, JPEG, GIF (first frame), WebP, BMP and TIFF.
- **Size:** with no `Width`/`Height` the widget takes the image's size, shrunk to its constraints. With one set, the other follows the aspect ratio.
- **Fit and appearance:** `Fit` works like CSS `object-fit` (`FitContain`, `FitCover`, `FitFill`, `FitWidth`, `FitHeight`, `FitNone`, `FitScaleDown`). There's also `Alignment`, `Radius` (rounded corners), `Opacity`, and `Pixelated` (nearest-neighbour).
- **Network:** you can also set `Timeout`, `MaxBytes`, a custom `Client` (proxy, TLS, cookies) and `Key`.
- **Cache:** `widgets.Images` holds up to 256 MiB of decoded pixels (LRU) and has `Evict(src)`, `Clear()` and `Stats()`. Large images are capped at `widgets.MaxImageSize` (4096 px) when decoded.
- **Custom sources:** implement `ImageSource`, which is just `CacheKey()` plus `Load(ctx) ([]byte, error)`.
- **GPU:** each image becomes a texture. When one is drawn at half size or smaller, the engine uploads a box-filtered, pre-shrunk copy instead, so thumbnails stay smooth and cost little memory. Textures that go unused are freed.
- **Tests:** in widget tests, `tt.PumpUntil(cond, timeout)` waits for background loads.

## Window

Any widget can control the native window through its context:

```go
win := widgets.WindowOf(ctx)
win.SetSize(1280, 800)          // content size in logical px
win.SetMinSize(640, 480)        // and SetMaxSize; 0, 0 = no limit
win.SetTitle("Untitled — Notes")
win.SetFullscreen(!win.IsFullscreen())
win.Maximize()                  // toggles; also Minimize, Close
w, h := win.Size()              // as of the last frame
```

- **Threading:** setters are safe from any goroutine. They queue the request, and `ui.App` applies it on the platform's main thread before the next frame, as AppKit requires.
- **After a resize:** the new size reaches layout like a user resize would.
- **Outside the app:** `ui.App.Window()` returns the same handle.
- **Secondary windows** (`ui.App.OpenWindow`) support the same calls: title, size limits, maximize, minimize and fullscreen.
- **Tests:** `tester.Tester.Window` fakes it. `SetSize` really resizes the test surface (`tt.Size`), clamped to the min/max size, and title, fullscreen and maximize are recorded for assertions.

## Custom title bar

`WithCustomTitleBar(true)` lets the app draw into the strip with the window buttons (title, tabs, toolbar buttons next to close / minimize / zoom):

```go
cfg := ui.DefaultConfig().WithTitle("Aether").WithCustomTitleBar(true)

widgets.Column{Cross: widgets.CrossStretch, Children: []widgets.Widget{
	material.TitleBar{
		Title:   "Aether",
		Leading: []widgets.Widget{widgets.Icon{Icon: icons.Terminal}},
		Center:  widgets.Text{Text: "Welcome & Quick Start"}, // centered on the window
		Actions: []widgets.Widget{material.IconButton{Icon: icons.Tune, OnPressed: openSettings}},
	},
	widgets.Expanded{Child: body},
}}
```

| Platform | What happens |
| --- | --- |
| macOS | The title bar turns transparent and the content goes under it. The system still draws the traffic lights at the top left (they sit in the top 28 px) and hides the native title text. |
| Windows, Linux (X11) | The window is frameless. `TitleBar` adds drawn minimize / maximize (restore) / close buttons, the empty parts of the bar move the window (double-click maximizes on Windows), and the outer 6 px resize it. |
| Linux (Wayland) | Not supported by gogpu yet (it can't move a frameless window there). The native title bar stays and `TitleBar` is an ordinary bar below it. |

- **`widgets.TitleBar`:** the theme-free version. `Child` goes between the system insets, `Center` is centered on the window, and `Buttons` styles the drawn window buttons. `material.TitleBar` wraps it with theme colors and a 40 px default height.
- **Build your own:** `widgets.WindowOf(ctx).TitleBar()` returns `Custom`, `SystemButtons`, `Height` and the `Leading`/`Trailing` widths the system buttons cover. `widgets.WindowDragArea` makes any widget move the window, and buttons and text fields inside it keep their clicks. `widgets.WindowButtons` are the drawn window buttons.
- **Reacting to window state:** `widgets.WatchWindow(ctx)` is `WindowOf` that also rebuilds when the window is maximized or goes fullscreen (the maximize glyph turns into restore, and on macOS the traffic-light inset goes away in fullscreen).
- **Titles:** `Window.Title` / `SetTitle` keep working for the app. On macOS the OS title text stays empty.
- **Tests:** `tester.WithTitleBar(tester.MacTitleBar)` or `tester.FramelessTitleBar` picks the platform, and `tt.Window.HitTest(x, y)` says whether a press would drag the window (`caption`), resize it, or reach the app (`client`).
- **Try it:** `CGO_ENABLED=0 go run ./examples/titlebar`.

### Choosing the window buttons

`WithWindowControls` picks which of close / minimize / maximize (the green fullscreen button on macOS) the window shows. Use it for a tool window with only close, or `NoControls` for a design with no standard buttons at all:

```go
cfg := ui.DefaultConfig().WithCustomTitleBar(true).WithWindowControls(widgets.CloseControl)
cfg := ui.DefaultConfig().WithCustomTitleBar(true).WithWindowControls(widgets.NoControls) // draw your own

// your own buttons call the window directly:
widgets.GestureDetector{OnTap: widgets.WindowOf(ctx).Close, Child: myCloseDot}

// change them at any time, e.g. while a task can't be interrupted:
widgets.WindowOf(ctx).SetControls(widgets.MinimizeControl)

// secondary windows too:
widgets.OpenWindow(ctx, widgets.WindowOptions{Title: "Inspector", Controls: widgets.CloseControl}, inspector{}, nil)
```

| Platform | What happens |
| --- | --- |
| macOS | The traffic lights are hidden one by one, with a native or a custom title bar. The visible ones keep their place (macOS doesn't reflow them), and `TitleBar`'s left inset shrinks to the last visible one: 36 px for close only, 0 for none. Hiding only hides, so Cmd+W, Cmd+M and the Window menu keep working. |
| Windows, custom title bar | `TitleBar` / `WindowButtons` draw only the chosen buttons. Without maximize, snapping and double-clicking the bar don't maximize either. |
| Windows, native title bar | Windows hides minimize and maximize when both are off and greys one when only one is. Close can only be greyed (which also blocks Alt+F4), unless all three are off, which removes them all. |
| Linux | With a custom title bar (X11) `TitleBar` draws only the chosen buttons. Native X11 / Wayland title bars keep theirs (the window manager draws them). |

- **The functions stay:** `Window.Close`, `Minimize`, `Maximize` and `SetFullscreen` work whatever is shown.
- **Reading it:** `Window.Controls()` returns the shown set, `TitleBarInfo.Controls` carries it into title bar layouts, and `WindowButtons{Show: ...}` overrides it for one row of drawn buttons.
- **Tests:** `tester.WithWindowControls(widgets.CloseControl)` sets it from the first frame. The tester's macOS bar moves its inset like a Mac does.

## Responsive layouts: `LayoutBuilder`

`LayoutBuilder` builds its child during layout, from the constraints its parent gives it (like Flutter's):

```go
widgets.LayoutBuilder{Builder: func(ctx widgets.BuildContext, c geom.Constraints) widgets.Widget {
	if c.MaxW >= 720 {
		return widgets.Row{Children: []widgets.Widget{sidebar, widgets.Expanded{Child: content}}}
	}
	return content // narrow: no sidebar
}}
```

- **Local space:** it sees the space of its own spot in the tree (a split pane, a dialog, a card), not the window. For the window size use `WindowOf(ctx).Size()`.
- **When it re-runs:** when its constraints change, when its parent rebuilds it, or when something the builder reads from `ctx` changes (a `Provider`, the theme). Frames that lay it out with the same constraints reuse the last build.
- **Sizing:** the child is laid out with the same constraints, and the `LayoutBuilder` takes the child's size.
- **Unbounded axes:** inside a scroll view or a `Row`, `c.MaxH` / `c.MaxW` can be `geom.Inf`. Check `c.HasBoundedWidth()` before dividing by them.
- **State:** state below it is kept when a re-run returns the same widget types in the same places. Give the branches keys to keep state across them.
- **Empty child:** returning `nil` leaves it empty.

## Engine building blocks

- **Drawing:** the `render.Canvas` draws rounded rects, strokes, arcs, soft shadows, ripples (a circle clipped to a rounded rect), vector icons and text. It also has clip and opacity stacks. Every shape is drawn analytically in one uber-shader.
- **Layout:** `Stack` / `Positioned`, `Wrap`, `TreeView` (expandable rows with lazy `Load` children, only visible rows built, `TreeController` for expand/select/reload from code, full keyboard navigation; `FileNodes` turns any `fs.FS` into nodes), `SplitView` (resizable panes with per-pane `Min`/`Max`: dragging a divider cascades to the next pane once one hits a limit, sizes scale with the window, and arrow keys move a focused divider), `Opacity`, `Translate`, `IgnorePointer`, `AbsorbPointer`, `SizeTransition`, `FractionallySizedBox`, and `CustomPaint` (give `Size` an axis of `geom.Inf` to fill that axis).
- **Scrolling:** `ScrollView`, `ListView`, `ListViewBuilder` and `ScrollController` (`JumpTo`, `AnimateTo`). Nested scroll views hand wheel events to the innermost one that can move.
  - `ListViewBuilder` builds only the visible rows: fixed `ItemExtent`, or `ItemExtentOf(i)` for rows of different sizes, and `IsHeader(i)` for section headers that stay pinned at the top until the next one pushes them away.
  - `ScrollView2D` scrolls both ways (wheel, Shift+wheel, trackpad, drag), with a controller per axis.
  - `DataGrid` is a virtualized table: only visible rows are built, the header stays on top and scrolls sideways with the rows, columns resize by dragging.
- **Docking:** `Dock` + `DockController` arrange panels in tab groups and splits. Users drag tabs to reorder them, move them to another group or drop them on a group's edge to split it. Layouts save and restore as JSON, and background tabs keep their state (`Offstage`). `material.Dock` styles it (`Theme.Dock`).
- **Drag and drop:** `Draggable` / `DragTarget` move data inside the app (with a feedback widget under the pointer); `FileDropTarget` receives files dropped from the OS file manager, with `OnEnter` / `OnMove` / `OnLeave` while they're dragged over it (to highlight the drop zone). In tests: `tt.DragFiles(paths, drop, points...)`.
- **Windows and dialogs:** `widgets.OpenWindow` (or `ui.App.OpenWindow`) opens more native windows, each with its own widget tree. `widgets.ShowOpenDialog` / `ShowSaveDialog` use the system file dialogs.
- **Semantics:** `Semantics` annotates a part of the UI with a role, label, value, state and default action. Material controls annotate themselves (`SemanticLabel` on controls without text). `widgets.SemanticsTree` collects them for tests and automation, and every window hands them to the OS screen reader after frames that change them:
  - Linux: AT-SPI2 over D-Bus (Orca), when an accessibility bus is running.
  - macOS: `NSAccessibilityElement`s under the window's view (VoiceOver).
  - Windows: UI Automation providers (Narrator, NVDA, JAWS).

  Screen readers see each node's role, name, value, hint, bounds, checked/selected/disabled state and focus, and can press nodes with a default action (`OnTap`). `Config.NoAccessibility` turns this off for a window.
- **Animation:** tickers driven by frames, `AnimationController` (forward, reverse, repeat), M3 easing curves, and `Animated` for implicit transitions. The app requests frames only while an animation runs.
- **Keyboard:** `Focus` / `FocusNode`. Key events bubble up through parent nodes, Tab moves focus in reading order, and `CatchAll` nodes catch keys nobody else handled.
- **Text editing:** `EditableText` + `TextEditingController`: caret, selection, word jumps, clipboard, obscured input, multiline.
- **Overlays:** `Overlay` for floating layers and `Navigator` for a route stack with transitions, modal barriers and Escape to dismiss.
- **Icons:** `vector` parses SVG path data. `material/icons` embeds 2,156 Material icons (`icons.Search`, `icons.ByName["search"]`). Regenerate them with `tools/genicons`.

## Material 3 (`ui/material`)

```go
app.Run(material.App{Theme: material.NewTheme(geom.Hex(0x6750A4), false), Dark: dark, Home: MyPage{}})
```

- **Theme:** `ColorScheme` is generated from a seed color, light and dark. It covers every M3 role, and the baseline seed reproduces the published baseline scheme. Also included: the M3 type scale, corner radii, elevation levels and state-layer opacities. Read them with `material.ThemeOf(ctx)`.
- **Actions:** Elevated, Filled, FilledTonal, Outlined and Text buttons, `IconButton` (4 variants plus toggle), `FloatingActionButton` (small, regular, large, extended), `SegmentedButton`.
- **Communication:** `Badge`, `LinearProgressIndicator`, `CircularProgressIndicator` (determinate and indeterminate), `ShowSnackBar`, `Tooltip`.
- **Containment:** `Card` (elevated, filled, outlined), `SplitView` (M3 pill drag handles), `TreeView` and `FileTree` (browse an `fs.FS` such as `os.DirFS`: folders first, file-type icons, `OnSelect` / `OnOpen` by path), `Carousel` (multi-browse, uncontained, hero, full-screen; `WheelScroll` lets the vertical wheel drive it and hands scrolling back to the page at either end), `ListTile` (+ checkbox, radio and switch tiles), `ExpansionTile`, `Divider`, `AlertDialog`, `SimpleDialog`, `ShowFullScreenDialog`, `ShowModalBottomSheet`, `SideSheet` (standard) / `ShowModalSideSheet`, `MaterialBanner`, `CircleAvatar`.
- **Selection:** `Checkbox` (with tristate and error), `Radio[T]`, `Switch`, `Slider`, `RangeSlider`, assist, filter, choice, input and suggestion chips, `ShowDatePicker` / `CalendarDatePicker`, `ShowTimePicker`, `ShowMenu` (submenus, check marks, ↑/↓/Enter/→/← navigation), `PopupMenuButton`, `DropdownMenu`.
- **Menus and pickers:** `MenuBar` (an application menu bar; hover or arrow keys move between menus), `ContextMenuRegion` (right-click menus), `QuickPick` / `ShowQuickPick` (fuzzy-filtered list, or your own `Search`), `ShowCommandPalette`. Menu items come from `ui/menu` and can name commands, which supply their label, shortcut and enabled state.
- **Notifications:** `ShowToast` (stacked in the corner, with actions, progress and update/close handles), `ShowTaskToast` (follows a background task), `HoverCard` / `RichTooltip`.
- **Text input:** `TextField` (filled or outlined, floating label, hint, helper, error, icons, counter), `SearchBar`.
- **Navigation:** `AppBar` (small, center, medium, large, with auto back), `NavigationBar`, `NavigationRail`, `NavigationDrawer`, `TabBar` / `TabBarView`, `BottomAppBar`, `Scaffold` (with drawer), `Push` / `Pop`.
- **Data:** `DataTable` (sorting, row selection), `Stepper`.
- **Building blocks for your own components:** `Surface` (a painted container) and `InkSurface`, which adds state layers, a ripple, a focus ring, keyboard activation and elevation on hover.

`CGO_ENABLED=0 go run ./examples/material` opens the gallery with every component, a dark-mode toggle and a seed picker.

Known gaps compared to Flutter: the time picker is input-only with 24-hour time (no dial), ripples in segmented buttons aren't clipped to the pill ends, carousel items are visual only (use `OnTap`; no buttons inside items), and fonts are the Go fonts, not Roboto.

### Component themes

Like Flutter's `ThemeData`, `Theme` holds one theme per component besides the color scheme and type scale: `FilledButton`, `ElevatedButton`, `FilledTonalButton`, `OutlinedButton`, `TextButton`, `IconButton`, `SegmentedButton` (all `ButtonStyle`), `FAB`, `Card`, `Divider`, `ListTile`, `ExpansionTile`, `Badge`, `Avatar`, `Tooltip`, `Banner`, `Progress`, `SnackBar`, `Dialog`, `BottomSheet`, `SideSheet`, `Carousel`, `Chip`, `Checkbox`, `Radio`, `Switch`, `Slider`, `Input` (text fields), `SearchBar`, `Menu`, `DropdownMenu`, `DatePicker`, `TimePicker`, `AppBar`, `NavigationBar`, `NavigationRail`, `NavigationDrawer`, `TabBar`, `BottomAppBar`, `TitleBar`, `DataTable`, `Stepper`, `TreeView` and `SplitView`, plus `ScaffoldBackground` and `FocusRingColor`.

```go
th := m.NewTheme(geom.Hex(0x0B57D0), false)
th.FilledButton = m.ButtonStyle{Radius: m.Dp(8), Padding: w.Ptr(geom.InsetsHV(20, 0))}
th.Card = m.CardTheme{Elevation: w.Ptr(0), BorderWidth: m.Dp(1)}
th.Input = m.InputDecorationTheme{Outlined: w.Ptr(true), Radius: m.Dp(12)}
th.NavigationBar = m.NavigationBarTheme{IndicatorColor: geom.Hex(0xFFD8E4)}
m.App{Theme: th, Home: page}                 // dark mode keeps them (Theme.WithDark)

// one widget: its Style wins over the theme, field by field
m.FilledButton{Label: "Delete", Style: m.ButtonStyle{BackgroundColor: th.Scheme.Error}}

// one subtree: copy the theme, change it, provide it
t := m.ThemeOf(ctx)
t.ListTile.Radius = m.Dp(12)
m.ThemeScope{Theme: t, Child: sidebar}
```

- **Precedence:** the widget's `Style` field, then the theme's component theme, then the M3 default from the scheme.
- **Unset values:** colors and text styles use their zero value (text styles merge field by field, like `Text`). Numbers, insets, elevation and flags are pointers, because 0 is meaningful (square corners, no shadow): `m.Dp(0)`, `w.Ptr(0)`, `w.Ptr(false)`. For an explicit "no color" use `m.Transparent`.
- **Outlines** need a color and a width: set only the color for a 1px line, or only the width for the default color.
- **Rebuilds:** a `Style` literal with pointer fields is a new value on every build, so that widget is never skipped as unchanged; keep often-used styles in package-level variables. Equal themes don't rebuild their dependents (`ThemeScope` compares them by value).
- `tests/theme_gallery_test.go` renders every component and checks that each theme field changes what's drawn.

### Theme editor: `nectar theme`

`nectar theme` opens a Nectar UI app for designing your light and dark themes visually, then writes them as Go code.

```sh
nectar theme                          # in an app: edits internal/theme (else the current folder)
nectar theme ui/brand -pkg brand -prefix Brand
nectar theme -new                     # start over, replacing the files on save
```

- **Light and dark are separate designs.** The *Light | Dark* switch picks which one you edit and preview. Each has its own seed, color scheme, type scale and component styles. Dark starts from the Material 3 dark baseline, not from your light edits. *Copy from light/dark* starts one from the other, and *Reset* only resets the one shown.
- **Edit:** the seed color, every color-scheme role, each style of the type scale (size, font weight, letter spacing, line height, color) and every field of every component theme. Colors have a hex field and a picker (hue, saturation, brightness, opacity, or a role of the current scheme). Sizes have sliders. Flags, elevation and fonts have segmented choices. The ↶ button next to an edited field resets it. Search finds components by name or by field.
- **Preview:** the selected component first, then all of them, live. The preview is a real `material.App` with the design, so its buttons open dialogs, sheets, menus, snack bars and pickers in the designed style.
- **Code:** *Code* shows the two files and copies either one. *Save* (or Ctrl/⌘+S) writes both: `theme_light.go` with `func LightTheme() m.Theme` and `theme_dark.go` with `func DarkTheme() m.Theme`. Each returns one struct literal: the full color scheme and type scale, plus the component fields you set:

```go
func LightTheme() m.Theme {
	return m.Theme{
		Seed: geom.Hex(0x006A6A),
		Scheme: m.ColorScheme{
			Primary:   geom.Hex(0x006A6A),
			OnPrimary: geom.Hex(0xFFFFFF),
			// … every role
		},
		Text: m.TextTheme{
			BodyLarge: text.Style{
				Font:          text.DefaultFont(),
				Size:          16,
				Color:         geom.Hex(0x151D1D),
				LineHeight:    1.5,
				LetterSpacing: 0.5,
			},
			// … every style
		},
		Card: m.CardTheme{
			Radius:    m.Dp(8),
			Elevation: w.Ptr(0),
		},
	}
}
```

Use them with `material.App{Theme: theme.LightTheme(), DarkTheme: widgets.Ptr(theme.DarkTheme()), Dark: dark}`. In a `nectar new` app, return them from `Light` and `Dark` in `internal/theme/theme.go`. Each file also stores its design in its last line, so `nectar theme` on the folder reopens both. Files it didn't write are never overwritten.

## Building larger apps: editors, IDEs, tools

These packages are generic: none knows about IDEs, so any app can use them.

**Commands and shortcuts** (`ui/commands`). Named commands with default keys, a keymap the user can change and save as JSON, and multi-key sequences:

```go
host := commands.NewHost()
host.Registry.Register(commands.Command{ID: "file.save", Title: "Save", Category: "File", Keys: []string{"Mod+S"}, Run: vm.Save})
host.Registry.Register(commands.Command{ID: "view.zen", Title: "Zen Mode", Keys: []string{"Ctrl+K Z"}, Run: vm.ToggleZen})
app := commands.Commands{Host: host, Child: page}                                   // shortcuts work anywhere below
editor := commands.Scope{Name: "editor", Commands: editorCommands, Child: editorView} // only while focused inside
material.ShowCommandPalette(ctx)                                                     // lists what applies at the focus
```

`Mod` is ⌘ on macOS and Ctrl elsewhere. The focused widget gets a key first (a text field keeps Ctrl+C), then the scopes around it, then the global commands. `host.Keymap.SetKeys(id, "F2")` rebinds; `json.Marshal(host.Keymap)` saves only the user's changes; `Keymap.Conflicts` finds clashes.

**Menus** (`ui/menu`). The same data drives `material.MenuBar`, `material.ContextMenuRegion` and the native menu bar on macOS and Windows (`app.SetNativeMenu(menus, host)`; `ui.HasNativeMenuBar()` tells where it's shown). Native items show their command's shortcut (macOS key equivalents, right-aligned text on Windows) and check marks.

**Extensions** (`ui/extension`). Declare typed extension points (`extension.NewPoint[StatusItem]("statusBar")`), register compiled-in extensions with a manifest, or load a folder of them with `host.LoadDir`. Extensions activate lazily on events (`onStartup`, `onCommand:<id>`, your own `onLanguage:go`), and whatever they register through their `Context` is undone when they're deactivated. With `host.UseCommands(registry)`, commands declared in a manifest's `contributes` appear in the palette before the extension's code is loaded. `extension/wasm` runs plugins compiled to WebAssembly (Go `GOOS=wasip1`, TinyGo, Rust) in a wazero sandbox: no files, network or environment, host functions gated by the manifest's permissions, capped memory and a per-call timeout.

**App services.**
- `ui/settings`: typed preferences in the user's config folder, saved atomically on change. A `Setting[T]` is observable, so `mvvm.Bind` redraws on change. `ui.Config.WithWindowState(store, "window")` reopens the window at its last size, maximized or fullscreen.
- `ui/tasks`: background work with progress, status text, cancellation, and panics turned into errors. `Then` callbacks run on the UI goroutine.
- `ui/fswatch`: file changes under a folder as they happen (inotify, kqueue, Windows notifications through fsnotify, recursive and debounced), with a polling fallback.
- `ui/i18n`: JSON catalogs, `{name}` placeholders, CLDR plural forms, `pt-BR` → `pt` → default fallback, the OS language, and `i18n.T(ctx, ...)` in widgets.
- `ui/undo`: undo/redo with groups, merging of rapid edits (typing) and a saved/dirty marker.
- `ui/fuzzy`: the matching behind QuickPick ("gtf" finds "Go To File").

**Developer tools** (`ui/devtools`). `Inspector` outlines the widget under the pointer; clicking it shows its type, size, constraints, ancestors and fields. `PerformanceOverlay` shows fps, build/layout/paint times and rebuild counts (`BuildOwner.Stats()` has the numbers). `DumpWidgets` / `DumpRender` print the trees.

## Testing widgets (`ui/tester`)

`tester.New(widget, w, h)` runs the same pipeline as the app without a window, much like Flutter's `WidgetTester`:

```go
tt := tester.New(MyApp{}, 800, 600)
tt.TapText("Save")               // also Tap, Drag, Hover, Scroll, Key, Type
tt.Settle()                      // run animations on a fake clock
if _, ok := tt.Find("Saved"); !ok { ... }
tt.SavePNG("out.png")            // renders with the wgpu CPU fallback adapter
```

More helpers: `tt.Semantics()` / `tt.FindSemantics("button", "Save")` (read the UI by role and label), `tt.SecondaryTap` (right-click), `tt.Dispatch` (any pointer event), `tt.DropFiles` (files dropped from the OS), and `tt.Window.OpenResult` / `SaveResult` to script file dialogs.

## Next steps

- Platform IME events in gogpu (NSTextInputClient on macOS, `WM_IME_*` on Windows, text-input-v3/XIM on Linux); the engine side is ready.
- A code-editing widget on top of rich text (a rope buffer, multiple cursors, gutters, folding, highlighter hooks), as an optional package.
- Send the `internal/gogpu` changes upstream (see its FORK.md) and go back to the released module.
- Richer screen-reader support: text ranges and caret navigation in text fields, live regions, and announcements.
- GPU-side caching for repaint boundaries (reuse vertex data too, not just the display list).
- Transforms (scale/rotate), per-corner radii, fling scrolling.
