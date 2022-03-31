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

func (cli Client) command(sudo bool, args []string) (string, []string) {
	if !sudo || (cli.Sudo == "") {
		return cli.Command, args
	}
	return cli.Sudo, append([]string{cli.Command}, args...)
}

func (cli Client) run(ctx context.Context, sudo bool, args ...string) (string, error) {
	command, args := cli.command(sudo, args)
	cmd := exec.CommandContext(ctx, command, args...)

	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return out.String(), err
}

func (cli Client) Status(ctx context.Context) (bool, error) {
	_, err := cli.run(ctx, false, "status")
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
