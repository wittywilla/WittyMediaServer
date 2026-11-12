# Design 2: features with no drawn element, from the clients (2026-11-12)

Collected by the design lead from W2 (web), W3 (Apple) and W4 (Android). Found by keyword search of the canvas boards, so each item must be checked before it is drawn. Server and module gaps are in `DESIGN-2-FEATURE-GAPS.md` (W1, W2). Order of drawing: the items that appear on more than one platform first.

## Appear on two or three platforms (draw first)
1. **Profiles and rules:** Who's watching, profile switcher, avatar picker, profile settings, You tab, parental controls and PIN, **screen time** (the "Screen time is over" screen, "More time from…" note, guard overlay), kids look.
2. **Connect and accounts:** choose a server, enter an address, manage servers and forget a server, saved-server cards, "This isn't the server you used before", certificate mismatch, sign-in with password, with a TV code, with a security code pad and QR, two-step, forgotten password help, pairing v2 and away join, legal notice, server or app too old.
3. **Skip markers:** skip intro, recap, credits and advert overlays; the skip settings; the admin skip editor.
4. **Remote and together:** Play on / remote hub (pick a device, controlled-device overlay), watch party and SyncPlay (start, join, invite, in step), downloads and offline.
5. **Listening beyond songs:** podcasts, internet radio and online music, audiobooks (resume, chapters, speed, **sleep timer**), equaliser, sound shaper, crossfade, gapless and normalisation settings, karaoke stem preparation job and its progress, microphone pickers, music video player, AirPlay, CarPlay, lock screen and Now Playing.
6. **Library tools:** Customise page (reorder, hide, pin, density, card size), Fix matches and Library review, Add library and the folder picker, artwork picker, collections and save to a collection, rating, share, download and upload buttons, subtitle/audio/view/track/play-info pickers, server-quality and bitrate picker.
7. **Pages that exist today without a board:** Books library and Comics library pages, the photo timeline and map, Creator pages, the CD player page, Cover Flow, disc sections (titles, versions, episodes, "why this title is the film"), Where to Watch panel.
8. **WittyTV:** reminders (alert and list), custom sets editor (create, rename, add to set, favourites), channel editor, tuner setup, EPG search panel, tuner or channel failure ("Watch X instead", "Try again"), "From the start" catch-up banner, record options sheet. (Recordings, sets list and Look are drawn.)
9. **Games, the existing pages:** library, game page, review, rigs, Game PCs (couch), Moonlight stream setup, the on-screen touch pad and its editor, controller assignment and remap, streaming states.
10. **System:** Activity button and tasks panel, notices and error banners ("The server isn't answering"), loading, empty and error states, key help dialog, legal notes bar, Help, Design pages, diagnostics and client log, About and licences, language, updates and the version tag with the ALPHA stage, the Menu drawer on phone and the floating bottom bar.
11. **Platform integration:** Top Shelf (tvOS), Android TV Watch Next and launcher cards, Open on phone (the browser fallback), module settings screens, Settings subpages beyond Appearance (hand-built, structure unchanged).

## What the clients say the canvas has that they cannot build yet
Rail drawer and mark, WittyRemote, layout and name editors, experience chooser, dashboard, crossbar, carousel and marquee components, Aero finish and scenes, theme import. All are blocked on the server contracts in `DESIGN-2-SDUI-GAPS.md` (experiences, `setup_required`, theme import, `brand.mark`, required-look policy, finish and scene tokens, the new components and nav styles). W1 owns them.
