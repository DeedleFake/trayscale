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

## What is / isn’t translated yet

**Wrapped (Go):** tray menu labels, notifications/toasts, dialog headings and buttons constructed in Go, sidebar section titles, and other `locale.Get` call sites under `internal/ui` and `internal/tray`.

**Not wrapped yet:**

- Static labels in GtkBuilder `.ui` / Cambalache layouts (`internal/ui/*.ui`) and `menu.ui`
- GSettings schema summaries (`dev.deedles.Trayscale.gschema.xml`)
- Desktop / AppStream metadata
- Proper nouns and dynamic Tailscale data (hostnames, IPs, region names from the control plane)

Wiring `.ui` strings through GtkBuilder’s gettext domain is a natural follow-up; until then those labels stay English at design-time.
