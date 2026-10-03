package commands

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/minelifes/nectar_ui/ui/widgets"
)

// Chord is one key press with its modifiers: Ctrl+S.
type Chord struct {
	Key  widgets.KeyCode
	Mods widgets.Modifiers
}

// Sequence is one or more chords pressed one after another: "Ctrl+K
// Ctrl+S" is two chords.
type Sequence []Chord

// ChordOf returns the chord of a key event.
func ChordOf(e widgets.KeyEvent) Chord { return Chord{Key: e.Key, Mods: e.Mods} }

// isMac picks macOS key names and the meaning of "Mod". Tests flip it.
var isMac = runtime.GOOS == "darwin"

var keyNames = map[widgets.KeyCode]string{
	widgets.KeyEscape: "Escape", widgets.KeyTab: "Tab", widgets.KeyBackspace: "Backspace",
	widgets.KeyEnter: "Enter", widgets.KeySpace: "Space", widgets.KeyInsert: "Insert",
	widgets.KeyDelete: "Delete", widgets.KeyHome: "Home", widgets.KeyEnd: "End",
	widgets.KeyPageUp: "PageUp", widgets.KeyPageDown: "PageDown", widgets.KeyLeft: "Left",
	widgets.KeyRight: "Right", widgets.KeyUp: "Up", widgets.KeyDown: "Down",
	widgets.KeyMinus: "-", widgets.KeyEqual: "=", widgets.KeyLeftBracket: "[",
	widgets.KeyRightBracket: "]", widgets.KeyBackslash: "\\", widgets.KeySemicolon: ";",
	widgets.KeyApostrophe: "'", widgets.KeyGrave: "`", widgets.KeyComma: ",",
	widgets.KeyPeriod: ".", widgets.KeySlash: "/",
}

var keyAliases = map[string]widgets.KeyCode{
	"esc": widgets.KeyEscape, "return": widgets.KeyEnter, "del": widgets.KeyDelete,
	"ins": widgets.KeyInsert, "pgup": widgets.KeyPageUp, "pgdn": widgets.KeyPageDown,
	"pagedn": widgets.KeyPageDown, "plus": widgets.KeyEqual, "minus": widgets.KeyMinus,
	"comma": widgets.KeyComma, "period": widgets.KeyPeriod, "slash": widgets.KeySlash,
	"backslash": widgets.KeyBackslash, "space": widgets.KeySpace,
}

func keyName(k widgets.KeyCode) string {
	switch {
	case k >= widgets.KeyA && k <= widgets.KeyZ:
		return string(rune('A' + k - widgets.KeyA))
	case k >= widgets.Key0 && k <= widgets.Key9:
		return string(rune('0' + k - widgets.Key0))
	case k >= widgets.KeyF1 && k <= widgets.KeyF12:
		return fmt.Sprintf("F%d", k-widgets.KeyF1+1)
	}
	if n, ok := keyNames[k]; ok {
		return n
	}
	return "?"
}

func parseKey(s string) (widgets.KeyCode, bool) {
	if len(s) == 1 {
		c := s[0]
		switch {
		case c >= 'a' && c <= 'z':
			return widgets.KeyA + widgets.KeyCode(c-'a'), true
		case c >= 'A' && c <= 'Z':
			return widgets.KeyA + widgets.KeyCode(c-'A'), true
		case c >= '0' && c <= '9':
			return widgets.Key0 + widgets.KeyCode(c-'0'), true
		}
	}
	var n int
	if _, err := fmt.Sscanf(strings.ToUpper(s), "F%d", &n); err == nil && n >= 1 && n <= 12 && fmt.Sprintf("F%d", n) == strings.ToUpper(s) {
		return widgets.KeyF1 + widgets.KeyCode(n-1), true
	}
	for k, name := range keyNames {
		if strings.EqualFold(name, s) {
			return k, true
		}
	}
	if k, ok := keyAliases[strings.ToLower(s)]; ok {
		return k, true
	}
	return 0, false
}

