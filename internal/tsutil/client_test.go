package tsutil

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
)

func TestSetUseExitNodeFallback(t *testing.T) {
	tests := []struct {
		name         string
		use          bool
		wantError    bool
		wantRequests []string
	}{
		{
			name:         "failed disable preserves the error",
			wantError:    true,
			wantRequests: []string{"POST /localapi/v0/set-use-exit-node-enabled?enabled=false"},
		},
		{
			name: "failed enable still selects a suggested exit node",
			use:  true,
			wantRequests: []string{
				"POST /localapi/v0/set-use-exit-node-enabled?enabled=true",
				"GET /localapi/v0/suggest-exit-node",
				"PATCH /localapi/v0/prefs",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var mu sync.Mutex
			var requests []string
			srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/localapi/v0/set-use-exit-node-enabled", "/localapi/v0/suggest-exit-node", "/localapi/v0/prefs":
					mu.Lock()
					requests = append(requests, r.Method+" "+r.URL.RequestURI())
					mu.Unlock()
				default:
					http.NotFound(w, r)
					return
				}
				switch r.URL.Path {
				case "/localapi/v0/set-use-exit-node-enabled":
					http.Error(w, "disable denied", http.StatusInternalServerError)
				case "/localapi/v0/suggest-exit-node":
					json.NewEncoder(w).Encode(apitype.ExitNodeSuggestionResponse{ID: "suggested"})
				case "/localapi/v0/prefs":
					json.NewEncoder(w).Encode(ipn.Prefs{})
				}
			})}
			l := listenLocalAPI(t)
			go srv.Serve(l)
			defer srv.Close()

			err := SetUseExitNode(t.Context(), test.use, SelectPreviousExitNode)
			if test.wantError {
				require.ErrorContains(t, err, "disable denied")
			} else {
				require.NoError(t, err)
			}
			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, test.wantRequests, requests)
		})
	}
}

func TestExitNodeClearsStaleIP(t *testing.T) {
	const selected = tailcfg.StableNodeID("selected")
	updatedPrefs := make(chan ipn.Prefs, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/localapi/v0/prefs" {
			http.NotFound(w, r)
			return
		}
		var edit ipn.MaskedPrefs
		if err := json.NewDecoder(r.Body).Decode(&edit); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		prefs := ipn.Prefs{ExitNodeIP: netip.MustParseAddr("100.64.0.1")}
		prefs.ApplyEdits(&edit)
		updatedPrefs <- prefs
		json.NewEncoder(w).Encode(prefs)
	})}
	l := listenLocalAPI(t)
	go srv.Serve(l)
	defer srv.Close()

	require.NoError(t, ExitNode(t.Context(), selected))
	select {
	case prefs := <-updatedPrefs:
		require.Equal(t, selected, prefs.ExitNodeID)
		require.False(t, prefs.ExitNodeIP.IsValid())
	default:
		t.Fatal("no exit-node preference update received")
	}
}

// fakeExitNodeDaemon serves the parts of the LocalAPI that exit node
// selection uses, keeping prefs between requests.
type fakeExitNodeDaemon struct {
	mu    sync.Mutex
	prefs ipn.Prefs
	// restore is applied when exit node use is turned on.
	restore ipn.Prefs
}

func (d *fakeExitNodeDaemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

	switch r.URL.Path {
	case "/localapi/v0/set-use-exit-node-enabled":
		if r.URL.Query().Get("enabled") == "true" {
			d.prefs.ExitNodeID = d.restore.ExitNodeID
			d.prefs.AutoExitNode = d.restore.AutoExitNode
		}
	case "/localapi/v0/suggest-exit-node":
		json.NewEncoder(w).Encode(apitype.ExitNodeSuggestionResponse{ID: "suggested"})
	case "/localapi/v0/prefs":
		if r.Method == http.MethodPatch {
			var edit ipn.MaskedPrefs
			if err := json.NewDecoder(r.Body).Decode(&edit); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			d.prefs.ApplyEdits(&edit)
			if edit.AutoExitNodeSet && edit.AutoExitNode.IsSet() && d.prefs.ExitNodeID == "" {
				d.prefs.ExitNodeID = "auto:any"
			}
		}
		json.NewEncoder(w).Encode(d.prefs)
	default:
		http.NotFound(w, r)
	}
}

func TestSetUseExitNodeSelection(t *testing.T) {
	tests := []struct {
		name     string
		sel      ExitNodeSelection
		restore  ipn.Prefs
		wantID   tailcfg.StableNodeID
		wantAuto ipn.ExitNodeExpression
	}{
		{
			name:     "recommended turns on automatic selection",
			sel:      SelectRecommendedExitNode,
			restore:  ipn.Prefs{ExitNodeID: "previous"},
			wantID:   "auto:any",
			wantAuto: ipn.AnyExitNode,
		},
		{
			name:    "previous keeps the restored node",
			sel:     SelectPreviousExitNode,
			restore: ipn.Prefs{ExitNodeID: "previous"},
			wantID:  "previous",
		},
		{
			name:    "previous leaves restored automatic selection",
			sel:     SelectPreviousExitNode,
			restore: ipn.Prefs{ExitNodeID: "resolved", AutoExitNode: ipn.AnyExitNode},
			wantID:  "resolved",
		},
		{
			name:    "previous pins a node when automatic selection is unresolved",
			sel:     SelectPreviousExitNode,
			restore: ipn.Prefs{ExitNodeID: "auto:any", AutoExitNode: ipn.AnyExitNode},
			wantID:  "suggested",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := &fakeExitNodeDaemon{restore: test.restore}
			srv := &http.Server{Handler: d}
			l := listenLocalAPI(t)
			go srv.Serve(l)
			defer srv.Close()

			require.NoError(t, SetUseExitNode(t.Context(), true, test.sel))
			d.mu.Lock()
			defer d.mu.Unlock()
			require.Equal(t, test.wantID, d.prefs.ExitNodeID)
			require.Equal(t, test.wantAuto, d.prefs.AutoExitNode)
		})
	}
}

func TestParseExitNodeSelection(t *testing.T) {
	for _, sel := range []ExitNodeSelection{SelectPreviousExitNode, SelectRecommendedExitNode} {
		require.Equal(t, sel, ParseExitNodeSelection(sel.String()))
	}
	require.Equal(t, SelectPreviousExitNode, ParseExitNodeSelection("bogus"))
}
