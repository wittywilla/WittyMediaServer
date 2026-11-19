# First-time setup: the flow

This page says what first-time setup asks, in what order, and why. It is the plan the setup pages
(`web/src/pages/setup/`, `SetupWizard.tsx`, `setup/plan.ts`) and the server's setup routes follow.
[FIRST-TIME-SETUP.md](FIRST-TIME-SETUP.md) is the guide for people setting up a server; this page is for
whoever changes setup.

## The rules

1. **Language and time zone come first**, before anything else, and are asked once.
2. Then the **account**, then **where data is kept** (a desktop install only), then the **server's name**.
3. Then **modules and libraries**: choose and install modules, add libraries, then **each installed module's
   own setup** (Live TV and its demo tuner, Games, Discs, and any later module that brings a step).
4. Then **looks and themes**, after the modules are set up, so the choices are about things the person
   now has. **WittyTV's look is offered only when WittyTuner is installed.**
5. Then **people**, then **away from home** (remote access in plain words, with a connection test), then
   **optional extras** (posters and details, logos, lyrics, the karaoke voice separator, streaming lists),
   then **Ready**.
6. Every step that can be skipped says where the same choice is later: "You can change this later in
   Settings → …", **once** per page.
7. Plain words. No "port", "P2P", "NFO", "metadata agent" or "API key" without saying what it is in the
   same sentence. Progress is always shown.
8. TV mode: a part that a remote does badly (typing long codes, choosing a file, colouring letters) says
   **"Leave TV mode to change this"** on a computer in TV mode, or **"Use the WittyMediaClient app on a
   Windows or Mac computer, or a web browser"** on a TV, from the same table as the rest of the app
   (`pkg/sdui/data/tvlevels.json`, docs/TV-MODE.md). Setup never strands someone: those parts can always
   be skipped.

## The steps

Twelve numbered steps. A step can have several pages ("Step 4 of 12 · This server · part 2 of 3"). The
count never changes under someone: a step with nothing to do on this server is still counted, drawn
dimmed with a reason, and passed over.

| # | Step | Pages | Who sees it | Skippable |
|---|------|-------|-------------|-----------|
| 1 | Language | Language and time zone | everyone | has a default; Continue |
| 2 | Welcome | Set up new, or restore a backup | everyone | – |
| 3 | Your account | Account; two-step sign-in (when ticked) | everyone | two-step page yes |
| 4 | This server | Where data is kept (desktop installs only); Name; Home network | everyone | Name and Home network yes |
| 5 | Modules | Choose and install; when there are new versions | when the module list can be read | yes |
| 6 | Libraries | Where your media is | everyone | yes |
| 7 | Set up your modules | one page per installed module with a step (Live TV, Games, Discs, …) | when such a module is installed | each page |
| 8 | Looks | Layout; Colours; WittyTV (only with WittyTuner) | everyone | yes |
| 9 | People | Profiles; rules | everyone | yes |
| 10 | Away from home | Reach it away from home; pair an app | everyone | yes |
| 11 | Extras | Posters and details; Logos; Lyrics and artists; Karaoke voices; Streaming lists | pages by what's on the server | every page, and "Skip the extras" |
| 12 | Ready | What's set up, and what's next | everyone | – |

### 1. Language

*"Language and time zone".* Asked before Welcome, in this browser, and saved when the account is made
(as today). Decides: the language for disc menus and sound tracks, the server's time zone.

- Counted as step 1, so the header no longer says "Welcome" over it.
- **Not asked again** on the Name page. Today Settings has no place for either, which is why Name asked
  again: this work adds *Language and time zone* to *Settings → Server*, and that is the "later" place.

### 2. Welcome

*"Let's set up your server"*: **Set up a new server** or **Restore from a backup** (as today). The list of
what the server does stays.

### 3. Your account

*"Create your account"*, and the two-step page when ticked (as today). Not skippable. On a service, Docker
or another device, the setup code panel (as today).

### 4. This server

- **Where data is kept** (only when the server says `ask`: a desktop install with no data folder chosen).
  As #473 made it. Not skippable, but "Keep it here" with the current folder is the default.
  "Somewhere else" on a TV: *use a computer* note (typing a folder path with a remote).
- **Name**: *"Name your server"*, the name editor with its colours and mark. Colouring letters and
  uploading a mark are `desktopOnly` parts on a TV; the name itself can be typed.
  Later: *Settings → Appearance → Name and logo*.
