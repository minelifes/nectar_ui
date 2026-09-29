# Nectar UI

A retained-mode UI engine in Go on top of wgpu (`gogpu/wgpu` + `gogpu/gogpu` for windowing).
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

`nectar templates` lists the templates. Their sources live in `cmd/nectar/templates` (`[[.Engine]]`-style placeholders, `_name` → `.name`). `NECTAR_BUILD_TEST=1 go test ./cmd/nectar` generates each one against this checkout and runs its tests.

## Packages (dependencies point downwards only)

```
ui            App: window + frame loop (build → layout → paint → GPU)
├── widgets   Widget / Element tree, State, BuildOwner, basic widgets
├── gpu       wgpu pipeline, WGSL uber-shader, batching, atlas upload
├── render    RenderObject tree: layout (constraints), paint → Canvas display list
├── text      Font, paragraph layout (wrap / align / ellipsis), glyph atlas
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
- **Shaping:** cmap lookup plus kerning. Contextual shaping for Arabic/Indic isn't supported yet (HarfBuzz-style shaping would slot into `text.shape`).
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
- **Fonts for CJK:** the built-in Go fonts don't include CJK glyphs. Load a CJK font (e.g. Noto Sans CJK) with `text.ParseFont` from your resources and use it in the text style.

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

**While developing**, use `WithResources("", os.DirFS("assets"))` so edits show up without rebuilding. Apps made with `nectar new` embed `assets/` and mount it in `main.go`.

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
- **Tests:** `tester.Tester.Window` fakes it. `SetSize` really resizes the test surface (`tt.Size`), clamped to the min/max size, and title, fullscreen and maximize are recorded for assertions.

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
- **Scrolling:** `ScrollView`, `ListView`, `ListViewBuilder` (lazy, fixed row height) and `ScrollController` (`JumpTo`, `AnimateTo`). Nested scroll views hand wheel events to the innermost one that can move.
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
- **Selection:** `Checkbox` (with tristate and error), `Radio[T]`, `Switch`, `Slider`, `RangeSlider`, assist, filter, choice, input and suggestion chips, `ShowDatePicker` / `CalendarDatePicker`, `ShowTimePicker`, `ShowMenu`, `PopupMenuButton`, `DropdownMenu`.
- **Text input:** `TextField` (filled or outlined, floating label, hint, helper, error, icons, counter), `SearchBar`.
- **Navigation:** `AppBar` (small, center, medium, large, with auto back), `NavigationBar`, `NavigationRail`, `NavigationDrawer`, `TabBar` / `TabBarView`, `BottomAppBar`, `Scaffold` (with drawer), `Push` / `Pop`.
- **Data:** `DataTable` (sorting, row selection), `Stepper`.
- **Building blocks for your own components:** `Surface` (a painted container) and `InkSurface`, which adds state layers, a ripple, a focus ring, keyboard activation and elevation on hover.

`CGO_ENABLED=0 go run ./examples/material` opens the gallery with every component, a dark-mode toggle and a seed picker.

Known gaps compared to Flutter: the time picker is input-only with 24-hour time (no dial), ripples in segmented buttons aren't clipped to the pill ends, carousel items are visual only (use `OnTap`; no buttons inside items), and fonts are the Go fonts, not Roboto.

## Testing widgets (`ui/tester`)

`tester.New(widget, w, h)` runs the same pipeline as the app without a window, much like Flutter's `WidgetTester`:

```go
tt := tester.New(MyApp{}, 800, 600)
tt.TapText("Save")               // also Tap, Drag, Hover, Scroll, Key, Type
tt.Settle()                      // run animations on a fake clock
if _, ok := tt.Find("Saved"); !ok { ... }
tt.SavePNG("out.png")            // renders with the wgpu CPU fallback adapter
```

## Next steps

- Platform IME events in gogpu (NSTextInputClient on macOS, `WM_IME_*` on Windows, text-input-v3/XIM on Linux); the engine side is ready.
- Font fallback (e.g. a CJK font behind the default one), so mixed-script text renders without choosing a font per style.
- GPU-side caching for repaint boundaries (reuse vertex data too, not just the display list).
- Transforms (scale/rotate), per-corner radii, fling scrolling.
