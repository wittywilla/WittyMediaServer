# Design 2 QA, pass 2d

Written 2026-11-12 by W11 (design QA). Build: `v0.8.0-alpha` at `26845e17`. Fresh data folder, scratch server on port 19810, generated media (films, episodes, an album with a timed `.lrc` for "Neon Rain", three photos). The module catalogue came from `scripts/make-test-catalog.sh` (WittyStats, WittyVisualizer, WittyPhotos, WittyInk, WittyPDF, WittyEbook, WittyComics, WittyIntegratedFileBrowser, WittyPlay). First run was walked in the browser from Welcome to Done. Earlier passes: [QA-1](DESIGN-2-QA-1.md) to [QA-4](DESIGN-2-QA-4.md).

## Rechecked and fixed

| Item | Result |
|---|---|
| N2 theme after the wizard | **Fixed.** Choosing Classic Cable and pressing Continue on Theme ends as `classicCable` with the Cable theme. |
| N8 wizard parts | **Fixed.** Account: "Name people see" and "Two-step sign-in (recommended)". Theme step: "How people use it" (look's own theme, let people import, WittyTV look). Name step: Time zone (419 zones) and Port. Language accepted as dropped. |
| D22 unnamed checkboxes | **Fixed** (the three account checkboxes now have names). |
| N4, N5 | Fixed (2c). |
| N7 phone remote keys (W13) | **Fixed**: no key under 44 px at 375x812. |
| M5 TV remote over the control row (W13) | **Fixed**: the sheet sits above the control bar (sheet bottom 900, bar top 990 at 1920x1080), no key under 66 px. One flaw: the right-hand key group is cut at the sheet's right edge (transport keys partly clipped). |
| M7 music bar over the WittyTV Dock | **Fixed**: the Dock rows sit above the bar. |
| M3 visualizer | **Mostly fixed**: all 18 effects listed with their calm or lively tag, Auto-change, Calm mode, Intensity, Speed, "Effect's own / Follows the theme". See P2. |
| M4 karaoke | **Works** with the timed `.lrc`: Follow along / Read, Behind the words (Album art / Canvas video / Visualizer), Voice on/lower/lowest, Key, Words on time. See P1. |
| M1 modules | **Mostly fixed**: navigation now lists Photos, WittyPlay Online, Statistics; Comics, Ebook, PDF, Visualizer and Stats report module settings; Photos, WittyPlay and Stats have pages. Ink's "Books" entry was not seen (no books library). WittyIntegratedFileBrowser still lists no UI. |
| Layout wireframes in the Look picker | **Good**: Settings, Appearance, Look shows a wireframe of each of the 14 looks. |
| Aero Day, Meadow, Sunset | Distinct (see P4). |

## New defects

**P1. The effects menu stays open over karaoke. Medium.** Playing a song, Visualizer, "More" opens the effects list; pressing Karaoke (from the same player) leaves that list floating over the words ("for the ... who come ashore", the first lines are hidden behind it). Rule: one Layer at a time; Back closes the layer. Owner: W10.

**P2. Visualizer mode still shows the effect only as thin bars at the sides. Medium.** With Visualizer chosen the stage shows the song title and a few tall bars at the edges, with a large empty middle; the board (`MU_lumen`, "TV visualizer") fills the stage with the effect. Owner: W10.

**P3. Karaoke controls differ from the board. Low.** Board: Vocals, Instruments, Backing vocals sliders and "Key, Mic". Here: Voice on, lower, lowest, Key, Words on time (the track is mono, so the note "This song is mono ..." shows). Owner: W10 to confirm the intended set.

**P4. Aero Sunset keeps a blue sky. Low.** Sunset colours the cards and hero peach, but the scene behind (sky and hill) is the same blue and green as Day and Meadow; the board's Sunset has an orange-pink sky. Owner: W2.

**P5. WittyPlay Online home is an empty page. Medium.** `/page/wittyplay-online` with no games library: the header and tabs (Recent, Classics, Together, Memories, Discover) and a row of round buttons, nothing else; no "Add games" prompt. A page needs a state for no games. Not walked further (focus-grow, together, room, quick menu and privacy form need a games library and emulator cores that the scratch server does not have). Owner: W10 (with the games owner).

**P6. Library Movie Channel (M6) and More info (D14) are better but still thin. Medium.** The guide row is one "We'll be right back" block for the whole day (12:00 AM to 12:00 AM). More info now shows the channel, times, "1440 min" and Watch now; no description, cast or where to watch. Possibly a result of 20-second test clips; check with real media. Owner: livetv owner, W2.

**P7. Stray controls above the sticky tab bar in Settings, Appearance. Low.** Scrolled to the Look section, "Crisp / Soft / Round" and the popover buttons "Use on this device" and "Add for everyone" show above the sticky "On this page" tabs. Owner: W2.

**P8. Photos page has no hierarchy under the Cable look. Low.** `/photos` is a title, a bar "PHOTOS" and three rows ("PIC3 2026"); no cover grid, albums, map or timeline entry. The look is Cable (rows), but even there the page reads plain. Owner: W10.

## W5's pages against their boards

Compared by eye with the board images (`GB2_1` and others): `playinfo` (matches: eyebrow, title, subtitle, grouped rows with values, chips, one primary button, readable column), `album`, `twofactor`, `tuner`, `filters`. All keep the board's structure and read as calm rows in a column of about 640 px. A weak spot: `twofactor`'s disabled Continue has low contrast. The 124 pages all open (2c); only the five above were compared one by one in 2d, not the 20 asked for, because the walk ran out of time.

## Polish: screens that still look plain

Photos (P8), WittyPlay Online (P5), the Dashboard look's Home (a Clock card that shows nothing under the title, a counter card), Statistics (dense cards of "Not available here" on a Mac), the Compact and Marquee Home pages (a list with no hierarchy beyond size). Pages that read well: the gap pages, Backup, Remote Access, the wizard Name step, the layout editor, Aero, Paper and Rail Home.

## Not walked in 2d

D12 (up-next card timing), W16's TV Dock legend (server), WittyPlay Online features beyond its empty home, Books (no books library), the Back ladder end to end, the Canvas video case (no canvas file), recordings with a real broadcast, the other 15 W5 boards one by one.
