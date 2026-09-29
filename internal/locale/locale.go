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
	"os/exec"
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
		if isCLocale(s) {
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

// SanitizeEnvironment ensures libc/GTK can call setlocale successfully.
//
// When LANG / LC_* name a locale that is not generated on this system
// (for example LANG=ja with no ja_JP.UTF-8 in locale -a), glibc prints
// "Cannot set LC_* to default locale" warnings and GTK/GLib can hang
// during init. This copies the user's language preference into LANGUAGE
// (if unset) so gotext still selects the right catalog, then forces a
// known-good UTF-8 libc locale (C.UTF-8 / C.utf8) via LC_ALL.
//
// Call once at process start before gtk.Init and before [Init].
func SanitizeEnvironment() {
	pref := firstEnv("LANGUAGE", "LC_ALL", "LC_MESSAGES", "LANG")
	if !needsLocaleSanitize() {
		return
	}

	if os.Getenv("LANGUAGE") == "" {
		if lang := languageFromLocale(pref); lang != "" {
			_ = os.Setenv("LANGUAGE", lang)
		}
	}

	fallback := fallbackLibcLocale()
	_ = os.Setenv("LC_ALL", fallback)
	if v := os.Getenv("LANG"); v != "" && !libcSupports(v) {
		_ = os.Setenv("LANG", fallback)
	}
}

func needsLocaleSanitize() bool {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LC_CTYPE", "LANG"} {
		if v := os.Getenv(key); v != "" && !libcSupports(v) {
			return true
		}
	}
	return false
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			// LANGUAGE may be colon-separated; take the first entry.
			if key == "LANGUAGE" {
				if i := strings.IndexByte(v, ':'); i >= 0 {
					v = strings.TrimSpace(v[:i])
				}
			}
			return v
		}
	}
	return ""
}

func isCLocale(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	base := s
	if i := strings.IndexAny(base, ".@"); i >= 0 {
		base = base[:i]
	}
	return base == "C" || base == "POSIX"
}

// languageFromLocale turns a locale name into a gettext language tag
// (ja_JP.UTF-8 → ja_JP, en-US → en_US).
func languageFromLocale(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || isCLocale(s) {
		return ""
	}
	s = strings.ReplaceAll(s, "-", "_")
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, '@'); i >= 0 {
		s = s[:i]
	}
	if isCLocale(s) {
		return ""
	}
	return s
}

var (
	libcLocalesOnce sync.Once
	libcLocalesSet  map[string]struct{}
	libcLocalesErr  error
)

func libcLocales() map[string]struct{} {
	libcLocalesOnce.Do(func() {
		libcLocalesSet = make(map[string]struct{})
		cmd := exec.Command("locale", "-a")
		// Avoid "Cannot set LC_*" warnings from locale(1) itself when the
		// process already has an unsupported LANG.
		cmd.Env = []string{
			"PATH=" + os.Getenv("PATH"),
			"LOCALE_ARCHIVE=" + os.Getenv("LOCALE_ARCHIVE"),
			"LC_ALL=C",
			"LANG=C",
		}
		out, err := cmd.Output()
		if err != nil {
			libcLocalesErr = err
			return
		}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			libcLocalesSet[normalizeLocaleName(line)] = struct{}{}
		}
	})
	return libcLocalesSet
}

func normalizeLocaleName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "-", "_")
	return strings.ToLower(s)
}

func libcSupports(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || isCLocale(name) {
		return true
	}
	avail := libcLocales()
	if libcLocalesErr != nil || len(avail) == 0 {
		// If we cannot enumerate locales, be conservative only for
		// bare language codes that commonly lack a generated locale
		// (e.g. LANG=ja). Full names like en_US.UTF-8 are left alone.
		base := languageFromLocale(name)
		return base == "" || strings.Contains(base, "_")
	}
	for _, cand := range localeMatchNames(name) {
		if _, ok := avail[cand]; ok {
			return true
		}
	}
	return false
}

func localeMatchNames(name string) []string {
	n := normalizeLocaleName(name)
	base := n
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	if i := strings.IndexByte(base, '@'); i >= 0 {
		base = base[:i]
	}
	out := []string{n, base, base + ".utf8", base + ".utf-8"}
	return out
}

func fallbackLibcLocale() string {
	avail := libcLocales()
	for _, cand := range []string{"C.UTF-8", "C.utf8", "c.utf8", "C"} {
		if _, ok := avail[normalizeLocaleName(cand)]; ok {
			// Prefer the canonical spelling when both exist.
			if cand == "c.utf8" {
				return "C.utf8"
			}
			return cand
		}
	}
	return "C"
}