- **Home network**: *"Finding this server at home"*: "Find this server automatically" (apps on your home
  network find it by name), and *"The number at the end of its address"* (the port), with the address
  shown in full (`http://192.168.1.20:9797`). Later: *Settings → Network*.
  The *Encrypted connections* line moves to step 10, and no link leaves setup from here.

### 5. Modules

*"What do you want on this server?"*, as today: Recommended first, then by kind, the gaming-PC ones folded
away; then *"When a module has a new version"*. Installing shows each module's progress.
Later: *Settings → Modules*.

### 6. Libraries

*"Where is your media?"*, as today, with one question moved here from Details:

- **Before the first folder is added**: *"Save posters and details beside your files?"* (`saveMetadata`).
  Adding a library starts a scan at once, and today the question comes two steps later, after files may
  already have been written into the media folders. The default stays on; the words say plainly that the
  videos and music themselves are never changed.

Later: *Settings → Libraries*.

### 7. Set up your modules

One page for each installed module that brings a first-run step (`integration.json` `web.setup`), in the
order the module list shows them. Today: **Live TV** (WittyTuner), **Games** (WittyPlay), **Discs**
(WittyDVD/WittyBluray, the server's own page). A module added later just adds a page.

- **Live TV**: WittyTuner's own page: find tuners, add a playlist or a single stream, and *"No tuner or
  playlist yet? Try a demo tuner"* (WittyTuner 0.4.4). Later: the module's own `later` (*Settings → Live TV
  & tuners*); the server no longer keeps its own different wording for it.
- **Games**: WittyPlay's own page. **Discs**: as today.
- Moving on (Continue or Skip) records the step, so Home doesn't say "Finish setting up" afterwards (#434).
- With no such module, the step is dimmed: *"Nothing to set up: no module here needs it."*
- Before the Modules step has been passed, the dimmed line says *"After you choose modules"*, never
  "Module not added" (today it says that from the very first page).

### 8. Looks

After the modules, so every choice is about something the person has.

- **Layout**: *"How should it be laid out?"* the fourteen looks, previewed as this device shows them (the
  lead stops saying "as a phone shows it" on a computer). On a phone the preview comes first and the cards
  below, so the page isn't sixteen cards long before anything else. Later: *Settings → Appearance*.
- **Colours**: *"Choose colours"*, as today, without "Let people import their own themes" (a rule: it
  moves to People). *Import a theme…* opens a file picker: `desktopOnly` on a TV.
- **WittyTV** (only when WittyTuner is installed): *"How should the TV guide look?"*, the guide's layout and
  colours (`tvTheme`), starting from what the chosen look brings ("Follow the look"). People can still pick
  their own in the guide. Later: *Settings → Appearance → WittyTV*.
- The *TV mode on this device* switch leaves this step: it is about one device, not the server. It moves to
  Ready ("Use this screen as a TV").

### 9. People

*"Who else watches?"*: add profiles and kids profiles, then *"Rules"*: a look or theme required of
everyone, of kids, or of one person (now after the looks it refers to), and *"Let people add their own
themes"* (moved from Colours). Later: *Settings → Profiles*.

### 10. Away from home

*"Watch away from home"*, done here rather than a list of links out of setup.

