#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
"""Checks, and with --fix adjusts, the built-in themes' text colours so they
meet WCAG 2.2 AA contrast (4.5:1 for normal text):

  --fg, --fg-2, --fg-3 on --bg, --bg-2 and --bg-3 (text sits on all three)
  --accent-fg on --accent (button labels)
  --accent-text, the accent as text (links, highlighted words), on all three

and Design 2's focus and ten-foot pairs (never adjusted, only reported, since they follow the
text colour; docs/THEMES.md "The TV pass"):

  --focus on all three surfaces: 4.5:1, and 3:1 against the raised fills it sits on (--bg-4)
  --tv-ring-color the same two, and --tv-text 7:1 on all three surfaces (across a room)
  --fg on --bg and the other way round: the "fill" focus style fills with --fg and sets the label in --bg

and words over a picture (the Home billboard, a title's header): behind them is a scrim in the page colour
(--bg) at --hero-scrim (web/src/styles.css .hero .info::before). Over a white and over a black picture, --fg
and --fg-2 must still read at 4.5:1. --fix raises --hero-scrim to the least that does, at most .9; a theme
that would need more has its --fg-2 (then --fg) moved just enough that .9 does, so the picture still shows.
The page as drawn, scene and glass included, is measured by web/scripts/test-contrast.mjs.

and the finish's gloss (theme.json finish.gloss, the "full" gloss paint of buttons and rows, whose label is --accent-fg):

  the label against the paint's colour at every point across the middle of the button, 30% to 70% down it
  (the paint is hi 0%, up 48%, lo 52%, glow 100%; web/src/finish.css --gloss-paint): 4.5:1 at each point. Only reported.

A failing colour is moved the shortest way that passes: lighter on dark
themes, darker on light ones, keeping its hue. Light accents (gold to teal)
on dark themes keep their colour and get dark labels instead. Run from the repository root:

  python scripts/theme-contrast.py          check: fails if any colour falls short (CI)
  python scripts/theme-contrast.py --fix    adjust, and bump each changed theme's version
"""
import colorsys, json, pathlib, re, sys

FIX = '--fix' in sys.argv
SCRIM_MAX = .9  # past this the scrim is a wall: the picture behind the words no longer shows
SCRIM_NEED = 4.6  # 4.5:1, with a little over for a picture's 8-bit pixels and the edges of the letters
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


def mix(a, b, t):
    """a laid over b at t (0 to 1), as a browser blends a gradient's colours."""
    return hexs(tuple(x * t + y * (1 - t) for x, y in zip(rgb(a), rgb(b))))


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
failed = 0
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
        # Only as strong as it has to be: a heavier outline is just noise. The high
        # contrast theme is the exception, where strong outlines are the point.
        if worst < 3 or (worst > 3.4 and f.parent.name != 'contrast'):
            start = v['bg-3'] if worst > 3.4 else v['control-line']
            new['control-line'] = move(start, surfaces, 3, lighter=dark)
            if new['control-line'] != v['control-line']:
                notes.append(f'--control-line {v["control-line"]} -> {new["control-line"]} ({min(ratio(new["control-line"], s) for s in surfaces):.2f})')
            else:
                del new['control-line']
    # Design 2: focus and the ten-foot pairs. Reported, not moved.
    final = {**new}
    for k, val in re.findall(r'--(tv-ring-color|tv-text|focus):\s*(#[0-9a-fA-F]{3,6})\b', root):
        final[k] = val
    scrim = re.search(r'--hero-scrim:\s*([\d.]+)', root)
    raised = [v['bg-4']] if 'bg-4' in v else []
    tvnotes = []
    pairs = [('focus', 4.5, surfaces), ('focus', 3, raised), ('tv-ring-color', 4.5, surfaces), ('tv-ring-color', 3, raised), ('tv-text', 7, surfaces)]
    for name, need, against in pairs:
        # The app's own default and a theme that doesn't set it follow --fg (ten feet) or --accent-text (the desk).
        col = final.get(name) or (final['fg'] if name != 'focus' else final.get('accent-text', final['accent']))
        worst = min((ratio(col, x) for x in against), default=99)
        if worst < need:
            tvnotes.append(f'--{name} {col} is {worst:.2f}:1, needs {need}:1 on {"the raised fill" if against is raised else "every surface"}')
    if ratio(final['fg'], final['bg']) < 4.5:
        tvnotes.append(f'the fill focus style (--bg label on --fg) is {ratio(final["fg"], final["bg"]):.2f}:1, needs 4.5:1')
    # Words over a picture: --fg and --fg-2 over the page colour at --hero-scrim, laid over a white and a black picture.
    s_ = float(scrim.group(1)) if scrim else None
    if s_ is not None:
        def scrim_ok(a, cols):
            return all(ratio(cols[k], mix(cols['bg'], pic, a)) >= SCRIM_NEED for k in ('fg', 'fg-2') for pic in ('#ffffff', '#000000'))
        def least(cols):
            return next((a / 100 for a in range(50, 101) if scrim_ok(a / 100, cols)), None)
        need = least(new)
        if need is None or need > SCRIM_MAX:
            for k in ('fg-2', 'fg'):
                if scrim_ok(SCRIM_MAX, new):
                    break
                # The least move of the text colour that reads over the scrim at its most, over either picture.
                against = [mix(new['bg'], pic, SCRIM_MAX) for pic in ('#ffffff', '#000000')] + surfaces
                moved = move(new[k], against, SCRIM_NEED, lighter=dark)
                if moved != new[k]:
                    notes.append(f'--{k} {new[k]} -> {moved}: reads over the hero scrim at {SCRIM_MAX}')
                    new[k] = moved
            need = least(new)
        want = max(s_, need or SCRIM_MAX)
        if need is None or not scrim_ok(min(want, 1), new):
            tvnotes.append(f'--hero-scrim: --fg and --fg-2 do not read over the scrim on a white or a black picture at any strength')
        elif s_ < need:
            notes.append(f'--hero-scrim {s_} -> {need}: --fg and --fg-2 read over it on a white and a black picture')
            new['hero-scrim'] = f'{need:g}'
    notes += tvnotes
    print(f'{f}: ' + ('; '.join(notes) if notes else 'passes'))
    if notes:
        failed += 1
    if FIX and new != v:
        changed += 1
        for k in new:
            if k == 'hero-scrim':
                css = re.sub(r'(--hero-scrim:\s*)[\d.]+', rf'\g<1>{new[k]}', css, count=1)
                continue
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
def gloss_worst(gloss, label):
    """The lowest contrast of label over the gloss paint from 30% to 70% of the way down."""
    def point(p):
        stops = [(0, gloss[0]), (48, gloss[1]), (52, gloss[2]), (100, gloss[3])]
        for (p0, c0), (p1, c1) in zip(stops, stops[1:]):
            if p0 <= p <= p1:
                t = (p - p0) / (p1 - p0)
                return hexs(tuple(a + (b - a) * t for a, b in zip(rgb(c0), rgb(c1))))
    return min(ratio(label, point(p)) for p in range(30, 71))


