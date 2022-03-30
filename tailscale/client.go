package tailscale

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

type Client struct {
	Command string
}

func (cli Client) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, cli.Command, args...)

	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return out.String(), err
}

func (cli Client) Status(ctx context.Context) (bool, error) {
	_, err := cli.run(ctx, "status")
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
