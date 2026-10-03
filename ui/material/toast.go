package material

import (
	"slices"
	"sync"
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/tasks"
	"github.com/minelifes/nectar_ui/ui/vector"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

// ToastKind picks a toast's icon and accent color.
type ToastKind uint8

const (
	ToastInfo ToastKind = iota
	ToastSuccess
	ToastWarning
	ToastError
	ToastPlain // no icon
)

// ToastAction is a button on a toast.
type ToastAction struct {
	Label     string
	OnPressed func()
}

// Toast is a notification that appears in the bottom-right corner,
// stacked with others: build results, background task progress, errors.
type Toast struct {
	Title   string
	Message string
	Kind    ToastKind
	Icon    *vector.Icon // overrides the kind's icon
	Actions []ToastAction
	// Duration is how long it stays: 0 = the theme's (5s), < 0 = until
	// closed. Hovering a toast keeps it open.
	Duration time.Duration
	// Progress in (0, 1] shows a bar filled that much; Busy shows an
	// indeterminate one. Toasts with either stay until closed.
	Progress float32
	Busy     bool
	// OnClose is called when the toast goes away (timeout, ×, Close).
	OnClose func()
}

// ToastHandle controls a shown toast.
type ToastHandle struct {
	t     *toaster
	toast Toast
	id    int
	shown time.Time
	hover bool
	gone  bool
}

// Update replaces the toast's content (progress, message), restarting its
// timer.
func (h *ToastHandle) Update(t Toast) {
	h.t.mu.Lock()
	h.toast, h.shown = t, h.t.now()
	h.t.mu.Unlock()
	h.t.changed()
}

// Close removes the toast.
func (h *ToastHandle) Close() { h.t.remove(h) }

// toaster is the stack of toasts of one overlay.
type toaster struct {
	mu     sync.Mutex
	ov     *w.OverlayState
	owner  *w.BuildOwner
	entry  *w.OverlayEntry
	ticker *w.Ticker
	toasts []*ToastHandle
	next   int
}

var (
	toastersMu sync.Mutex
	toasters   = map[*w.OverlayState]*toaster{}
)

func (t *toaster) now() time.Time {
	if t.owner.Now != nil {
		return t.owner.Now()
	}
	return time.Now()
}

// ShowToast shows t and returns a handle to update or close it.
func ShowToast(ctx w.BuildContext, t Toast) *ToastHandle {
	ov := w.OverlayOf(ctx)
	if ov == nil {
		return &ToastHandle{t: &toaster{}, gone: true}
	}
	toastersMu.Lock()
	ts := toasters[ov]
	if ts == nil {
		ts = &toaster{ov: ov, owner: ctx.Owner()}
		toasters[ov] = ts
	}
	toastersMu.Unlock()
	ts.mu.Lock()
	ts.next++
	h := &ToastHandle{t: ts, toast: t, id: ts.next, shown: ts.now()}
	ts.toasts = append(ts.toasts, h)
	first := ts.entry == nil
	if first {
		ts.entry = &w.OverlayEntry{Builder: ts.build}
	}
	ts.mu.Unlock()
	if first {
		ov.Insert(ts.entry)
		ts.ticker = ts.owner.NewTicker(func(time.Duration) { ts.expire() })
		ts.ticker.Start()
	} else {
		ts.changed()
	}
	return h
}

func (t *toaster) changed() {
	if t.entry != nil {
		t.entry.MarkNeedsBuild()
	}
}

func (t *toaster) remove(h *ToastHandle) {
	t.mu.Lock()
	if h.gone {
		t.mu.Unlock()
		return
	}
	h.gone = true
	t.toasts = slices.DeleteFunc(t.toasts, func(x *ToastHandle) bool { return x == h })
	empty := len(t.toasts) == 0
	t.mu.Unlock()
	if h.toast.OnClose != nil {
		h.toast.OnClose()
	}
	if empty {
		t.dispose()
		return
	}
	t.changed()
}

func (t *toaster) dispose() {
	toastersMu.Lock()
	delete(toasters, t.ov)
	toastersMu.Unlock()
	if t.ticker != nil {
		t.ticker.Stop()
	}
	if t.entry != nil {
		t.entry.Remove()
		t.entry = nil
	}
}

// expire closes toasts whose time is up (runs every frame while any show).
func (t *toaster) expire() {
	th := ThemeOf(t.ov.Context())
	def := th.Toast.Duration
	if def == 0 {
		def = 5 * time.Second
	}
	now := t.now()
	t.mu.Lock()
	var due []*ToastHandle
	for _, h := range t.toasts {
		d := h.toast.Duration
		if d == 0 {
			d = def
		}
		if h.hover {
			h.shown = now // hovering keeps it open
		}
		if d > 0 && h.toast.Progress <= 0 && !h.toast.Busy && now.Sub(h.shown) >= d {
			due = append(due, h)
		}
	}
	t.mu.Unlock()
	for _, h := range due {
		t.remove(h)
	}
}

func (t *toaster) build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	width := pickF(th.Toast.Width, 360)
	t.mu.Lock()
	list := slices.Clone(t.toasts)
	t.mu.Unlock()
	cards := make([]w.Widget, 0, len(list))
	for _, h := range list {
		h := h
		cards = append(cards, w.KeyedSubtree{ID: h.id, Child: w.MouseRegion{
			OnEnter: func(w.PointerEvent) { h.hover = true },
			OnExit:  func(w.PointerEvent) { h.hover = false },
			Child:   ToastCard{Toast: h.toast, OnClose: h.Close},
		}})
	}
	return w.Stack{Expand: true, Children: []w.Widget{
		w.Positioned{Right: w.At(16), Bottom: w.At(16), Width: w.At(width),
			Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 8, Children: cards}},
	}}
}

