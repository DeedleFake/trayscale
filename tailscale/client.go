package tailscale

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"tailscale.com/client/tailscale"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
)

var (
	// ErrNotAuthorized is returned by functions that require polkit
	// authorization but fail to get it.
	ErrNotAuthorized = errors.New("polkit: not authorized")
)

const defaultAuthAction = "com.github.DeedleFake.trayscale.run-tailscale"

var defaultAuthActionError = fmt.Sprintf("Action %v is not registered", defaultAuthAction)

// Client is a client for Tailscale's services. Some functionality is
// handled via the Go API, and some is handled via execution of the
// Tailscale CLI binary.
type Client struct {
	// Command is the command to call for the Tailscale CLI binary. It
	// defaults to "tailscale".
	Command string
}

// authorize attempts to gain authorization from polkit. It will
// attempt to get authorization first for the given action. If that
// fails, it will default to a general action that will allow
// execution of the Tailscale CLI binary.
//func (c *Client) authorize(action string) error {
//	if action == "" {
//		action = defaultAuthAction
//	}
//
//	ok, err := polkit.CheckAuthorization(
//		int32(os.Getpid()),
//		uint32(os.Getuid()),
//		action,
//		nil,
//		polkit.CheckAllowInteraction,
//	)
//	if err != nil {
//		if err.Error() == defaultAuthActionError {
//			return c.authorize("org.freedesktop.policykit.exec")
//		}
//		return fmt.Errorf("polkit: %w", err)
//	}
//	if !ok {
//		return ErrNotAuthorized
//	}
//	return nil
//}

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
	st, err := tailscale.Status(ctx)
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
	//err := c.authorize("")
	//if err != nil {
	//	return fmt.Errorf("authorize: %w", err)
	//}

	_, err := c.run(ctx, "up")
	return err
}

// Stop disconnects the local peer from the Tailscale network.
func (c *Client) Stop(ctx context.Context) error {
	//err := c.authorize("")
	//if err != nil {
	//	return fmt.Errorf("authorize: %w", err)
	//}

	_, err := c.run(ctx, "down")
	return err
}

// ExitNode uses the specified peer as an exit node, or unsets
// an existing exit node if peer is nil.
func (c *Client) ExitNode(ctx context.Context, peer *ipnstate.PeerStatus) error {
	//err := c.authorize("")
	//if err != nil {
	//	return fmt.Errorf("authorize: %w", err)
	//}

	var name string
	if peer != nil {
		name = peer.TailscaleIPs[0].String()
	}

	_, err := c.run(ctx, "up", "--exit-node", name)
	return err
}
