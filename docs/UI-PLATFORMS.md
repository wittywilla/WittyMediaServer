<!-- SPDX-License-Identifier: AGPL-3.0-or-later -->
<!-- Copyright (C) 2026 WittyWilla and contributors -->
# One fix, every screen

How an interface fix reaches the web app, the desktop app, Android, iOS, tvOS, TV and Plasma Big Screen from
one place. For the client apps' maintainers as much as for this repository.

## The layers, from the most shared down

| Layer | Lives in | Reaches |
|---|---|---|
| Tokens: colour, type, space, shape, depth, motion, bar sizes | `web/src/styles.css`, written out by `scripts/design-tokens.py` to `themes/tokens.json` and `themes/*/theme.json` | every client, as data |
| Page specs: what a page contains and how it is arranged | `pkg/sdui` (the server sends them) | every client that renders SDUI |
| Shared parts: the page shell, header and hero, cards, rows, menus, dialogs, empty/loading/error states | `web/src/ui/`, `web/src/sdui/render/` | the web app and the desktop app; native clients implement the same part names |
| A page's own CSS | `web/src/**/*.css` | that page in the web only |

A fix belongs in the highest layer that can hold it. If you are writing a pixel value in a page's CSS, ask whether it
is really a token (a bar's height, a gutter, a scrim) or a part (a row, a header) that other pages and the apps
also need.

## Rules

1. **Spacing is the grid.** `--sp-1` to `--sp-16` (4px steps) and `--gutter`, `--section-gap`. Hairlines and optical
   nudges of 1 to 3px are fine. Any other pixel value for padding, margin, gap or inset is a problem.
2. **Bar offsets are tokens.** Content clears the top bar with `--topbar-h` (and the TV's `--tv-topbar-*`), the
   phone's bottom bar with `--tabbar-h` and `--tabbar-space`, the player and audio bar with `--dock-space`. Writing
   64px, 88px or 96px for "under the bar" is how content ends up beneath it on one screen and 30px too low on
   another.
3. **Loading looks like what is coming.** A loading page is rows of cards, a loading list is rows, never one
   block. Loading, empty and error use `PageSkeleton`, `EmptyState` and `LoadError`, so every page and every look
   shows them the same way, and the SDUI renderer's `PageLoading` uses the same shapes.
4. **Rows are rows.** A list of destinations (Settings, You) uses one row part: icon tile, title, one line of
   description, chevron. Anything that has no description of its own says where it comes from. Rows are the same
   height whatever supplies them.
5. **One element, one place.** A page's help link, notices and back link share the top of the page: the back link at
   the left, the help link in the corner on the same row, notices below.
6. **Touch targets are 40px or more** on a phone or tablet (`--control-h`), 24px on a pointer, with the touch size
   reached by padding, not by drawing a larger control.

## Checked automatically

- `python scripts/design-check.py` fails on raw colours, font sizes, radii, z-indexes, blur and now `spacing`
  and `bar-offset` (see the top of the script). Existing code is listed in `scripts/design-check-baseline.json`;
  the count can only go down.
- `python scripts/theme-contrast.py` checks every theme's text against its surfaces.
- `node web/scripts/ui-audit.mjs <folder>` opens every page of a running test server at each size and theme,
  takes a picture and runs detectors (sideways scroll, text under a bar, contrast, small targets, clipped text,
  overlapping buttons, broken pictures, empty pages). Pictures are named `<page>--<size>--<theme>.png`, so two runs
  (before and after a change) can be compared side by side. It needs only Node and an installed Chromium browser.

```
WMS_SERVER=http://audit.localhost:19516 WMS_PASSWORD=... node web/scripts/ui-audit.mjs out
WMS_SIZES=phone,desktop WMS_THEMES=default,light WMS_ROUTES=/,/settings node web/scripts/ui-audit.mjs out
```

`WMS_SIZES` takes `phone`, `phone360`, `tablet`, `desktop`, `wide` and `tv`. The detectors are a net, not a
verdict: look at the pictures. Wait for the machine to be quiet first; a page caught mid-fade or mid-load is
not a bug.

## For the client apps

Native clients read tokens from `themes/tokens.json` and render pages from the same SDUI specs. When a bug is
fixed in a token or in a spec, the client needs no change. When it is fixed in a shared web part, the client
should have the part of the same name (card, row, header and hero, skeleton, empty state); if it draws its own,
copy the rule above that the web fix is based on rather than the web's CSS. `docs/audits/ui-audit-0.8.1.md` lists
what has to be checked on iOS, tvOS, macOS and Android.
