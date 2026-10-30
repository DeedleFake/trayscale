package ui

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sync"
	"time"
	"unsafe"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gdk"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/autosave"
	"deedles.dev/trayscale/internal/giofs"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/locale"
	"deedles.dev/trayscale/internal/metadata"
	"deedles.dev/trayscale/internal/tray"
	"deedles.dev/trayscale/internal/tsutil"
	"github.com/inhies/go-bytesize"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
)

//go:embed app.css
var appCSS string

// App is the main type for the app, containing all of the state
// necessary to run it.
type App struct {
	bus      *tsutil.Bus
	files    *tsutil.Files
	profiles *tsutil.Profiles
	online   bool
	hold     switchHold

	app           *adw.Application
	win           *MainWindow
	settings      *gio.Settings
	tray          *tray.Tray
	activeProfile ipn.ProfileID

	// The latest statuses, for what opens after they arrive. Only
	// update sets them.
	ipn           *tsutil.IPNStatus
	engine        *tsutil.EngineStatus
	profileStatus *tsutil.ProfileStatus

	spinnum        int
	operatorCheck  bool
	waiting        *[]apitype.WaitingFile
	autoSaving     sync.Map // waiting-file name -> struct{} while save is in flight
	autoSaveFailed sync.Map // waiting-file name -> struct{} after a failed auto-save attempt
	autoSaveDirBad string   // destination dir last logged as unusable; avoids log spam
}

func (a *App) clip(text string) {
	display := gdk.DisplayGetDefault()
	defer display.Unref()
	clipboard := display.GetClipboard()
	defer clipboard.Unref()
	clipboard.SetText(text)
}

func (a *App) notify(title, body string) {
	n := gio.NewNotification(title)
	defer n.Unref()
	n.SetBody(body)
	if icon, err := gio.IconNewForString(metadata.AppID); gutil.Err(err) == nil && icon != nil {
		n.SetIcon(icon)
		gutil.Obj(icon).Unref()
	}

	a.app.SendNotification("tailscale-status", n)
}

func (a *App) spin() {
	gutil.IdleAdd(func() {
		a.spinnum++
		if a.win != nil {
			a.win.WorkSpinner.SetVisible(a.spinnum > 0)
		}
	})
}

func (a *App) stopSpin() {
	gutil.IdleAdd(func() {
		a.spinnum--
		if a.win != nil {
			a.win.WorkSpinner.SetVisible(a.spinnum > 0)
		}
	})
}

func (a *App) showOperatorDialog() {
	Info{
		Heading: locale.Get("User is not Tailscale Operator"),
		Body:    locale.Get("Some functionality may not work as expected. To resolve, run the following command in a terminal:"),
		Extra: func() *gtk.Widget {
			const command = "sudo tailscale set --operator=$USER"
			w := gtk.NewLabel(command)
			w.AddCssClass("code")
			w.AddCssClass("monospace")
			w.AddCssClass("frame")
			w.SetSelectable(true)
			return &w.Widget
		},
	}.Show(a, nil)
}

func (a *App) update(status tsutil.Status) {
	switch status := status.(type) {
	case *tsutil.IPNStatus:
		wasHeld := a.hold.active()
		hold, start, gen := a.hold.offer(a.online, status)
		if hold {
			a.waiting = nil
			if start {
				a.setSwitching(true)
				gutil.TimeoutAdd(switchTimeout, func() {
					if s := a.hold.expire(gen); s != nil {
						a.showIPN(s)
						a.setSwitching(false)
					}
				})
			}
			return
		}
		a.showIPN(status)
		if wasHeld {
			a.setSwitching(false)
		}

	case *tsutil.EngineStatus:
		a.engine = status
		if a.win != nil {
			a.win.Update(status)
		}

	case *tsutil.FileStatus:
		enabled, dir := a.autoSaveSettings()
		autoSaveOn := autosave.Enabled(enabled, dir)

		// Auto-save handles the files right away, so there's nothing
		// to tell.
		if !autoSaveOn {
			for _, file := range status.Files {
				if slices.Contains(status.Arrived, file.Name) {
					body := fmt.Sprintf("%v (%v)", file.Name, bytesize.ByteSize(file.Size))
					a.notify(locale.Get("New Incoming File"), body)
				}
			}
		}
		a.waiting = &status.Files
		a.maybeAutoSaveFiles()

		if a.win != nil {
			a.win.Update(status)
		}

	case *tsutil.ProfileStatus:
		a.profileStatus = status
		a.activeProfile = status.Profile.ID
		if a.tray != nil {
			a.tray.SetProfiles(status)
			a.tray.SetActiveProfile(status.Profile.ID)
		}

		if a.win != nil {
			a.win.Update(status)
		}
	}
}

