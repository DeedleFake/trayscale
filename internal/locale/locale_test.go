package locale_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"testing/fstest"

	"deedles.dev/trayscale/internal/locale"
	"github.com/stretchr/testify/require"
)

func TestGetFallsBackToSource(t *testing.T) {
	require.Equal(t, "Hello", locale.Get("Hello"))
	require.Equal(t, "Hello, world", locale.Get("Hello, %s", "world"))
	require.Equal(t, "1 file", locale.GetN("%d file", "%d files", 1, 1))
	require.Equal(t, "5 files", locale.GetN("%d file", "%d files", 5, 5))
}

func TestInitLanguageLoadsPO(t *testing.T) {
	fsys := fstest.MapFS{
		"es/default.po": &fstest.MapFile{Data: []byte(`
msgid ""
msgstr ""
"Content-Type: text/plain; charset=UTF-8\n"
"Language: es\n"
"Plural-Forms: nplurals=2; plural=(n != 1);\n"

msgid "Show"
msgstr "Mostrar"

msgid "Quit"
msgstr "Salir"

msgid "Sending %v file(s) to %v..."
msgstr "Enviando %v archivo(s) a %v..."
`)},
	}

	locale.InitLanguage(fsys, "es")
	require.Equal(t, "es", locale.Language())
	require.Equal(t, "Mostrar", locale.Get("Show"))
	require.Equal(t, "Salir", locale.Get("Quit"))
	require.Equal(t, "Enviando 3 archivo(s) a bob...", locale.Get("Sending %v file(s) to %v...", 3, "bob"))
	require.Equal(t, "Missing", locale.Get("Missing"))
}

func TestInitSelectsFromLANGUAGE(t *testing.T) {
	fsys := sampleCatalogFS()

	t.Setenv("LANGUAGE", "es_ES:es")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "C")
	locale.Init(fsys)
	require.Equal(t, "es", locale.Language())
	require.Equal(t, "Mostrar", locale.Get("Show"))
}

func TestInitSelectsFromLANG(t *testing.T) {
	fsys := sampleCatalogFS()

	t.Setenv("LANGUAGE", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "ja_JP.UTF-8")
	locale.Init(fsys)
	require.Equal(t, "ja", locale.Language())
	require.Equal(t, "表示", locale.Get("Show"))
}

func TestSanitizeEnvironmentPromotesUnsupportedLANG(t *testing.T) {
	if !localeCommandWorks(t) {
		t.Skip("locale -a unavailable")
	}
	if libcHas(t, "ja") {
		t.Skip("system has a ja locale; cannot reproduce missing-locale path")
	}

	t.Setenv("LANGUAGE", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "ja")

	locale.SanitizeEnvironment()

	require.Equal(t, "ja", os.Getenv("LANGUAGE"), "LANGUAGE should keep UI language")
	lcAll := os.Getenv("LC_ALL")
	require.True(t, lcAll == "C.UTF-8" || lcAll == "C.utf8" || lcAll == "C",
		"LC_ALL should be a safe libc locale, got %q", lcAll)
	lang := os.Getenv("LANG")
	require.True(t, lang == "C.UTF-8" || lang == "C.utf8" || lang == "C",
		"LANG should be rewritten away from unsupported ja, got %q", lang)

	// gotext selection must still see Japanese via LANGUAGE.
	fsys := sampleCatalogFS()
	locale.Init(fsys)
	require.Equal(t, "ja", locale.Language())
	require.Equal(t, "表示", locale.Get("Show"))
}

func TestSanitizeEnvironmentNoopWhenSupported(t *testing.T) {
	t.Setenv("LANGUAGE", "es")
	t.Setenv("LC_ALL", "C.UTF-8")
	t.Setenv("LANG", "C.UTF-8")

	locale.SanitizeEnvironment()

	require.Equal(t, "es", os.Getenv("LANGUAGE"))
	require.Equal(t, "C.UTF-8", os.Getenv("LC_ALL"))
	require.Equal(t, "C.UTF-8", os.Getenv("LANG"))
}

func sampleCatalogFS() fstest.MapFS {
	return fstest.MapFS{
		"en/default.po": &fstest.MapFile{Data: []byte(`
msgid ""
msgstr ""
"Language: en\n"

msgid "Show"
msgstr "Show"
`)},
		"es/default.po": &fstest.MapFile{Data: []byte(`
msgid ""
msgstr ""
"Language: es\n"

msgid "Show"
msgstr "Mostrar"
`)},
		"ja/default.po": &fstest.MapFile{Data: []byte(`
msgid ""
msgstr ""
"Language: ja\n"

msgid "Show"
msgstr "表示"
`)},
	}
}

func localeCommandWorks(t *testing.T) bool {
	t.Helper()
	cmd := exec.Command("locale", "-a")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LC_ALL=C", "LANG=C"}
	return cmd.Run() == nil
}

func libcHas(t *testing.T, prefix string) bool {
	t.Helper()
	cmd := exec.Command("locale", "-a")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LC_ALL=C", "LANG=C"}
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	prefix = strings.ToLower(prefix)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.ToLower(strings.TrimSpace(line))
		if line == prefix || strings.HasPrefix(line, prefix+"_") || strings.HasPrefix(line, prefix+".") {
			return true
		}
	}
	return false
}
