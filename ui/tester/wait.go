package tester

import (
	"time"

	"github.com/minelifes/nectar_ui/ui/geom"
	"github.com/minelifes/nectar_ui/ui/widgets"
)

// PumpUntil pumps frames until cond returns true or timeout passes (real
// time), for work that finishes on other goroutines, like loading images.
// It reports whether cond became true.
func (t *Tester) PumpUntil(cond func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		t.Pump()
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// Compose simulates an input method showing preedit (e.g. pinyin or kana
// before conversion) in the focused field, with the caret at byte offset
// cursor inside it (-1 = at the end). The first call starts composing.
func (t *Tester) Compose(preedit string, cursor int) {
	fm := t.Build.Focus()
	if !t.composing {
		t.composing = true
		fm.HandleIME(widgets.IMEEvent{Kind: widgets.IMEStart})
	}
	if cursor < 0 {
		cursor = len(preedit)
	}
	fm.HandleIME(widgets.IMEEvent{Kind: widgets.IMEUpdate, Text: preedit, Cursor: cursor})
	t.Pump()
}

// Commit ends the composition, inserting text (what the user picked).
func (t *Tester) Commit(text string) {
	t.composing = false
	t.Build.Focus().HandleIME(widgets.IMEEvent{Kind: widgets.IMEEnd, Text: text})
	t.Pump()
}

// CancelIME ends the composition without inserting anything.
func (t *Tester) CancelIME() { t.Commit("") }

// IMERect is where the focused field reports its caret (window
// coordinates): where the platform puts the IME candidate window.
func (t *Tester) IMERect() (geom.Rect, bool) { return t.Build.Focus().IMERect() }
