# Design 2 QA, pass 2a

Written 2026-11-12 by W11 (design QA). Build: `v0.8.0-alpha` at `9df95931` (W12 editors and wizard, W2's D3/D9/D11/D13, W5 gap pages merged; W13's finishes were not in). Fresh data folder, scratch server on port 19810, generated media only, 1440x900 unless stated, plus 375x812. Pass 1 is [DESIGN-2-QA-1.md](DESIGN-2-QA-1.md); ids D1 to D27 are its ids, N1 and up are new.

Not walked in 2a (said plainly): WittyTV recordings and the guide's look under every UI, remote at TV width, the wizard's Skip buttons on every step, theme import (the file chooser is native and cannot be driven), modules (a later slot), the Back ladder end to end, D4/D5/D2/D15 (W13's merge was not in), D6 (W10).

## Pass 1 items rechecked

| Id | Result |
|---|---|
| D1 first run refused after the account | **Fixed.** Libraries, settings and look save through the wizard; no 503. |
| D7 first run steps | **Mostly fixed.** 14 steps with a left step list and "Step n of 14", a Name step (name editor, previews), a Look step with TV / Desktop / Phone / Headset previews, People with Required looks and themes. Still missing: see N8. |
| D3 UI brings its default theme | **Partly.** Choosing Classic Cable in the wizard applies the Cable theme at once. But see N2 (it goes back to Witty after the Theme step) and N1 (the Appearance look picker could not be tried, locked). |
| D9 rail | **Mostly fixed.** The menu button pins the full menu, "Keep menu open" switch follows, libraries show counts, the drawer starts after the rail, headed "Movies / 3 titles". Still: the drawer's views are drawn centred, not left-aligned (board `RailStates`). |
| D10 WittyRemote see-through | **Fixed on desktop** (opaque graphite, dimmed keys dashed, note about unused keys). Phone: see N7. |
| D11 mini player over audio bar | **Partly.** The film's mini player now sits above the music bar, but its lower edge and controls are still covered by the bar (N6). |
| D12 up-next at 2 s | **Open** (now at about 6 s of a 20 s clip; the card is also translucent over the picture). |
| D13 WittyTV look precedence | **Fixed for Rail** (Settings says "Classic is what your app theme suggests", your own choice wins). Lumen to Modern not re-walked. |
| D22 unnamed checkboxes (account step) | **Open** (two `checkbox` with no accessible name). |
| D6, D2, D4, D5, D14 to D21, D23 to D27 | Not rechecked. |

## New defects

**N1. A required look or theme cannot be taken away again, and the administrator is locked out. High.** Screen: Settings, Appearance, and Settings, Pages & menu, Looks. Rail with Evergreen set as required for Everyone in the wizard. Afterwards Appearance says "Required by qa-admin for everyone. It can't be changed here." with only the required look offered, and the administrator who set it is locked too. Clearing both selects on Pages & menu, Looks and pressing "Save to draft" gives "A draft is waiting, the same as what is published" and "Publish" does nothing; after a reload the policy is still `required: everyone rail / evergreen`. Appearance has no link to the page where it is changed. Rule: principle 14, `SB_*` boards ("Required by ..."). Owner: W12 (policy and Looks page), with W2 (Appearance link).

**N2. Cable look, but the Witty theme once the wizard is done. Medium.** Choosing Classic Cable on the Look step shows the Cable theme at once; after pressing Continue on the Theme step without choosing a theme, the page is `classicCable` with theme `default` (violet). Rule: principle 2, `UiTheme`, and the Theme step's own text ("start with the look's own theme"). Owner: W12.

**N3. The wizard's required look is not applied until reload. Low.** After "People and rules" saved Rail and Evergreen, the app still wore Cable and Witty until a reload; Appearance then says "Witty in use on this device" while the applied theme is Evergreen. Owner: W12 and W2.

**N4. Layout editor: footer below the fold, phone tab draws a left menu. Low to Medium.** Opened from Appearance, Edit layout. Works: For everyone / For me, five device tabs, presets, keyboard pick up (Enter), move (arrows), drop (Enter) announced "Dock: moved to bottom", Undo and Redo, Reset to Rail, Escape cancels. Wrong: on the Phone, Headset and TV tabs the buttons Save, Undo, Redo, Reset and Cancel sit below the dialog's fold at 1440x900 (not sticky); the Phone tab still draws "Menu · Left" and "Dock · Top" (the board keeps the rail as a bottom bar on phones). Owner: W12.

**N5. Name editor details. Low.** Works: Name, letter and word selection by keyboard, follow-the-theme and fixed colours, any colour, gradients, presets (Accent on Media, Two-tone, One colour, Plain, Rainbow letters, Gradient), mark (Letter, Initials, Your own, Logo), live previews in side menu, top bar, rail header, sign-in, light and black, and a contrast note ("Media is 1 to 1 on the side menu ... should be at least 4.5 to 1") when a bad colour is chosen. Wrong: the letter row wraps inside the word ("S" at the end of one line, "erver" on the next); the sign-in preview wraps "Server" onto a second line; the contrast note sits far below the colour controls and does not stop saving a name nobody can read. Owner: W12.

**N6. Music bar hides the left of itself under the pinned rail, and covers the film's mini player controls. Medium.** Pinned rail, 1440, album playing while a film is in the mini player: the cover, title and progress start behind the rail; the film's mini player controls are behind the bar. Rule: `DESIGN-2.md` zones (one Layer at a time), `MU_*`. Owner: W2.

**N7. Phone WittyRemote sheet is translucent and its keys are small. Medium.** 375x812, Remote on a film. A two-page sheet (Keys, 1 2 3) is right, but the player bar shows through it ("+30" behind AUDIO), the "You might also like" card is half hidden beneath it, and 21 of its keys are under 44 px. Rule: principle 10 and 16, `PB_lumen` phone boards. Owner: W13 (finishes) / W2.

**N8. First run still lacks parts of the boards. Medium.** Welcome has no Language and time zone; Account has no display name and no Two-step sign-in; Theme has no "How people use it" (start with the look's theme / let people choose) and no WittyTV look. Rule: `OF_lumen`. Owner: W12.

**N9. The Looks policy page has two steps for one action and the message contradicts itself. Low.** "Save to draft" then "Publish"; a banner says "A draft is waiting, the same as what is published" after a change. Owner: W12 (see N1).

## What works well (so it is not undone)

The step list and "Step n of 14"; the look preview pane with device tabs; the name editor and its previews; keyboard pick-up and drop in the layout editor with announcements and Undo; the locked "Required by ..." state with only the required look shown; WittyRemote opaque with dimmed dashed keys; the rail's pin switch and library counts; the Classic suggestion for Rail; `/design/gaps/<key>` pages open (for example `/design/gaps/playinfo`).