// switchTimeout is how long a profile switch can hold the last
// connected status before the status that tailscaled reports shows.
const switchTimeout = 5 * time.Second

// setSwitching disables the window and the tray menu while a profile
// switch is held, and enables them again after.
func (a *App) setSwitching(on bool) {
	a.tray.SetSwitching(on)
	if a.win != nil {
		a.win.SetSwitching(on)
	}
}

// showIPN shows status everywhere, notifying if that connects or
// disconnects.
func (a *App) showIPN(status *tsutil.IPNStatus) {
	a.ipn = status
	online := status.Online()
	if a.online != online {
		a.online = online

		body := locale.Get("Tailscale is not connected.")
		if online {
			body = locale.Get("Tailscale is connected.")
		}
		a.notify(locale.Get("Tailscale Status"), body) // TODO: Notify on startup if not connected?
	}

	// LookupAction ref-sinks a transfer-none result. Drop that ref.
	if action := a.app.LookupAction("use_exit_node"); action != nil {
		defer gutil.Obj(action).Unref()
		if useExitNodeAction, ok := gutil.Cast[gio.SimpleAction](action, gio.SimpleActionGLibType()); ok {
			useExitNodeAction.SetEnabled(online)
			useExitNodeAction.SetState(glib.NewVariantBoolean(status.ExitNodeActive()))
		}
	}

	if online && !a.operatorCheck {
		a.operatorCheck = true
		if !status.OperatorIsCurrent() {
			a.showOperatorDialog()
		}
	}

	if !online {
		a.waiting = nil
	}

	a.tray.Update(status)

	if a.win != nil {
		a.win.Update(status)
	}
}

func addHideWindowOption(app interface{ GoPointer() uintptr }) {
	gutil.ApplicationAddMainOption(app, "hide-window", 0, glib.GOptionFlagNoneValue, glib.GOptionArgNoneValue, locale.Get("Hide window on initial start"))
}

func (a *App) init(ctx context.Context) {
	gtk.Init()

	a.app = adw.NewApplication(metadata.AppID, gio.GApplicationHandlesOpenValue)

	css := gtk.NewCssProvider()
	css.LoadFromString(appCSS)
	display := gdk.DisplayGetDefault()
	gtk.StyleContextAddProviderForDisplay(display, css, uint32(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION))
	css.Unref()

	theme := gtk.IconThemeGetForDisplay(display) // Kept for the app's lifetime.
	display.Unref()
	gutil.Connect(theme, theme.ConnectChanged, func(gtk.IconTheme) {
		if a.win == nil {
			return
		}
		for _, page := range a.win.pages {
			if page, ok := page.(*PeerPage); ok {
				page.updateIcon()
			}
		}
	})

	var hideWindow bool
	addHideWindowOption(a.app)
	gutil.Connect(a.app, a.app.ConnectHandleLocalOptions, func(_ gio.Application, options uintptr) int32 {
		if gutil.Ptr[glib.VariantDict](options).Contains("hide-window") {
			hideWindow = true
		}

		return -1
	})

	gutil.Connect(a.app, a.app.ConnectOpen, func(_ gio.Application, files uintptr, n int32, hint string) {
		a.onAppOpen(ctx, fileArray(files, n))
	})

	gutil.Connect(a.app, a.app.ConnectStartup, func(gio.Application) {
		a.app.Hold()
	})

	gutil.Connect(a.app, a.app.ConnectActivate, func(gio.Application) {
		if hideWindow {
			hideWindow = false
			return
		}
		a.onAppActivate(ctx)
	})
}

// fileArray copies a C array of n GFile pointers, as passed to
// GApplication::open, taking a reference to each. The caller releases
// them with unrefFiles.
func fileArray(p uintptr, n int32) []*gio.FileBase {
	ptrs := unsafe.Slice(*(**uintptr)(unsafe.Pointer(&p)), n)
	files := make([]*gio.FileBase, 0, n)
	for _, ptr := range ptrs {
		f := &gio.FileBase{Ptr: ptr}
		gutil.Obj(f).Ref()
		files = append(files, f)
	}
	return files
}

