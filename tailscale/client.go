package tailscale

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"tailscale.com/client/tailscale"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
)

var localClient tailscale.LocalClient

// Client is a client for Tailscale's services. Some functionality is
// handled via the Go API, and some is handled via execution of the
// Tailscale CLI binary.
type Client struct {
	// Command is the command to call for the Tailscale CLI binary. It
	// defaults to "tailscale".
	Command string
}

// run runs the Tailscale CLI binary with the given arguments. It
// returns the combined stdout and stderr of the resulting process.
func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	command := "tailscale"
	if c.Command != "" {
		command = c.Command
	}
	cmd := exec.CommandContext(ctx, command, args...)

	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return out.String(), err
}

// Status returns the status of the connection to the Tailscale
// network. If the network is not currently connected, it returns
// nil, nil.
func (c *Client) Status(ctx context.Context) (*ipnstate.Status, error) {
	st, err := localClient.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("get tailscale status: %w", err)
	}
	if st.BackendState != ipn.Running.String() {
		return nil, nil
	}

	return st, nil
}

// Start connects the local peer to the Tailscale network.
func (c *Client) Start(ctx context.Context) error {
	_, err := c.run(ctx, "up")
	return err
}

// Stop disconnects the local peer from the Tailscale network.
func (c *Client) Stop(ctx context.Context) error {
	_, err := c.run(ctx, "down")
	return err
}

// ExitNode uses the specified peer as an exit node, or unsets
// an existing exit node if peer is nil.
func (c *Client) ExitNode(ctx context.Context, peer *ipnstate.PeerStatus) error {
	prefs, err := localClient.GetPrefs(ctx)
	if err != nil {
		return fmt.Errorf("get prefs: %w", err)
	}

	if peer == nil {
		prefs.ClearExitNode()
		_, err = localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
			Prefs:         *prefs,
			ExitNodeIDSet: true,
			ExitNodeIPSet: true,
		})
		if err != nil {
			return fmt.Errorf("edit prefs: %w", err)
		}
		return nil
	}

	status, err := localClient.Status(ctx)
	if err != nil {
		return fmt.Errorf("get status: %w", err)
	}

	prefs.SetExitNodeIP(peer.TailscaleIPs[0].String(), status)
	_, err = localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:         *prefs,
		ExitNodeIDSet: true,
		ExitNodeIPSet: true,
	})
	if err != nil {
		return fmt.Errorf("edit prefs: %w", err)
	}

	return nil
}

// AdvertiseExitNode enables and disables exit node advertisement for
// the current node.
func (c *Client) AdvertiseExitNode(ctx context.Context, enable bool) error {
	prefs, err := localClient.GetPrefs(ctx)
	if err != nil {
		return fmt.Errorf("get prefs: %w", err)
	}
	prefs.SetAdvertiseExitNode(enable)

	_, err = localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:              *prefs,
		AdvertiseRoutesSet: true,
	})
	if err != nil {
		return fmt.Errorf("edit prefs: %w", err)
	}

	return nil
}

// IsExitNodeAdvertised checks if the current node is advertising as
// an exit node or not.
func (c *Client) IsExitNodeAdvertised(ctx context.Context) (bool, error) {
	prefs, err := localClient.GetPrefs(ctx)
	if err != nil {
		return false, fmt.Errorf("get prefs: %w", err)
	}
	return prefs.AdvertisesExitNode(), nil
}

func (c *Client) AllowLANAccess(ctx context.Context, allow bool) error {
	prefs, err := localClient.GetPrefs(ctx)
	if err != nil {
		return fmt.Errorf("get prefs: %w", err)
	}

	prefs.ExitNodeAllowLANAccess = allow
	_, err = localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:                     *prefs,
		ExitNodeAllowLANAccessSet: true,
	})
	if err != nil {
		return fmt.Errorf("edit prefs: %w", err)
	}

	return nil
}

func (c *Client) IsLANAccessAllowed(ctx context.Context) (bool, error) {
	prefs, err := localClient.GetPrefs(ctx)
	if err != nil {
		return false, fmt.Errorf("get prefs: %w", err)
	}

	return prefs.ExitNodeAllowLANAccess, nil
}
