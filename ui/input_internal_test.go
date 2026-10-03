package ui

import (
	"testing"

	"github.com/minelifes/nectar_ui/ui/tester"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

// fieldWith mounts a focused text field and returns it with its tester.
func fieldWith(t *testing.T, initial string) (*tester.Tester, *widgets.TextEditingController) {
	t.Helper()
	ctrl := widgets.NewTextController(initial)
	tt := tester.New(widgets.EditableText{Controller: ctrl, Autofocus: true}, 300, 100)
	tt.Pump()
	return tt, ctrl
}

// send routes platform-style events through the app's input queue logic.
func send(tt *tester.Tester, q *inputQueue, events ...queuedEvent) {
	for _, e := range events {
		q.deliver(tt.Build.Focus(), e)
	}
	tt.Pump()
}

func key(k widgets.KeyCode, mods widgets.Modifiers) queuedEvent {
	return queuedEvent{key: &widgets.KeyEvent{Key: k, Mods: mods}}
}

func txt(s string) queuedEvent { return queuedEvent{text: s} }

// shortcut is the platform's copy/paste modifier.
func shortcut() widgets.Modifiers {
	if (widgets.Modifiers(widgets.ModSuper)).Shortcut() {
		return widgets.ModSuper
	}
	return widgets.ModControl
}

// macOS reports ⌘V as the key press plus the text "v", X11 and Wayland do
// the same for Ctrl+V: the text must not reach the field.
func TestPasteDoesNotTypeTheLetter(t *testing.T) {
	for _, mods := range []widgets.Modifiers{widgets.ModSuper, widgets.ModControl} {
		tt, ctrl := fieldWith(t, "")
		tt.Build.Clipboard.WriteText("pasted")
		q := &inputQueue{}
		send(tt, q, key(widgets.KeyV, shortcut()), txt("v"))
		if got := ctrl.Text(); got != "pasted" {
			t.Fatalf("paste gave %q, want %q", got, "pasted")
		}
		// The other shortcut modifier sends its letter too; it's dropped.
		send(tt, q, key(widgets.KeyV, mods), txt("v"))
		if got := ctrl.Text(); got != "pasted" && got != "pastedpasted" {
			t.Fatalf("mods %v: %q", mods, got)
		}
	}
}

func TestCopyAndCutDoNotTypeTheLetter(t *testing.T) {
	tt, ctrl := fieldWith(t, "hello")
	q := &inputQueue{}
	send(tt, q, key(widgets.KeyA, shortcut()), txt("a"), key(widgets.KeyC, shortcut()), txt("c"))
	if got := ctrl.Text(); got != "hello" {
		t.Fatalf("select all + copy changed the text to %q", got)
	}
	if got := tt.Clipboard(); got != "hello" {
		t.Fatalf("clipboard = %q", got)
	}
	send(tt, q, key(widgets.KeyX, shortcut()), txt("x"))
	if ctrl.Text() != "" || tt.Clipboard() != "hello" {
		t.Fatalf("cut: text %q clipboard %q", ctrl.Text(), tt.Clipboard())
	}
}

func TestTypingStillWorks(t *testing.T) {
	tt, ctrl := fieldWith(t, "")
	q := &inputQueue{}
	send(tt, q,
		key(widgets.KeyV, shortcut()), txt("v"), // a shortcut...
		key(widgets.KeyA, 0), txt("a"), // ...doesn't swallow the next letters
		key(widgets.KeyB, widgets.ModShift), txt("B"),
		key(widgets.KeyE, widgets.ModAlt), txt("é"), // Option / Alt compose
		key(widgets.KeyQ, widgets.ModControl|widgets.ModAlt), txt("@"), // AltGr on Windows
	)
	if got := ctrl.Text(); got != "aBé@" {
		t.Fatalf("typed %q, want %q", got, "aBé@")
	}
	// Text with no key event before it (IME commit, on-screen keyboard).
	send(tt, q, txt("你"))
	if got := ctrl.Text(); got != "aBé@你" {
		t.Fatalf("typed %q", got)
	}
}
