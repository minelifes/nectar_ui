// Package i18n translates an app's text: message catalogs per language
// (from code, JSON files or embedded folders), "{name}" placeholders,
// plural forms for many languages, fallback from "pt-BR" to "pt" to the
// default language, and the user's language detected from the OS.
//
//	b := i18n.NewBundle("en")
//	b.LoadFS(locales, "locales/*.json") // locales/en.json, locales/de.json ...
//	app := i18n.Localizations{Localizer: b.Localizer(i18n.Detect()), Child: page}
//
//	// en.json: {"greeting": "Hello, {name}!", "files.one": "{n} file", "files.other": "{n} files"}
//	i18n.T(ctx, "greeting", "name", user)  // "Hello, Ana!"
//	i18n.N(ctx, "files", count)            // "3 files"
package i18n

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/minelifes/nectar_ui/ui/widgets"
)

// Bundle holds the catalogs of every language.
type Bundle struct {
	mu       sync.RWMutex
	fallback string
	catalogs map[string]map[string]string
}

// NewBundle makes a bundle whose last-resort language is fallback ("en").
func NewBundle(fallback string) *Bundle {
	return &Bundle{fallback: Canonical(fallback), catalogs: map[string]map[string]string{}}
}

// Add merges messages into lang's catalog.
func (b *Bundle) Add(lang string, messages map[string]string) {
	lang = Canonical(lang)
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.catalogs[lang]
	if c == nil {
		c = map[string]string{}
		b.catalogs[lang] = c
	}
	for k, v := range messages {
		c[k] = v
	}
}

// LoadJSON adds a catalog from JSON: an object of key → message. Nested
// objects make dotted keys ({"menu": {"file": "File"}} is "menu.file").
func (b *Bundle) LoadJSON(lang string, data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("i18n: %s: %w", lang, err)
	}
	msgs := map[string]string{}
	var flatten func(prefix string, m map[string]any) error
	flatten = func(prefix string, m map[string]any) error {
		for k, v := range m {
			switch x := v.(type) {
			case string:
				msgs[prefix+k] = x
			case map[string]any:
				if err := flatten(prefix+k+".", x); err != nil {
					return err
				}
			default:
				return fmt.Errorf("i18n: %s: %s%s is not a string", lang, prefix, k)
			}
		}
		return nil
	}
	if err := flatten("", raw); err != nil {
		return err
	}
	b.Add(lang, msgs)
	return nil
}

// LoadFS loads every file matching pattern ("locales/*.json"); a file's
// name without extension is its language ("pt-BR.json").
func (b *Bundle) LoadFS(fsys fs.FS, pattern string) error {
	files, err := fs.Glob(fsys, pattern)
	if err != nil {
		return err
	}
	for _, f := range files {
		data, err := fs.ReadFile(fsys, f)
		if err != nil {
			return err
		}
		lang := strings.TrimSuffix(path.Base(f), path.Ext(f))
		if err := b.LoadJSON(lang, data); err != nil {
			return err
		}
	}
	return nil
}

// Languages returns the languages with a catalog.
func (b *Bundle) Languages() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]string, 0, len(b.catalogs))
	for l := range b.catalogs {
		out = append(out, l)
	}
	return out
}

// Localizer returns a localizer for the user's preferred languages, best
// first ("pt-BR", "en"); each is followed by its base language, and the
// bundle's fallback comes last.
func (b *Bundle) Localizer(langs ...string) *Localizer {
	var chain []string
	seen := map[string]bool{}
	add := func(l string) {
		if l != "" && !seen[l] {
			seen[l] = true
			chain = append(chain, l)
		}
	}
	for _, l := range langs {
		l = Canonical(l)
		add(l)
		if i := strings.IndexByte(l, '-'); i > 0 {
			add(l[:i])
		}
	}
	add(b.fallback)
	return &Localizer{b: b, chain: chain}
}

// Localizer translates messages for one language preference.
type Localizer struct {
	b     *Bundle
	chain []string
}

// Language returns the preferred language.
func (l *Localizer) Language() string { return l.chain[0] }

// lookup finds key in the first catalog of the chain that has it.
func (l *Localizer) lookup(key string) (string, string, bool) {
	l.b.mu.RLock()
	defer l.b.mu.RUnlock()
	for _, lang := range l.chain {
		if m, ok := l.b.catalogs[lang][key]; ok {
			return m, lang, true
		}
	}
	return "", "", false
}

// Has reports whether key is translated in some language of the chain.
func (l *Localizer) Has(key string) bool { _, _, ok := l.lookup(key); return ok }

// T translates key and fills its "{name}" placeholders from args, given as
// name, value pairs. A missing key returns the key itself, so untranslated
// text is easy to spot.
func (l *Localizer) T(key string, args ...any) string {
	msg, _, ok := l.lookup(key)
	if !ok {
		msg = key
	}
	return format(msg, args)
}

