package locale_test

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"deedles.dev/trayscale/internal/locale"
	"github.com/stretchr/testify/require"
)

func TestBindGettextDGettext(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("LANGUAGE", "ja")
	t.Setenv("LC_ALL", "C.UTF-8")
	t.Setenv("LANG", "C.UTF-8")
	t.Setenv("LC_MESSAGES", "C.UTF-8")

	fsys := fstest.MapFS{
		"ja/default.po": &fstest.MapFile{Data: []byte(`
msgid ""
msgstr ""
"Content-Type: text/plain; charset=UTF-8\n"
"Language: ja\n"

msgid "_Quit"
msgstr "終了(_Q)"

msgid "Use _Exit Node"
msgstr "出口ノードを使用(_E)"
`)},
		"en/default.po": &fstest.MapFile{Data: []byte(`
msgid ""
msgstr ""
"Language: en\n"

msgid "_Quit"
msgstr "_Quit"
`)},
	}

	require.NoError(t, locale.BindGettext(fsys))

	moPath := filepath.Join(cache, "trayscale-locale", "ja", "LC_MESSAGES", "trayscale.mo")
	_, err := os.Stat(moPath)
	require.NoError(t, err, "expected compiled MO at %s", moPath)

	require.Equal(t, "終了(_Q)", locale.DGettextForTest(locale.TranslationDomain, "_Quit"))
	require.Equal(t, "出口ノードを使用(_E)", locale.DGettextForTest(locale.TranslationDomain, "Use _Exit Node"))
}