func unrefFiles(files []*gio.FileBase) {
	for _, f := range files {
		gutil.Obj(f).Unref()
	}
}

// getIPN returns the latest status, or ctx's error if there's none
// before ctx is done, as when tailscaled has been down since launch.
func (a *App) getIPN(ctx context.Context) (*tsutil.IPNStatus, error) {
	return a.bus.Latest(ctx)
}

// setRunning starts Tailscale if up is true and stops it otherwise,
// offering to log in instead if starting needs it. It reports whether
// it tried. The result arrives as a status update, and errors as a
// toast. It blocks, so don't call it from the main thread.
func (a *App) setRunning(ctx context.Context, up bool) (bool, error) {
	if up {
		status, err := a.getIPN(ctx)
		if err != nil {
			return false, err
		}
		if status.NeedsAuth() {
			gutil.IdleAdd(func() {
				Confirmation{
					Heading: locale.Get("Login Required"),
					Body:    locale.Get("Open a browser to authenticate with Tailscale?"),
					Accept:  locale.Get("_Open Browser"),
					Reject:  locale.Get("_Cancel"),
				}.Show(a, func(accept bool) {
					if accept {
						a.app.ActivateAction("login", nil)
					}
				})
			})
			return false, nil
		}
	}

	err := a.bus.SetRunning(ctx, up)
	if err != nil {
		slog.Error("set Tailscale status", "err", err)
		a.toast(locale.Get("Failed to start or stop Tailscale"))
	}
	return true, nil
}

// toast shows msg in the main window, if it's open. It's safe to call
// from any thread.
func (a *App) toast(msg string) {
	gutil.IdleAdd(func() {
		if a.win != nil {
			a.win.Toast(msg)
		}
	})
}

// toggleConn starts Tailscale if it's offline and stops it otherwise.
// It doesn't block, so it's safe to call from any thread.
func (a *App) toggleConn(ctx context.Context) {
	go func() {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		err := func() error {
			status, err := a.getIPN(ctx)
			if err != nil {
				return err
			}
			_, err = a.setRunning(ctx, !status.Online())
			return err
		}()
		if err != nil {
			slog.Error("set Tailscale status from tray", "err", err)
		}
	}()
}

