# Design 2 QA, pass 2b

Written 2026-11-12 by W11 (design QA). Build: `v0.8.0-alpha` at `39cbabbd` (W13 finishes, W10 music and modules, W5 round 2, W12 editors merged). Scratch server on port 19810, fresh data folder, generated media only; the account and libraries were made through the API (first run was walked in [pass 2a](DESIGN-2-QA-2.md)). Modules came from `scripts/make-test-catalog.sh` with WittyStats, WittyVisualizer, WittyPhotos, WittyInk, WittyPDF, WittyEbook, WittyComics, WittyIntegratedFileBrowser, WittyDVD and WittyBluray (all API 4, all installed). Ids D1 to D27 are from [pass 1](DESIGN-2-QA-1.md), N1 to N9 from pass 2a, M1 and up are new.

## Rechecked

| Id | Result |
|---|---|
| D2 Cable look hides the name | **Fixed.** Classic Cable with the Cable theme: "Witty" 13.7, "Media" 9.9 contrast. |
| D3 UI default theme | **Fixed in the Look picker**: Classic Cable gives the Cable theme, Lumen the Witty one. |
| D4 TV mode has no name | **Name fixed** (1920x1080, `?tv=1`: "WittyMediaServer" in the bar, Statistics in the pills). Still open: Search is second, not last before You; no Dock or key legend. |
| D5 phone has no name | **Open.** At 375x812 the name is drawn, but the top strip is scrolled so "Media" sits at x = -134: the name and Home are off screen; the first thing seen is "Movies Music TV Shows". |
| D6 music views | **Mostly fixed.** The full player has Album art / Canvas video / Visualizer, Bars, Waves, Drift, More, Slow, Styles, Lyrics, Karaoke, Read, speed, a key legend with V for the visualizer. See M3 and M4. |
| D13 WittyTV look | **Fixed** for Lumen (Modern, dark grey) and Rail (Classic). |
| D14 More info content | Not rechecked (the channel is still "We'll be right back, 12:00 AM to 12:00 AM", see M6). |
| D15 Crossbar overflow | **Fixed**: nothing clipped at 1440 (page 1430 wide). |
| D10, N7 remote | Desktop fine (2a). **Phone** two-page sheet translucent (N7 open). **TV width**: see M5. |
| D12 up-next card early | Open (2a). |

## New defects

**M1. Eight of the ten installed modules add no pages, navigation or settings. High.** `GET /api/modules`, `uiAdds`: WittyInk (`wittyink-library`, `wittyink-books`) and WittyPhotos (`wittyphotos-library`, `wittyphotos`) list pages; WittyStats, WittyVisualizer, WittyPDF, WittyEbook, WittyComics, WittyIntegratedFileBrowser, WittyDVD and WittyBluray list `pages: [], navigation: [], settings: false`. ORCHESTRATION says every module ships its own SDUI and the server refuses an API 4 module without it. Statistics does appear in the More menu (navigation), so it comes from somewhere else; the module's own pages are not exposed. Rule: principle 15, `ModuleMatrix`, `M_lumen`, `M_aero`, `M_classicCable`. Owner: W10 (with W1 for the module check).

**M2. Module pages with no library are a bare title. Medium.** `/photos` and `/books` (and `/page/wittyphotos`, `/page/wittyink-books`) show only "Photos" or "Books" with "Edit for everyone" and no empty state or "Add a library" action. Rule: `DESIGN-2.md` states (loading, empty, failed); `gaps/` empty-state boards. Owner: W10.

**M3. Visualizer mode is small and sits behind the cover. Medium.** Album playing, Visualizer chosen: the cover stays centred and a few thin bars show at its sides; the board (`MU_lumen` "TV visualizer") fills the stage with the effect, the effect chips and intensity, speed, colour sliders. Chips here: Bars, Waves, Drift, More, Slow, Styles. I did not open More (the 18 effects) so the full list and the Aurora Rings effect are not verified. Owner: W10.

**M4. Karaoke and lyrics not walked fully. Info.** The test files have no lyrics (karaoke shows "No words for this song"); Follow along, Sooner, Later and the three karaoke sliders could not be seen. Needs a file with a timed `.lrc`. Owner: W10 to say how to check.

**M5. TV-width WittyRemote is a short wide strip, translucent, with small keys. Medium.** `/play/1?tv=1` at 1920x1080, Remote: a 920x256 panel at the bottom right of the picture, keys 22 of them under 66 px (TV target), the player's own control row shows through and is covered. The boards (`PB_lumen`) have a two-page sheet with large keys and focus ring. Owner: W13.

**M6. A library channel with three short films shows no programme. Medium.** `/tv` after adding the Movie Channel: title "We'll be right back", "12:00 AM to 12:00 AM", no description or logo, "No preview for this channel". The guide row is one block for the whole day. The same on the Rail and Lumen looks. Rule: principle 11 (title, times, description, More info). Owner: W2 (screen) with the livetv owner for the schedule from short files.

**M7. The music bar covers the WittyTV Dock. Low.** `/tv` with music playing: the key legend (Look, Day, Channels, Filter, More info, Recordings, Record, Enter, Esc ...) is drawn underneath the music bar. Same family as N6. Owner: W2.

**M8. TV mode stays on after `?tv=1`, even on a phone width. Low.** Visiting `/?tv=1` once stores it; at 375x812 the next visit still draws the TV tabs (`tvm-tabs`, layout 942 px wide) until the stored flag is cleared. Check that the phone width wins or that there is a way out. Owner: W2.

## Not walked in 2b

Modules' own pages with data (needs photos, books, discs; the catalogue installs were checked by their `uiAdds` and by the pages above), WittyTV recordings with a real broadcast (library channels cannot be recorded; the empty state reads well), the theme import file chooser, the effects list under Visualizer More, lyrics views, the Back ladder end to end (Escape closes the layout editor and the player menu; a full climb from a deep page to Home to leaving was not done), W5's converted pages (only `/design/gaps/playinfo` was opened, in 2a), the gaps against the boards one by one.
