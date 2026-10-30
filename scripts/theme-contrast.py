#!/usr/bin/env python3
"""Checks, and with --fix adjusts, the built-in themes' text colours so they
meet WCAG 2.2 AA contrast (4.5:1 for normal text):

  --fg, --fg-2, --fg-3 on --bg, --bg-2 and --bg-3 (text sits on all three)
  --accent-fg on --accent (button labels)
  --accent-text, the accent as text (links, highlighted words), on all three

A failing colour is moved the shortest way that passes: lighter on dark
themes, darker on light ones, keeping its hue. Light accents (gold to teal)
on dark themes keep their colour and get dark labels instead. Run from the repository root:

  python scripts/theme-contrast.py          report
  python scripts/theme-contrast.py --fix    adjust, and bump each changed theme's version
"""
import colorsys, json, pathlib, re, sys

FIX = '--fix' in sys.argv
INK = '#0e1013'  # dark label colour for light accents
FILES = sorted(pathlib.Path('themes').glob('*/theme.css')) + [pathlib.Path('web/src/styles.css')]


def rgb(h):
    h = h.lstrip('#')
    if len(h) == 3:
        h = ''.join(c * 2 for c in h)
    return tuple(int(h[i:i + 2], 16) / 255 for i in (0, 2, 4))


def hexs(c):
    return '#' + ''.join(f'{round(max(0, min(1, v)) * 255):02x}' for v in c)


def lum(c):
    f = lambda v: v / 12.92 if v <= 0.03928 else ((v + 0.055) / 1.055) ** 2.4
    r, g, b = map(f, c)
    return 0.2126 * r + 0.7152 * g + 0.0722 * b


def ratio(a, b):
    la, lb = lum(rgb(a)), lum(rgb(b))
    return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)


def move(color, against, need, lighter):
    """The colour, nudged in lightness until it reaches need against every
    colour in against."""
    h, l, s = colorsys.rgb_to_hls(*rgb(color))
    for _ in range(200):
        c = hexs(colorsys.hls_to_rgb(h, l, s))
        if all(ratio(c, x) >= need for x in against):
            return c
        l = min(1, l + 0.005) if lighter else max(0, l - 0.005)
    return c


changed = 0
for f in FILES:
    css = f.read_text(encoding='utf-8')
    root = css[css.index(':root'):]
    root = root[:root.index('}')]
    v = dict(re.findall(r'--([\w-]+):\s*(#[0-9a-fA-F]{3,6})\b', root))
    if not all(k in v for k in ('bg', 'bg-2', 'bg-3', 'fg', 'fg-2', 'fg-3', 'accent', 'accent-fg')):
        print(f'{f}: skipped (not every colour is a hex value)')
        continue
    dark = lum(rgb(v['bg'])) < 0.2
    surfaces = [v['bg'], v['bg-2'], v['bg-3']]
    new = dict(v)
    notes = []
    for k in ('fg', 'fg-2', 'fg-3'):
        worst = min(ratio(v[k], s) for s in surfaces)
        if worst < 4.5:
            new[k] = move(v[k], surfaces, 4.5, lighter=dark)
            notes.append(f'--{k} {v[k]} ({worst:.2f}) -> {new[k]} ({min(ratio(new[k], s) for s in surfaces):.2f})')
    r = ratio(v['accent-fg'], v['accent'])
    if r < 4.5:
        hue = colorsys.rgb_to_hls(*rgb(v['accent']))[0] * 360
        if dark and 20 <= hue <= 200 and ratio(INK, v['accent']) >= 4.5:
            # On dark themes gold, orange, green and teal keep their colour and take dark labels.
            new['accent-fg'] = INK
            notes.append(f'--accent-fg {v["accent-fg"]} on {v["accent"]} ({r:.2f}) -> {INK} ({ratio(INK, v["accent"]):.2f})')
        else:
            # Blue, purple, pink and red deepen just enough for their light labels.
            label_dark = lum(rgb(v['accent-fg'])) < 0.2
            against = [v['accent-fg']] + ([] if dark else surfaces[:2])  # light themes: also readable as text on the page
            new['accent'] = move(v['accent'], against, 4.5, lighter=label_dark)
            notes.append(f'--accent {v["accent"]} on --accent-fg ({r:.2f}) -> {new["accent"]} ({ratio(v["accent-fg"], new["accent"]):.2f})')
    # The accent as text (links, highlighted words, the active item): its own
    # colour, readable on every surface.
    text = v.get('accent-text', new['accent'])
    if min(ratio(text, x) for x in surfaces) < 4.5:
        text = move(text, surfaces, 4.5, lighter=dark)
    if text != v.get('accent-text') and f.name != 'styles.css':  # the app's default leaves it to the theme
        new['accent-text'] = text
        notes.append(f'--accent-text {text} ({min(ratio(text, x) for x in surfaces):.2f})')
    # Status colours are also text, and the outlines of fields must be visible.
    for k in ('danger', 'ok', 'warn'):
        if k in v and min(ratio(v[k], s) for s in surfaces) < 4.5:
            new[k] = move(v[k], surfaces, 4.5, lighter=dark)
            notes.append(f'--{k} {v[k]} -> {new[k]} ({min(ratio(new[k], s) for s in surfaces):.2f})')
    if 'control-line' in v:
        worst = min(ratio(v['control-line'], s) for s in surfaces)
        # Only as strong as it has to be: a heavier outline is just noise.
        if worst < 3 or worst > 3.4:
            start = v['bg-3'] if worst > 3.4 else v['control-line']
            new['control-line'] = move(start, surfaces, 3, lighter=dark)
            if new['control-line'] != v['control-line']:
                notes.append(f'--control-line {v["control-line"]} -> {new["control-line"]} ({min(ratio(new["control-line"], s) for s in surfaces):.2f})')
            else:
                del new['control-line']
    print(f'{f}: ' + ('; '.join(notes) if notes else 'passes'))
    if FIX and new != v:
        changed += 1
        for k in new:
            if k not in v and f.name == 'styles.css':
                continue  # the app's own default: uploaded themes may not set it, so it falls back to --accent
            if k not in v:
                # A new variable: after the accent's label colour.
                line = f'--{k}: {new[k]};   /* the accent as text: links, highlighted words */'
                css = re.sub(r'(\n([ \t]*)--accent-fg:[^\n]*\n)', lambda m: m.group(1) + m.group(2) + line + '\n', css, count=1)
            elif new[k] != v[k]:
                css = re.sub(rf'(--{re.escape(k)}:\s*){re.escape(v[k])}', rf'\g<1>{new[k]}', css, count=1)
        if new['accent'] != v['accent']:
            r8, g8, b8 = (round(x * 255) for x in rgb(new['accent']))
            css = re.sub(r'(--accent-rgb:\s*)[\d ]+', rf'\g<1>{r8} {g8} {b8}', css, count=1)
        f.write_text(css, encoding='utf-8')
        meta = f.parent / 'theme.json'
        if meta.exists():
            m = json.loads(meta.read_text(encoding='utf-8'))
            m['version'] = str(int(m.get('version', '1')) + 1)
            if 'preview' in m and new['accent'] != v['accent'] and m['preview'].get('accent', '').lower() == v['accent'].lower():
                m['preview']['accent'] = new['accent']
            meta.write_text(json.dumps(m, indent=2, ensure_ascii=False) + '\n', encoding='utf-8')
if FIX:
    print(f'{changed} files changed')
