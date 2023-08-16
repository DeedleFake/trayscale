package tsutil

import (
	"context"
	"fmt"
	"io"
	"net/netip"
	"os/exec"
	"strings"
	"time"

	"tailscale.com/client/tailscale"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/net/netcheck"
	"tailscale.com/tailcfg"
)

var (
	localClient    tailscale.LocalClient
	netcheckClient = netcheck.Client{
		Logf: func(format string, v ...any) {
			// Do nothing.
		},
	}

	defaultClient Client
)

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
	return st, nil
}

// Prefs returns the options of the local node.
func (c *Client) Prefs(ctx context.Context) (*ipn.Prefs, error) {
	return localClient.GetPrefs(ctx)
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
	if peer == nil {
		var prefs ipn.Prefs
		prefs.ClearExitNode()
		_, err := localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
			Prefs:         prefs,
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

	var prefs ipn.Prefs
	prefs.SetExitNodeIP(peer.TailscaleIPs[0].String(), status)
	_, err = localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:         prefs,
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
	var prefs ipn.Prefs
	prefs.SetAdvertiseExitNode(enable)

	_, err := localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:              prefs,
		AdvertiseRoutesSet: true,
	})
	if err != nil {
		return fmt.Errorf("edit prefs: %w", err)
	}

	return nil
}

func (c *Client) AdvertiseRoutes(ctx context.Context, routes []netip.Prefix) error {
	prefs, err := c.Prefs(ctx)
	if err != nil {
		return fmt.Errorf("get prefs: %w", err)
	}
	exit := prefs.AdvertisesExitNode()
	prefs.AdvertiseRoutes = routes
	prefs.SetAdvertiseExitNode(exit)

	_, err = localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:              *prefs,
		AdvertiseRoutesSet: true,
	})
	if err != nil {
		return fmt.Errorf("edit prefs: %w", err)
	}

	return nil
}

// AllowLANAccess enabled and disables the ability for the current
// node to get access to the regular LAN that it is connected to while
// an exit node is in use.
func (c *Client) AllowLANAccess(ctx context.Context, allow bool) error {
	prefs := ipn.Prefs{
		ExitNodeAllowLANAccess: allow,
	}

	_, err := localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs:                     prefs,
		ExitNodeAllowLANAccessSet: true,
	})
	if err != nil {
		return fmt.Errorf("edit prefs: %w", err)
	}

	return nil
}

func (c *Client) NetCheck(ctx context.Context, full bool) (*netcheck.Report, *tailcfg.DERPMap, error) {
	dm, err := localClient.CurrentDERPMap(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("current DERP map: %w", err)
	}

	if full {
		netcheckClient.MakeNextReportFull()
	}
	r, err := netcheckClient.GetReport(ctx, dm)
	if err != nil {
		return nil, nil, fmt.Errorf("get netcheck report: %w", err)
	}

	return r, dm, nil
}

func (c *Client) PushFile(ctx context.Context, target tailcfg.StableNodeID, size int64, name string, r io.Reader) error {
	return localClient.PushFile(ctx, target, size, name, r)
}

func (c *Client) GetWaitingFile(ctx context.Context, name string) (io.ReadCloser, int64, error) {
	return localClient.GetWaitingFile(ctx, name)
}

func (c *Client) DeleteWaitingFile(ctx context.Context, name string) error {
	return localClient.DeleteWaitingFile(ctx, name)
}

// WaitingFiles polls for any pending incoming files. It blocks for an
// extended period of time.
func (c *Client) WaitingFiles(ctx context.Context) ([]apitype.WaitingFile, error) {
	// TODO: https://github.com/tailscale/tailscale/issues/8911
	return localClient.AwaitWaitingFiles(ctx, time.Second)
}
