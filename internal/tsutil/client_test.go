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

			err := SetUseExitNode(t.Context(), test.use)
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
