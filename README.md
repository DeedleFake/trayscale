Trayscale
=========

[![Go Report Card](https://goreportcard.com/badge/deedles.dev/trayscale)](https://goreportcard.com/report/deedles.dev/trayscale)

Trayscale is an unofficial GUI interface for the Tailscale daemon particularly for use on Linux, as no official Linux GUI client exists. It provides a basic system tray icon and a fairly comprehensive UI with support for many of Tailscale's features.

_Disclaimer: This project is in a beta state. There may still be undiscovered bugs or compatibility issues. Use at your own risk._

![screenshot](https://github.com/DeedleFake/trayscale/assets/326750/103125df-2e6e-48ce-9711-82e408fddc67)

Tailscale Config
----------------

Trayscale interfaces with the Tailscale daemon, `tailscaled`, to perform many of its operations. In order for this to work, the daemon must have been configured with the current user as the "operator". To do this, run `sudo tailscale set --operator=$USER` from the command-line at least once manually.

Installation
------------

<a href='https://flathub.org/apps/details/dev.deedles.Trayscale'><img width='240' alt='Download on Flathub' src='https://flathub.org/assets/badges/flathub-badge-en.svg'/></a>

Note that the above config note about the current user being designated as an "operator" still applies to the Flatpak version of this app. However, the `tailscale` binary is bundled into the Flatpak and thus does _not_ need to be in your `$PATH`.

### AUR

If you are on Arch Linux or a derivative, [Trayscale is available from the AUR](https://aur.archlinux.org/packages/trayscale).

### Manual

First, make sure that you have dependencies installed:

* Go >= 1.21
* GTK >= 4.0
* Libadwaita >= 1.4

The main Trayscale binary can be installed with `go install`:

```bash
$ go install deedles.dev/trayscale/cmd/trayscale@latest
```

If you would like, you can also copy the `.desktop` file, the icon, and other pieces of extra metadata into the places that they need to be put to function properly:

* `dev.deedles-trayscale.desktop` -> `$HOME/.local/share/applications/`
* `dev.deedles.Trayscale.png` -> `$HOME/.local/share/icons/hicolor/256x256/apps/`

Note that without copying both of these files into the correct locations, notifications will likely not function correctly in GNOME. Also keep in mind that if the `trayscale` binary is not in your `$PATH` in a way that the desktop environment can locate then the `.desktop` file will not be considered valid. If this is an issue, modify the file manually and change the `Exec=` line to point directly to the binary with an absolute path.

Donate
------

<a href="https://www.buymeacoffee.com/DeedleFake" target="_blank"><img src="https://cdn.buymeacoffee.com/buttons/v2/default-green.png" alt="Buy Me A Coffee" style="height: 60px !important;width: 217px !important;" ></a>
