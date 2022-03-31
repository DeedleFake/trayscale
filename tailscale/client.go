package tailscale

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

type Client struct {
	Sudo    string
	Command string
}

func (c Client) command(sudo bool, args []string) (string, []string) {
	if !sudo || (c.Sudo == "") {
		return c.Command, args
	}
	return c.Sudo, append([]string{c.Command}, args...)
}

func (c Client) run(ctx context.Context, sudo bool, args ...string) (string, error) {
	command, args := c.command(sudo, args)
	cmd := exec.CommandContext(ctx, command, args...)

	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return out.String(), err
}

func (c Client) Status(ctx context.Context) (bool, error) {
	_, err := c.run(ctx, false, "status")
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			if exit.ExitCode() == 1 {
				return false, nil
			}
		}
		return false, err
	}

	return true, nil
}

func (c Client) Start(ctx context.Context) error {
	_, err := c.run(ctx, true, "up")
	return err
}

func (c Client) Stop(ctx context.Context) error {
	_, err := c.run(ctx, true, "down")
	return err
}