// ParseChord parses one chord: modifiers and a key joined by "+", like
// "Ctrl+Shift+P", "Alt+Left", "F5" or "Mod+/". "Mod" is the platform's
// shortcut modifier (⌘ on macOS, Ctrl elsewhere); "Cmd"/"Meta"/"Super"/
// "Win" name the ⌘/Windows key, "Option"/"Opt" Alt. Case doesn't matter.
func ParseChord(s string) (Chord, error) {
	var c Chord
	parts := strings.Split(strings.TrimSpace(s), "+")
	// "Ctrl++" (the key is "+") splits into a trailing empty part.
	if len(parts) >= 2 && parts[len(parts)-1] == "" && parts[len(parts)-2] == "" {
		parts = append(parts[:len(parts)-2], "=")
	}
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if i < len(parts)-1 {
			switch strings.ToLower(p) {
			case "ctrl", "control":
				c.Mods |= widgets.ModControl
			case "shift":
				c.Mods |= widgets.ModShift
			case "alt", "option", "opt":
				c.Mods |= widgets.ModAlt
			case "cmd", "command", "meta", "super", "win":
				c.Mods |= widgets.ModSuper
			case "mod":
				c.Mods |= modKey()
			default:
				return Chord{}, fmt.Errorf("commands: unknown modifier %q in %q", p, s)
			}
			continue
		}
		k, ok := parseKey(p)
		if !ok {
			return Chord{}, fmt.Errorf("commands: unknown key %q in %q", p, s)
		}
		c.Key = k
	}
	return c, nil
}

func modKey() widgets.Modifiers {
	if isMac {
		return widgets.ModSuper
	}
	return widgets.ModControl
}

// ParseKeys parses a key sequence: chords separated by spaces, like
// "Mod+S" or "Ctrl+K Ctrl+S".
func ParseKeys(s string) (Sequence, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil, fmt.Errorf("commands: empty key sequence")
	}
	seq := make(Sequence, 0, len(fields))
	for _, f := range fields {
		c, err := ParseChord(f)
		if err != nil {
			return nil, err
		}
		seq = append(seq, c)
	}
	return seq, nil
}

// MustParseKeys is ParseKeys that panics on error.
func MustParseKeys(s string) Sequence {
	seq, err := ParseKeys(s)
	if err != nil {
		panic(err)
	}
	return seq
}

// String returns the chord the way the platform shows shortcuts in menus:
// "⌃⇧P" style symbols on macOS, "Ctrl+Shift+P" elsewhere.
func (c Chord) String() string {
	if isMac {
		var b strings.Builder
		if c.Mods&widgets.ModControl != 0 {
			b.WriteString("⌃")
		}
		if c.Mods&widgets.ModAlt != 0 {
			b.WriteString("⌥")
		}
		if c.Mods&widgets.ModShift != 0 {
			b.WriteString("⇧")
		}
		if c.Mods&widgets.ModSuper != 0 {
			b.WriteString("⌘")
		}
		b.WriteString(keyName(c.Key))
		return b.String()
	}
	var parts []string
	if c.Mods&widgets.ModControl != 0 {
		parts = append(parts, "Ctrl")
	}
	if c.Mods&widgets.ModAlt != 0 {
		parts = append(parts, "Alt")
	}
	if c.Mods&widgets.ModShift != 0 {
		parts = append(parts, "Shift")
	}
	if c.Mods&widgets.ModSuper != 0 {
		parts = append(parts, "Win")
	}
	return strings.Join(append(parts, keyName(c.Key)), "+")
}

// Spec returns the chord in the portable form ParseChord reads, with
// explicit modifiers ("Ctrl+Shift+P", "Cmd+S").
func (c Chord) Spec() string {
	var parts []string
	if c.Mods&widgets.ModControl != 0 {
		parts = append(parts, "Ctrl")
	}
	if c.Mods&widgets.ModAlt != 0 {
		parts = append(parts, "Alt")
	}
	if c.Mods&widgets.ModShift != 0 {
		parts = append(parts, "Shift")
	}
	if c.Mods&widgets.ModSuper != 0 {
		parts = append(parts, "Cmd")
	}
	return strings.Join(append(parts, keyName(c.Key)), "+")
}

// String shows the sequence for menus and tooltips.
func (s Sequence) String() string {
	parts := make([]string, len(s))
	for i, c := range s {
		parts[i] = c.String()
	}
	return strings.Join(parts, " ")
}

// Spec returns the sequence in the form ParseKeys reads.
func (s Sequence) Spec() string {
	parts := make([]string, len(s))
	for i, c := range s {
		parts[i] = c.Spec()
	}
	return strings.Join(parts, " ")
}

// Equal reports whether two sequences are the same keys.
func (s Sequence) Equal(o Sequence) bool {
	if len(s) != len(o) {
		return false
	}
	for i := range s {
		if s[i] != o[i] {
			return false
		}
	}
	return true
}

// hasPrefix reports whether p is a proper prefix of s.
func (s Sequence) hasPrefix(p Sequence) bool {
	return len(p) < len(s) && s[:len(p)].Equal(p)
}
