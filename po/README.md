# Localization

Trayscale uses the same embedded GNU gettext catalogs under this directory for two runtimes:

- **Go strings** — [gotext](https://github.com/leonelquinteros/gotext) loads `po/<lang>/default.po` directly (`locale.Get` / `locale.GetN`).
- **GtkBuilder `.ui` labels** — at startup a small pure-Go `.po`→`.mo` compiler materializes catalogs under `$XDG_CACHE_HOME/trayscale-locale/<lang>/LC_MESSAGES/trayscale.mo` (or `~/.cache/...`), then `bindtextdomain("trayscale", …)` so GTK translates attributes marked `translatable="yes"` via `dgettext`. No host `msgfmt` is required at build or run time.

## Layout

```
po/<lang>/default.po
```

- `en/default.po` — English source catalog (msgid == msgstr). Copy this when starting a new language.
- `es/default.po` — Spanish
- `pt/default.po` — Portuguese
- `ja/default.po` — Japanese

Language codes follow the usual gettext forms (`es`, `en_US`, …). At startup the app picks the best match from `LANGUAGE` / `LC_*` / `LANG` for gotext, and the same preference list drives libintl catalog lookup after `locale.SanitizeEnvironment()`.

## Adding a language

1. Copy `po/en/default.po` to `po/<lang>/default.po` (for example `po/fr/default.po`).
2. Set the `Language:` header to your language code.
3. Fill in `msgstr` values. Leave a string as `msgstr ""` to fall back to English.
4. Rebuild: `go build -o trayscale ./cmd/trayscale` (or `go test -vet=all ./...`).
5. Run with your locale, for example `LANGUAGE=fr ./trayscale`.

Keep mnemonics in both msgid and msgstr when the UI uses them (for example `_Quit` → `終了(_Q)`), so GTK menus show the underlined accelerator.

Optional extraction helper (install separately):

```bash
go run github.com/leonelquinteros/gotext/cli/xgotext@latest -in . -out po/templates
```

Merge newly extracted msgids into each language file. Strings wrapped in Go look like `locale.Get("…")` / `locale.GetN(…)`. For Builder XML, mark label (and similar) attributes with `translatable="yes"`.

## Locale environment notes

`LANGUAGE` is preferred for catalog selection. Bare `LANG=ja` (or any language without a *generated* system locale in `locale -a`) makes glibc/`setlocale` fail: you get `locale: Cannot set LC_* to default locale` warnings and GTK/GLib can hang on startup.

At process start Trayscale calls `locale.SanitizeEnvironment()`: if `LANG`/`LC_*` are unsupported it copies the language into `LANGUAGE` (when unset) and sets `LC_ALL` to `C.UTF-8` / `C.utf8` so libc/GTK stay happy while gotext and libintl still load the right catalog via `LANGUAGE`.

Prefer `LANGUAGE=ja` (or install the system locale, e.g. `ja_JP.UTF-8`) when testing.

On hosts that only provide `C` / `C.UTF-8`, `BindGettext` clones `C.utf8` into the cache as a synthetic `en_US.UTF-8` and sets `LOCPATH` so libintl still honors `LANGUAGE` (GNU gettext ignores `LANGUAGE` in the C locale).

## What is / isn’t translated yet

**Wrapped (Go):** tray menu labels, notifications/toasts, dialog headings and buttons constructed in Go, sidebar section titles, and other `locale.Get` call sites under `internal/ui` and `internal/tray`.

**Builder `.ui` files:** Static `title` / `subtitle` / `label` / `tooltip-text` / `description` (and menu `label` attributes in `menu.ui`) marked `translatable="yes"` use GtkBuilder gettext with domain `trayscale`.

**Not wrapped yet:**

- GSettings schema summaries (`dev.deedles.Trayscale.gschema.xml`)
- Desktop / AppStream metadata
- Proper nouns and dynamic Tailscale data (hostnames, IPs, region names from the control plane)