- One switch: *"Let my devices reach this server away from home"* (peer-to-peer, #477): *"Your phone or
  laptop connects straight to this server, with nothing to change on your router. If a direct connection
  can't be made, a relay passes it along, encrypted."* Off by default; the status words come from the
  server (the same ones Settings uses).
- **Test the connection** (#477's test) answers in a sentence.
- *Encrypted connections*: the server's own words (moved here from the Name page).
- *Pair an app*: the code and QR code shown in place, not a link away.
- Later: *Settings → Remote Access*.

### 11. Extras

*"A few extras, all optional"*. First page has *Skip the extras* (straight to Ready). Each page skippable.

- **Posters and details**: the free sources, optional TMDB and TheTVDB keys (*"a free code that lets your
  server ask them"*), and, when a library already has details beside its files, the #467 convert panel
  (*"Details already in your folders"*), with *Leave them as they are* the default. Pasting keys:
  `desktopOnly` on a TV. Later: *Settings → Metadata agents*.
- **Logos**: as today. Later: *Settings → Metadata agents → Logos*.
- **Lyrics and artists** (music can be added): as today.
- **Karaoke voices** (music can be added): *"Separate voices for karaoke"*: the one-press Demucs install
  (#436, and the DirectML work), saying the download size, that it uses the graphics card when it can, and
  that karaoke works without it (songs use the quick version). Installing goes on in the background;
  Continue isn't held. Later: *Settings → Metadata agents → Demucs for karaoke* (where the row is today).
- **Streaming lists** (movies or shows can be added): as today, reusing a TMDB key given on the first page.

### 12. Ready

*"You're all set"*: what the server says is set up now (as today, plus Live TV, looks, people and away
from home), with where each is changed. Then only what's still useful:

- *Watch on every screen at home*: the address, as today, without a second port editor.
- *Use this screen as a TV* (the TV mode switch, this device only).
- *Add live TV* only when WittyTuner isn't installed; *TV channels from your libraries* as today.
- *Add a Kids profile* only when People added none.
- **Start watching** / **Open Settings**.

## WittyTV after setup

WittyTV's look belongs to WittyTuner:

- While WittyTuner isn't installed, setup has no WittyTV page, and *Settings → Appearance* has no WittyTV
  section or WittyTV module colours. (The guide itself stays: channels made from libraries need no
  module.)
- **Whenever WittyTuner is freshly installed** (not an update), during setup or later from the store, its
  WittyTV look is offered:
  - in setup, as the WittyTV page of step 8;
  - after setup, as a pending module step `wittytuner.wittytv-look`: the server adds it to the module's
    steps on a fresh install, so every app shows *"Finish setting up WittyTuner: choose how the TV guide
    looks"* on Home until it is done or skipped; the web store also opens the same picker right after the
    install finishes ("WittyTuner is ready. How should the TV guide look?" with **Use this** and
    **Later**).
  - Removing WittyTuner forgets that the step was done, so installing it again asks again.

## What setup remembers

The server keeps where setup was left (`GET/PUT /api/setup/state`). Its step names become: `language`,
`welcome`, `account`, `server`, `modules`, `libraries`, `modulesetup`, `looks`, `people`, `remote`,
`extras`, `ready`. The old names (`name`, `look`, `theme`, `details`, `livetv`, `games`, `discs`) are still
accepted and mapped, so a setup left half-way on 0.8.0 carries on. No app reads the names.

## What this fixed (0.8.1-alpha before this change)

Found on a fresh data folder with the open setup PRs merged (#466, #474, #475, #477; #467's Details
change read from its branch), desktop and service-style, desktop and phone width:

1. The language page says "Step 1 of 14 · Welcome".
2. From the first page, Live TV, Games and Discs say "Module not added", before modules were offered.
3. Language and time zone are asked twice (before Welcome, and again on Name and address), and after
   setup no page in Settings changes either.
4. Look and Theme come before Modules, and the WittyTV look is offered with no WittyTuner installed.
5. The Look page says it draws "as a phone shows it" on a computer; on a phone it is sixteen cards before
   the preview, WittyTV and TV mode.
6. TV mode (one device) sits in the Look step (the whole server).
7. Adding a library starts scanning with `saveMetadata` on, two steps before the question that decides it.
8. Name and address mixes the name, a second language/time zone, discovery, encrypted connections (with a
   link out of setup) and the port; Ready offers the port again.
9. Remote access and apps is only links out of setup.
10. "You can change this later" appears twice on every skippable page; a module's own page shows two
    "Skip for now" buttons.
11. With the released WittyTuner, Live TV says "Live TV couldn't be loaded" (fixed in WittyTuner 0.4.3+,
    which this release publishes).
12. Ready suggests adding live TV after WittyTuner was set up, offers a Kids profile after People, and
    says "Devices away from home: not allowed, by your choice" when nobody was asked.
13. Live TV's "later" place differs: "Settings, Channels & tuners" in setup, "Settings, Live TV & tuners"
    in the module.
14. The karaoke voice separator isn't offered in setup.
15. On a TV, setup has no TV-level rules: the theme ZIP import opens a file picker, letter colouring and key
    pasting have no note.
16. "Details and artwork" is four pages of unrelated questions in one numbered step, between Libraries and
    the module steps.

---

<sub>WittyMediaServer documentation</sub>
