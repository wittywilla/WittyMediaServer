#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
"""Keeps the interface to docs/DESIGN.md. It fails on anything that bypasses the
design system, so a page can't quietly drift away from the rest:

  raw-colour      a colour written as a literal in CSS or a style={}: use a token
  font-size       a font size that isn't a token (--text-*, --page-title, --display)
  radius          a corner radius that isn't a token (--r-*)
  z-index         a stacking number over 9 that isn't a token (--z-*)
  transition-all  transition: all, which makes every change of anything animate
  blur            backdrop-filter or filter: blur on the page (repaints every frame)
  icon-label      a button holding only an icon, with no name for a screen reader
  input-label     a field with neither a label nor aria-label
  img-alt         an image with no alt (alt="" is right for decoration)
  click-div       a click handler on a div or span, which a remote can never reach
  native-dialog   confirm(), alert() or prompt(): use ask() from web/src/ui

Existing code that hasn't been moved over yet is listed, file by file, in
scripts/design-check-baseline.json. The count may go down, never up, and a file
that isn't listed must be clean. So:

  python scripts/design-check.py            check (CI)
  python scripts/design-check.py --update   after clearing things, lower the baseline
  python scripts/design-check.py --list F   show every problem in one file

A line can opt out, when there is a real reason, by saying why:
  /* design-ok: the disc's own colour */      {/* design-ok: ... */}      // design-ok: ...

In CSS, a drawing of a real object that keeps its own colours in every theme, such as
a remote control, can say so once for the whole stretch. Its colours and corners are then
its own; the other rules still apply:
  /* design-ok-begin: a drawn remote control */   ...   /* design-ok-end */
"""
import json, pathlib, re, sys

ROOT = pathlib.Path('web/src')
BASELINE = pathlib.Path('scripts/design-check-baseline.json')
# Where colours and scales are defined, and code that isn't interface: not checked.
SKIP = {'styles.css'}
# Inside :root and theme blocks of styles.css colours are the definition.
# What a drawing of an object, between design-ok-begin and design-ok-end, may have of its own.
DRAWN_OWN = {'raw-colour', 'radius'}
OK_COLOUR_WORDS = {'transparent', 'currentcolor', 'inherit'}

CSS_COLOUR = re.compile(r'#[0-9a-fA-F]{3,8}\b|\brgba?\(\s*\d|\bhsla?\(\s*\d')
CSS_WHITE = re.compile(r':\s*(?:[^;{}]*\s)?(white|black)\b(?!-)')
FONT_SIZE = re.compile(r'font-size:\s*([^;}]+)')
FONT_SHORT = re.compile(r'\bfont:\s*(\d[\d.]*(?:px|rem|em)\b[^;}]*)')
RADIUS = re.compile(r'border-(?:[a-z]+-)*radius:\s*([^;}]+)')
ZINDEX = re.compile(r'z-index:\s*(\d+)')
TSX_STYLE_COLOUR = re.compile(r'style=\{\{[^}]*(?:#[0-9a-fA-F]{3,8}\b|rgba?\(\d)')
BUTTON = re.compile(r'<button\b((?:\{(?:[^{}]|\{(?:[^{}]|\{[^{}]*\})*\})*\}|[^<>{])*?)>\s*(<Icon\w+[^>]*/>)\s*</button>')
INPUT = re.compile(r'<(input|select|textarea)\b((?:\{(?:[^{}]|\{(?:[^{}]|\{[^{}]*\})*\})*\}|[^<>{])*?)/?>')
IMG = re.compile(r'<img\b((?:\{(?:[^{}]|\{(?:[^{}]|\{[^{}]*\})*\})*\}|[^<>{])*?)/?>')
CLICK_DIV = re.compile(r'<(div|span)\b((?:\{(?:[^{}]|\{(?:[^{}]|\{[^{}]*\})*\})*\}|[^<>{])*?)\bonClick=')
DIALOG = re.compile(r'(?<![\w.])(?:window\.)?(confirm|alert|prompt)\(')


def good_size(v):
    v = v.strip().replace('!important', '').strip()
    return v.startswith('var(--') or v in ('inherit', 'initial', '0', '1em', 'unset') or re.fullmatch(r'[\d.]+em', v) is not None or v.startswith('calc(var(--') or v.startswith('clamp(')


def good_radius(v):
    v = v.strip().replace('!important', '').strip()
    return all(p == '0' or p == 'inherit' or p.endswith('%') or 'var(--' in p for p in v.split()) or 'var(--' in v