// ToastCard draws a toast (ShowToast stacks them in the overlay; use it
// directly to show one inline).
type ToastCard struct {
	Toast   Toast
	OnClose func() // the × button; nil = none
	// Style overrides Theme.Toast.
	Style ToastTheme
}

func (c ToastCard) Build(ctx w.BuildContext) w.Widget {
	th := ThemeOf(ctx)
	sc := th.Scheme
	st := merge(th.Toast, c.Style)
	t := c.Toast
	accent, icon := pick(st.InfoColor, sc.Primary), iconInfo
	switch t.Kind {
	case ToastSuccess:
		accent, icon = pick(st.SuccessColor, geom.Hex(0x2e7d32)), iconCheckCircle
	case ToastWarning:
		accent, icon = pick(st.WarningColor, geom.Hex(0xb26a00)), iconWarning
	case ToastError:
		accent, icon = pick(st.ErrorColor, sc.Error), iconError
	case ToastPlain:
		icon = nil
	}
	if t.Icon != nil {
		icon = t.Icon
	}
	titleSt := pickTC(st.TitleStyle, th.Text.TitleSmall, sc.OnSurface)
	msgSt := pickTC(st.MessageStyle, th.Text.BodyMedium, sc.OnSurfaceVariant)
	body := []w.Widget{}
	if t.Title != "" {
		body = append(body, w.Text{Text: t.Title, Style: titleSt})
	}
	if t.Message != "" {
		body = append(body, w.Text{Text: t.Message, Style: msgSt})
	}
	if t.Progress > 0 || t.Busy {
		ps := ProgressIndicatorTheme{Color: pick(st.ProgressColor, accent)}
		if t.Busy {
			body = append(body, w.Padding{Padding: geom.InsetsLTRB(0, 6, 0, 2), Child: LinearProgressIndicator{Indeterminate: true, Style: ps}})
		} else {
			body = append(body, w.Padding{Padding: geom.InsetsLTRB(0, 6, 0, 2), Child: LinearProgressIndicator{Value: min(t.Progress, 1), Style: ps}})
		}
	}
	if len(t.Actions) > 0 {
		btns := make([]w.Widget, len(t.Actions))
		for i, a := range t.Actions {
			a := a
			btns[i] = TextButton{Label: a.Label, OnPressed: func() {
				if a.OnPressed != nil {
					a.OnPressed()
				}
				if c.OnClose != nil {
					c.OnClose()
				}
			}, Style: ButtonStyle{ForegroundColor: pick(st.ActionColor, sc.Primary)}}
		}
		body = append(body, w.Row{Main: w.MainEnd, Spacing: 4, Children: btns})
	}
	row := []w.Widget{}
	if icon != nil {
		row = append(row, w.Icon{Icon: icon, Size: 22, Color: accent})
	}
	row = append(row, w.Expanded{Child: w.Column{Cross: w.CrossStretch, ShrinkMain: true, Spacing: 2, Children: body}})
	if c.OnClose != nil {
		row = append(row, w.GestureDetector{OnTap: c.OnClose, Child: w.MouseRegion{Cursor: w.CursorPointer,
			Child: w.Icon{Icon: iconClose, Size: 18, Color: pick(st.CloseIconColor, sc.OnSurfaceVariant)}}})
	}
	return w.AbsorbPointer{Child: Surface{Color: pick(st.BackgroundColor, sc.SurfaceContainerHigh), Radius: pickF(st.Radius, CornerMedium),
		Elevation: pickI(st.Elevation, 3), ShadowColor: st.ShadowColor,
		Child: w.Padding{Padding: geom.InsetsLTRB(14, 12, 12, 12), Child: w.Row{Cross: w.CrossStart, Spacing: 12, Children: row}}}}
}

// ShowTaskToast shows a toast that follows a background task: its title,
// status and progress while it runs (with a Cancel button), then whether
// it succeeded or failed.
func ShowTaskToast(ctx w.BuildContext, t *tasks.Task) *ToastHandle {
	owner := ctx.Owner()
	view := func() Toast {
		switch t.State() {
		case tasks.Done:
			return Toast{Title: t.Title(), Message: "Done", Kind: ToastSuccess}
		case tasks.Failed:
			msg := "Failed"
			if err := t.Err(); err != nil {
				msg = err.Error()
			}
			return Toast{Title: t.Title(), Message: msg, Kind: ToastError, Duration: 10 * time.Second}
		case tasks.Cancelled:
			return Toast{Title: t.Title(), Message: "Cancelled", Kind: ToastPlain}
		}
		p := t.Progress()
		return Toast{Title: t.Title(), Message: t.Status(), Kind: ToastPlain, Progress: max(p, 0.001), Busy: p < 0,
			Actions: []ToastAction{{Label: "Cancel", OnPressed: t.Cancel}}}
	}
	h := ShowToast(ctx, view())
	var mu sync.Mutex
	pending := false
	var cancel func()
	cancel = t.Subscribe(func() {
		mu.Lock()
		if pending {
			mu.Unlock()
			return
		}
		pending = true
		mu.Unlock()
		owner.Post(func() {
			mu.Lock()
			pending = false
			mu.Unlock()
			if h.gone {
				return
			}
			h.Update(view())
			if t.State() != tasks.Running {
				cancel()
			}
		})
	})
	return h
}
