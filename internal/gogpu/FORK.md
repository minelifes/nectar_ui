# gogpu fork

This is a copy of [gogpu](https://github.com/gogpu/gogpu) **v0.54.0** (MIT,
see LICENSE), the windowing and GPU-surface layer nectar_ui runs on. It
lives here so nectar_ui can use platform features gogpu doesn't have yet;
every change is meant to be sent upstream, after which this copy can go
away again.

The copy has gogpu's non-test Go sources with the import path changed from
`github.com/gogpu/gogpu` to `github.com/minelifes/nectar_ui/internal/gogpu`
(commit "Vendor gogpu v0.54.0 as internal/gogpu"). Examples, docs, tests and
golden images are left out.

## Changes on top of v0.54.0

See the git history of this folder; each change is its own commit and is
listed here:

1. **Menu shortcuts and check marks.** `MenuItem` gets `KeyEquivalent`,
   `KeyModifiers`, `ShortcutText` and `Checked`: macOS shows and handles
   the key equivalent (function and navigation keys included) and the
   check state; Windows shows the shortcut right-aligned and the check
   mark. (`menu.go`, `app.go`, `internal/platform/platform.go`,
   `platform_darwin.go`, `menu_windows.go`)
2. **Secondary window control.** `Window` gets `SetMinSize`,
   `SetMaxSize`, `Maximize`, `IsMaximized`, `Minimize`, `SetFullscreen` and
   `IsFullscreen`, forwarding to the platform window the primary window
   already uses. (`window_manager.go`)
3. **File drag hover.** `App.OnFileDrag` reports enter / move / leave / drop
   of files dragged from the OS (the backends already produced these events
   on macOS, X11 and Wayland; they were dropped). Windows gets an OLE
   `IDropTarget` (64-bit), with `WM_DROPFILES` as the fallback. Drag
   positions are logical pixels on every platform (X11 and Windows
   reported physical ones). (`app.go`, `platform_linux.go`,
   `platform_windows.go`, new `drop_target_windows*.go`)
4. **Screen readers.** `Window.SetAccessibilityTree` / `App.SetAccessibilityTree`
   take a tree of accessible nodes (role, name, value, description,
   bounds, state, children, focus) and expose it to the OS:
   - Linux: AT-SPI2 over D-Bus (`internal/platform/atspi`, using
     `godbus/dbus`). The app registers with the accessibility bus in the
     background and implements Accessible, Component, Action, Value and
     Application, plus change events.
   - macOS: `NSAccessibilityElement`s set as the content view's
     accessibility children, with a press action and change notifications.
   - Windows: UI Automation fragment providers, built as COM objects in
     pure Go and served on `WM_GETOBJECT` (64-bit).

   (new `accessibility.go`, `internal/platform/accessibility_*.go`,
   `internal/platform/axtypes`, `internal/platform/atspi`,
   `internal/platform/darwin/accessibility.go`,
   `internal/platform/uia_windows.go`; `platform_windows.go` hooks
   `WM_GETOBJECT`)
5. **Window buttons.** `Config.WithWindowButtons(close, minimize, maximize)`,
   `Window.SetWindowButtons` / `App.SetWindowButtons` and
   `WindowButtonsInset` choose the native window buttons one by one. macOS
   hides individual traffic lights (`setHidden:` on `standardWindowButton:`,
   re-applied after style-mask changes) and reports where the visible ones
   end; Windows maps minimize / maximize to `WS_MINIMIZEBOX` /
   `WS_MAXIMIZEBOX`, greys `SC_CLOSE` and drops `WS_SYSMENU` when none is
   left (native title bar only). New optional `platform.WindowButtonsSetter`.
   (new `window_buttons.go`, `internal/platform/window_buttons_windows.go`;
   `config.go`, `app.go`, `window_manager.go`, `internal/platform/platform.go`,
   `platform_darwin.go`, `darwin/window.go`, `darwin/selectors.go`)

## Updating

1. Copy the new gogpu release over this folder (same file selection), and
   rewrite the import path again.
2. Re-apply the changes listed above that upstream hasn't merged.
