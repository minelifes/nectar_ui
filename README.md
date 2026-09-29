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

## Text

- **Fonts:** `golang.org/x/image/font/sfnt` parses TTF/OTF. The Go fonts are embedded, so there's no setup (Latin, Cyrillic, Greek).
- **Shaping:** cmap lookup plus kerning. Contextual shaping for Arabic/Indic isn't supported yet (HarfBuzz-style shaping would slot into `text.shape`).
- **Layout:** greedy word wrap at spaces and hyphens, mid-word break for long words, `\n` hard breaks, `LineHeight`, `LetterSpacing`, alignment, and `MaxLines` + ellipsis.
- **Atlas:** glyphs are rasterized on demand at *physical* pixel size with 4 horizontal subpixel positions, so text stays crisp at any DPI. They're stored in an RGBA atlas (1024², shelf packer). Only dirty rows are uploaded, and the atlas resets itself when it fills up.

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

**Resources** (`ui/resources`) are files shipped inside the app. Embed a folder and mount it once; everything can then read by name, wherever the app is installed:

```go
//go:embed images fonts
var files embed.FS

resources.Mount("", files)             // later mounts override earlier ones
resources.Mount("plugin", pluginFiles) // under plugin/; returns an unmount func
data, err := resources.ReadFile("images/logo.png")
resources.FS()                         // merged io/fs view (ReadDir, Stat, Glob, …)
```

In development you can mount the folder on disk instead, so edits show up without rebuilding: `resources.Mount("", os.DirFS("assets"))`. Apps made with `nectar new` do the embedding for you in `assets/assets.go`.

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

- IME composition for CJK text input, and soft wrapping in multiline text fields.
- Layers / repaint boundaries (cache display lists of static subtrees).
- Images (same atlas/texture path), transforms (scale/rotate), per-corner radii, fling scrolling.
