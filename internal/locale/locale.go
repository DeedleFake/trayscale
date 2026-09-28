// Package locale provides gettext-style string translation for Trayscale
// using [github.com/leonelquinteros/gotext], matching the usual approach
// for gotk4 GNOME apps.
//
// Catalogs live under po/<lang>/default.po (embedded at build time).
// Call [Init] once at process start before showing UI. Untranslated
// strings fall back to the English source msgid.
package locale

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"

	"github.com/leonelquinteros/gotext"
)

const domain = "default"

var (
	mu      sync.RWMutex
	current = gotext.NewLocale("", "C")
)

// Init loads translations from poFS for the best matching system
// language. poFS should contain directories named by locale code
// (for example "en", "es", "en_US") each with a default.po file.
// It is safe to call with a nil FS; Get then returns the source string.
func Init(poFS fs.FS) {
	if poFS == nil {
		return
	}
	lang := matchLanguage(poFS, languageCandidates())
	if lang == "" {
		return
	}
	InitLanguage(poFS, lang)
}

// InitLanguage loads the given language catalog from poFS.
func InitLanguage(poFS fs.FS, lang string) {
	if poFS == nil || lang == "" {
		return
	}
	loc := gotext.NewLocaleFS(lang, poFS)
	loc.AddDomain(domain)

	mu.Lock()
	current = loc
	mu.Unlock()
}

// Language returns the loaded locale code, or "C" when none matched.
func Language() string {
	mu.RLock()
	defer mu.RUnlock()
	if current == nil {
		return "C"
	}
	return current.GetLanguage()
}

// Get returns the translation of str for the current locale.
// Optional vars are formatted with fmt.Sprintf / gotext placeholders.
func Get(str string, vars ...any) string {
	mu.RLock()
	loc := current
	mu.RUnlock()
	if loc == nil {
		return format(str, vars...)
	}
	return loc.Get(str, vars...)
}

// GetN returns the plural form of a translation.
func GetN(singular, plural string, n int, vars ...any) string {
	mu.RLock()
	loc := current
	mu.RUnlock()
	if loc == nil {
		if n == 1 {
			return format(singular, vars...)
		}
		return format(plural, vars...)
	}
	return loc.GetN(singular, plural, n, vars...)
}

func format(str string, vars ...any) string {
	if len(vars) == 0 {
		return str
	}
	return fmt.Sprintf(str, vars...)
}

// languageCandidates mirrors the useful part of g_get_language_names:
// LANGUAGE (colon-separated), then LC_ALL / LC_MESSAGES / LANG.
func languageCandidates() []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || s == "C" || s == "POSIX" {
			return
		}
		for _, part := range strings.Split(s, ":") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if _, ok := seen[part]; ok {
				continue
			}
			seen[part] = struct{}{}
			out = append(out, part)
		}
	}

	add(os.Getenv("LANGUAGE"))
	add(os.Getenv("LC_ALL"))
	add(os.Getenv("LC_MESSAGES"))
	add(os.Getenv("LANG"))
	return out
}

func matchLanguage(poFS fs.FS, candidates []string) string {
	for _, cand := range candidates {
		for _, code := range localeVariants(cand) {
			if localeAvailable(poFS, code) {
				return code
			}
		}
	}
	if localeAvailable(poFS, "en") {
		return "en"
	}
	return ""
}

func localeVariants(lang string) []string {
	lang = strings.ReplaceAll(lang, "-", "_")
	var out []string
	seen := map[string]struct{}{}
	add := func(s string) {
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	add(lang)
	if i := strings.IndexByte(lang, '.'); i >= 0 {
		add(lang[:i])
		lang = lang[:i]
	}
	if i := strings.IndexByte(lang, '@'); i >= 0 {
		add(lang[:i])
		lang = lang[:i]
	}
	if i := strings.IndexByte(lang, '_'); i >= 0 {
		add(lang[:i])
	}
	return out
}

func localeAvailable(poFS fs.FS, code string) bool {
	for _, name := range []string{
		code + "/LC_MESSAGES/" + domain + ".po",
		code + "/" + domain + ".po",
		code + "/LC_MESSAGES/" + domain + ".mo",
		code + "/" + domain + ".mo",
	} {
		if _, err := fs.Stat(poFS, name); err == nil {
			return true
		}
	}
	return false
}
