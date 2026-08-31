package peersearch_test

import (
	"net/netip"
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
		if _, ok := peersearch.Score(peer, tt.query); ok != tt.want {
			t.Errorf("Score(%q) ok=%v, want %v", tt.query, ok, tt.want)
		}
	}
}

func TestFields(t *testing.T) {
	peer := testPeer(t, "workstation.example.ts.net.", "workstation", "100.64.1.2/32")
	text := strings.Join(peersearch.Fields(peer), " ")
	if _, ok := peersearch.Score(peer, "workstation"); !ok {
		t.Errorf("fields %q missing display/FQDN", text)
	}
	if _, ok := peersearch.Score(peer, "100.64.1.2"); !ok {
		t.Errorf("fields %q missing address", text)
	}
}

func TestScoreOrder(t *testing.T) {
	prefix := testPeer(t, "srv-backup.example.ts.net.", "srv-backup", "")
	weak := testPeer(t, "server.example.ts.net.", "server", "")
	sPrefix, ok1 := peersearch.Score(prefix, "srv")
	sWeak, ok2 := peersearch.Score(weak, "srv")
	if !ok1 || !ok2 {
		t.Fatalf("both peers should match srv: prefix=%v weak=%v", ok1, ok2)
	}
	if sPrefix <= sWeak {
		t.Errorf("score(srv-backup)=%d want > score(server)=%d", sPrefix, sWeak)
	}
}

func TestScoreSubsequence(t *testing.T) {
	peer := testPeer(t, "desktop.example.ts.net.", "desktop", "")
	if _, ok := peersearch.Score(peer, "dsk"); !ok {
		t.Error("dsk should match desktop")
	}
	if _, ok := peersearch.Score(peer, "td"); ok {
		t.Error("td should not match desktop")
	}
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
