Trayscale
=========

[![Go Report Card](https://goreportcard.com/badge/deedles.dev/trayscale)](https://goreportcard.com/report/deedles.dev/trayscale)

Trayscale is an unofficial GUI wrapper around the Tailscale CLI client, particularly for use on Linux, as no official Linux GUI client exists. Despite the name, it does _not_ provide a tray icon, as support for them has been removed in Gtk4. If support can ever be brought back, however, a tray icon would be nice.

_Disclaimer: This project is in an alpha state. If it bricks your machine, it's not my fault. Use at your own risk._

![image](https://user-images.githubusercontent.com/326750/163421383-87b57d9f-7602-4112-8308-a92926b1942f.png)

Tailscale Config
----------------

Trayscale makes calls to the Tailscale CLI for some operations. In order for this to work, the `tailscale` command must be in your `$PATH`. Additionally, the daemon must have been configured with the current user as the "operator". To do this, run `sudo tailscale up --operator=$USER` from the command-line at least once manually.

Installation
------------

### AUR

If you are on an Arch Linux or a derivative, [Trayscale is available from the AUR](https://aur.archlinux.org/packages/trayscale).

### Manual

First, make sure that you have dependencies installed:

* Go >= 1.18
* GTK >= 4.0
* Libadwaita >= 1.0

The main Trayscale binary can be installed with `go install`:

```bash
$ go install deedles.dev/trayscale@latest
```

If you would like, you can also copy the `.desktop` file, the icon, and other pieces of extra metadata into the places that they need to be put to function properly:

* `dev.deedles-trayscale.desktop` -> `$HOME/.local/share/applications/`
* `com.tailscale-tailscale.png` -> `$HOME/.local/share/icons/hicolor/256x256/apps/`

Note that without copying both of these files into the correct locations, notifications will likely not function correctly in GNOME. Also keep in mind that if the `trayscale` binary is not in your `$PATH` in a way that the desktop environment can locate then the `.desktop` file will not be considered valid. If this is an issue, modify the file manually and change the `Exec=` line to point directly to the binary with an absolute path.
