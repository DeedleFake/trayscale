package ui

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"time"

	"deedles.dev/trayscale/internal/metadata"
	"deedles.dev/trayscale/internal/tray"
	"deedles.dev/trayscale/internal/tsutil"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/inhies/go-bytesize"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"
)

//go:embed app.css
var appCSS string

// App is the main type for the app, containing all of the state
// necessary to run it.
type App struct {
	poller *tsutil.Poller
	online bool

	app      *adw.Application
	win      *MainWindow
	settings *gio.Settings
	tray     *tray.Tray

	spinnum       int
	operatorCheck bool
	files         *[]apitype.WaitingFile
}

func (a *App) clip(v *glib.Value) {
	gdk.DisplayGetDefault().Clipboard().Set(v)
}

func (a *App) notify(title, body string) {
	icon, iconerr := gio.NewIconForString(metadata.AppID)

	n := gio.NewNotification(title)
	n.SetBody(body)
	if iconerr == nil {
		n.SetIcon(icon)
	}

	a.app.SendNotification("tailscale-status", n)
}

func (a *App) spin() {
	glib.IdleAdd(func() {
		a.spinnum++
		if a.win != nil {
			a.win.WorkSpinner.SetVisible(a.spinnum > 0)
		}
	})
}

func (a *App) stopSpin() {
	glib.IdleAdd(func() {
		a.spinnum--
		if a.win != nil {
			a.win.WorkSpinner.SetVisible(a.spinnum > 0)
		}
	})
}

func (a *App) update(status tsutil.Status) {
	switch status := status.(type) {
	case *tsutil.IPNStatus:
		online := status.Online()
		a.tray.Update(status)
		if a.online != online {
			a.online = online

			body := "Tailscale is not connected."
			if online {
				body = "Tailscale is connected."
			}
			a.notify("Tailscale Status", body) // TODO: Notify on startup if not connected?
		}

		if online && !a.operatorCheck {
			a.operatorCheck = true
			if !status.OperatorIsCurrent() {
				Info{
					Heading: "User is not Tailscale Operator",
					Body:    "Some functionality may not work as expected. To resolve, run\n<tt>sudo tailscale set --operator=$USER</tt>\nin the command-line.",
				}.Show(a, nil)
			}
		}

		if a.win != nil {
			a.win.Update(status)
		}

	case *tsutil.FileStatus:
		if a.files != nil {
			for _, file := range status.Files {
				if !slices.Contains(*a.files, file) {
					body := fmt.Sprintf("%v (%v)", file.Name, bytesize.ByteSize(file.Size))
					a.notify("New Incoming File", body)
				}
			}
		}
		a.files = &status.Files

		if a.win != nil {
			a.win.Update(status)
		}

	case *tsutil.ProfileStatus:
		if a.win != nil {
			a.win.Update(status)
		}
	}
}

func (a *App) init(ctx context.Context) {
	gtk.Init()

	a.app = adw.NewApplication(metadata.AppID, gio.ApplicationHandlesOpen)

	css := gtk.NewCSSProvider()
	css.LoadFromString(appCSS)
	gtk.StyleContextAddProviderForDisplay(gdk.DisplayGetDefault(), css, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)

	var hideWindow bool
	a.app.AddMainOption("hide-window", 0, glib.OptionFlagNone, glib.OptionArgNone, "Hide window on initial start", "")
	a.app.ConnectHandleLocalOptions(func(options *glib.VariantDict) int {
		if options.Contains("hide-window") {
			hideWindow = true
		}

		return -1
	})

	a.app.ConnectOpen(func(files []gio.Filer, hint string) {
		a.onAppOpen(ctx, files)
	})

	a.app.ConnectStartup(func() {
		a.app.Hold()
	})

	a.app.ConnectActivate(func() {
		if hideWindow {
			hideWindow = false
			return
		}
		a.onAppActivate(ctx)
	})

	a.initSettings(ctx)
}

func (a *App) startTS(ctx context.Context) error {
	status := <-a.poller.GetIPN()
	if status.NeedsAuth() {
		Confirmation{
			Heading: "Login Required",
			Body:    "Open a browser to authenticate with Tailscale?",
			Accept:  "_Open Browser",
			Reject:  "_Cancel",
		}.Show(a, func(accept bool) {
			if accept {
				a.app.ActivateAction("login", nil)
			}
		})
		return nil
	}

	err := tsutil.Start(ctx)
	if err != nil {
		return err
	}
	<-a.poller.Poll()
	return nil
}

func (a *App) stopTS(ctx context.Context) error {
	err := tsutil.Stop(ctx)
	if err != nil {
		return err
	}
	<-a.poller.Poll()
	return nil
}

func (a *App) onAppOpen(ctx context.Context, files []gio.Filer) {
	type selectOption = SelectOption[tailcfg.NodeView]

	s := <-a.poller.GetIPN()
	if !s.Online() {
		return
	}
	options := func(yield func(selectOption) bool) {
		for _, peer := range s.Peers {
			if !s.FileTargets.Contains(peer.StableID()) || tsutil.IsMullvad(peer) {
				continue
			}

			option := selectOption{
				Title: peer.DisplayName(true),
				Value: peer,
			}
			if !yield(option) {
				return
			}
		}
	}

	Select[tailcfg.NodeView]{
		Heading: "Send file(s) to...",
		Options: slices.SortedFunc(options, func(o1, o2 selectOption) int {
			return cmp.Compare(o1.Title, o2.Title)
		}),
	}.Show(a, func(options []selectOption) {
		for _, option := range options {
			a.notify("Taildrop", fmt.Sprintf("Sending %v file(s) to %v...", len(files), option.Title))
			for _, file := range files {
				go a.pushFile(ctx, option.Value.StableID(), file)
			}
		}
	})
}

