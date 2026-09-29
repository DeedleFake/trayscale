#!/usr/bin/env python3
"""Mark GtkBuilder .ui user-visible properties as translatable and list msgids.

Idempotent. Marks property names title/subtitle/label/tooltip-text when the
text content is non-empty after trim and translatable= is not already present.
Also extracts sorted unique msgids from ALL internal/ui/*.ui files that have
translatable="yes" (properties and attributes).
"""
from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
UI_DIR = ROOT / "internal" / "ui"

MARK_FILES = [
    "mainwindow.ui",
    "preferences.ui",
    "peerpage.ui",
    "selfpage.ui",
    "mullvadpage.ui",
    "offlinepage.ui",
]

PROP_NAMES = ("title", "subtitle", "label", "tooltip-text")
_names = "|".join(re.escape(n) for n in PROP_NAMES)
# Match <property name="TITLE"...>TEXT</property>, capturing head/attrs/text.
PROP_RE = re.compile(
    r'(<property\s+name="(?:' + _names + r')")'
    r'((?:\s+\w[\w:-]*="[^"]*")*)'
    r'>'
    r'([^<]*)'
    r'</property>'
)

EXTRACT_RE = re.compile(
    r'<(?:property|attribute)\s+[^>]*\btranslatable="yes"[^>]*>([^<]*)</(?:property|attribute)>'
)


def should_mark(attrs: str, text: str) -> bool:
    if 'translatable="' in attrs:
        return False
    if not text.strip():
        return False
    return True


def mark_file(path: Path) -> int:
    src = path.read_text(encoding="utf-8")
    marked = 0

    def repl(m: re.Match[str]) -> str:
        nonlocal marked
        head, attrs, text = m.group(1), m.group(2), m.group(3)
        if not should_mark(attrs, text):
            return m.group(0)
        marked += 1
        return f'{head}{attrs} translatable="yes">{text}</property>'

    out = PROP_RE.sub(repl, src)
    if out != src:
        path.write_text(out, encoding="utf-8")
    return marked


def extract_msgids() -> list[str]:
    found: set[str] = set()
    for path in sorted(UI_DIR.glob("*.ui")):
        text = path.read_text(encoding="utf-8")
        for m in EXTRACT_RE.finditer(text):
            msgid = m.group(1)
            if msgid.strip():
                found.add(msgid)
    return sorted(found, key=lambda s: s.lower())


def main() -> int:
    total = 0
    for name in MARK_FILES:
        path = UI_DIR / name
        if not path.is_file():
            print(f"missing: {path}", file=sys.stderr)
            return 1
        n = mark_file(path)
        print(f"marked {n} properties in {name}")
        total += n
    print(f"total newly marked: {total}")
    print("--- msgids ---")
    for msgid in extract_msgids():
        print(msgid)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
