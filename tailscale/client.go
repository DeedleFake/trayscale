package tailscale

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/snapcore/snapd/polkit"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"tailscale.com/client/tailscale"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
)

var (
	ErrNotAuthorized = errors.New("polkit: not authorized")
)

const defaultAuthAction = "com.github.DeedleFake.trayscale.run-tailscale"

var defaultAuthActionError = fmt.Sprintf("Action %v is not registered", defaultAuthAction)

type Client struct {
	Command string
}

func (c *Client) authorize(action string) error {
	if action == "" {
		action = defaultAuthAction
	}

	ok, err := polkit.CheckAuthorization(
		int32(os.Getpid()),
		uint32(os.Getuid()),
		action,
		nil,
		polkit.CheckAllowInteraction,
	)
	if err != nil {
		if err.Error() == defaultAuthActionError {
			return c.authorize("org.freedesktop.policykit.exec")
		}
		return fmt.Errorf("polkit: %w", err)
	}
	if !ok {
		return ErrNotAuthorized
	}
	return nil
}

func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, c.Command, args...)

	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return out.String(), err
}

func (c *Client) Status(ctx context.Context) ([]*ipnstate.PeerStatus, error) {
	st, err := tailscale.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("get tailscale status: %w", err)
	}
	if st.BackendState != ipn.Running.String() {
		return nil, nil
	}

	peers := maps.Values(st.Peer)
	peers = append(peers, st.Self)
	peers[0], peers[len(peers)-1] = peers[len(peers)-1], peers[0]
	slices.SortFunc(peers[1:], func(p1, p2 *ipnstate.PeerStatus) bool {
		return p1.HostName < p2.HostName
	})

	return peers, nil
}

func (c *Client) Start(ctx context.Context) error {
	err := c.authorize("")
	if err != nil {
		return fmt.Errorf("authorize: %w", err)
	}

	_, err = c.run(ctx, "up")
	return err
}

func (c *Client) Stop(ctx context.Context) error {
	err := c.authorize("")
	if err != nil {
		return fmt.Errorf("authorize: %w", err)
	}

	_, err = c.run(ctx, "down")
	return err
}
