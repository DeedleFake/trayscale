package locale_test

import (
	"testing"

	"deedles.dev/trayscale/internal/locale"
	"github.com/leonelquinteros/gotext"
	"github.com/stretchr/testify/require"
)

func TestCompilePOtoMORoundTrip(t *testing.T) {
	po := []byte(`
msgid ""
msgstr ""
"Content-Type: text/plain; charset=UTF-8\n"
"Language: ja\n"
"Plural-Forms: nplurals=1; plural=0;\n"

msgid "_Quit"
msgstr "終了(_Q)"

msgid "Show _Offline Peers"
msgstr "オフラインのピアを表示(_O)"

msgid "%d file"
msgid_plural "%d files"
msgstr[0] "%d ファイル"
`)

	mo, err := locale.CompilePOtoMOForTest(po)
	require.NoError(t, err)
	require.Greater(t, len(mo), 28)

	parsed := gotext.NewMo()
	parsed.Parse(mo)
	require.Equal(t, "終了(_Q)", parsed.Get("_Quit"))
	require.Equal(t, "オフラインのピアを表示(_O)", parsed.Get("Show _Offline Peers"))
	require.Equal(t, "3 ファイル", parsed.GetN("%d file", "%d files", 3, 3))
	require.Equal(t, "Missing", parsed.Get("Missing"))
}
