package tsutil

import (
	"context"
	"os"
	"testing"

	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
)

// These tests run against real tailscaled daemons from the
// drive-trayscale skill's tsharness. Run them with
// .agents/skills/verify-exit-node-selection/scripts/run-harness-tests,
// which starts the daemons and runs only these tests, since they
// repoint the shared LocalAPI client.

func useHarness(t *testing.T, env string) {
	sock := os.Getenv(env)
	if sock == "" {
		t.Skip(env + " not set")
	}
	// localClient keeps the transport that TestMain set, which dials
	// fakeSocket.
	old := fakeSocket
	t.Cleanup(func() { fakeSocket = old })
	fakeSocket = sock

	// Start with no exit node, nothing remembered, and nothing advertised.
	err := AdvertiseExitNode(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	// Clearing ExitNodeID through the LocalAPI also clears the prior.
	_, err = localClient.EditPrefs(t.Context(), &ipn.MaskedPrefs{
		ExitNodeIDSet:   true,
		ExitNodeIPSet:   true,
		AutoExitNodeSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func harnessPrefs(t *testing.T) *ipn.Prefs {
	t.Helper()
	prefs, err := Prefs(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return prefs
}

// harnessExitNodes returns the node that Tailscale recommends and an
// online exit node that it doesn't.
func harnessExitNodes(t *testing.T) (recommended, other tailcfg.StableNodeID) {
	t.Helper()
	suggested, err := localClient.SuggestExitNode(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	st, err := localClient.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range st.Peer {
		if p.ExitNodeOption && p.Online && p.ID != suggested.ID {
			other = p.ID
		}
	}
	if suggested.ID == "" || other == "" {
		t.Fatalf("need a recommended exit node and another online one, got %q and %q", suggested.ID, other)
	}
	return suggested.ID, other
}

func TestHarnessPreviousWithNothingToUse(t *testing.T) {
	useHarness(t, "TSHARNESS_NOSUGGEST_SOCKET")

	err := SetUseExitNode(t.Context(), true, SelectPreviousExitNode)
	if err == nil {
		t.Fatalf("turning on reported success with ExitNodeID %q", harnessPrefs(t).ExitNodeID)
	}
	if id := harnessPrefs(t).ExitNodeID; id != "" {
		t.Fatalf("failed turn on left ExitNodeID %q", id)
	}
}

func TestHarnessPreviousAfterWaitingRecommended(t *testing.T) {
	useHarness(t, "TSHARNESS_NOSUGGEST_SOCKET")

	if err := SetUseExitNode(t.Context(), true, SelectRecommendedExitNode); err != nil {
		t.Fatal(err)
	}
	if err := SetUseExitNode(t.Context(), false, SelectRecommendedExitNode); err != nil {
		t.Fatal(err)
	}

	err := SetUseExitNode(t.Context(), true, SelectPreviousExitNode)
	prefs := harnessPrefs(t)
	if err == nil {
		t.Fatalf("turning on reported success with ExitNodeID %q", prefs.ExitNodeID)
	}
	if prefs.ExitNodeID != "" || prefs.AutoExitNode.IsSet() {
		t.Fatalf("failed turn on left ExitNodeID %q, AutoExitNode %q", prefs.ExitNodeID, prefs.AutoExitNode)
	}
	if prefs.InternalExitNodePrior != "auto:any" {
		t.Fatalf("Tailscale remembers %q, not Recommended", prefs.InternalExitNodePrior)
	}
}

func TestHarnessPreviousAfterTurningOffElsewhere(t *testing.T) {
	tests := []struct {
		name string
		off  func(context.Context) error
	}{
		{"peer or Mullvad page switch", DisableExitNode},
		{"advertising this machine", func(ctx context.Context) error {
			// The self page's Advertise exit node switch.
			if err := DisableExitNode(ctx); err != nil {
				return err
			}
			if err := AdvertiseExitNode(ctx, true); err != nil {
				return err
			}
			return AdvertiseExitNode(ctx, false)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			useHarness(t, "TSHARNESS_SOCKET")
			_, node := harnessExitNodes(t)

			if err := ExitNode(t.Context(), node); err != nil {
				t.Fatal(err)
			}
			if err := test.off(t.Context()); err != nil {
				t.Fatal(err)
			}
			if id := harnessPrefs(t).ExitNodeID; id != "" {
				t.Fatalf("still using %q after turning it off", id)
			}

			if err := SetUseExitNode(t.Context(), true, SelectPreviousExitNode); err != nil {
				t.Fatal(err)
			}
			prefs := harnessPrefs(t)
			if prefs.ExitNodeID != node || prefs.AutoExitNode.IsSet() {
				t.Fatalf("Previous got ExitNodeID %q, AutoExitNode %q, want %q", prefs.ExitNodeID, prefs.AutoExitNode, node)
			}
		})
	}
}

func TestHarnessPreviousAfterRecommended(t *testing.T) {
	useHarness(t, "TSHARNESS_SOCKET")
	recommended, _ := harnessExitNodes(t)

	if err := SetUseExitNode(t.Context(), true, SelectRecommendedExitNode); err != nil {
		t.Fatal(err)
	}
	prefs := harnessPrefs(t)
	if prefs.ExitNodeID != recommended || !prefs.AutoExitNode.IsSet() {
		t.Fatalf("Recommended got ExitNodeID %q, AutoExitNode %q, want %q", prefs.ExitNodeID, prefs.AutoExitNode, recommended)
	}
	if err := DisableExitNode(t.Context()); err != nil {
		t.Fatal(err)
	}

	if err := SetUseExitNode(t.Context(), true, SelectPreviousExitNode); err != nil {
		t.Fatal(err)
	}
	prefs = harnessPrefs(t)
	if prefs.ExitNodeID != recommended || prefs.AutoExitNode.IsSet() {
		t.Fatalf("Previous got ExitNodeID %q, AutoExitNode %q, want %q", prefs.ExitNodeID, prefs.AutoExitNode, recommended)
	}
}