def derived_gloss(accent):
    """The gloss a theme without its own gets (web/src/finish.css --gloss-*): the accent, lit toward --on-image (white)
    at the top and the glow, deepened toward --picture-bg (black) below the middle."""
    return [mix(accent, '#ffffff', .55), accent, mix(accent, '#000000', .86), mix(accent, '#ffffff', .78)]


# A theme without a gloss of its own can be worn with a look that has the full gloss, so its worked-out gloss is
# checked too; --fix deepens (or, under a dark label, lightens) its accent just enough.
for meta in sorted(pathlib.Path('themes').glob('*/theme.json')):
    m = json.loads(meta.read_text(encoding='utf-8'))
    gloss = (m.get('finish') or {}).get('gloss')
    own = bool(gloss and len(gloss) >= 4)
    label = m['colors']['accentFg']
    accent = m['colors']['accent']
    worst = gloss_worst(gloss if own else derived_gloss(accent), label)
    if worst < 4.5 and FIX and not own:
        dark_label = lum(rgb(label)) < 0.2
        new = accent
        for _ in range(200):
            if gloss_worst(derived_gloss(new), label) >= 4.5:
                break
            h, li, sa = colorsys.rgb_to_hls(*rgb(new))
            new = hexs(colorsys.hls_to_rgb(h, min(1, li + 0.005) if dark_label else max(0, li - 0.005), sa))
        css_f = meta.parent / 'theme.css'
        css = css_f.read_text(encoding='utf-8')
        css = re.sub(r'(--accent:\s*)' + re.escape(accent), r'\g<1>' + new, css, count=1, flags=re.I)
        r8, g8, b8 = (round(x * 255) for x in rgb(new))
        css = re.sub(r'(--accent-rgb:\s*)[\d ]+', rf'\g<1>{r8} {g8} {b8}', css, count=1)
        css_f.write_text(css, encoding='utf-8')
        if m.get('preview', {}).get('accent', '').lower() == accent.lower():
            m['preview']['accent'] = new
            meta.write_text(json.dumps(m, indent=2, ensure_ascii=False) + '\n', encoding='utf-8')
        print(f'{meta}: gloss label: --accent {accent} -> {new} ({worst:.2f}:1 -> {gloss_worst(derived_gloss(new), label):.2f}:1)')
        changed += 1
        continue
    print(f'{meta}: {"gloss" if own else "worked-out gloss"} label ' + ('passes' if worst >= 4.5 else f'is {worst:.2f}:1 over the middle of the gloss, needs 4.5:1'))
    if worst < 4.5:
        failed += 1


# The scene (theme.json "scene"): the sky behind the page is fixed in place, so any words can come to sit on any
# part of it, and on glass that lets it show through. Every text colour (--fg, --fg-2, --fg-3, --accent-text) has
# to read on the sky's colours, along each gradient and the hill's, with the glow of light laid over them at its
# strongest (on a dark sky it is what brings the sky nearest light words). --fix moves only the sky colours
# that fall short, the shortest way: lighter on a light theme, darker on a dark one, keeping their hue.
def rgba(c):
    m = re.match(r'rgba\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*([\d.]+)\s*\)', c)
    if m:
        return hexs(tuple(int(m.group(i)) / 255 for i in (1, 2, 3))), float(m.group(4))
    return c, 1.0


