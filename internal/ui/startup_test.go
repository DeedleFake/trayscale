package ui

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"codeberg.org/puregotk/puregotk/v4/gio"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/metadata"
)

// TestSecondaryInstance checks that a second launch activates the primary
// without starting its own services. A private bus keeps the test away from
// any Trayscale instance running in the user's session.
func TestSecondaryInstance(t *testing.T) {
	mode := os.Getenv("TRAYSCALE_STARTUP_TEST")
	if mode == "" {
		if _, err := exec.LookPath("dbus-run-session"); err != nil {
			t.Skip("dbus-run-session is unavailable")
		}
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "dbus-run-session", "--", os.Args[0], "-test.run=^TestSecondaryInstance$", "-test.v")
		cmd.Env = append(os.Environ(), "TRAYSCALE_STARTUP_TEST=primary", "GSETTINGS_BACKEND=memory", "GTK_A11Y=none")
		// Desktop services may outlive the subprocess and keep a pipe open.
		log, err := os.CreateTemp(t.TempDir(), "session-*.log")
		if err != nil {
			t.Fatal(err)
		}
		defer log.Close()
		cmd.Stdout, cmd.Stderr = log, log
		err = cmd.Run()
		out, readErr := os.ReadFile(log.Name())
		if readErr != nil {
			t.Fatal(readErr)
		}
		if err != nil {
			t.Fatalf("private session: %v\n%s", err, out)
		}
		t.Log(string(out))
		return
	}
	requireGTK(t)
	if mode == "secondary" {
		// GApplication must receive application arguments, not Go test flags.
		os.Args = []string{"trayscale"}
		a := new(App)
		a.Run(t.Context())
		if a.bus != nil || a.settings != nil || a.tray != nil {
			t.Fatalf("secondary initialized primary-only services: bus=%t settings=%t tray=%t", a.bus != nil, a.settings != nil, a.tray != nil)
		}
		return
	}

	primary := gio.NewApplication(metadata.AppID, gio.GApplicationFlagsNoneValue)
	defer primary.Unref()
	var activated bool
	gutil.Connect(primary, primary.ConnectActivate, func(gio.Application) { activated = true })
	if _, err := primary.Register(nil); gutil.Err(err) != nil {
		t.Fatal(err)
	}
	if primary.GetIsRemote() {
		t.Fatal("test must own its private session bus name")
	}
	primary.Hold()
	defer primary.Release()

	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestSecondaryInstance$", "-test.v")
	cmd.Env = append(os.Environ(), "TRAYSCALE_STARTUP_TEST=secondary")
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var done atomic.Bool
	var err error
	go func() {
		err = cmd.Wait()
		done.Store(true)
	}()
	iterateUntil(t, done.Load)
	if !activated {
		t.Error("secondary instance did not activate primary")
	}
	if err != nil {
		t.Fatalf("secondary process: %v\n%s", err, &output)
	}
	t.Log(output.String())
}