func (a *App) onAppOpen(ctx context.Context, files []*gio.FileBase) {
	type selectOption = SelectOption[tailcfg.NodeView]

	s := a.ipn
	if s == nil || !s.Online() {
		unrefFiles(files)
		return
	}
	options := func(yield func(selectOption) bool) {
		for _, peer := range s.Peers {
			if !s.FileTargets.Contains(peer.StableID()) || tsutil.IsMullvad(peer) || tsutil.IsShareeNode(peer) {
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
		Heading: locale.Get("Send file(s) to..."),
		Options: slices.SortedFunc(options, func(o1, o2 selectOption) int {
			return cmp.Compare(o1.Title, o2.Title)
		}),
	}.Show(a, func(options []selectOption) {
		var wg sync.WaitGroup
		for _, option := range options {
			a.notify(locale.Get("Taildrop"), locale.Get("Sending %v file(s) to %v...", len(files), option.Title))
			for _, file := range files {
				wg.Go(func() { a.pushFile(ctx, option.Value.StableID(), file) })
			}
		}
		go func() {
			wg.Wait()
			gutil.IdleAdd(func() { unrefFiles(files) })
		}()
	})
}

// addAction adds an action to the app, connecting activate if it's
// not nil, and drops the caller's reference.
func (a *App) addAction(action *gio.SimpleAction, activate func(param *glib.Variant)) {
	if activate != nil {
		gutil.Connect(action, action.ConnectActivate, func(_ gio.SimpleAction, p uintptr) {
			var param *glib.Variant
			if p != 0 {
				param = gutil.Ptr[glib.Variant](p)
			}
			activate(param)
		})
	}
	a.app.AddAction(action)
	action.Unref()
}

func (a *App) onAppActivate(ctx context.Context) {
	if a.win != nil {
		a.win.MainWindow.Present()
		return
	}

	useExitNodeAction := gio.NewSimpleActionStateful("use_exit_node", nil, glib.NewVariantBoolean(false))
	gutil.Connect(useExitNodeAction, useExitNodeAction.ConnectChangeState, func(_ gio.SimpleAction, state uintptr) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		s := gutil.Ptr[glib.Variant](state).GetBoolean()
		err := tsutil.SetUseExitNode(ctx, s, a.exitNodeSelection())
		if err != nil {
			slog.Error("failed to set exit node state", "state", s, "err", err)
			if a.win != nil {
				a.win.Toast(locale.Get("Failed to toggle exit node"))
			}
		}
	})
	useExitNodeAction.SetEnabled(false)
	a.addAction(useExitNodeAction, nil)

	if a.settings != nil {
		action := a.settings.CreateAction("show-offline-peers")
		a.app.AddAction(action)
		gutil.Obj(action).Unref()
	} else {
		showOffline := gio.NewSimpleActionStateful("show-offline-peers", nil, glib.NewVariantBoolean(true))
		gutil.Connect(showOffline, showOffline.ConnectChangeState, func(action gio.SimpleAction, state uintptr) {
			v := gutil.Ptr[glib.Variant](state)
			action.SetState(v)
			if a.win != nil {
				a.win.SetShowOffline(v.GetBoolean())
			}
		})
		a.addAction(showOffline, nil)
	}

	a.addAction(gio.NewSimpleAction("change_control_server", nil), func(*glib.Variant) { a.showChangeControlServer(ctx) })
	a.addAction(gio.NewSimpleAction("preferences", nil), func(*glib.Variant) { a.showPreferences() })
	a.addAction(gio.NewSimpleAction("admin_dashboard", nil), func(*glib.Variant) { a.launchURI(tsutil.AdminDashboardURL) })
	a.addAction(gio.NewSimpleAction("about", nil), func(*glib.Variant) { a.showAbout() })
	a.addAction(gio.NewSimpleAction("quit", nil), func(*glib.Variant) { a.Quit() })
	a.app.SetAccelsForAction("app.quit", []string{"<Ctrl>q"})

	a.addAction(gio.NewSimpleAction("search-peers", nil), func(*glib.Variant) {
		if a.win != nil {
			a.win.OpenPeerSearch()
		}
	})
	a.app.SetAccelsForAction("app.search-peers", []string{"<Ctrl>f"})

	a.addAction(gio.NewSimpleAction("login", nil), func(*glib.Variant) { a.login(ctx) })

	a.win = NewMainWindow(a)
	gutil.Connect(a.win.MainWindow, a.win.MainWindow.ConnectCloseRequest, func(gtk.Window) bool {
		win := a.win
		a.win = nil
		// The window is destroyed after this returns; drop ours once
		// the main loop is idle.
		gutil.IdleAdd(win.Widget().Unref)
		return false
	})

	// Nothing reports some changes made outside of Trayscale, such as
	// deleting a profile that isn't the current one.
	a.files.Refresh()
	a.profiles.Refresh()
	a.win.MainWindow.Present()
	if a.ipn != nil {
		a.showIPN(a.ipn)
	}
	a.win.SetSwitching(a.hold.active())
	if a.engine != nil {
		a.win.Update(a.engine)
	}
}

// login starts an interactive login and opens the login URL. It
// doesn't block, so it's safe to call from any thread.
func (a *App) login(ctx context.Context) {
	go func() {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		fail := func() {
			gutil.IdleAdd(func() {
				if a.win != nil {
					a.win.Toast(locale.Get("Failed to start login"))
				}
			})
		}

		status, err := a.getIPN(ctx)
		if err != nil {
			slog.Error("failed to start login", "err", err)
			fail()
			return
		}
		if !status.OperatorIsCurrent() {
			gutil.IdleAdd(a.showOperatorDialog)
			return
		}

		err = tsutil.StartLogin(ctx)
		if err != nil {
			slog.Error("failed to start login", "err", err)
			fail()
			return
		}

		for {
			status, err := a.bus.Next(ctx)
			if err != nil {
				fail()
				return
			}
			if status.BrowseToURL != "" {
				gutil.IdleAdd(func() { a.launchURI(status.BrowseToURL) })
				return
			}
		}
	}()
}

// launchURI opens uri with the default handler.
func (a *App) launchURI(uri string) {
	l := gtk.NewUriLauncher(uri)
	l.Launch(a.window(), nil, nil, 0)
	l.Unref()
}

func (a *App) initTray(ctx context.Context) {
	if a.tray != nil {
		a.startTray()
		return
	}

	a.tray = &tray.Tray{
		OnShow: func() {
			gutil.IdleAdd(func() {
				if a.app != nil {
					a.app.Activate()
				}
			})
		},

		OnConnToggle: func() { a.toggleConn(ctx) },

		OnExitToggle: func() {
			sel := a.exitNodeSelection()
			go func() {
				ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()

				s, err := a.getIPN(ctx)
				if err != nil {
					slog.Error("toggle exit node from tray", "err", err)
					return
				}
				toggle := !s.ExitNodeActive()
				err = tsutil.SetUseExitNode(ctx, toggle, sel)
				gutil.IdleAdd(func() {
					if err != nil {
						a.notify(locale.Get("Toggle exit node"), err.Error())
						slog.Error("toggle exit node from tray", "err", err)
						return
					}

					if toggle {
						a.notify(locale.Get("Exit node"), locale.Get("Enabled"))
						return
					}
					a.notify(locale.Get("Exit node"), locale.Get("Disabled"))
				})
			}()
		},

		OnSelfNode: func() {
			go func() {
				ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()

				s, err := a.getIPN(ctx)
				if err != nil {
					return
				}
				addr := s.SelfAddr()
				if !addr.IsValid() {
					return
				}
				gutil.IdleAdd(func() {
					a.clip(addr.String())
					if a.win != nil {
						a.notify(locale.Get("Trayscale"), locale.Get("Copied address to clipboard"))
					}
				})
			}()
		},

		OnProfileSwitch: func(id ipn.ProfileID) {
			gutil.IdleAdd(func() {
				if a.tray == nil || id == a.activeProfile {
					return
				}

				go func() {
					ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
					defer cancel()

					err := a.profiles.Switch(ctx, id)
					if err != nil {
						slog.Error("switch profile from tray", "err", err)
						gutil.IdleAdd(func() {
							a.notify(locale.Get("Switch account"), err.Error())
						})
					}
				}()
			})
		},

		OnAccountsOpen: func() { a.profiles.Refresh() },

		OnQuit: func() {
			a.Quit()
		},
	}

	a.startTray()
}

// startTray shows the tray icon with the latest statuses, if there
// are any yet, and update fills in the rest.
func (a *App) startTray() {
	err := a.tray.Start(a.profileStatus)
	if err != nil {
		slog.Error("failed to start tray icon", "err", err)
		return
	}
	if a.ipn != nil {
		a.tray.Update(a.ipn)
	}
	a.tray.SetSwitching(a.hold.active())
}

// Quit exits the app completely, causing Run to return. It may be
// called from any goroutine.
func (a *App) Quit() {
	gutil.IdleAdd(func() {
		a.tray.Close()
		a.app.Quit()
	})
}

// Run runs the app, initializing everything and then entering the
// main loop. It will return if either ctx is cancelled or Quit is
// called.
func (a *App) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.init(ctx)
	stop := context.AfterFunc(ctx, a.Quit)
	defer func() {
		// Stop before freeing the app so that the deferred cancel can't
		// quit it afterwards.
		stop()
		a.app.Unref()
	}()

	c, release := giofs.Cancellable(ctx)
	_, err := a.app.Register(c)
	release()
	if err = gutil.Err(err); err != nil {
		slog.Error("register application", "err", err)
		return
	}

	if a.app.GetIsRemote() {
		// Forward this invocation without starting another bus or tray.
		a.app.Run(int32(len(os.Args)), os.Args)
		return
	}

	a.initSettings(ctx)

	update := func(s tsutil.Status) { gutil.IdleAdd(func() { a.update(s) }) }
	a.bus = &tsutil.Bus{
		New:       func(s *tsutil.IPNStatus) { update(s) },
		NewEngine: func(s *tsutil.EngineStatus) { update(s) },
	}
	a.files = &tsutil.Files{Bus: a.bus, New: func(s *tsutil.FileStatus) { update(s) }}
	a.profiles = &tsutil.Profiles{Bus: a.bus, New: func(s *tsutil.ProfileStatus) { update(s) }}
	go a.bus.Run(ctx)
	go a.files.Run(ctx)
	go a.profiles.Run(ctx)

	a.app.Run(int32(len(os.Args)), os.Args)
}
