package commands

import (
	"encoding/json"
	"testing"

	"github.com/minelifes/nectar_ui/ui/widgets"
)

func TestParseAndFormatKeys(t *testing.T) {
	defer func(m bool) { isMac = m }(isMac)
	for _, mac := range []bool{false, true} {
		isMac = mac
		mod := widgets.ModControl
		if mac {
			mod = widgets.ModSuper
		}
		cases := []struct {
			in   string
			want Sequence
		}{
			{"Mod+S", Sequence{{widgets.KeyS, mod}}},
			{"ctrl+shift+p", Sequence{{widgets.KeyP, widgets.ModControl | widgets.ModShift}}},
			{"Ctrl+K Ctrl+S", Sequence{{widgets.KeyK, widgets.ModControl}, {widgets.KeyS, widgets.ModControl}}},
			{"F5", Sequence{{widgets.KeyF5, 0}}},
			{"Alt+Left", Sequence{{widgets.KeyLeft, widgets.ModAlt}}},
			{"Cmd+/", Sequence{{widgets.KeySlash, widgets.ModSuper}}},
			{"Ctrl++", Sequence{{widgets.KeyEqual, widgets.ModControl}}},
			{"Shift+Esc", Sequence{{widgets.KeyEscape, widgets.ModShift}}},
			{"Option+PgDn", Sequence{{widgets.KeyPageDown, widgets.ModAlt}}},
		}
		for _, c := range cases {
			got, err := ParseKeys(c.in)
			if err != nil || !got.Equal(c.want) {
				t.Errorf("mac=%v ParseKeys(%q) = %v, %v", mac, c.in, got, err)
				continue
			}
			// Spec round-trips.
			if again, err := ParseKeys(got.Spec()); err != nil || !again.Equal(got) {
				t.Errorf("Spec %q doesn't round-trip: %v %v", got.Spec(), again, err)
			}
		}
	}
	for _, bad := range []string{"", "Ctrl+", "Hyper+S", "Ctrl+Foo", "F13"} {
		if _, err := ParseKeys(bad); err == nil {
			t.Errorf("ParseKeys(%q) accepted", bad)
		}
	}
	isMac = false
	if s := MustParseKeys("Ctrl+Shift+P").String(); s != "Ctrl+Shift+P" {
		t.Errorf("String = %q", s)
	}
	if s := MustParseKeys("Ctrl+K Ctrl+S").String(); s != "Ctrl+K Ctrl+S" {
		t.Errorf("String = %q", s)
	}
	isMac = true
	if s := MustParseKeys("Mod+Shift+P").String(); s != "⇧⌘P" {
		t.Errorf("mac String = %q", s)
	}
}

func TestKeymapOverridesAndJSON(t *testing.T) {
	defer func(m bool) { isMac = m }(isMac)
	isMac = false
	save := Command{ID: "file.save", Keys: []string{"Mod+S"}}
	other := Command{ID: "x", Keys: []string{"Ctrl+K"}}
	k := NewKeymap()
	if ks := k.KeysFor(save); len(ks) != 1 || ks[0].Spec() != "Ctrl+S" {
		t.Fatalf("default keys: %v", ks)
	}
	if err := k.SetKeys("file.save", "Ctrl+K Ctrl+S", "F2"); err != nil {
		t.Fatal(err)
	}
	if err := k.SetKeys("file.save", "Nope+S"); err == nil {
		t.Fatal("bad keys accepted")
	}
	if ks := k.KeysFor(save); len(ks) != 2 || ks[0].Spec() != "Ctrl+K Ctrl+S" {
		t.Fatalf("override: %v", ks)
	}
	// Ctrl+K is now a prefix of save's Ctrl+K Ctrl+S.
	if cf := k.Conflicts([]Command{save, other}); len(cf) != 1 || len(cf[0].Commands) != 2 {
		t.Fatalf("conflicts: %+v", cf)
	}
	data, err := json.Marshal(k)
	if err != nil {
		t.Fatal(err)
	}
	k2 := NewKeymap()
	if err := json.Unmarshal(data, k2); err != nil {
		t.Fatal(err)
	}
	if ks := k2.KeysFor(save); len(ks) != 2 || ks[1].Spec() != "F2" {
		t.Fatalf("reloaded: %s → %v", data, ks)
	}
	k2.SetKeys("file.save") // unbind
	if ks := k2.KeysFor(save); len(ks) != 0 || !k2.Overridden("file.save") {
		t.Fatalf("unbound: %v", ks)
	}
	k2.Reset("file.save")
	if ks := k2.KeysFor(save); len(ks) != 1 {
		t.Fatalf("reset: %v", ks)
	}
}

func TestRegistryShadowing(t *testing.T) {
	r := NewRegistry()
	n := 0
	r.Subscribe(func() { n++ })
	r.Register(Command{ID: "a", Title: "First"}, Command{ID: "b", Title: "B"})
	undo := r.Register(Command{ID: "a", Title: "Second"})
	if c, _ := r.Get("a"); c.Title != "Second" || len(r.All()) != 2 {
		t.Fatalf("shadowing: %+v %v", c, r.All())
	}
	undo()
	if c, _ := r.Get("a"); c.Title != "First" {
		t.Fatalf("after removal: %+v", c)
	}
	if n != 3 {
		t.Fatalf("%d notifications", n)
	}
}
