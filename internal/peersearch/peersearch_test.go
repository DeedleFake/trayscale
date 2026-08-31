package peersearch_test

import (
	"net/netip"
	"slices"
	"strings"
	"testing"

	"deedles.dev/trayscale/internal/peersearch"
	"tailscale.com/tailcfg"
)

func TestScore(t *testing.T) {
	peer := testPeer(t, "workstation.example.ts.net.", "workstation", "100.64.1.2/32")
	tests := []struct {
		query string
		want  bool
	}{
		{"", true},
		{"workstation", true},
		{"work station", true},
		{"  work   station  ", true},
		{"work missing", false},
		{"wrkstn", true},
		{"100.64", true},
		{"100.64.1.2", true},
		{"100.64 missing", false},
		{"WORK STATION", true},
		{"dsk", false},
	}
	for _, tt := range tests {
		if _, ok := score(peer, tt.query); ok != tt.want {
			t.Errorf("Score(%q) ok=%v, want %v", tt.query, ok, tt.want)
		}
	}
}

func TestFields(t *testing.T) {
	peer := testPeer(t, "workstation.example.ts.net.", "workstation", "100.64.1.2/32")
	var fields []string
	for f := range peersearch.Fields(peer) {
		fields = append(fields, f)
	}
	text := strings.Join(fields, " ")
	if _, ok := score(peer, "workstation"); !ok {
		t.Errorf("fields %q missing display/FQDN", text)
	}
	if _, ok := score(peer, "100.64.1.2"); !ok {
		t.Errorf("fields %q missing address", text)
	}
}

func TestScoreOrder(t *testing.T) {
	prefix := testPeer(t, "srv-backup.example.ts.net.", "srv-backup", "")
	weak := testPeer(t, "server.example.ts.net.", "server", "")
	sPrefix, ok1 := score(prefix, "srv")
	sWeak, ok2 := score(weak, "srv")
	if !ok1 || !ok2 {
		t.Fatalf("both peers should match srv: prefix=%v weak=%v", ok1, ok2)
	}
	if sPrefix <= sWeak {
		t.Errorf("score(srv-backup)=%d want > score(server)=%d", sPrefix, sWeak)
	}
}

func TestScoreSubsequence(t *testing.T) {
	peer := testPeer(t, "desktop.example.ts.net.", "desktop", "")
	if _, ok := score(peer, "dsk"); !ok {
		t.Error("dsk should match desktop")
	}
	if _, ok := score(peer, "td"); ok {
		t.Error("td should not match desktop")
	}
}

func TestScoreUnicode(t *testing.T) {
	peer := testPeer(t, "host.example.ts.net.", "Café", "")
	if _, ok := score(peer, "Café"); !ok {
		t.Error("Café should match hostname Café")
	}
	if _, ok := score(peer, "café"); !ok {
		t.Error("café should match hostname Café")
	}
	if _, ok := score(peer, "caf"); !ok {
		t.Error("caf should prefix-match hostname Café")
	}

	// Kelvin sign folds to k and is 3 UTF-8 bytes, so a byte-length
	// prefix of "kelvin" is not a valid slice of "Kelvin".
	peer = testPeer(t, "host.example.ts.net.", "Kelvin", "")
	if _, ok := score(peer, "kelvin"); !ok {
		t.Error("kelvin should match hostname Kelvin")
	}
	if _, ok := score(peer, "KELVIN"); !ok {
		t.Error("KELVIN should match hostname Kelvin")
	}
}

func TestScoreFields(t *testing.T) {
	fields := slices.Values([]string{"us-nyc-wg-301", "New York", "United States", "US", "NYC"})
	tests := []struct {
		query string
		want  bool
	}{
		{"", true},
		{"new york", true},
		{"NYC", true},
		{"us-nyc", true},
		{"united", true},
		{"sweden", false},
		{"new missing", false},
		{"nwrk", true},
	}
	for _, tt := range tests {
		if _, ok := peersearch.ScoreFields(fields, strings.Fields(tt.query)); ok != tt.want {
			t.Errorf("ScoreFields(%q) ok=%v, want %v", tt.query, ok, tt.want)
		}
	}
}

func score(peer tailcfg.NodeView, query string) (int, bool) {
	return peersearch.Score(peer, strings.Fields(query))
}

func testPeer(t *testing.T, name, hostname, addr string) tailcfg.NodeView {
	t.Helper()
	hi := &tailcfg.Hostinfo{Hostname: hostname}
	n := &tailcfg.Node{
		Name:     name,
		Hostinfo: hi.View(),
	}
	if addr != "" {
		n.Addresses = []netip.Prefix{netip.MustParsePrefix(addr)}
	}
	n.InitDisplayNames("example.ts.net")
	return n.View()
}