// N translates a message with plural forms: key+".zero" (optional, for
// n == 0), ".one", ".two", ".few", ".many" and ".other", picked by n with
// the rules of the language the message is found in. "{n}" is n; args add
// more placeholders.
func (l *Localizer) N(key string, n int, args ...any) string {
	args = append([]any{"n", n}, args...)
	if n == 0 {
		if msg, _, ok := l.lookup(key + ".zero"); ok {
			return format(msg, args)
		}
	}
	// The language that has the "other" form decides the rules.
	_, lang, ok := l.lookup(key + ".other")
	if !ok {
		return format(key, args)
	}
	form := PluralForm(lang, n)
	if msg, _, ok := l.lookup(key + "." + form); ok {
		return format(msg, args)
	}
	msg, _, _ := l.lookup(key + ".other")
	return format(msg, args)
}

func format(msg string, args []any) string {
	if len(args) < 2 || !strings.Contains(msg, "{") {
		return msg
	}
	pairs := make([]string, 0, len(args))
	for i := 0; i+1 < len(args); i += 2 {
		pairs = append(pairs, "{"+fmt.Sprint(args[i])+"}", fmt.Sprint(args[i+1]))
	}
	return strings.NewReplacer(pairs...).Replace(msg)
}

// PluralForm returns the CLDR plural category of integer n in lang: "one",
// "two", "few", "many" or "other". It knows the rules of the common
// European and Asian languages; others use the English rule.
func PluralForm(lang string, n int) string {
	if n < 0 {
		n = -n
	}
	base := Canonical(lang)
	if i := strings.IndexByte(base, '-'); i > 0 {
		base = base[:i]
	}
	m10, m100 := n%10, n%100
	switch base {
	case "ja", "zh", "ko", "vi", "th", "id", "ms":
		return "other"
	case "fr", "pt":
		if n == 0 || n == 1 {
			return "one"
		}
		return "other"
	case "ru", "uk", "be", "sr", "hr", "bs":
		switch {
		case m10 == 1 && m100 != 11:
			return "one"
		case m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14):
			return "few"
		}
		return "many"
	case "pl":
		switch {
		case n == 1:
			return "one"
		case m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14):
			return "few"
		}
		return "many"
	case "cs", "sk":
		switch {
		case n == 1:
			return "one"
		case n >= 2 && n <= 4:
			return "few"
		}
		return "other"
	case "lt":
		switch {
		case m10 == 1 && (m100 < 11 || m100 > 19):
			return "one"
		case m10 >= 2 && (m100 < 11 || m100 > 19):
			return "few"
		}
		return "other"
	case "ar":
		switch {
		case n == 0:
			return "zero"
		case n == 1:
			return "one"
		case n == 2:
			return "two"
		case m100 >= 3 && m100 <= 10:
			return "few"
		case m100 >= 11:
			return "many"
		}
		return "other"
	case "he":
		switch {
		case n == 1:
			return "one"
		case n == 2:
			return "two"
		}
		return "other"
	}
	if n == 1 {
		return "one"
	}
	return "other"
}

// Canonical normalizes a language tag: "pt_BR.UTF-8" → "pt-BR", "EN" →
// "en".
func Canonical(tag string) string {
	tag = strings.TrimSpace(tag)
	if i := strings.IndexAny(tag, ".@"); i >= 0 {
		tag = tag[:i]
	}
	tag = strings.ReplaceAll(tag, "_", "-")
	parts := strings.Split(tag, "-")
	for i, p := range parts {
		if i == 0 {
			parts[i] = strings.ToLower(p)
		} else if len(p) == 2 {
			parts[i] = strings.ToUpper(p)
		} else if len(p) == 4 {
			parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		}
	}
	return strings.Join(parts, "-")
}

// Detect returns the user's preferred languages from the environment
// (LANGUAGE, LC_ALL, LC_MESSAGES, LANG), best first; "C" and "POSIX" are
// skipped. Empty if nothing is set (pass the result to Bundle.Localizer,
// which then uses the fallback).
func Detect() []string {
	var out []string
	if v := os.Getenv("LANGUAGE"); v != "" {
		for _, l := range strings.Split(v, ":") {
			out = append(out, l)
		}
	}
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			out = append(out, v)
			break
		}
	}
	res := out[:0]
	for _, l := range out {
		if c := Canonical(l); c != "" && c != "c" && c != "posix" {
			res = append(res, c)
		}
	}
	return res
}

// ---------------------------------------------------------------------------
// Widgets

// Localizations provides a Localizer to its subtree; replace it (a new
// Localizer) to switch languages: everything using T or N rebuilds.
type Localizations struct {
	Localizer *Localizer
	Child     widgets.Widget
}

func (w Localizations) Build(widgets.BuildContext) widgets.Widget {
	return widgets.Provider[*Localizer]{Value: w.Localizer, Child: w.Child}
}

// Of returns the Localizer above ctx (one with no catalogs if there's
// none, which returns keys as they are).
func Of(ctx widgets.BuildContext) *Localizer {
	if l, ok := widgets.Of[*Localizer](ctx); ok && l != nil {
		return l
	}
	return empty
}

var empty = NewBundle("en").Localizer()

// T translates key with the Localizer above ctx (see Localizer.T).
func T(ctx widgets.BuildContext, key string, args ...any) string { return Of(ctx).T(key, args...) }

// N translates a plural message with the Localizer above ctx (see
// Localizer.N).
func N(ctx widgets.BuildContext, key string, n int, args ...any) string {
	return Of(ctx).N(key, n, args...)
}