def css_problems(text):
    found = []
    text = re.sub(r'/\*(?!\s*design-ok).*?\*/', lambda m: '\n' * m.group(0).count('\n'), text, flags=re.S)
    drawn = False
    for n, line in enumerate(text.split('\n'), 1):
        if 'design-ok-begin' in line or 'design-ok-end' in line:
            drawn = 'design-ok-begin' in line
            continue
        if 'design-ok' in line:
            continue
        start = len(found)
        # A declaration of a variable is a definition, not a use.
        body = re.sub(r'--[\w-]+:\s*[^;{}]+;', '', line)
        for m in CSS_COLOUR.finditer(body):
            found.append((n, 'raw-colour', m.group(0)))
        for m in CSS_WHITE.finditer(body):
            found.append((n, 'raw-colour', m.group(1)))
        for m in FONT_SIZE.finditer(body):
            if not good_size(m.group(1)):
                found.append((n, 'font-size', m.group(1).strip()))
        for m in FONT_SHORT.finditer(body):
            found.append((n, 'font-size', m.group(1).strip()))
        for m in RADIUS.finditer(body):
            if not good_radius(m.group(1)):
                found.append((n, 'radius', m.group(1).strip()))
        for m in ZINDEX.finditer(body):
            if int(m.group(1)) > 9:
                found.append((n, 'z-index', m.group(1)))
        if re.search(r'transition:\s*all\b', body):
            found.append((n, 'transition-all', 'transition: all'))
        if re.search(r'backdrop-filter:\s*(?!none)|[^-]filter:\s*blur', body):
            found.append((n, 'blur', 'blur'))
        if drawn:
            found[start:] = [f for f in found[start:] if f[1] not in DRAWN_OWN]
    return found


def line_of(text, pos):
    return text.count('\n', 0, pos) + 1


def opted_out(text, pos):
    start = text.rfind('\n', 0, pos) + 1
    end = text.find('\n', pos)
    return 'design-ok' in text[start:end if end >= 0 else len(text)]


def tsx_problems(text):
    found = []
    for rx, kind in ((TSX_STYLE_COLOUR, 'raw-colour'),):
        for m in rx.finditer(text):
            if not opted_out(text, m.start()):
                found.append((line_of(text, m.start()), kind, m.group(0)[:50]))
    for m in BUTTON.finditer(text):
        a = m.group(1)
        if 'aria-label' not in a and 'aria-hidden' not in a and not opted_out(text, m.start()):
            found.append((line_of(text, m.start()), 'icon-label', m.group(2)))
    for m in INPUT.finditer(text):
        a = m.group(2)
        if 'type="hidden"' in a or 'type="file"' in a or 'hidden' in a.split() or 'aria-label' in a or 'aria-labelledby' in a or 'id=' in a or '{...' in a or opted_out(text, m.start()):
            continue
        # Inside a <label> or a Field or Row, which names it.
        before = text[max(0, m.start() - 400):m.start()]
        wrapped = max(before.rfind('<label'), before.rfind('<Field'), before.rfind('<Row')) > max(before.rfind('</label>'), before.rfind('</Field>'), before.rfind('</Row>'))
        if not wrapped:
            found.append((line_of(text, m.start()), 'input-label', '<' + m.group(1)))
    for m in IMG.finditer(text):
        if 'alt=' not in m.group(1) and not opted_out(text, m.start()):
            found.append((line_of(text, m.start()), 'img-alt', '<img'))
    for m in CLICK_DIV.finditer(text):
        a = m.group(2)
        if 'stopPropagation' in text[m.end():m.end() + 40] or 'role=' in a or 'modal-backdrop' in a or 'backdrop' in a or opted_out(text, m.start()):
            continue
        found.append((line_of(text, m.start()), 'click-div', '<' + m.group(1) + ' onClick'))
    for m in DIALOG.finditer(text):
        if not opted_out(text, m.start()):
            found.append((line_of(text, m.start()), 'native-dialog', m.group(1) + '()'))
    return found


def scan():
    results = {}
    for f in sorted(ROOT.rglob('*')):
        rel = f.relative_to(ROOT).as_posix()
        if f.suffix not in ('.css', '.tsx') or f.name in SKIP:
            continue
        text = f.read_text(encoding='utf-8').replace('\r\n', '\n')
        found = css_problems(text) if f.suffix == '.css' else tsx_problems(text)
        if found:
            results[rel] = found
    return results


def summary(found):
    out = {}
    for _, kind, _ in found:
        out[kind] = out.get(kind, 0) + 1
    return dict(sorted(out.items()))


def main():
    results = scan()
    if '--list' in sys.argv:
        want = sys.argv[sys.argv.index('--list') + 1].replace('\\', '/').removeprefix('web/src/')
        for n, kind, what in results.get(want, []):
            print(f'{want}:{n}: {kind}: {what}')
        return
    if '--update' in sys.argv:
        data = {k: summary(v) for k, v in sorted(results.items())}
        BASELINE.write_text(json.dumps(data, indent=1) + '\n', encoding='utf-8', newline='\n')
        total = sum(sum(v.values()) for v in data.values())
        print(f'baseline written: {total} known problems in {len(data)} files')
        return
    base = json.loads(BASELINE.read_text(encoding='utf-8')) if BASELINE.exists() else {}
    failed = False
    for rel, found in results.items():
        allowed = base.get(rel, {})
        now = summary(found)
        over = {k: n - allowed.get(k, 0) for k, n in now.items() if n > allowed.get(k, 0)}
        if over:
            failed = True
            print(f'{rel}: new design problems: ' + ', '.join(f'{k} +{n}' for k, n in over.items()))
            for n, kind, what in found:
                if kind in over:
                    print(f'  web/src/{rel}:{n}: {kind}: {what}')
    total = sum(len(v) for v in results.values())
    if failed:
        print('\nUse a token or a shared part (docs/DESIGN.md), or say why on the line with design-ok.')
        sys.exit(1)
    print(f'design check passed: {total} older problems left to clear, none new')


main()
