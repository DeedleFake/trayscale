# Localization

Trayscale uses [gotext](https://github.com/leonelquinteros/gotext) (GNU gettext catalogs) for runtime translation of Go-constructed UI strings. Catalogs are embedded from this directory at build time.

## Layout

```
po/<lang>/default.po
```

- `en/default.po` — English source catalog (msgid == msgstr). Copy this when starting a new language.
- `es/default.po` — Spanish
- `pt/default.po` — Portuguese
- `ja/default.po` — Japanese

Language codes follow the usual gettext forms (`es`, `en_US`, …). At startup the app picks the best match from the GTK/GLib language list (`LANG` / `LANGUAGE` / locale settings).

## Adding a language

1. Copy `po/en/default.po` to `po/<lang>/default.po` (for example `po/fr/default.po`).
2. Set the `Language:` header to your language code.
3. Fill in `msgstr` values. Leave a string as `msgstr ""` to fall back to English.
4. Rebuild: `go build -o trayscale ./cmd/trayscale` (or `go test -vet=all ./...`).
5. Run with your locale, for example `LANGUAGE=fr ./trayscale`.

Optional extraction helper (install separately):

```bash
go run github.com/leonelquinteros/gotext/cli/xgotext@latest -in . -out po/templates
```

Merge newly extracted msgids into each language file. Strings wrapped in Go look like `locale.Get("…")` / `locale.GetN(…)`.

## Locale environment notes

`LANGUAGE` is preferred for catalog selection. Bare `LANG=ja` (or any language without a *generated* system locale in `locale -a`) makes glibc/`setlocale` fail: you get `locale: Cannot set LC_* to default locale` warnings and GTK/GLib can hang on startup.

At process start Trayscale calls `locale.SanitizeEnvironment()`: if `LANG`/`LC_*` are unsupported it copies the language into `LANGUAGE` (when unset) and sets `LC_ALL` to `C.UTF-8` / `C.utf8` so libc/GTK stay happy while gotext still loads the right catalog.

Prefer `LANGUAGE=ja` (or install the system locale, e.g. `ja_JP.UTF-8`) when testing.

## What is / isn’t translated yet

**Wrapped (Go):** tray menu labels, notifications/toasts, dialog headings and buttons constructed in Go, sidebar section titles, and other `locale.Get` call sites under `internal/ui` and `internal/tray`.

**Menus from `menu.ui`:** Main and page menu labels are rewritten through `locale.Get` when the UI XML is loaded.

**Not wrapped yet:**

- Other static labels in GtkBuilder `.ui` / Cambalache layouts (`mainwindow.ui`, page `.ui` files, preferences, etc.)
- GSettings schema summaries (`dev.deedles.Trayscale.gschema.xml`)
- Desktop / AppStream metadata
- Proper nouns and dynamic Tailscale data (hostnames, IPs, region names from the control plane)

Wiring remaining `.ui` strings through GtkBuilder’s gettext domain (or the same load-time rewrite) is a natural follow-up.