def along(stops):
    """The colours along a gradient, at its stops and between them."""
    cols = [s['color'] for s in stops]
    out = []
    for a, b in zip(cols, cols[1:]):
        out += [mix(b, a, t / 8) for t in range(9)]
    return out or cols


for meta in sorted(pathlib.Path('themes').glob('*/theme.json')):
    m = json.loads(meta.read_text(encoding='utf-8'))
    layers = (m.get('scene') or {}).get('layers') or []
    painted = [l for l in layers if l.get('stops') and l['type'] in ('gradient', 'shape')]
    if not painted:
        continue
    css = (meta.parent / 'theme.css').read_text(encoding='utf-8')
    root = css[css.index(':root'):]
    v = dict(re.findall(r'--([\w-]+):\s*(#[0-9a-fA-F]{3,6})\b', root[:root.index('}')]))
    texts = {k: v[k] for k in ('fg', 'fg-2', 'fg-3', 'accent-text') if k in v}
    dark = lum(rgb(v['bg'])) < 0.2
    # The white (or any) light laid over the sky, at its strongest: the radial glow and the most solid bubble, together.
    # (The bubbles are left out: small, drifting and see-through, words pass over them; the drawn page is measured too.)
    lights = [rgba(l['color']) for l in layers if l['type'] == 'radial']
    def lit(c):
        out = [c]
        for col, a in sorted(lights, key=lambda x: -x[1])[:2]:
            out.append(mix(col, out[-1], a))
        return out
    def short(c, keys=('fg', 'fg-2', 'fg-3', 'accent-text')):
        return [(k, min(ratio(texts[k], x) for x in lit(c))) for k in keys if k in texts and min(ratio(texts[k], x) for x in lit(c)) < 4.5]
    SKY = ('fg', 'fg-2')  # the sky moves for these; the fainter text colours move for the sky
    notes = []
    moved = False
    name = lambda l: l['type'] + ('/' + l['kind'] if l.get('kind') else '')
    for l in painted:
        bad = [x for c in along(l['stops']) for x in short(c)]
        if not bad:
            continue
        k, r = min(bad, key=lambda x: x[1])
        if not FIX:
            notes.append(f'{name(l)} {"/".join(s_["color"] for s_ in l["stops"])}: --{k} reads at {r:.2f}:1 on part of it, needs 4.5:1')
            continue
        # A stop moves while it, or the gradient between it and a stop beside it, falls short.
        before = [s_['color'] for s_ in l['stops']]
        st = l['stops']
        for _ in range(200):
            # The stops that fall short themselves; failing that, the weaker end of a stretch between two that don't.
            nudge = [i for i in range(len(st)) if short(st[i]['color'], SKY)]
            if not nudge:
                worst = lambda c: min((ratio(texts[k], x) for k in SKY for x in lit(c)), default=99)
                nudge = sorted({min((i, i + 1), key=lambda n: worst(st[n]['color'])) for i in range(len(st) - 1) if any(short(c, SKY) for c in along(st[i:i + 2]))})
            if not nudge:
                break
            for i in nudge:
                h, li, sa = colorsys.rgb_to_hls(*rgb(st[i]['color']))
                st[i]['color'] = hexs(colorsys.hls_to_rgb(h, max(0, li - 0.005) if dark else min(1, li + 0.005), sa))
        if [s_['color'] for s_ in st] != before:
            notes.append(f'{name(l)} {"/".join(before)} -> {"/".join(s_["color"] for s_ in st)} (--{k} was {r:.2f}:1)')
            moved = True
    # Then the fainter text colours (hints, the accent as text) move, the shortest way, to read on the sky as it now is.
    sky = [x for l in painted for c in along(l['stops']) for x in lit(c)]
    if FIX:
        for k in ('fg-3', 'accent-text'):
            if k in texts and min(ratio(texts[k], c) for c in sky) < 4.5:
                to = move(texts[k], sky + [v['bg'], v['bg-2'], v['bg-3']], 4.5, lighter=dark)
                notes.append(f'--{k} {texts[k]} -> {to}: reads on the scene')
                css = re.sub(r'(--' + re.escape(k) + r':\s*)' + re.escape(texts[k]), rf'\g<1>{to}', css, count=1)
                texts[k] = to
                (meta.parent / 'theme.css').write_text(css, encoding='utf-8')
                moved = True
    if FIX and moved:
        m['version'] = str(int(m.get('version', '1')) + 1)
        meta.write_text(json.dumps(m, indent=2, ensure_ascii=False) + '\n', encoding='utf-8')
        print(f'{meta}: scene ' + '; '.join(notes))
        changed += 1
    else:
        print(f'{meta}: scene ' + ('passes' if not notes else '; '.join(notes)))
        if notes:
            failed += 1


if FIX:
    print(f'{changed} files changed')
elif failed:
    print(f'\n{failed} files fall short. python scripts/theme-contrast.py --fix moves each colour the shortest way that passes.')
    sys.exit(1)