func (a *App) onAppActivate(ctx context.Context) {
	if a.win != nil {
		a.win.MainWindow.Present()
		return
	}

	changeControlServerAction := gio.NewSimpleAction("change_control_server", nil)
	changeControlServerAction.ConnectActivate(func(p *glib.Variant) { a.showChangeControlServer() })
	a.app.AddAction(changeControlServerAction)

	preferencesAction := gio.NewSimpleAction("preferences", nil)
	preferencesAction.ConnectActivate(func(p *glib.Variant) { a.showPreferences() })
	a.app.AddAction(preferencesAction)

	aboutAction := gio.NewSimpleAction("about", nil)
	aboutAction.ConnectActivate(func(p *glib.Variant) { a.showAbout() })
	a.app.AddAction(aboutAction)

	quitAction := gio.NewSimpleAction("quit", nil)
	quitAction.ConnectActivate(func(p *glib.Variant) { a.Quit() })
	a.app.AddAction(quitAction)
	a.app.SetAccelsForAction("app.quit", []string{"<Ctrl>q"})

	loginAction := gio.NewSimpleAction("login", nil)
	loginAction.ConnectActivate(func(p *glib.Variant) {
		status := <-a.poller.GetIPN()
		if !status.OperatorIsCurrent() {
			Info{
				Heading: "User is not Tailscale Operator",
				Body:    "Login via Trayscale is not possible unless the current user is set as the operator. To resolve, run\n<tt>sudo tailscale set --operator=$USER</tt>\nin the command-line.",
			}.Show(a, nil)
			return
		}

		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		err := tsutil.StartLogin(ctx)
		if err != nil {
			slog.Error("failed to start login", "err", err)
			if a.win != nil {
				a.win.Toast("Failed to start login")
			}
			return
		}

		for {
			select {
			case <-ctx.Done():
				if a.win != nil {
					a.win.Toast("Failed to start login")
				}
				return
			case status := <-a.poller.NextIPN():
				if status.BrowseToURL != "" {
					gtk.NewURILauncher(status.BrowseToURL).Launch(ctx, a.window(), nil)
					return
				}
			}
		}
	})
	a.app.AddAction(loginAction)

	a.win = NewMainWindow(a)
	a.win.MainWindow.ConnectCloseRequest(func() bool {
		a.win = nil
		return false
	})

	<-a.poller.Poll()
	a.win.MainWindow.Present()

	glib.IdleAdd(func() {
		a.update(<-a.poller.GetIPN())
	})
}

func (a *App) initTray(ctx context.Context) {
	if a.tray != nil {
		err := a.tray.Start(<-a.poller.GetIPN())
		if err != nil {
			slog.Error("failed to start tray icon", "err", err)
		}
		return
	}

	a.tray = &tray.Tray{
		OnShow: func() {
			glib.IdleAdd(func() {
				if a.app != nil {
					a.app.Activate()
				}
			})
		},

		OnConnToggle: func() {
			glib.IdleAdd(func() {
				ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()

				f := a.stopTS
				if !a.online {
					f = a.startTS
				}

				err := f(ctx)
				if err != nil {
					slog.Error("set Tailscale status from tray", "err", err)
					return
				}
			})
		},

		OnExitToggle: func() {
			glib.IdleAdd(func() {
				ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()

				s := <-a.poller.GetIPN()
				toggle := !s.ExitNodeActive()
				err := tsutil.SetUseExitNode(ctx, toggle)
				if err != nil {
					a.notify("Toggle exit node", err.Error())
					slog.Error("toggle exit node from tray", "err", err)
					return
				}

				if toggle {
					a.notify("Exit node", "Enabled")
					return
				}
				a.notify("Exit node", "Disabled")
			})
		},

		OnSelfNode: func() {
			glib.IdleAdd(func() {
				s := <-a.poller.GetIPN()
				addr := s.SelfAddr()
				if !addr.IsValid() {
					return
				}
				a.clip(glib.NewValue(addr.String()))
				if a.win != nil {
					a.notify("Trayscale", "Copied address to clipboard")
				}
			})
		},

		OnQuit: func() {
			a.Quit()
		},
	}

	err := a.tray.Start(<-a.poller.GetIPN())
	if err != nil {
		slog.Error("failed to start tray icon", "err", err)
	}
}

// Quit exits the app completely, causing Run to return.
func (a *App) Quit() {
	a.tray.Close()
	a.app.Quit()
}

// Run runs the app, initializing everything and then entering the
// main loop. It will return if either ctx is cancelled or Quit is
// called.
func (a *App) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.init(ctx)

	err := a.app.Register(ctx)
	if err != nil {
		slog.Error("register application", "err", err)
		return
	}

	a.poller = &tsutil.Poller{
		Interval: a.getInterval(),
		New:      func(s tsutil.Status) { glib.IdleAdd(func() { a.update(s) }) },
	}
	go a.poller.Run(ctx)

	go func() {
		<-ctx.Done()
		a.Quit()
	}()

	a.app.Run(os.Args)
}
