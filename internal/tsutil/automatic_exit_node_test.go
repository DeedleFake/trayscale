package tsutil

import (
	"encoding/json"
	"net/http"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
)

func TestAutomaticExitNodePreservesOtherPreferences(t *testing.T) {
	tests := []struct {
		name   string
		enable bool
		prefs  ipn.Prefs
	}{
		{name: "enable without a selected node", enable: true},
		{name: "enable with a selected node", enable: true, prefs: ipn.Prefs{ExitNodeID: "current"}},
		{name: "disable keeps the current node", prefs: ipn.Prefs{AutoExitNode: ipn.AnyExitNode, ExitNodeID: "current"}},
		{name: "disable keeps a legacy IP", prefs: ipn.Prefs{AutoExitNode: ipn.AnyExitNode, ExitNodeIP: netip.MustParseAddr("100.64.0.1")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.prefs.CorpDNS = true
			tt.prefs.RouteAll = true
			tt.prefs.WantRunning = true
			tt.prefs.ExitNodeAllowLANAccess = true
			updated := make(chan ipn.Prefs, 1)
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
				prefs := tt.prefs
				prefs.ApplyEdits(&edit)
				updated <- prefs
				json.NewEncoder(w).Encode(prefs)
			})}
			l := listenLocalAPI(t)
			go srv.Serve(l)
			defer srv.Close()

			require.NoError(t, SetAutomaticExitNode(t.Context(), tt.enable))
			want := tt.prefs
			want.AutoExitNode = ""
			if tt.enable {
				want.AutoExitNode = ipn.AnyExitNode
			}
			require.Equal(t, want, <-updated)
		})
	}
}

func TestAutomaticExitNodeFailure(t *testing.T) {
	tests := []struct {
		name   string
		enable bool
		status int
		body   string
		want   string
	}{
		{name: "denied", enable: true, status: http.StatusForbidden, body: "permission denied", want: "permission denied"},
		{name: "unsupported by daemon", enable: true, status: http.StatusOK, body: `{}`, want: "did not apply"},
		{name: "enforced by policy", status: http.StatusOK, body: `{"AutoExitNode":"any"}`, want: "did not apply"},
		{name: "missing preferences", enable: true, status: http.StatusOK, body: `null`, want: "did not apply"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPatch || r.URL.Path != "/localapi/v0/prefs" {
					http.NotFound(w, r)
					return
				}
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			})}
			l := listenLocalAPI(t)
			go srv.Serve(l)
			defer srv.Close()
			require.ErrorContains(t, SetAutomaticExitNode(t.Context(), tt.enable), tt.want)
		})
	}
}

func TestExitNodeLeavesAutomaticMode(t *testing.T) {
	for _, selected := range []string{"manual", ""} {
		t.Run("select "+selected, func(t *testing.T) {
			updated := make(chan ipn.Prefs, 1)
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
				prefs := ipn.Prefs{AutoExitNode: ipn.AnyExitNode, ExitNodeID: "automatic", ExitNodeIP: netip.MustParseAddr("100.64.0.1")}
				prefs.ApplyEdits(&edit)
				updated <- prefs
				json.NewEncoder(w).Encode(prefs)
			})}
			l := listenLocalAPI(t)
			go srv.Serve(l)
			defer srv.Close()
			require.NoError(t, ExitNode(t.Context(), tailcfg.StableNodeID(selected)))
			prefs := <-updated
			require.Empty(t, prefs.AutoExitNode)
			require.EqualValues(t, selected, prefs.ExitNodeID)
			require.False(t, prefs.ExitNodeIP.IsValid())
		})
	}
}
