package locale

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// TranslationDomain is the libintl / GtkBuilder gettext domain.
// Embedded catalogs remain po/<lang>/default.po for gotext; at Init they
// are compiled to <cache>/trayscale-locale/<lang>/LC_MESSAGES/trayscale.mo.
const TranslationDomain = "trayscale"

const (
	localeCacheName    = "trayscale-locale"
	syntheticLocale    = "en_US.UTF-8"
	syntheticLocaleDir = "lib/locale"
)

// BindGettext compiles each embedded default.po into a GNU MO catalog under
// the user cache directory and binds TranslationDomain so GtkBuilder can
// translate attributes marked translatable="yes" via dgettext.
//
// Layout: <UserCacheDir>/trayscale-locale/<lang>/LC_MESSAGES/trayscale.mo
//
// When the process is stuck on a C/C.UTF-8 libc locale (typical after
// [SanitizeEnvironment] on systems without generated language locales),
// BindGettext also installs a synthetic UTF-8 locale under the cache and
// points LOCPATH at it so GNU gettext will honor LANGUAGE.
func BindGettext(poFS fs.FS) error {
	if poFS == nil {
		return nil
	}
	cacheRoot, err := localeCacheRoot()
	if err != nil {
		return err
	}
	langs, err := listPOLanguages(poFS)
	if err != nil {
		return err
	}
	for _, lang := range langs {
		data, err := fs.ReadFile(poFS, lang+"/"+domain+".po")
		if err != nil {
			return fmt.Errorf("read %s/%s.po: %w", lang, domain, err)
		}
		mo, err := compilePOtoMO(data)
		if err != nil {
			return fmt.Errorf("compile %s: %w", lang, err)
		}
		dir := filepath.Join(cacheRoot, lang, "LC_MESSAGES")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		path := filepath.Join(dir, TranslationDomain+".mo")
		if err := os.WriteFile(path, mo, 0o644); err != nil {
			return err
		}
	}
	if err := ensureGettextLocale(cacheRoot); err != nil {
		return err
	}
	bindTextDomain(TranslationDomain, cacheRoot)
	return nil
}

func localeCacheRoot() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, localeCacheName), nil
}

func listPOLanguages(poFS fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(poFS, ".")
	if err != nil {
		return nil, err
	}
	var langs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if localeAvailable(poFS, name) {
			langs = append(langs, name)
		}
	}
	return langs, nil
}

// ensureGettextLocale makes sure the active libc locale is not C/POSIX so
// that gettext consults LANGUAGE for catalog selection.
func ensureGettextLocale(cacheRoot string) error {
	cur := setLocaleAll("")
	if cur != "" && !isCLikeLocaleName(cur) {
		return nil
	}

	locRoot := filepath.Join(cacheRoot, syntheticLocaleDir)
	synPath := filepath.Join(locRoot, syntheticLocale)
	if err := installSyntheticLocale(synPath); err != nil {
		return err
	}

	if err := prependPathEnv("LOCPATH", locRoot); err != nil {
		return err
	}

	// Prefer a non-C UTF-8 locale for message lookup while LANGUAGE carries
	// the UI language chosen by SanitizeEnvironment / the user.
	_ = os.Setenv("LC_ALL", syntheticLocale)
	if v := os.Getenv("LANG"); v == "" || isCLikeLocaleName(v) {
		_ = os.Setenv("LANG", syntheticLocale)
	}

	if setLocaleAll("") == "" {
		return fmt.Errorf("setlocale(%s) failed even with synthetic LOCPATH", syntheticLocale)
	}
	return nil
}

func installSyntheticLocale(dst string) error {
	if _, err := os.Stat(filepath.Join(dst, "LC_CTYPE")); err == nil {
		return nil // already installed
	}

	src := findCUTF8LocaleDir()
	if src == "" {
		return fmt.Errorf("no C.UTF-8 locale data found to clone for gettext")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	_ = os.RemoveAll(dst)
	return copyDir(src, dst)
}

func findCUTF8LocaleDir() string {
	for _, p := range []string{
		"/usr/lib/locale/C.utf8",
		"/usr/lib/locale/C.UTF-8",
		"/usr/lib64/locale/C.utf8",
		"/usr/lib64/locale/C.UTF-8",
	} {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return ""
}

func copyDir(src, dst string) error {
	return fs.WalkDir(os.DirFS(src), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, path)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(filepath.Join(src, path))
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func prependPathEnv(key, dir string) error {
	cur := os.Getenv(key)
	if cur == "" {
		return os.Setenv(key, dir)
	}
	parts := filepath.SplitList(cur)
	for _, p := range parts {
		if p == dir {
			return nil
		}
	}
	return os.Setenv(key, dir+string(os.PathListSeparator)+cur)
}

func isCLikeLocaleName(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	s = strings.ReplaceAll(s, "-", "_")
	base := s
	if i := strings.IndexAny(base, ".@"); i >= 0 {
		base = base[:i]
	}
	return strings.EqualFold(base, "C") || strings.EqualFold(base, "POSIX")
}
