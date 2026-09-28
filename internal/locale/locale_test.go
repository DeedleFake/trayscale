package locale_test

import (
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
	fsys := fstest.MapFS{
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
	}

	t.Setenv("LANGUAGE", "es_ES:es")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "C")
	locale.Init(fsys)
	require.Equal(t, "es", locale.Language())
	require.Equal(t, "Mostrar", locale.Get("Show"))
}
