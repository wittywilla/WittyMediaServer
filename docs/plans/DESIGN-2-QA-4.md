# Design 2 QA, pass 2c

Written 2026-11-12 by W11 (design QA). Build: `v0.8.0-alpha` at `95ff4701`. Fresh data folder, scratch server on port 19810, generated media only; account and libraries made through the API. Earlier passes: [QA-1](DESIGN-2-QA-1.md), [QA-2](DESIGN-2-QA-2.md), [QA-3](DESIGN-2-QA-3.md). Ids D, N and M are from those files.

## Corrections to earlier passes

- **N1 is withdrawn.** "A required look cannot be removed" was my mistake: "Publish" on Pages & menu, Looks opens a confirmation ("Publish the draft?") that my script did not press. With the confirmation, setting and clearing a required look both work (checked on `/api/v1/sdui/config/experience`). What remains (Low): Appearance says "Required by ... It can't be changed here" with no link to where it is changed (not rechecked in 2c).
- A brand contrast of 1.0 for Classic Satellite in this pass's script was a false alarm (a gradient bar the helper cannot read); the name is legible in a screenshot. Also a transient 22 px overflow on Rail was the hero video while loading; nothing overflows.

## The grid: 17 looks, Home, 1440x900

Each look set through Settings, Appearance, then Home. Checked: the look's own default theme, the server name's contrast, horizontal overflow, clipped bar items.

| Look | Theme it brought | Name contrast (Witty / Media) | Overflow or clipping |
|---|---|---|---|
| Lumen | Witty | 15.7 / 5.5 | none |
| Rail | Cinema (amber) | closed rail: mark only | none |
| Stage | Midnight | 15.9 / 7.6 | none |
| Aero | Aero Day | 9.8 / 4.8 | none |
| Classic | Witty | 14.5 / 5.1 | none |
| Classic Cable | Cable blue | 13.7 / 9.9 | none |
| Classic Satellite | Satellite | legible by eye | none |
| Compact | True black | mark only | none |
| Paper | Paper (cream) | 15.0 / 5.3 | none |
| Marquee | Cinema | 16.0 / 9.9 | none |
| Crossbar | Evergreen | 15.8 / 9.5 | none (D15 fixed) |
| Big picture | High contrast | 21 / 16.1 | none |
| Dashboard | Midnight | mark only | none |
| Retro | Teletext | 21 / 19.6 | none |

Screenshots of Rail, Paper, Aero and Retro show distinct looks wearing their own themes (amber Rail, cream Paper, sky-and-meadow Aero, blocky Retro). Not shot in this pass: Aero Night, Meadow and Sunset (three Aero themes) as separate boards; the other looks at phone and TV widths; light and dark for each.

## W5's pages

- **`/design/gaps/<key>`: all 124 keys open.** None showed an error text, a horizontal overflow or an empty page (each renders 340 to 690 characters of the canvas wording). Compared by eye with `GB2_1` to `GB2_7` for `filters`, `playinfo`; not compared one by one.
- **Real pages** (all render with a heading, no error text, no overflow): `/help`, `/pair`, `/review`, `/settings/{diagnoser, backup, repair, remote, security, about, legal, logs, transcoding, library-transcoding, modules, activity, users}`, `/collections`, `/search`. Screenshots of Backup and Remote Access wear the look and tokens correctly.
- Not done: a per-page comparison of the converted dialogs (library filters, save to a list, bulk edit, collection rules, share, creators, radio, photo albums, games letter jump) with their boards.

## Rechecks of reported fixes

| Item | Result |
|---|---|
| D2 Cable name | Fixed (see grid). |
| D3 default theme per look | **Fixed** for all 14 looks. |
| D4 Search last before You (W2) | **Fixed**: Home, Live TV, Libraries, More, Search, You. TV Dock legend still absent (W16, server). |
| D5 phone name (W13) | **Fixed**: "WittyMediaServer" is the first link in the top strip at 375x812 (two lines). |
| N7 phone remote sheet (W13) | **Mostly fixed**: solid graphite sheet, two pages (Keys, 1 2 3). 7 of 22 keys are still under 44 px. |
| M5 TV remote (W13) | **Mostly fixed**: a 920x346 sheet, 0 keys under 66 px; it covers the player's own control row while open. |
| N4 layout editor footer and Phone tab (W12) | **Fixed**: Save, Undo, Redo, Reset stay in view on every device tab; Phone shows "Menu · Bottom", TV "Top", Headset "Left". |
| N5 name editor (W12) | **Fixed**: a one-colour name that fails shows the contrast note and the button reads "Save anyway". |
| N2, N3, N8 wizard | Not re-walked in 2c (the wizard needs a fresh server run). |
| M7, M8, M6 (W2) | Not rechecked in 2c: the test channel was not added in this run. |

## New defects

None found in 2c beyond the notes above. The consent banner ("Understood") that covers the top of every page in this run appears because first run was skipped through the API; it is not a product defect.

## Still open (carried)

M1 (module repos' `sdui.json` contribute nothing yet: **module owners**, S8 and S7 are adding them), M2, M3, M4 (W10, 2d), M5 remainder (keys, covering the control row), N7 remainder (7 keys under 44 px), TV Dock legend (W16, server), M6 library channel with no programme (livetv owner), D12 up-next at about 6 s of a 20 s clip, D22 account checkboxes unnamed (not rechecked), D14 More info content (not rechecked).

## Not walked in 2c

W10's M1 to M4 and W13's round 4 (2d); a timed `.lrc` was generated beside "Neon Rain" in the scratch media for M4. The wizard run (N2, N3, N8), the Back ladder end to end, theme import, WittyTV recordings, and the Aero Night, Meadow and Sunset boards.
