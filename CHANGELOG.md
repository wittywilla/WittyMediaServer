# Changelog

All notable changes to WittyMediaServer. Versions follow [semantic versioning](https://semver.org);
`-alpha` releases may change settings and APIs between versions.

## Unreleased

## 0.8.0-alpha

- Added: profiles on a shared screen, and parental controls. **Switch profile** changes who a signed-in device belongs to without signing out, asking for the other profile's password or PIN (`POST /api/v1/profiles/switch`). Each profile can have a picture. Limits gain an age for each kind of media (films, TV, games, music, books) shown in each country's ratings, whole collections and tags to allow or hide, and "only these genres"; switches for games, recording, downloads, share links, comments, use away from home, and changing one's own name and picture; and screen time, with a daily allowance for school days and weekends, bedtimes, and "ask a parent" for more with a parents' PIN. The server counts the time and stops what is playing. A parent can see what a profile watched and what it was refused. See [docs/PROFILES.md](docs/PROFILES.md). Features `profile-switching`, `profile-avatars`, `parental-controls` and `screen-time`.
- Fixed: recommendations could name a genre ("because you watch…") from titles an account's limits had hidden since it watched them.
- Licence: the server and every Witty module are now AGPL-3.0-or-later (they were Apache-2.0), and the client apps are GPL-3.0-or-later. Code forked or derived from another project keeps its own licence, such as WittyCouch Host's Moonlight-derived parts under the GPL-3.0.
- Added `pkg/sdui` server-driven UI library (schema v1, validator, renderer, differ, templates). See [docs/SDUI.md](docs/SDUI.md).
- Added: server-driven screens over HTTP. `GET /api/v1/sdui/screens/{screen}` renders a screen for the account and device (platform, form factor, viewport, locale, capabilities), with ETags and 304, and `…/diff?since=<etag>` answers with patches. The screens live in `ui_config.json` in the data folder, made from the stock screens on first start, with the last 10 revisions kept for going back, recovery from a damaged file, and hand edits picked up while running. Administrators read, replace (with `If-Match`), list and roll back the configuration under `/api/v1/sdui/config`. See [docs/SDUI.md](docs/SDUI.md#http-api).
- Added: the theme engine. `GET /api/v1/theme` resolves an account's colours from the server's theme and global overrides, the account's own choice, and per-module overrides, with WCAG AA contrast warnings. Administrators set them with `PUT /api/v1/theme/global` and `/api/v1/theme/modules/{module}`. `/api/themes` is unchanged. See [docs/THEMES.md](docs/THEMES.md#the-theme-engine).
- API revision 11: features `sdui`, `sdui-diff`, `theme-engine` and `theme-module-overrides`.
- Added: secure pairing at `/api/v1/auth/pair`. A TV shows a QR code (`wms://pair?v=1&s=…&u=…&c=…&k=…`, with an ordinary `approveUrl` for phones without the app) and a 6-digit PIN that changes every 30 seconds (RFC 6238 TOTP from a secret only the server holds). The person approving types the PIN shown now. Each PIN works once, five wrong ones end the session, and a session lasts 5 minutes. The plain codes at `/api/pair` are unchanged, apart from `needsPin` in `/pair/info` and an optional `pin` in `/pair/approve` for these sessions. See [docs/CLIENT-API.md](docs/CLIENT-API.md#with-a-qr-code-and-a-pin-secure-pairing).
- API revision 12: features `pair-totp` and `pair-qr`.
- Added: canvas, a short looping video behind a song or title. It's found beside the media (`Song.canvas.mp4`, `canvas.mp4`, `.canvas/`) or in the metadata folder, or an administrator uploads one (MP4 or WebM, up to 30 MB, kept in the server's own folder). `GET /api/v1/media/{id}/canvas` and `…/canvas/file`, with byte ranges and stream tokens. See [docs/MUSIC.md](docs/MUSIC.md#canvas-a-looping-video-behind-the-song).
- Added: karaoke stems. A song's separated parts (`vocals`, `instrumental`, `drums`, `bass`, `other`) made by a tool such as Demucs are found beside it (`Song.vocals.flac`, `Song.stems/`, `stems/`), with Native Instruments `.stem.mp4` files listed as one multitrack file, or uploaded by an administrator. `GET /api/v1/media/{id}/stems`, and `stems` in a song's playback answer. The server doesn't separate songs itself. See [docs/LYRICS-KARAOKE.md](docs/LYRICS-KARAOKE.md#stems-the-real-vocal-track).
- Added: skip markers. Recaps and credits are found from chapter names, as intros were, in English, German, French, Spanish and romanized Japanese, and intros now also from those languages. An `.edl` or `.skip.json` beside a file adds advert breaks and named ranges. `GET /api/v1/media/{id}/markers`; administrators set them by hand with `PUT`, which no scan replaces. The playback answer adds `recap`, `credits` and `commercials` beside `intro`. See [docs/PLAYING.md](docs/PLAYING.md#recaps-credits-and-adverts).
- Added: where to watch. `GET /api/v1/media/{id}/where-to-watch` puts a title's copies here (quality and versions) beside the services that stream, rent or sell it in the region, from TMDB, and `GET /api/v1/discover/where-to-watch?tmdb=&type=` does the same for any TMDB title. Answers are kept as long as streaming listings; without a TMDB key it answers with `reason: "no-api-key"`. See [docs/METADATA.md](docs/METADATA.md#where-to-watch).
- API revision 13: features `canvas`, `karaoke-stems`, `skip-markers` and `where-to-watch-xref`.
- Added: WittyPlay's Docker backend through the server, under `/api/v1/modules/wittyplay/docker/...` and the KasmVNC desktop at `/api/v1/modules/wittyplay/kasm/{sid}/...` (WebSocket included). The server now tells a module's service who is asking with `X-Witty-User` and `X-Witty-Admin`, and always drops any `X-Witty-` header a browser sends. Administrators' routes are checked by the server as well as by WittyPlay. See [docs/MODULES.md](docs/MODULES.md#who-is-asking).
- API revision 14: feature `module-identity`.
- Small machines: the server fits itself to the machine it starts on. On a Raspberry Pi 3, an old Celeron mini PC or a container given little memory, it keeps its own memory within about a quarter of the machine's, reads fewer files and makes fewer thumbnails at once, and a library scan gives way while a video is being transcoded or a channel is running, then carries on. `WMS_MACHINE` overrides what it works out. See [docs/HARDWARE.md](docs/HARDWARE.md#small-machines).
- A film sent with a speed limit for remote access is still sent by the system straight from the disk, as it is at home, and the API's answers and a module's stream borrow their buffers instead of allocating them.
- Fixed: a DVD's sound drifting out of step with the picture after a join between cells. Where a cell starts part way into a sound frame, the 32 ms lost there was stretched back at one sample a second, for minutes, and more with every join. A gap of 10 ms or more is now filled or cut at once, in a video's stream and on a channel.
- Fixed: on a Mac, a module's helper could signal any program of the server's account, the server included. The sandbox's rule against signalling others didn't hold there; it now allows signals only within the helper's own sandbox, and the Mac test requires that (#139).
- `bump-server.sh`, `publish-all.sh` and `check-executable.sh` run on a Mac, with its own bash 3.2 and BSD sed.
- Fixed: on an Intel Mac, Apple VideoToolbox never worked: FFmpeg there won't start it with a quality setting, so the server's test encode failed. It now gets a bitrate for the picture's size there, for streams and library transcoding; Apple silicon is unchanged (#138).
- Fixed: on a Mac, the menu-bar app's `Info.plist` now says why it asks for Desktop, Documents, Downloads, removable and network drives, and a capture device's camera and microphone (#137).
- Fixed: on a Mac, installing over an earlier background service whose property list couldn't be read took the error message for the data folder's name, and the uninstaller couldn't find a data folder with `&` in its name (#136).
- Added: the server as an AppImage, a Flatpak (`com.wittywilla.WittyMediaServer`) and a snap, for `amd64` and `arm64`, on the Releases page beside the `.deb` and `.rpm` and in the same signed checksum list. The AppImage and the Flatpak open the tray app; the snap runs a service. None of the three has been run yet, and none replaces its own files: a new version is downloaded and installed the same way. See [docs/INSTALL.md](docs/INSTALL.md#appimage-flatpak-and-snap).

## 0.7.2-alpha

- Fixed: updating any module failed with "the catalogue and the release disagree" on 0.7.1-alpha.

## 0.7.1-alpha

- Modules made for earlier 0.x servers (such as 0.6.x) now install and run on this server when their module API fits.

## 0.7.0-alpha

- **Layouts follow you, per console and shape of device.** `PUT /consoles/{console}/controls` keeps a `layout` for a phone upright or sideways, a tablet upright or sideways, a computer or a TV, and `saved.layouts` gives them all back. A layout places the on-screen controls, the menu button, the shortcuts bar and the picture, or each screen of a DS or 3DS apart, and `null` puts it back as it came. The server checks it itself. A two-screen console says where each screen sits in the frame (`pad.screens`, `capabilities.screens.frame`). WittyPlay's Edit layout uses all of this.
- **A game's settings open where you press for them.** "Settings for this game" on a game's page opened far down the page, away from its button, with nothing to take you there. It now opens in a sheet over the page: from the side on a wide screen, up from the bottom on a phone, with focus in it. It closes with ×, Esc, a controller's B, a tap outside, or Back (a phone's gesture or the browser's button, which no longer leaves the page), and focus goes back to its button. A console's **Look and settings** opens that console's settings, not every game's. Every menu over a game is easy to leave as well, from WittyPlay's next version: Back and × Back to the game stay in view, and a phone's Back gesture closes a menu instead of leaving the game.
- **Films get posters without a TMDB key, where a free one exists.** A film matched through Wikidata gets its poster from Wikimedia Commons: the film poster Wikidata names, or its picture when that's plainly a poster. That's mostly older and public-domain films, as most posters aren't free. They show in Fix matches, a title's match and Artwork tabs, and on films after a scan or **Check for changes**. Each one's licence and author are listed under **Settings → Legal**, *Posters from Wikimedia Commons*. A TMDB key, when set, still comes first. TV shows already get posters, backdrops and episode pictures from TVmaze without a key.
- **Choosing a core is one picker.** On a game's page, **Change core** beside the core that plays it; in **Settings → Consoles & cores**, **Change core** on each console. Each core says what it's good at (accuracy, speed or a balance of the two), where it runs and how it draws on your server, whether it needs a BIOS file of your own and whether it's installed. Choosing one that isn't installed installs it, with its progress, then uses it. It works on a phone, with a mouse, and with a TV remote or controller, and closes with ×, Escape, B or Back, or a tap outside. A game's own core is set the same way, or left to its console's. During a game, the player can open the same picker (`chooseCore`), which offers to restart on the new core: in-game saves stay, and save states stay with the core that made them.
- **Nine more cores to choose from**, each offered only where a build is published: QuickNES for NES; today's bsnes for Super Nintendo; Gearboy for Game Boy; VBA-M and gpSP for Game Boy Advance; Beetle PCE and Beetle SuperGrafx for PC Engine; Beetle Lynx for Lynx (which needs your own `lynxboot.img`); and RACE for Neo Geo Pocket. They come with the next WittyPlay. bsnes played 16 retail Super Nintendo games here, special chips included, with no firmware files.
- **For apps:** `GET /gaming/consoles/{console}/core` and `GET /gaming/games/{id}/core` give the picker's `choices`, and `GET /gaming/cores/{id}/install` how far an install has got. `GET /games/consoles` gives each core's `coreAbout`. See [Choosing a core](docs/GAMING-API.md#choosing-a-core).

- **WittyTV keeps playing in the background.** Switching to another tab or minimising the window no longer stops a channel: the guide's preview carries on with its sound, and a player that goes quiet in the background tells the server it's still there, so the channel isn't stopped behind it. **Keep playing when this tab is in the background**, in WittyTV's look and in the player's settings, turns it off for one device; then a channel stops when you switch away, freeing a tuner, and starts again when you come back. Administrators choose what new devices start with under **Options**. Apps get the same default as `backgroundPlay` in the guide and tune answers.
- **On a Mac whose WittyPlay can use the graphics card, PlayStation 2 plays on Play!, on the card.** LRPS2 comes first for PlayStation 2, but can't use a Mac's card, so it would still have drawn on the processor, which can't keep a Gran Turismo 4 race at full speed. A game Play! can't open still goes on to LRPS2, and someone who chose LRPS2 keeps it.
- **Comics whose pages are PNGs called .jpg get their cover.** Several public-domain comics name their PNG pages `000.jpg`, and the server's picture tool chose how to read a picture by its name, so their covers failed during scans ("bits 193 is invalid", "unsupported coding type") and the comics had none. It now reads what a picture really is from its first bytes, for every book and comic cover.
- **A theme made with the theme maker shows again.** The server's own security policy refused the stylesheet the page made for it, so the page fell back to the plain built-in look, in every browser. It's now applied in a way the policy allows.
- **On a big screen, the account menu opens and the sign-in screen fits.** At 2200 pixels wide and more the page was enlarged with CSS zoom, which Firefox and Chrome also apply to positions on the screen and to the window's height: the account menu opened off the right edge, so clicking your name seemed to do nothing, and the sign-in screen was a quarter taller than the window, so it scrolled and sat low. Big screens now grow their text, spacing and frame instead, and look much the same.
- **Menus always open on screen, and stay open.** A menu from a button, such as the account menu, is placed wholly on the screen even when its button can't be measured, and a list or row scrolling elsewhere on the page no longer closes it as it opens; the page scrolling its button away still does.
- **WittyPlay's games home is shelves.** Each console is a shelf, and its games stand on it as their console's own kind of case, covers showing (a box, a clamshell, a jewel case, a keep case), as cartridges, cards or discs, or as spines on a bookshelf: a switch above the shelves, kept on each device. Where you left off leads the page with the picture you stopped at; a console's page opens with its collection band; a game playing on another of your devices offers **Continue here**. A game's page says **Moments** for its save states.
- **Rewind is on by default for the 8-bit, 16-bit and older handheld consoles**, whose games take little memory to rewind. An administrator can still turn it off for everyone, and anyone for themselves.
- **Play one game from several devices.** The host of a game makes a four-letter code that lasts ten minutes, and anyone signed in to the server joins with it, if their profile may open that game. Saving, loading and stopping stay the host's.
- **Apps learn what each console can do** from `GET /games/consoles`: its era, its screens, its controls, discs and memory cards, its starting look and its case and cartridge, so they can build their menus from data. See [Games: the API for clients](docs/GAMING-API.md#what-a-console-can-do).
- **With a WittyPlay older than 0.3.5, version 2 of the game stream says it needs an update** (`needs-update`), rather than failing with an error, and isn't offered. Version 1, which every app and the website use with that WittyPlay, plays as before.
- **A game on several discs starts sooner.** Its archived discs are unpacked side by side instead of one after another. Final Fantasy VII, with its first disc as `.bin/.cue` and two more as `.7z`, waited two minutes to start from a USB drive before its first disc could play.
- **An encrypted 3DS game's page says so before Play is pressed**, in the same words as Play: no server can play that copy, so it comes before anything this server lacks. A copy you decrypted yourself, from a console you own, will play.
- **Ready for a Mac's graphics card.** When WittyPlay's runner can draw on a Mac's card, PSP, PlayStation and PlayStation 2 games on Play! use it, at full speed on a fraction of a processor core. Nintendo 64 and LRPS2 stay drawn by the processor there, as their card renderers draw black pictures or nothing on a Mac, and the game page says why.
- **On a phone, the website and the apps are one design.** The row of sections at the top gains a frosted surface once the page scrolls under it, instead of floating over the cards, and the activity button no longer cuts off its last name. Pages below Home, a library or a tab have a round Back button in its place. A film's or show's page has one row of actions, Play, Save and "⋯" for the rest, and its picture fades into the page with no line under it. A library's tools (All films, Fix matches, Categories, Scan) are behind one "⋯" beside Filters instead of taking two rows. Search shows three posters across. The avatar in the bar opens **You**, as in the apps: your name and Switch profile, then everything about your account, this device and, for an owner, the server, as grouped lists that lead to each page, then this server and Sign out. Settings is a grouped list with coloured icons. Pages load as grey shapes of what's coming, and a title with no picture shows its name on its own colour. Wider screens are unchanged.
- **Every core says which server platforms it runs on, and how it draws on yours.** Settings → Consoles & cores shows it for each core, the core choices say how each draws here, and a game the server can't run says which servers would. Every device plays what the server can run, since the server streams it.
- **No more broken core downloads.** A core that isn't published for your server's platform isn't offered to install, and says what plays the same console instead: Beetle Supafaust has no macOS build, so a Mac suggests bsnes 2014 Performance. A weekly check finds a download that's gone before anyone meets it.
- **PlayStation 2 plays on a Mac**, drawn by the processor like Nintendo 64 and PSP, with your own BIOS. On an Apple M3 Pro it ran mostly at full speed, slower in places, and the game's page says so first.
- **BIOS files are found by what they are, not what they're called.** Keep them in a `BIOS` folder beside your games under any name: the server recognises each by its checksum after a library scan or when you press Play, copies what a core needs to its own BIOS folder, and never uses a file whose contents don't match. **Settings → Consoles & cores** shows each one's region, where it came from and which is in use, and a game's page names the file. A PlayStation 2 game starts on a BIOS from its own region when there's one.
- **The game page no longer says a PlayStation 2 BIOS is missing when it's there** under another name than `scph39001.bin`.
- **An encrypted game says what would play:** a copy you decrypted yourself, from a console you own. Nothing here decrypts a game or looks for keys.
- **Nintendo 64 and PSP play on a Mac.** A server without a graphics card games can use, such as a Mac, now has its processor draw these two consoles, at their own resolution. A game's page says so before you press Play, as they can run slower and keep several processor cores busy. On an Apple M3 Pro, Nintendo 64 games ran at full speed, and a demanding PSP game mostly did.
- **Games that can't play on this server say so, and why, instead of crashing.** On a Mac or any server without a usable graphics card, PlayStation 2, Dreamcast and 3DS games now show **Not on this server** with the reason on their page, in the library and in Settings, and Play turns them away before anything starts. Apps get the same answer from the API: `reason: "gpu"` with `message` and `gpuMissing`.
- **One console's games failing no longer takes every game away.** Three games in a row that couldn't start, such as Nintendo 64, PlayStation 2 and PSP on a Mac, counted against WittyPlay itself, which then went out of use for every game on every device. Now only WittyPlay's own helper failing counts. A game that won't start is recorded against its core, and every other console keeps playing. A version that went out of use this way is put back in use once.
- **Stopping the server waits for what it was doing by itself**, such as looking for BIOS files, checking the graphics card or noting channel logos, and for activity still being saved, before it closes its database.
- **Stopping the server waits for Live TV's work too**, such as a channel scan, a guide update or a recording, before it closes its database. Anything still going after 10 seconds is left, and the log says what it was. A recording the stop cuts short keeps what it recorded and says the server stopped during it, where it could look finished before.
- **One way to stream a game to every device.** Browsers, the Apple app and the Android app can all connect to a game through one route, `/play/{id}/stream`, with the same messages over WebRTC and, where UDP can't get through, over a WebSocket. It's version 2 of the stream, chosen from measurements on real games and simulated networks: [docs/GAMING-STREAM.md](docs/GAMING-STREAM.md) has the decision, the numbers and the wire protocol. Several devices of one account can play one game, each holding its own players, and a phone can join as a controller with no picture. The website's player uses it with WittyPlay 0.3.5. Version 1's routes keep working until the apps have moved.
- **TV mode.** The website can be laid out and driven like the Apple TV and Android TV apps, for a TV across the room: Home, Libraries, Games, Live TV and Search along the top with You at the right, a Search with letters on the screen, You in two columns, and Who's watching with large faces. It works with a TV remote, a game controller (the bumpers change tab, Start opens a page's options), a keyboard, and still a mouse or touch. Turn it on in **Settings → Appearance**, with **TV mode** in the account menu, or with `?tv=1`, which the desktop app on a TV and Plasma Bigscreen already use; **Leave TV mode** is in You. It replaces Big screen mode, which only made the words larger: anyone who had that on gets TV mode. With TV mode off, nothing changes.
- **Record is offered only where a channel can be recorded, the same in every app.** The server decides it once and sends `canRecord`, true or false, wherever it describes a channel: the guide, a channel's details, search, reminders and the channel lists. Only channels from a tuner can be recorded, never channels made from your library, and not encrypted channels or ones marked not to be recorded. Recording and series recording refuse anything else with a plain reason, and on the website R no longer tries to record a programme that has finished.
- **Games start on a Mac.** Every game failed with "The emulator crashed while running this game", whatever the core. The server put WittyPlay's helper in Apple's sandbox, and a Mac won't start one sandbox inside another, so the sandbox each game gets of its own couldn't start, and neither could the game. On a Mac, WittyPlay's helper now runs outside the server's sandbox, and each game keeps its own, which is the stricter: no network, no other programs, and nothing to write but its saves. **Settings → Modules** says so. After three such failures the server had taken the WittyPlay version out of use, so games answered "WittyPlay isn't on this server". The first time a Mac server starts with this fix, it puts back in use, once, any version taken out of use because its game runner kept stopping, and the module's history says so. A version that fails that way again stays out of use. Tried on macOS 27 on Apple silicon with the app as the build makes it: a Game Boy game on Gambatte and an NES game on FCEUmm started, and one played in a browser.
- **The Mac app's server can be found and reached on your network.** macOS's Local Network privacy blocked it without asking, so phones, TVs and other computers never listed it under **On your network**. The app and its server had only the signature the linker gives every program, all called "a.out", which macOS can't ask about by name. They're now signed as WittyMediaServer, in the `.pkg` and the `.tar.gz`, and the first time the server reaches the network macOS asks whether it may: choose **Allow**.
- **Apps on the server's own computer find it.** The server's announcements on the home network never reached the computer it runs on, so an app there, or the iPhone simulator on that Mac, could miss it under *On your network*, lose it after a couple of minutes, or keep showing it once it had stopped. Other devices on the network weren't affected.
- **On a Mac, choosing Allow is enough: no restart.** When macOS asks whether the server may use the local network, what it had opened before the answer stayed blocked after **Allow**, so nothing found it until it was restarted. It now notices it can hear nothing and opens again, within a minute, and its log says when it can't hear the local network.

## 0.6.2-alpha

- **The Windows installer updates an installed copy again.** Installing 0.6.1-alpha's `.msi` over 0.6.0-alpha stopped with "The error code is 2613" and put the old version back. The step that stops a running copy was placed where Windows Installer doesn't allow it, which only shows when one installer version replaces another. Every release's Windows installer is now tested installing over the one before, for everyone, for one account and as a service, before it's published.

## 0.6.1-alpha

- **DVDs and Blu-rays in Firefox.** Menus, intros, trailers and films no longer show shifted down or cut off, and a menu's buttons line up with its picture again, at any display scaling and in the mini player. The picture is now placed in the same rectangle as the buttons, in the disc's own shape, instead of being left for the browser to fit, which Firefox got wrong. The fix is in the server's web app, so it comes with this server version: no module needs updating.
- **A stream link is accepted only exactly as it was given out.** Another spelling of the same signature, with spare bits at its end set, used to be read as the same link. It couldn't open anything the real link couldn't, but a link now has one form only.

## 0.6.0-alpha

- **Nintendo 64 homebrew built with libdragon plays.** It goes to ParaLLEl N64 with the settings it needs, on the CPU alone. Without that core, the message says to install it, rather than that the emulator crashed. Some newer homebrew still stops on its own error screen, and the game says so when it starts.
- **Slow core installs finish.** Every request that wasn't a read was cancelled after two minutes, however well it was going, which cut off core installs on slow connections.
- **Lighter on a small server.** Checking a library that hasn't changed takes a tenth of the processor it did: about 1 second instead of 10 for 43,000 files, and it happens every six hours. First scans of films and shows use half the processor, as a title's picture now comes from the nearest keyframe (on a 7 GB film, 110 ms and 77 MB instead of 2.5 s and 219 MB). A library page of one library is about 80 times quicker to look up in the database on a server with a big music library. An idle server wakes a quarter as often and starts with less memory. The install is 6 MB smaller.
- **Quicker pages, especially away from home.** Lists are sent compressed, so a page of a library is a tenth of the size to fetch. The first page no longer brings the code for game pages, adding a library or the player's controls, and the video engine waits until something plays: the TV guide and photo pages are 180 KB lighter.
- **A damaged picture no longer costs two minutes of a whole processor core every time it's looked at.** A `.jxl` file that isn't really JPEG XL isn't listed any more, and a picture that can't be made isn't tried again for an hour.
- **Thumbnails, disc stills and playlist files have limits on disk** (4 GB, 1 GB and 64 MB), and what's oldest goes. Anything removed is made again if it's asked for.
- **Fixed:** a machine too busy to answer FFmpeg's questions at the wrong moment left HDR flat and dim until the next restart.

- **WittyTV with a remote:** arrows change channel again after the first press, as focus no longer lands on the player's buttons. Back from the player to the guide can't lead back into the player. Down from the bar above the guide's grid always reaches the grid, and Up from below it does too, with whatever is lined up around it. The guide's first focus is Watch full screen. Coming back to a page that's slow to load, focus still finds its way, and OK with nothing focused goes to the page's main action rather than doing nothing.
- **Controllers:** a DualSense's D-pad works in Android's web view, where it reports no layout. An app can offer a way out for when Back has nothing left to go back through.
- **Big screen mode:** a long name of the server's own wraps in the side menu rather than being cut off, and the menu has no sideways scrollbar. A web view's own choice of focus no longer stands in for a page's first focus.
- **Games on a current Linux.** melonDS, Play!, SwanStation and PPSSPP now start: each failed for its own reason, none of them a game's fault, and nothing was loosened in the sandbox to do it. A core that needs shared memory has a private folder of its own instead of the system's. PlayStation and Dreamcast discs whose sheets name their files in the wrong capitals play on Linux as they did on Windows, and Dreamcast memory cards now last, where a save to one used to be lost. Every core offered was played through the real stream under the sandbox.
- **3DS packages.** An unencrypted `.cia` plays, from a small test program built for the purpose, with or without the marks and ticket a decrypted package keeps. Its program is copied out once and kept for next time, where a big game used to be copied at every start. Encrypted files are refused exactly as before: `wittyplay-probe cia-info` prints the bytes that say which one a file is.
- **WittyTV has themes of its own, apart from the app's theme.** Modern, Classic, Satellite, Teletext, Newspaper, Big picture, Outline, Compact and Custom replace the two looks and five colour sets, each drawn as a small picture of itself and each really built differently. They cover the guide, the player's banner and mini guide, recordings and the other TV pages.
  - Modern, Big picture, Outline and Compact take their colours from your app theme, and say so. Any colour can be overridden, per theme, with a way back, and it stays readable whatever you pick. Custom keeps your own layout, type and colours.
  - **Look** on the guide and **Settings, Appearance** both have it, and say which wins: your choice, then what your app theme suggests, then the owner's default for new viewers. Looks saved earlier moved over as they were.
  - Every theme has real room above its first line, at every size, and clears TV overscan across the room. The guide now fills the space under the app's bars, whatever else is showing there.
  - The preview starts well above its old 320 pixels: a share of the window that suits the theme, up to half the width in Big picture, and the whole width on a phone. A size you chose stays yours, and it can still be dragged, moved with the arrow keys or chosen by name.
- **Logos, anywhere one is needed.** Tuner and playlist channels now find theirs by name, only exact matches take one by themselves, and a person can put a wrong one right, per channel, through new channel lists. For a network, studio, game studio, console, streaming service or channel you can search by name, choose among results with their source and licence in view, keep your own upload, or choose none. **Settings, Metadata agents, Logos** finds any of them. Until you've agreed to downloads, these places explain and link to the one agreement.
  - Every screen with a logo now has its rights notice, including shows, titles and the TV player. The list of where logos came from is complete, and everyone signed in can read it under **Legal**.

- **Security, second pass over what this release added.** Attacks were tried against the store, backups, support bundles and game cores:
  - Restoring a backup on a new server asks for the setup code, as setting one up does. A stranger on your network could otherwise get
    there first and restore a backup with an account of their own.
  - A module from another store now obeys `WMS_NO_MODULE_FORKS` and the refusal on systems that can't box it in, and a store you turn off can't
    install anything or be fetched. It keeps at most 64 MB of pictures. The key you were shown is the key that's pinned.
  - Looking at a store on your own network, and giving Tdarr a new address, ask for your password: what comes back says what's listening there.
  - Passphrases, several kinds of API key and `password: x` as a program prints it are taken out of logs. One log file or crash report goes
    through the same check as a bundle, and only asking in so many words turns sanitizing off.
  - An administrator's uninstall doesn't follow links left in other people's folders, and the Windows and Mac installers won't set rules
    through a link or junction someone left where the data folder goes.
- **What a module is held to, in plain words.** Helpers had no limits at all on FreeBSD or a Mac. On FreeBSD they now can't start programs, open a
  socket or write into a file, and as root they also go in a jail. On a Mac they run in Apple's sandbox, which hasn't been run on a real Mac yet. Neither
  is a full sandbox, so modules from other publishers stay refused there. Settings, Modules and the store now say what a helper is held to wherever it's
  short of one, Windows included.
- **PlayStation 2 runs under the sandbox on Linux,** where its core used to crash on loading. It keeps its shader cache between games, and backups leave it out.
- On a Mac, a game's sandbox refused to start the game at all. Found by reading, as this hasn't been run on a real Mac.
Modules are now separate from the server, with a store to find them in; there are real installers and uninstallers on each system; and help is built into the app. TV, music, games, metadata and the look of everything have been rebuilt.

### Before you update: what you'll notice

Most of these come from a security review. [Config file and folders](docs/CONFIG.md) explains every setting named here.

**Modules and programs**

- **Modules are downloads now.** The server starts with none it doesn't need and offers each one when a page asks for it. Modules from before this release can't run: the store offers the current version.
- **Scripts that run `wms` need the new name,** `wittymediaserver`. A small `wms` stays in the `.zip` and `.tar.gz` for this release. `wms.json`, `WMS_` settings and your data folders haven't moved.

**Setting up and choosing folders**

- **Setting up from another device needs the setup code,** which is on the server's console, in `docker logs` and in `setup-code.txt` in the data folder. `setupFromHome` goes back to not asking at home.
- **Network shares and device paths can't be chosen from another device** unless `allowNetworkPaths` is set. Libraries already on a share keep working.
- **The Java for Blu-ray menus is set in the config file** (`javaPath`, `WMS_JAVA`), no longer in Settings. A location saved earlier is ignored.

**Live TV**

- **A playlist from the internet can only hold web streams,** http or https. RTSP, RTMP, SRT, UDP and DASH channels in one stop working; add them as single streams. A name that resolves differently at home (split DNS) counts as the internet, so its channels on your own network stop: use the private address or a `.local` name.

**Games, modules and themes**

- **Custom game cores need `WMS_GAMING_CUSTOM_CORES=1`** on the machine. If you use them, set this after updating.
- **`WMS_NO_MODULE_FORKS=1`** keeps forks of modules off, whatever an administrator asks.
- **Themes can't load images or fonts from other sites.** A theme that does is refused. Put them in its ZIP.

**Accounts**

- **Recordings belong to each person.** An administrator sees all of them.
- **A Guest profile can't upload games, and only adults can add patches.**
- **Only an album's owner can make a link to it.**

**Limits you might reach**

- **Connections:** 2048 in all and 128 from one address. `maxConnections` changes the total. Docker Desktop makes everyone look like one address, so it reaches the second limit sooner.
- **Stalled clients** are let go after 2 minutes, or 15 for a file being played.
- **Requests:** JSON bodies are limited to 1 MB, and anything sent without signing in to 16 KB and 15 seconds.
- **Failed sign-ins** are counted until six hours of quiet.
- **Per person:** 50 recordings waiting and 20 series, a 100 GB upload total (in **Settings, Sharing & photos**), 5000 comments, 200 collections, 200 albums and 4 open discs. Opening a fifth closes your own oldest.
- **NFO files** can be at most 1 MB.

**FFmpeg**

- **The bundled FFmpeg moves from a rolling build to the 9.0.2 release** on Windows, Linux and macOS, and only reads library files as real media formats. 32-bit Linux packages keep the newest build there is for them, which was 7.0.2 when this was checked.

### What hasn't been run yet

This is an alpha, so here's what has and hasn't been tried.

- **Windows:** the `.msi` was built and tested under Wine, not on a real Windows machine. ARM64 wasn't checked.
- **macOS:** never built or run here. That covers the `.pkg`, the uninstaller, modules and the sandbox written for a helper.
- **FreeBSD:** the amd64 package and service passed in virtual machines, and a helper's limits were tried from inside one. No real hardware and no arm64.
- **Apps:** Android ran on emulators only, including Android TV. iOS and tvOS have never been built.
- **Game delay figures** were measured with the server and browser on one machine, not from a real screen. Firefox, Safari, phones, TVs and real networks weren't run.
- **Tdarr** itself has never been run against this, because downloading it binds you to its licence.

### Modules

- **Repair no longer says a module has a copy in the server.** It said modules carry on with "built-in copies", which stopped being true when modules moved out.
- **The module store:** search by name, description or category, browse by category, and filter by installed, updates,
  publisher and platform, with a page for each module: what it does, screenshots, what's new, what it asks for, what it's
  built on and which servers it fits. In Settings, Modules, and at `/settings/modules/store`.
- **Update All Modules:** updates everything in order of what's built on what. One failure doesn't stop the rest. Each
  module shows its progress, the run ends with a summary, and it can all be taken back.
- **Errors a person can follow:** every failed install or update says what happened and what to try, with Show details,
  the module's own log lines, a copy button and a link to the repair tools. Each module has its own log view.
- **What's new:** the latest changes from your modules on the Modules page and each module's page, and a plain line when
  there's nothing to report.
- **Other stores:** add a store run by someone else, after a plain warning about what a module can do, who holds its key
  and that nobody here reviewed it. Its key is pinned, and a change is refused loudly. Everything from one is labelled, and
  it can be turned off or removed.
- **Which modules run:** modules start with this release. One built for an older server can't run, and the store and the
  installed list say why and offer the current version. Later servers may keep running a module built for this release or
  after, as a limited courtesy that isn't promised until the beta.
- **Modules are completely separate from the server.** The server has none of their code in it, starts and is fully
  usable with none of them, and a page whose module is missing says so and offers to add it. Adding, updating, removing or
  turning one off needs no new server and no restart. Each runs as a program of its own in a sandbox, or in the browser, and
  is checked against its signature before anything runs.
- **Modules start here.** They work with this release and later ones, and nothing older. A version built for less is
  refused, and the server says why. Limited backward compatibility may exist unofficially, but nothing promises it.
- **A signed catalogue** lists every module and its versions, from a new repository, WittyModuleCatalog. A release of the server
  can carry a starter set of signed modules for a machine with no network.
- **Each module carries the licenses of its own libraries,** which a server shows at `GET /api/modules/{id}/notices`.
- **Every module with a program comes for Windows, Linux, macOS and FreeBSD.** One with no build for a system says that in plain
  words in place of the button to add it, and so does anything built on it. WittyCouch Host has no FreeBSD build yet, so WittyCouch
  says that on FreeBSD.
- **`wms dvd`, `wms bluray` and `wms cd` are gone,** along with the built-in copy of each module. The disc tools are for developers now, in those modules' repositories.
- **Every module and app is now a project of its own.** WittyPlay, WittyDVD, WittyBluray, WittyCD, WittyDisc, WittyTuner, WittyPhotos,
  WittyCouch, WittyCouchHost, WittyRigs, WittyIntegratedFileBrowser, WittyModule, the reader plugins, WittyVisualizer, WittyReader,
  WittyMediaPlayer and WittyMediaClient each have their own repository, history, changelog and releases. The older `clients/desktop`
  and `clients/mobile` shells are retired: WittyMediaClient replaces them.

### First-time setup

Rebuilt so someone who's never done this can finish it, with a keyboard, a remote, a phone or a screen reader.

- **A step belongs to what needs it.** The games and disc pages appear only with their modules, lyrics only where music can be played, and each page says where in Settings to change its answer.
- **Restore from a backup** is one of the two buttons on the first page. Restoring from there now moves on by itself when the server comes back, where it used to sit on its last message.
- **Choosing modules** downloads them through the store's own install, one at a time with progress, and says plainly what didn't work. It no longer says nothing is downloaded. It also asks what to do when a module has a new version.
- **Logos, lyrics, Wikipedia and streaming lists** are each a question of their own, off until answered, with what's sent and who owns what said beside them.
- **Setting up from another device:** the page says where the setup code is, on each system. **A share chosen from another device** lays out the three ways forward as soon as it's typed.
- **Posters beside your files:** setup now asks. It's still on unless you untick it.
- **Remotes:** every step has an address, so Back goes back a step, and OK now ticks an ordinary checkbox or chooses a radio button anywhere in the app. Before, those only answered to the space bar, which a remote doesn't have.
- **The last page** reads what you chose back from the server, rather than from what was pressed.

### Help

- **Pictures, all through the guides and on the project's pages,** taken from a real server full of openly licensed films, music, books and games, with a credit under each one that shows someone's work. Help shows small copies, loaded only as they come into view.
- **First-time setup, step by step:** a new guide with a picture of every page.
- **Help, built in.** Every guide is in the app under **Help**, in the account menu, at the top of each settings page and on the sign-in page, with search. It works with no internet connection, and anyone can read it. Nothing sends you to GitHub to find out how something works any more.
- **Guides rewritten** against the code, with new ones for libraries, accounts, apps, playing, a tour of Settings, what each media type supports, lyrics and karaoke, books, games, the metadata folder, uninstalling, the module store and Tdarr.

### Installing and uninstalling

- **The download page names WittyMediaClient.** The README, [Installing](docs/INSTALL.md) and each release now say which file is the server, and send phones, TVs and other computers to a browser or the app.
- **Real installers, on every system.** Windows gets a normal `.msi` setup: just for you or for everyone, Start menu entry, optional background service,
  firewall rule, start at sign-in, desktop shortcut and command line, each asked for in plain words, with silent flags for admins, and updating in place.
  macOS gets a `.pkg` with a background service. Linux gets a `.deb` and an `.rpm` with a systemd service and a layout ready for a package repository. Each
  takes over from a copy that 0.5.2-alpha or older installed, keeping your library. These replace `wms-setup.exe` and `install.ps1`. FreeBSD gets a `.pkg` with an
  rc.d service, and `install.sh` still works there and on any other Linux.
- **Uninstalling removes everything, if you ask.** Windows' Installed apps, **Uninstall WittyMediaServer** on a Mac, and `wittymediaserver --uninstall` anywhere, FreeBSD included,
  ask whether to keep your library, accounts and settings. A full clean-up takes the program, services, tasks, firewall rules, startup entries, shortcuts,
  registry keys, data, caches, logs and downloaded modules, then lists what it removed and what it couldn't. It never touches your media or anything it didn't
  make, and recorded TV and backups have a question each.
- **Programs you can tell apart.** You open `WittyMediaServer`. Everything else is in a `bin` folder and named for its job: `wms` is now
  `bin/wittymediaserver`, and `wittyplay-host` is `bin/wittyplay-game-runner`. On Windows each says what it is in its Properties and in Task Manager. Scripts that
  run `wms` need the new name; a small `wms` stays in the `.zip` and `.tar.gz` for this release. In Docker the command is `wittymediaserver`, and the compose service
  is too. `wms.json`, the `WMS_` settings and your data folders haven't moved.
- **The tray app shows a server that's already running**, such as a background service, instead of failing to start a second one.
- **Windows' background service is a real service,** under a limited account, in place of a boot task that ran as SYSTEM.
- Test builds are no longer signed with the release key unless `SIGN=1` says it's a real release.

### WittyTV

- **WittyTV guide:** rebuilt around the highlighted programme: channel, title, times, how far in, what's next, and Watch
  full screen, Start over, Open the film, favourite, channel up and down, and the whole day. Filters for favourites and
  categories, jumps to tonight or tomorrow, a search across the schedule, reminders, a mini guide over the player, and
  an *On TV now* row on Home. Very short programmes are handled properly in the rows.
- **WittyTV preview:** resize it by dragging its corner, with the keyboard or remote, or to a named size. It sits beside
  the programme rather than over the guide, so at any size it covers nothing, and it can be hidden. It's remembered for
  your account.
- **WittyTV looks:** the classic cable box stays, joined by a modern look, Midnight colours, your own two colours, a
  colour for each channel, and a theme can suggest a look.
- **WittyTV discs:** a channel can play a disc's main feature or scenes you pick by chapter, from thumbnails, and fine-tune
  to the second. DVDs and Blu-rays alike. Encrypted discs stay refused, with the usual message.
- **WittyTV bumpers:** a bumper library (upload, or choose from your media), and rules per channel: before or after a
  programme, between commercials, or a break every so many minutes, mandatory or only for particular films, shows,
  genres or collections.
- **WittyTV timetable:** "cut commercials short" keeps programmes starting on time, with clear choices about what may give
  way, and the editor shows the whole day with what was cut. Commercial breaks can now come inside a programme, and give
  way the same way.
- **WittyTV logos:** channels wear their network's or studio's logo from the logo service, or one you pick, and the modern
  look shows a title's own logo art.
- **Fixed:** uploaded bumpers keep playing when the data folder moves, as after restoring a backup somewhere else.
- **WittyTV channels:** one click for studios and your collections as well as genres, decades and networks, with *Add all*.
  Holiday seasons and marathon blocks. Channels you already have carry on as they were.

### Titles, posters and details

- **Edit details:** one dialog for a film, an episode or a show, in sections, with Delete, Cancel and Save always in view. Every field says where its value came from, can be locked so nothing changes it (no scan, refresh, new match, NFO or file beside it, matched or not), and can go back to what its source said. It also has artwork from every source with your own uploads, banners and title logos, where the intro ends (typed, or picked in the player), bonus features, versions, moving to another library, and logos.
- **Search and match:** every source at once, or a TMDB, IMDb, TheTVDB, TVmaze, AniList or Wikidata id or address. Each result says how likely it is and why. Choosing one shows what would change, field by field, before anything does. A match you choose stays through scans and refreshes, and a wrong one can be removed, keeping only what you typed.
- **Refresh shows what will change** before it changes anything, and keeps the match a title has. It used to search again and could lose it.
- **Fix matches:** a list of everything that needs a look, worst first, with suggestions and reasons, a key for each action, several at once, editing many titles together, renumbering episodes, naming rules and ids in file names, and a history where anything can be undone.
- **Studios and networks:** first-class data with a row of logo cards and a page of everything from each. Logos are an optional download from Wikimedia Commons, only with your agreement, kept and served by your own server, in white and dark so they read on any theme. Without them, the names look right as text.
- **Categories:** made from genres per library, with spellings of the same genre put together, and they can be hidden, reordered, renamed and merged, with art from what's in them.
- **Browse by anything:** type, category, studio, network, year, decade, age rating, score, picture, sound, audio and subtitle language, edition, tags and whether it's been watched, sorted by Shuffle, Recently added, Title, Newest, Oldest, Highest rated or Best quality. Views can be saved and pinned to Home.
- **Streaming listings:** optional, off until you accept each statement of what it is. Sorts the media on your server by what services currently list it, with a date and credit. Nothing plays from those services.
- **Collections from film series:** when a source says films belong to a series, a library shows them as a collection by itself, and the Collection field changes it.
- **A library's header stays in view** as you scroll, with its name, sort and Filters.
- **Arrow keys and a remote reach everything** in the edit dialog, Fix matches and Pick in player, and carry on from nearby when the button you pressed goes away.
- **One library header** on film, TV, disc, music, audiobook and book libraries, with sort direction everywhere.

### Music

- **A new front page for music, audiobooks and podcasts:** covers to flip through with their reflections, artists as round photos, albums and songs as shelves with **See all**, and colours taken from the artwork. **Play my music** shuffles it all.
- **Full-screen player:** artwork with a visualizer behind it, the full transport, and volume, playlists, a sleep timer, the queue and likes along the top. Keys for everything, and arrows alone reach every control.
- **The bar along the bottom** is new: floating, with shuffle, repeat, like, volume and the queue, and it collapses to a small button. It leaves room under the page, and steps behind the account menu.
- **Lyrics that follow the music.** From a `.lrc` file beside the song, the song's own tags, or, if the owner agrees, LRCLIB, which is free and needs no key. A paid Musixmatch key or your own LRCLIB server can be added. Press a line to go to it, nudge the timing for yourself, or correct it for everyone with **Tap along**.
- **Karaoke:** big words that fill in as they're sung, a count-in, the next line waiting, the voice turned down, and the key moved up or down. The screen says what the voice reduction can and can't do.
- **Visualizer:** Bars, Waves and Drift are on behind the artwork in the full-screen player, with a large button and a **Slow** switch. **It no longer changes effect by itself** unless you choose an interval, now up to every 10 minutes.
- **Sound:** no gaps between songs, crossfade, ReplayGain and a ten-band equaliser.
- **What's playing comes back after a reload,** stopped where it was, with the queue, shuffle and repeat, and it waits to be asked before fetching anything.
- **The queue can be edited as it plays,** and saved as a playlist. Shuffle now really reorders it. **Liked songs, Most played, Played lately** and **stations** made from a song.
- **Artist pages,** with discography, and a biography and photo from Wikipedia if the owner agrees. An `artist.jpg` in their folder comes first. Multi-disc albums are divided by disc, with other versions of an album offered below it.
- **Read:** a booklet or text file beside an album, from the full-screen player.

### Games

- **Games:** far less delay. From a press to the screen went from 75 to 270 ms to 11 to 31 ms, or about 50 on PlayStation 2, measured with the server and browser on one machine. Bands and a grid in some PlayStation 2 games are gone, colours are right, and streams come back by themselves from a dropped network or a stuck encoder. The on-screen numbers show where the delay goes.
- **Game libraries know what's a game.** A file's own bytes now decide which console it's for, and whether it's a game at all: headers, boot sectors, `SYSTEM.CNF` and `PARAM.SFO`,
  even inside a compressed `.cso`. A readme called `.md`, a BIOS, a copier's firmware, an update or a music CD no longer becomes a game, and a PlayStation 2 disc in the wrong folder lands in the right console.
- **Needs a look.** What can't be proven doesn't become a game. It goes on a list on the library's page with its reason and a one-click fix, beside a list of what was left out and why.
- **One game, however it's kept.** Discs come together as one game, in folders, in archives or a mix, with the disc changed from the game's menu. Copies are found by checksum. Regions and revisions
  share a tile, with each still its own game. A set's best version plays, and another can be chosen.
- **Fix this game**, on its page: the wrong console, not a game, which version, split a version off, and why the server believes what it does.
- **Importing thousands of games** takes no attention. It runs in the background, survives a restart and shows how far it's got. Every game gets its name, checksum match and box art from the local
  database first, in about a minute, before any online source is asked. A fan's hack isn't named after the game it was made from, and a set's right region is what's named.
- **PlayStation 2 saves work.** The core had nowhere it could write its memory cards, so games said no card was inserted. Each account now has its own cards for each game.
- **Which version comes first** is yours to set, in Settings, Consoles & cores: USA first isn't right for everyone. It decides which file of an archive plays, which version leads a tile and whose names are used. Games someone has already played keep the version they have.
- **A title you typed stays.** The first scan after an upgrade goes over every game, and could put the file's name back.
- **3DS:** whether a file is encrypted now comes from its bytes, and an encrypted one says so on its page before you press Play. New 3DS games tell the core to be one. Updates and extra content aren't listed as games.
- **Console and studio marks:** logos for consoles and game studios from the logo service, optional as all logos are, and a typed label that looks right without them.
- **A full server says so straight away,** rather than unpacking and patching a game first.
- **A new home for your games.** A hero of covers on a shiny floor with the page tinted by the cover at the front, Continue
  playing with a picture of where you left off, Consoles with their game counts, then recently added, favourites, most played
  and shelves by genre and series. Sort by most popular, newest first, oldest first or A to Z, filter, and jump to a letter.
  Any number of games stays quick: only what's on screen is ever on the page.
- **A new page for each game**: its art, screenshots, year, publisher and players, save states with pictures, cheats, controls,
  and what it needs to play. If something's stopping it from starting, the page says so and takes you to the fix.
- **A new game menu**, with large tiles for save, load, rewind, fast forward, screenshots, controllers, settings and quit, which
  work the same by finger, keyboard, remote or controller. Rewind can be turned on from the menu.
- **A loading screen that says what's happening**, and error screens that say what to do about it.
- **Touch controls redrawn for each console**, with one tap to hide them, and they step aside when a controller is connected.
  The editor can resize them all at once.
- **Couch mode**: the games on their own, full screen and large, with an on-screen keyboard for searching.
- **Button prompts** that match the controller in your hands: letters, shapes or a keyboard's keys.
- **Settings, Consoles & cores** now starts with the consoles you have games for, whether each is ready, and the one thing to do
  if it isn't. There's a button to install every recommended core at once. Everything else is under Advanced.
- Game PCs and gaming PC libraries match the rest, and a gaming PC's library is no longer limited to 500 games.
- **Controllers and My game settings look like the rest of the app**, on every width: tabs, chips and cards from the app's own set,
  button prompts, and larger targets on a phone or from across the room. Choosing a console is a row of chips rather than a drop-down
  list, which a remote can use.

### Every format

- **Every format, everywhere.** One list of file types now drives scanning, playback, serving, the file browser and the
  docs. It covers over 200 extensions: every common video container, codec and raw stream, FLAC to DSD to tracker and game
  music, MIDI through a synthesiser of the server's own, RAW from every major camera, Kindle, FictionBook, Word, Rich Text and
  Markdown in the reader, and RAR, 7-Zip and TAR comics. See **Settings, About, Supported formats**.
- **A file's content decides.** A file with a wrong or missing extension is recognised from its first bytes; a program
  called `film.mkv` is left out and the log says why; a `.cbz` that's really a RAR still opens.
- **Nothing is converted that needn't be.** Browsers now get rewrapped streams, and streams with only the sound converted, so
  an H.264 MKV costs the server next to nothing. The browser says what it really decodes, so HEVC plays directly where it can.
  The new **ⓘ** in the player says how a file is playing, and why.
- **Subtitles beside a video** are found by themselves: SRT, ASS, SSA, WebVTT, MicroDVD, SubViewer, SBV, TTML, DFXP, SAMI,
  LRC, MKS, SCC and more as text, whatever encoding they're in, and VobSub and PGS drawn into the picture. Text no longer forces
  a conversion. ASS can still be drawn in, to keep its styling.
- **Films split across files** (`CD1`, `part2`) are one title. **Cue sheets** split any sound file into tracks, not only WAV
  and FLAC. **Playlists** in a music library (M3U, PLS, XSPF, ASX) become collections.
- **Motion photos** with their clip inside the picture play it. **SVG pictures** are shown, with anything active taken out first.
- Cover art at the front of a video no longer gets taken for the picture. Mono sound that FFmpeg's AAC encoder refused now
  converts. DVDs in `.img` files, which FFmpeg took for pictures, get their thumbnails and play.

### Library transcoding and Tdarr

- **Library transcoding.** Convert a library's files ahead of time, under **Settings, Library transcoding**. Three plain presets (play on any
  device, save space, keep the best quality), with every detail open to change: sound tracks and subtitles by language, commentary, audio description, a
  stereo track beside surround, lossless and object sound, forced and picture subtitles, HDR, Dolby Vision, 3D, files with several pictures, and more. A
  preview says what becomes of each track of a file, a dry run says what becomes of a whole library, and a 20-second test measures this machine. Each new file is checked before
  anything happens to the old one, which you can keep, replace or leave in use, and put back later. What's been watched and your edits stay. It's off until you've read how
  it affects streaming, and can hold still while people watch or outside chosen hours. Copy-protected files are refused. It shows in the activity panel, with pause, cancel and retry. See
  [the guide](docs/LIBRARY-TRANSCODING.md).
- **New files can convert by themselves.** Turn it on for a library and, after a scan finds new files, each is left until it stops changing, then converted within the same limits, showing in the activity panel. A file that fails isn't tried again until it changes.
- **HDR that doesn't mark its own frames** no longer stops live playback with "no path between colorspaces". It's made an ordinary picture like any other.
- **Connect your own Tdarr.** See its nodes and what they're working on in the activity panel, and hand it libraries, over its documented API. Tdarr is a separate program under its own
  licence, and isn't included.
- **The Transcoding page** is now **Live transcoding**, to tell it apart. Its controls have been checked, its text made plain, and a claim that a resolution limit leaves direct play alone,
  which wasn't true, put right.
- **Libraries can be read-only**: nothing the server does may change their files.

### A new look, the same everywhere

- **Design standards**: [the standards](docs/DESIGN.md) now covers every colour, size, part, pattern and rule, and a check in CI keeps the interface to it. Administrators can see every part at `/design`.
- **Shared parts** for buttons, fields, menus, dialogs, toasts, tooltips, cards and tables, which all work with a keyboard, a remote and a screen reader. Settings and the pages around it moved onto them, and "are you sure?" questions are now the app's own instead of the browser's.
- **Themes**: eight well-made themes in place of twenty colour variations, each shown as a small live piece of the app. If you were using one of the old ones, you've been moved to the closest. **Make a theme** builds a whole readable theme from a few choices, with no code. [The guide](docs/THEMES.md) is rewritten for someone who has never written CSS.
- **The top bar stays manageable**: what doesn't fit goes under **More**, and you can see the page you're on even when it's in there. Drag or use the keyboard to reorder, hide what you don't use, and group into Watch, Listen, Read and Play. The side menu folds to a rail of icons. Everyone can have their own arrangement, and the owner can set the default.
- **Phones** get a row of sections along the top and a floating bar at the bottom.
- **Remote and keyboard**: arrow keys reach everything, coming back to a row lands where you left it, and Back returns to the control you left. **Big screen mode** in Appearance, or `?tv=1`, keeps the larger text and stronger focus on for a TV.
- **Moving posters**: movie and show posters lean toward the pointer with a soft moving light, or settle in as a remote reaches them. Turn it off in Appearance; it also stays off on modest devices and when a device asks for less motion.
- **Profile names and usernames can differ**: you sign in with a unique username, and everyone else sees a profile name anyone can choose. Each person picks which one shows at the top right. Usernames now count as taken whatever their case, and can't contain invisible characters.
- **Posters are larger,** about a seventh of the window on a wide screen, and a library's name, sorts and Filters stay tidy at any width.
- **Across the room:** a page starts on its main action, the side menu and top bar grow with the type, a row's heading leads into its row, and going down a page no longer falls into the side menu.
- **On a phone:** every control is at least 44 pixels to the finger, words over artwork stay readable, and rows of people and buttons no longer squeeze a name to nothing.
- **Asking first is the app's own dialog everywhere,** including deleting a channel or recording and removing a tuner, so a remote can always answer. The dialog that asks for your password is the same one.
- An address that isn't a page now says so, with a way home. A library's folder keeps its leading slash on screen.
- **Module store:** a library such as WittyDisc says it comes with other modules, Modules and the store both say Install and show each module's own picture, and a change refused because another is running says so plainly.

### Activity and statistics

- **An activity button in the top bar.** A ring and a count show library scans and other background work as they happen, and it
  stays quiet when nothing is going on. Its panel lists each task library by library, with the file being worked on, the speed, the
  time left, and pause, resume, cancel and try again. A history keeps what finished, with errors that say what happened and what
  to do. It works from the keyboard and with a remote, and is a sheet on phones. Administrators can hide it for themselves, and
  the owner can turn it off or give others a read-only view with no file names and nobody else's details, in **Settings, Activity**.
- **One place for background work.** Scans, game details, logos, streaming listings, match suggestions, radio checks, module installs,
  server updates, the TV guide, channel scans, recordings, WittyDiagnoser and transcodes all report to it, live, with no polling.
  Modules can report too, and apps can follow it through the API: see [Background tasks](docs/TASKS.md).
- **Server statistics, and the WittyStats module.** A screen you arrange yourself: processor per core, memory, graphics, each disk,
  each network connection, the server itself, who is watching what and why it's converting, what's in your libraries, duplicates, scan times, the
  log over time and more. Add, move and resize widgets by drag or by keyboard, keep several layouts, or go back to stock. Any chart can
  be a table, and any widget opens into more detail with CSV and JSON export. The server only samples while someone is looking, and keeps a
  small history for charts. Apps get the same numbers: see [Server statistics](docs/STATS.md).
- **Apps:** a live channel can now be played by a player that can't set headers, which is every Apple TV: `GET /tv/{key}/tune?streamToken=1` adds an address with a stream token for that channel alone. `/capabilities` says the oldest version of each app it works with (`minClientVersions`), names what has landed since the last revision, and lists games and photos, which it had left out. The API guide says how compatibility is decided, and how live channels work.

### Backups, repair and logs

- **Backup and restore:** *Settings → Backup* and `wittymediaserver backup` make one ZIP of your database, settings, metadata, artwork, accounts, watch history, channels, game saves and module settings, with a tick for each, and a size estimate first. It runs in the background while the server carries on, and shows in the activity panel. Your media is never in it, and neither is any program.
- **Real encryption:** a password seals the whole backup with AES-256-GCM, with the key made by Argon2id, and brings your API keys and two-factor sign-in along. With no password, secrets are left out.
- **Scheduled backups:** every day or week, keeping the newest few.
- **Restore,** from Settings, the command line, or first-time setup, so a reset no longer means setting everything up again. An archive is treated as hostile: every file is checked first, and anything that tries to leave its folder, a link, a ZIP bomb, a program, a picture that isn't one or a tampered database is refused, saying why. It shows who could sign in and which folders it would use before you agree. What's there now moves to a safety copy, goes back by itself if anything fails, and can be put back later. Every sign-in ends, on purpose.
- **Metadata files:** *Settings → Repair → Metadata files* writes a plain text file for each film, show, episode, disc, home video, book and game, to fix by hand in any text editor. It shows what your edits would change, field by field, before anything does. Anything wrong is named by file and line, and one bad file doesn't hold up the rest. Changes lock themselves and can be undone in one step. A picture can be dropped beside a file. It works from the command line too, with the server stopped: `wittymediaserver metadata export|check|apply|undo`. See [The metadata folder](docs/METADATA-FOLDER.md).
- **Safer metadata files:** everything read from an NFO, `.metadata.json`, `.info.json`, `.wittycd.json`, or a book's or comic's own details is limited in size and cleaned to plain text: no HTML, scripts, hidden characters or file addresses. `DOCTYPE` and `ENTITY` declarations are refused, and links that lead out of a folder aren't followed. Problems name the file and the line.
- **Repair:** *Settings → Repair* and `wittymediaserver doctor` find and fix problems with metadata, the database, modules and the install, on every platform, even when the server won't start. Each finding says what it found, what the fix does, and whether it's safe. It only looks until you say so, and a safety copy is made before anything changes.
- **A damaged database can be recovered:** a new one is built from everything that can still be read, it says what couldn't be saved, and the damaged file is kept.
- **Moved a drive?** `wittymediaserver doctor remap` or *Settings → Repair* changes where the server looks, after checking the files are really there. Watch progress, matches and edits stay.
- **A forgotten password no longer means a reset.** Whoever manages a profile can give it a new password or PIN under *Settings → Profiles*. An administrator who's locked out, with no second one to ask, runs `wittymediaserver reset-password <username>` on the machine with the server, which keeps everything else. It has no address on the network and isn't in *Settings → Repair*, so it can't be done from another device.
- **Sanitized logs:** *Settings → WittyLog* has **Download logs as ZIP**, with **Sanitize for sharing** right beside it, on by default. IP addresses, computer and web names, network card addresses, account names, emails, file paths, folders, library names and titles become placeholders such as `[path-3]`, the same number all through the file, so a problem can still be followed. It shows the files, how much is replaced, and lines before and after, first. Versions, codecs, status codes, timings, routes and errors stay.
- **`wittymediaserver logs zip` and `wittymediaserver logs sanitize`:** the same with no server running, even with a broken database.
- **Getting to the logs:** every WittyLog tab, a page that fails to load, a crash notice and the page shown when the app itself hits an error now link to the download.

### Security

- **Live TV:** FFmpeg no longer reads an internet stream itself. It reads through a small gate on this machine, so an encryption key that appears in a playing stream is refused within a second, before FFmpeg sees it, and a playlist can't lead the server to your home network. A stream's address and login are no longer on FFmpeg's command line.
- **Fixed, serious:** a web page could claim a fresh server from the browser of anyone at home, or read the list of accounts and sign in as a password-free profile, by sending a made-up header. A page elsewhere never counts as being at home now.
- **Fixed, serious:** a 100-byte picture from any adult's upload could stop the server, and stop it again at every scan.
- **Security:** setup from anywhere but the server's own desktop asks for the setup code, so someone else on your network can't get there first. The code is on the console and in `setup-code.txt` in the data folder; `setupFromHome` goes back to not asking at home.
- **Security:** a file in a library that is really FFmpeg's list format, a playlist, or text no longer opens: FFmpeg only reads library files as real media formats.
- **Security:** the Java for Blu-ray menus is set in the config file (`javaPath`, `WMS_JAVA`) and no longer in Settings. A location saved earlier is ignored.
- **Security:** a playlist from the internet can't send the server to your home network or to a file. Its guide has to be a web address, and its channels web streams. RTSP, RTMP, SRT, UDP and DASH channels in one stop working; add them as single streams.
- **Security:** network shares and device paths can't be chosen from another device, unless `allowNetworkPaths` is set. Libraries already on a share keep working.
- **Security:** a link in a library folder can't hand over files from elsewhere on the machine. Everything that sends a library file now opens it in a way that can't leave the library.
- **Security:** only administrators see file locations and the web addresses of radio stations and podcast episodes, everywhere. Tuner and recording errors no longer show a stream's address or login.
- **Security:** someone without an account can no longer fill the server's memory with unfinished requests: every request sent without signing in is limited to 16 KB and 15 seconds. Ordinary API requests are limited to 1 MB, down from 4.
- **Security:** limits on open connections (2048 in all, 128 from one address, `maxConnections` to change it), with room kept for your home network. A client that stops reading is let go after 2 minutes, or 15 for a file being played.
- **Security:** a game session takes only so many connections, so an account allowed to play can't run the machine out.
- **Security:** helpers can't open a file to other accounts through its access list, and can't reach the kernel's more exposed corners.
- **Sign-in:** a flood of made-up names can't wipe the sign-in limits, one address can't lock a name for everyone else, and counts last until six hours of quiet. Your own password and two-factor code, when asked again, count as they start, so a burst from a stolen session can't get through.
- **Accounts:** names that differ only in case are now taken. Confirming two-factor again changes nothing. Comments, collections and albums have a generous limit per account.
- **Password-free profiles:** their sign-ins now only work at home, those made earlier included.
- **Reverse proxies:** an address a trusted proxy sends that can't be read counts as the internet, never as this machine. Ports and brackets are understood.
- **Pairing:** strangers can't use up the places for pairing or stop a code working at home, and a pending code ends with the sign-in that made it.
- **Administrators:** adding or removing one, or changing another's password, asks for your password and code.
- **Custom game cores:** they now also need `WMS_GAMING_CUSTOM_CORES=1` on the machine. If you use them, set this after updating.
- **Modules:** using a fork, going back to an older version or rolling back asks for your password and code, and is written down. `WMS_NO_MODULE_FORKS=1` keeps forks off.
- **Share links** are removed with what they share. A link to an album stops working when its owner makes the album private, only its owner can link it, and a shared page no longer shows who shared it.
- **Recordings:** each person sees and manages their own, an administrator all of them. Channels that are turned off can't be recorded, one person can have 50 waiting and 20 series, and a recording stops when under 2 GB is left on its disk.
- **Uploads:** each person has a total, 100 GB by default, in Settings, Sharing & photos, and uploads stop when under 2 GB is left on the disk. Downloaded episodes stop with 512 MB left.
- **Games:** a Guest profile can't upload anything, and only adults can add patches.
- **Limits:** a limited account can no longer use a stream or disc session of something it may not see, and show pages, next episodes, album covers and the year in review keep to what it may see. Opening a fifth disc closes your own oldest rather than someone else's.
- **Updates:** a server version can be withdrawn by a signed list, and a server running one says so in Settings. Someone on a full release isn't moved to a test version.
- **Logs:** more kinds of key are hidden, a podcast's address stays out, and apps' reports have a daily budget so nobody can push older lines out.
- **Fixed:** a slow guide host could stop the server saving anything for up to 10 minutes. A huge or unusual file beside the media (an NFO, a metadata file, a pipe) could stop library scans or run the server out of memory: NFO files must now be plain files of at most 1 MB, and JSON metadata files at most 256 KB. A crafted book could hold a processor for days, or take gigabytes of memory to open. A rating or length of "Inf" in a file emptied every list holding that item. A title with `=` or `\` in its file name couldn't have subtitles burned in. A programme called CON or similar failed to record on Windows. With `-listen` set to one address, the local discovery port also answered on the machine's other networks.
- **Containers and services:** the compose file and Kubernetes deployment run with a read-only root, no capabilities and nothing to gain, and the systemd unit is sandboxed further. A new data folder on Windows is private to the server's account.
- **Bundled FFmpeg:** by exact file and hash.
- **Fixed:** a crafted lyrics file could run the server out of memory. Lyrics, booklets and artist pictures beside a song are read the same safe way as other library files, and an artist picture must be a picture.
- **Limits:** liked songs, logo credits and a title's extras keep to what an account may see, and logo and listing totals are for administrators.
- **Lyrics:** your own lyrics server stays the only place that connection goes: redirects stay on its host. A logo's preview waits until logo downloads are agreed to. Guide searches take turns. Bumper uploads leave room on the disk.
- **Backups:** downloading a backup, deleting one, keeping fewer of them, and starting to replace a library's originals each ask for your password, so a stolen session can't take the way back. A backup with no password can't bring keys, this server's identity or a share on another machine when restored, and libraries that convert by themselves come back off. Backups can't turn on custom cores. Uploads and checks leave room on the disk.
- **Removing a recording** only ever removes a file in the recordings folder.
- **Conversions:** a split film's parts, and files being converted, are held to the same list of formats as anything else played from a library. Tdarr's key goes only to the address it was saved for, and changing the address asks for it again. Repair won't look at a share on another machine unless you allow those.
- **Books and lyrics:** timed lyrics can't use up the processor or the memory, and book pages draw two at a time. Files inside an EPUB are never sent as scripts.
- **Themes:** can no longer load anything from other sites. A stylesheet that names an outside address, even in a comment, is refused, and one already installed stops being served. Put images and fonts in the theme's ZIP. A theme's files opened on their own are sandboxed.
- **Web:** the redirect to HTTPS only ever keeps the path and query. Files sent to a signed-in account are marked private for caches in front of the server. HTTPS stays on (HSTS) behind a trusted proxy that says the connection is encrypted. Links from metadata, stations and modules only ever go to web addresses.
- **FFmpeg:** which one a server runs is shown only to administrators.
- **Docs:** [HTTP routes](docs/SECURITY-ROUTES.md) lists every route, who may call it and what it validates, including every route that came with this release.
- **Libraries:** everything updated to its newest stable version, including Vite 8, TypeScript 7 and pdf.js 6. The same older browsers and TVs are still reached.

### Fixed

- **Widescreen NTSC discs play.** Their menus and films came out 853 pixels wide, which the video encoder refuses, so a real 16:9 DVD sat on a spinner.
- **Restoring a backup during first-time setup** from another device on your home network skipped the setup code, and a backup can bring accounts with it. It now takes the same code as making the first account, unless you've allowed setup from home.
- **A request hung up on at the right moment, or an accounts table that couldn't be read,** could make a server that's set up look like a new one to the backup routes. Only a database that answers and has no accounts counts as a new server now.
- **A sign-in under way** could finish after a password change and stay signed in. It now fails if the password isn't still the one it checked.
- `wittymediaserver reset-2fa` could make a new, empty database in a mistyped folder. `wittymediaserver doctor` could leave folders behind that a server running as someone else couldn't use. Neither makes anything now until it's sure.
- Asking for a password on the command line no longer shows what's typed if the terminal can't hide it, or leaves the terminal hiding everything after Ctrl-C.
- A password shorter than the rule could pass if it was made of emoji: its length was being counted in bytes.
- A Vertical videos library now has the **Upload** button its settings promised, and Space and K work when the video has the focus.
- **This library is read-only** now stops uploads and kept podcast episodes too, as it said it did.
- **Who else can see statistics** is now a choice under *Settings → Activity*. It could only be set through the API, though the page said Settings.
- **Maximum resolution** has **2160p (4K)**, which is the only way to send a 4K Blu-ray as 4K.
- A result from Musixmatch under **Other versions** can be chosen. It used to answer that the source wasn't turned on.
- Radio and anime libraries can be saved with no folder, as the server always allowed.
- **Recordings** is reachable from the guide on a phone.
- With WittyCD away, a disc says the module is missing and offers it, instead of saying the disc may be damaged.
- **Low-power mode for discs** says what it does: it was always for Blu-rays as well as DVDs.
- The player's key list says what each key does on a disc, and lists **I**. A PDF with a password says so in plain words.
- Uninstalling on Linux lists and removes what `sudo systemctl edit` wrote, and folders you moved with `XDG_DATA_HOME` and `XDG_CONFIG_HOME`.
- A copy-protected EPUB opened to unreadable chapters. It's now found and refused with a plain message, and nothing tries to decrypt it. An EPUB whose embedded fonts are merely scrambled opens as usual, and a locked copy gives way to another copy of the same book.
- A quick press on a controller could be lost in the menus, in a game's menu as much as anywhere, if the browser happened to hold back
  its drawing for a moment. Controllers are now read on a timer of their own, which costs less, since no frames are asked for just to
  read them. Buttons still held as a game's menu opens are no longer taken for presses in that menu.
- A game's screenshots never showed on a server reached over plain http, or in an app that signs in with a token.
- The console logo for the NES could be its Japanese counterpart's. A logo in English, or marked as preferred, now comes first.
- On a game's page, the first key pressed could move the focus to the Play button rather than doing what it was pressed for.
- In Settings, scrolling with the pointer over the menu moved the page beside it. The menu now scrolls by itself.
- With internet radio or music playing, the account menu could open behind the audio bar, and the mini player could cover dialogs. Everything over the page now has one stacking order.

## 0.5.2-alpha

0.5.0-alpha was never published as a release, so this is the first release with everything from it as
well (see its section below). This one adds the fixes from a full code and design audit, *Settings → Reset*,
and a remote access guide written around security.

- **Fixed:** on Linux, an installed module could fail to start on a busy machine and be counted against its version: its
  address space limit was too close to what Go sets aside when it starts. Real memory use stays limited as before.
- **Server 0.5.2-alpha:** The code and design audits: security and reliability fixes across the server, games, discs, modules, apps and the web UI, and a remote access guide written as a security guide.

- **Modules:** a withdrawn version can't be missed: the list is read on every check, not only when the
  list of releases changes. A file uploaded again is noticed by its upload time.
- **Fixed:** a normal install and the Mac app left out the game runner, so games couldn't start.
- **Security:** the helper sandbox refuses x32 system calls and io_uring, which got around its limits on
  sockets. Signing checks a release's file names exactly, so another version can't pass as this one, and
  more kinds of password and key are hidden in logs.
- **Fixed:** a server stopped by a power cut in a container now reports it, even with the same process
  number each time. A restart now brings a container back on any container runtime.
- **Containers:** `:latest` moves only for full releases, and `:alpha` is the newest alpha.
  `docker-compose.yml` follows `:alpha`.
- **Scripts:** the cutover stops if `moduleKey` isn't the key given, or if something it removes wouldn't be
  available afterwards, such as WittyCouch Host's source. `mirror.sh` refuses to push workflows naming an
  unpublished tool version. ScreenScraper's developer id is described as what it is: not built in today.
- **Privacy:** collections, recent games and CD pages no longer show non-admins where files are on the server, and an HLS
  radio station's address no longer reaches listeners or the logs.
- **Limits:** each person's game saves and patches have a 10 GB total for imports, and imports really can be as large as they say.
  Radio is limited to 4 streams a person and 32 a server. Pictures from game PCs can't pile up, patched games are prepared one at a
  time, and a core is only asked what it offers one at a time, after its file is checked. Game PC stream limits now hold when
  several requests arrive together.
- **Accounts:** deleting one removes its saves, patches, images and game PC identity, and ends its games and streams, so a new
  account can't inherit them. Whoever started a game on a shared PC stops counting as its owner once it has ended.
- **Audit of game streaming, fixed:**
  - A playlist in an archive that named itself crashed the server. Lists can't name lists now.
  - The Linux sandbox had ways round it: x32 call numbers and io_uring reached the network and more, and a core
    could empty or change the mode of any file the server's account could write.
  - Stopping could hang for ever on a runner that said it was ready twice, on one that left a child behind, or
    on one that had stopped reading. Stopping a game still starting could take the server's slot with it.
  - A game still loading was stopped after a minute for having nobody watching, and that counted as a crash.
  - A resume state the core couldn't load was overwritten when you left. It's kept as the previous save now.
  - Starting another game while one loaded could bring the first back on another core. A core trying after
    another now gets its own graphics card access and memory limits, and its option list is kept under its name.
  - An encoder that died during a game froze the picture. Another starts now, and one that keeps dying is left out.
  - Holding rewind at the start of what's kept ended the game without saving.
  - Refresh took a game's lock off, and a rescan could replace a title typed by hand. Games on a gaming PC were
    looked up by the PC's number for them, rather than by name.
  - An IPS patch writing the same place over and over could hold the server for minutes, and a file of unknown
    size could fill the disk past what's left free. TheGamesDB's names failed to load if a count came with them.

- **WittyCouch:** a game's name from a PC can no longer break out of the quotes in the Moonlight command people copy
  and paste, in PowerShell or a POSIX shell, and a name with an apostrophe now reaches Moonlight whole. The host
  program's version check gets nothing of the server's and runs in its sandbox, a program is only looked for in `.build`
  by development builds, and a viewer arriving just as a stream ends can't leave a connection behind.
- **WittyMediaPlayer:** a crashed disc helper can't take the window with it, and dialogs closed while waiting on it are
  safe. Ripped tracks after the first start at zero. A long file whose video ends early no longer reads all its sound
  into memory. Closing with a drive stuck on a bad sector is safe and no longer waits on it, and a disc image never
  overwrites a file that appeared during the copy.
- **Fixed, from the audit of the web app:** leaving a Game PC stream while it starts now ends it, and opening its menu no
  longer leaves keys held down on the PC. The visualizer's flashing notice is each person's own. Filters in a games library
  stay put during a scan or lookup. V no longer opens the visualizer over a game. A disc, and music after an audiobook, play
  at the right speed. Stations using HLS resume first time. Setting every keyboard button at once makes one profile. A
  refused cheat isn't shown as added, and one Esc no longer loses an edited settings file. Games no longer put a sign-in in
  the WebSocket address over HTTPS.
- **Design audit:** a page that can't load now says so, with Try again, rather than looking empty. On narrower screens,
  Settings is a menu of its own with each item explained, and every page has a way back to it. The phone's tab bar says
  where each button goes. Pages carry one legal note, at their foot. *Gaming* is now *Consoles & cores*, *Games* is
  *My game settings*, and the disc options moved from Transcoding to *Playback & discs*. There's an **Automatic** theme
  that follows the device, the crash notice can be dismissed, and small controls are easier to tap.
### Discs, audio and TV: what the audit found

- **Fixed:** a panic during a Blu-ray title action could take the whole server down, and a button's pause or resume never reached the player.
- **Encrypted media stays refused:** a scrambled DVD title asked for directly and any action on an encrypted Blu-ray's refusal screen now get the refusal. IPTV checks every HLS variant, and only calls a stream clear when all of them were read.
- **Encryption that starts later:** an IPTV HLS stream's playlists are read again while it plays or records. A key that appears stops the stream, marks the channel as encrypted and shows the usual refusal; a recording keeps what was clear and stops for the same reason, and a network failure ends nothing.
- **One wording:** encrypted discs are refused in the same words everywhere a viewer sees them.
- **Crafted files:** menu, subtitle and picture sizes are limited on every path, a helper's answers are limited before decoding, a station can't make the server build a huge playlist, and a CD sheet can't leave its folder through a linked folder.
- **Older helpers:** a WittyDVD helper from before chapter runs keeps the disc instead of being blamed, and a cut-short disc image no longer counts against an honest helper.
- **Fixed:** a stopped stream's clean-up could delete a new stream's files, AIFF files are now checked to be CD audio, and a playlist from elsewhere can't reach this machine by a name or a redirect.

- **Reset this server:** *Settings → Reset* puts a server back as it was first installed, after warnings, your
  password and typing RESET. It keeps a copy you can bring back from the same page, unless you choose to delete
  everything. Media files are never touched. `wms reset` does the same when you can't sign in.
- **Server 0.5.1-alpha:** Everything from this round's work together: games on every console, modules with repositories of their own, WittyCD, WittyCouch, WittyRigs, the visualizer, radio search, WittyLog, WittyDiagnoser and the Legal page.

### Internet radio

- **Search a directory of stations**: by name, genre, country or language, with logos and details, and add with one
  click. Stations come from [Radio Browser](https://www.radio-browser.info/), a free community database. Typing an
  address or importing a playlist still works.
- **Pages to match**: station cards with logos, browsing by genre and country, favourites and recently played, and a page
  for each station with its details and website. Stations without a logo get art made from their name.
- **What's on now**: the song, from the stream's own metadata, in the player and on lock screen and headphone controls.
- **HLS stations** now play, and `.pls` and `.m3u` addresses are unwrapped when a station plays. Old servers that answer
  `ICY 200 OK` work.
- **Stations that go quiet are noticed** and marked off the air, from playing them, from the directory, and from a check
  you can run. Checking in the background is a setting, off unless you turn it on, because it contacts stations when nobody
  is listening.
- **Pausing a station hangs up**, so the server isn't relaying it for nobody. A stream that drops reconnects by itself,
  and stations have a sleep timer.
- For apps: `/items/{id}/playback` plays stations, and new routes under `/radio` (see
  [CLIENT-API.md](docs/CLIENT-API.md#internet-radio)).
- **Fixed:** listeners who weren't administrators were sent each station's address, which can hold a private key.
### WittyPlay: make a game yours

Everything here is per person, can be set for all games, a console or one game, and is kept on the server, so it
follows you. See [GAMING.md](docs/GAMING.md).

- **The picture, drawn on your device:** scanlines, CRT curve, phosphor pattern, glow, dark corners, an LCD grid,
  frame blend, sharp, pixel or smooth scaling, whole-number scale, shape (as the game says, 4:3, 16:9, square
  pixels, fill, custom), overscan crop, rotation, and colour, brightness, contrast and gamma. A colour or your own
  image around the picture, with presets to start from and a live preview. Effects cost the server nothing and add
  almost no delay. With no effect on, nothing at all sits between the video and the screen. Without WebGL, CSS
  does what it can. The one effect that flickers is off by default, says so, and stays off for anyone whose device
  asks for reduced motion.
- **On-screen numbers:** frames shown and frames run, network delay, encode time, bitrate, drops, resolution, core
  and encoder, in the corner you choose. Off by default.
- **Speed:** frame skip (automatic or fixed), fast-forward (hold or toggle), slow motion, pause and frame advance,
  rewind, run-ahead, and pausing when the tab is hidden. Fast-forward sends no more pictures than normal, so the
  encoder doesn't pay for it. An administrator sets the fastest speed and rewind's memory, and rewind and
  run-ahead back off or turn themselves off, saying so, on a machine that can't spare the time.
- **Sound:** volume, mute and a buffer size against crackling.
- **States:** 99 slots with pictures and dates, quick save and load, undo a load, automatic save on exit, and a
  choice of resuming always, never or on offer. Starting over now keeps the old automatic save. States and battery
  saves can be downloaded and imported, with what comes in checked.
- **Screenshots and short clips**, saved to your device.
- **Cheats**, through the core's cheat interface, typed in or from your own file. **IPS and BPS patches** of your
  own, applied to a copy when the game starts. Nothing is bundled.
- **Core options:** every option a core really has, with its own names and descriptions, in its categories, in the menu and on the
  settings page, changing the running game. Options a core hides because they don't apply stay out of the way.
- **Custom settings in JSON:** edit, import, export and share a settings file, to set anything a core offers, and
  to define your own presets and controls: a toggle, slider or choice with your own label. Mistakes are shown at
  their line, with the list of what the core accepts beside them. Anything unknown is listed as ignored. A file
  can't run anything, name files or reach the network, and imports are limited in size, depth and count.
- **Safe mode:** start with defaults, from the game's page or the error when a game won't start, so that no setting
  can lock you out of a game.
- **Shortcuts:** from the keyboard, from a controller by holding Select and pressing another button, and from
  on-screen buttons on a phone. All can be changed. The game menu holds everything above, and works with a
  controller, a TV remote and touch.
- **The game's page:** favourites, your own notes, play count, last played, and a resume banner with a picture.
  Administrators can set what everyone starts with.

### More reliable

- **Games stay playable on a busy Windows server.** The game runner used to run at below-normal priority, and Windows gives that next to
  nothing while other programs are busy: a game could fail with "took too long to load", or crawl at one frame a second. It now runs at
  normal priority there, and shares the machine fairly with a transcode.
- **Save pictures and your own images load in apps that can't send a sign-in cookie.**
- **On Linux, the game runner's limits hold on all of its threads.** Writing outside the save folder could get past them from a thread
  other than the one that set them. A system that can't cover every thread now says so in the log.

### Lower delay

- **Games on the software encoder lost about a quarter of a second of delay** on machines with many cores: it was
  holding back a frame for each of its threads. The new encode-time number showed it.

### For client developers

- API 1.4: game settings, cheats, patches, images, saved-file transfer and session numbers, listed in
  [GAMING-API.md](docs/GAMING-API.md). Nothing about them lives only in the browser.
### Add your own game cores

- **Custom cores for WittyPlay.** An administrator can add a libretro core by uploading it, from a web address, or from the cores folder,
  and use it for a console, for one game, or as a console's default, with other cores still available. See
  [Adding your own core](docs/GAMING.md#adding-your-own-core).
- **Tested before anyone can use it.** Each file first loads in the same sandbox a game runs in, and what the core reports is shown: its name,
  version, file types, BIOS, graphics card and whether it's built for this server. A crash or a hang only stops the test.
- **Safe by default.** Off until an administrator turns it on, with the password each time. The SHA-256 is recorded, and a file that
  changes won't run until someone looks at it. Web addresses are https and public only. Everything is in a history on the page.
- **Custom consoles,** by form or JSON, for a core for a system our list doesn't have. They appear when adding a library.
- **Timing calls for cores:** the game runner now answers the libretro performance interface, which some cores refuse to start without.
- **Going back:** earlier versions of a custom core are kept, and replacing or removing one never touches saves.
- **Which core works best:** a game's page shows administrators how each core has really gone on it.
- **Honest licensing:** a custom core shows its licence if known, or "Unknown license", with non-commercial ones labelled. They're
  marked third-party and unverified, and this project never offers them.
### WittyCD: audio CD images

- **CD images play in a CD player,** a window in the spirit of the desktop players of the early 2000s, with the disc going in as DVDs do, a lit display, a list of
  tracks, and room on its stage for WittyVisualizer: `.cue` with `.bin`, `.wav` or a single `.flac`, `.toc`, `.ccd`, `.nrg`, `.chd`, and a bare
  `.iso` or `.img` that is really audio. In a Music library a disc is an album with its tracks; in a Discs library it sits on the shelf.
- **Gapless by design:** the whole disc is sent as one stream, as lossless FLAC or WAV, with AAC for slow connections. Repeats,
  shuffle, your own program, index points, hidden tracks, pre-emphasis correction, and your place on the disc for each person.
- **Identified by table of contents** through MusicBrainz and the Cover Art Archive, sending nothing else, with a dialog when
  several albums match. It can be turned off in Settings, Metadata agents.
- **Native apps** get it through `GET /cd/{id}`, `GET /cd/{id}/audio` and the ordinary playback door: see *Audio CDs* in
  `docs/CLIENT-API.md`.
- **Tools:** `wms cd info`, `wms cd check` and a standalone `wittycd` command. The module's reader is a public library: track map, samples, disc ID and the lookup (`wittycd/lookup`), for WittyMediaPlayer and anyone else.
### WittyVisualizer

- **Light shows that follow your music,** in the audio player and in WittyMediaPlayer, from one set of shader files:
  spectrum bars, scopes, plasma, a tunnel, a starfield, feedback, a kaleidoscope, particles, fire, a wireframe landscape, a
  fractal and more. Seventeen effects written for this project, with beat-reactive colours, smooth transitions, random order,
  favourites and an automatic change of effect.
- **Press V or the bars button,** on the whole screen, in place of the artwork in the full player, or small in the bar. Keyboard,
  remote and controller all work. Choices are saved per person on a server.
- **Safe by design:** off until you turn it on, a notice before the first use, a calm mode that's the default when the system
  asks for reduced motion, and a flash guard no effect can get around. It's in the last step of drawing. A black and white
  strobe comes out as a dim steady glow.
- **Leaves playback alone:** it listens to a copy of the sound where the browser allows, and on the desktop reads a lock-free
  copy. It runs only on your device, loads nothing until opened, adapts its resolution, and stops fully when hidden or off.
- **You can write your own,** as a small text file, in the web app. It's checked as untrusted and tried in a throwaway graphics context first.

### WittyMediaPlayer

- **Plays and copies audio CDs**, from a drive or an image, on a CD screen with a tracklist, a big now-playing area and a
  visualizer. Copies go to FLAC, ALAC, WAV, MP3, AAC, Opus or Ogg Vorbis, with naming templates, tags and cover art, and
  careful reads that retry, compare two passes, correct for the drive's offset and report on every track. It can also make
  a `.cue` and `.bin` image of a disc. Names and covers come from MusicBrainz only when you press the button, which sends
  nothing but the disc's track layout. Discs that won't read normally get a plain message. The visuals are off until you
  turn them on, tell you about flashing light first, and go through a flash limiter.
- **A new desktop app**, under `apps/wittymediaplayer`, that needs no server. It's a first release for Windows amd64,
  as a portable folder, built with Qt 6 and FFmpeg's libraries.
- **Plays ordinary video and audio files**, with audio and subtitle track choice, seeking, volume, full screen, keyboard
  shortcuts, drag and drop, and subtitle files found beside a video.
- **Plays DVDs and Blu-rays with their real menus,** by arrow keys, Enter and mouse, from images, folders and directly
  from a disc drive. It uses WittyDVD and WittyBluray with the same rules as the server's player, without transcoding.
- **Makes `.iso` and `.img` images of discs** for a server's library. The copy is checked by opening it with the same
  engines the server uses, and it never overwrites anything.
- **Can play through the VLC libraries,** by VideoLAN and the VLC contributors, chosen in Settings and loaded from a VLC
  already on the computer. They aren't included. Disc menus still come from WittyDVD and WittyBluray.
- **WittyDVD and WittyBluray update on their own,** without a new player, under the same rules as a server's modules: a built-in copy
  that always works, signed releases only, version ranges, and going back by itself if a version keeps failing. Looking for updates is
  the only time the player uses the internet, and only when asked.
- **Refuses copy-protected discs,** for playing and for copying, in plain words. It carries no decryption library, no key
  database and nothing that could load one, VLC's disc modules aren't used, and it never uses the network. See
  its design note.
### WittyLog: logging that captures problems across everything

- **Every area has its own level**, changed live with no restart: playback, transcoding, discs, games, Live TV,
  library, modules, updates, remote access, sign-ins, app errors, web requests and storage.
- **Every module has its own controls too.** A level of its own and advanced options, such as a helper's every
  call or a disc's navigation. A module that is installed later is noticed and listed by itself. All of it is in
  *Settings → WittyLog*, and a module can list options of its own in its manifest.
- **Everything about one playback is tied together.** One session links the request that started it, FFmpeg's
  output, a disc's or game's steps, and what the player reported on your device. One press shows it all.
- **A viewer in Settings:** live tail, filters by level, area, module, session and text, and common failures
  explained in plain words with a suggested fix. It works on a phone and with a TV remote.
- **Catch a problem:** one button records everything for a while, then goes back to your usual settings by itself.
- **Crash reports.** A fatal error, a caught panic, or a run that ended without shutting down (a power cut, say)
  leaves a report that survives, and the next start points it out. Each start also logs a summary: version, modules,
  system, graphics card and encoders, disk space and settings that matter.
- **FFmpeg, game runners and module helpers** each leave how they ended, in words, with what they printed, bounded.
- **Failures that used to vanish are recorded:** database errors, failed requests, scans, recordings, installs,
  updates, certificates, port mapping, sign-in lock-outs and background tasks that crash.
- **The apps report too.** The web app sends uncaught errors, failed requests and player and game errors to your
  server. The desktop, Android and Apple apps keep a small log of their own and report crashes to your server.
- **One-click support bundle:** recent logs, crash reports and a system summary as a zip, with a preview and a
  switch to hide titles and file paths. It's downloaded by you, and nothing is uploaded anywhere.
- **It can't get in the way.** Logging never makes anything wait: when a disk is slow, full or gone, messages are
  dropped with a count and the server carries on. Files are limited by size and age. A quiet line costs about 40 ns
  and a written one about 3 µs.
- **Secrets are removed centrally,** tested with secrets planted every way we could think of. Text from outside can't
  forge a log line. The setup code is now only printed on the console, and not kept in the log files.
- Failed requests are now recorded by default. So are app errors, which you can turn off.
- [How every part logs](docs/LOGGING.md), for anyone adding code.

### WittyDiagnoser: check discs and drives without a command line

- *Settings → WittyDiagnoser* checks **a disc**: whether it's encrypted (detected and explained, never bypassed),
  its structure, titles and menus, every menu button pressed with dead ends reported, Blu-ray and Java menu details,
  and damaged or unreadable sectors. The report it saves has nothing copyrighted in it.
- It checks **storage**: free space, permissions, unreadable and missing files, read speed and whether that keeps up
  with a stream, network-drive and file-system quirks, and the drive's own health where the system tells without
  administrator rights. Nothing is changed. The only thing written is a small temporary file, never in a media folder.
- Long checks run in the background, one at a time, with progress and cancel, and give way to anyone watching.
  [What it checks](docs/DIAGNOSER.md).
### Legal notices

- **Short notices across the app** say that web results and third-party content aren't created by,
  associated with, endorsed by or checked by the project, and that a person's own files and what they host are
  theirs. They come from one source, so every screen says the same thing, and apps get the words from
  `GET /api/legal/notices`. The connect screens of the WittyMediaClient apps show one too.
- **A Legal page** in Settings, and at `/legal` for anyone: terms of use, no warranty, no affiliation, who's
  responsible for what, trademarks, games and BIOS files, copy protection, flashing images, age ratings, and a
  privacy list of every outside service, what it receives and what controls it.
  *About & legal* is now *About* and *Legal*.
- **A server's owner can add their own notice** and contact details for the people using their server.
- **One tick in setup** confirms that your files and what you share are your responsibility. Servers set up
  earlier show a one-time bar. The answer stays on your server.
- **Corrected:** the README and setup said nothing leaves your home except titles. Update checks, other
  lookups and more do: the list is on the Legal page. Messages and docs about encrypted discs now just say
  they're refused. Share links say they show your profile name. TheTVDB, Wikipedia on game pages, the
  dictionary service and podcast search now have their credits.
- **Removed from the README:** older screenshots. Docs use made-up titles. Every module's README has a Legal
  section. None of this is legal advice.
- **Privacy:** live TV channel logos now load through the server, so the sites a playlist names never see a
  viewer's address. Only internet addresses are fetched, never devices on the home network. As with other
  artwork, SVG logos are not served, so a channel with one shows no logo. **WittyTuner 0.1.1** ignores a
  playlist logo that isn't a web address.
- **Fixed:** a Blu-ray playlist item that ends or starts part-way through its video could lose up to a second of
  picture at the join, show a little that should not be seen, or leave the sound short. Every item now plays
  exactly from its first frame to its last, with its sound, and an item that carries straight on from the last one
  is read as one recording. None of the discs tested here has such an item, and their main features play as before.
  This was checked with made-up discs built to have them.

## 0.5.0-alpha
### Smooth DVD playback

- **Fixed:** a DVD could freeze or skip for a moment around a chapter change, with nothing pressed, and the
  sound could slip with it. The disc's sections were read one by one and placed by their earliest sound, which
  lies up to a quarter of a second ahead of the picture and differs each time. Sections that follow each other on
  the disc are now read as one recording, and where a join is real its first picture lands exactly in place.
- **Fixed:** a smaller stutter every few minutes on DVD and Blu-ray, at the moment the server starts converting
  again after resting. Sound and picture now carry on from exactly where they left off.
- **Fixed:** skipping within a DVD or Blu-ray could hold the first picture for up to half a second. It now lands
  on the right frame.
- **Fixed:** picking a chapter on a DVD's page could start at your saved place instead of that chapter.
- **Fixed:** the same late landing on TV recordings and other .ts, .m2ts, .vob and .mpg files when skipping or
  when conversion starts again.
- **New for developers:** a measuring check compares a stream with the disc, frame by frame and sound by sound,
  at every chapter. See [DVD.md](docs/DVD.md#diagnosing-a-disc).
### Blu-ray Java menus tested on more than our own test disc

BD-J has now run open-source sample applications, on Java 8, 21 and 25. It has still not run a retail
disc: neither Blu-ray image on hand has Java menus.

- **Security:** a hostile test disc found that disc code could read and write any file and reach the network,
  through XML parsers, colour profiles and RMI. All are closed, and the test now runs every time. Windows share
  and device names are refused before anything touches them. No socket of the Java process can connect now,
  whoever asks.
- **Runaway discs:** past its limits on memory, threads or processor time, a disc's Java process is ended and the
  feature plays. Message floods and requests for playlists and titles are paced. A disc can no longer fill a drive
  through a sparse file.
- **Fixed:** arrow keys stopped reaching a disc's menu after the first press of OK. A disc's saved scores and
  bookmarks were lost whenever the server restarted. Discs that wait to hear a picture has loaded waited for ever.
  Animation drawn straight to the screen didn't update. A finished application's picture stayed on screen, and when
  an application quit the disc had nothing to do; it now returns to the disc's menu. Java stops 3 minutes after the
  viewer has gone, not 30, and a hard stop no longer leaves copies of discs' programs behind.
- **Added:** about 300 more platform classes from the published specifications, among them HAVi widgets and focus,
  Java TV services and timers, DSM-CC files, and `DVBClassLoader`. Several were in the wrong package, or could not
  be linked by a real disc at all.
### Licenses you can read

- **Settings, About** now lists everything in the server that someone else made: every library with its version, license and
  full license text, the separate programs, the game cores, and the online services and their terms. Before, it pointed to a file.
### Game details and ratings

- **Games now get covers, summaries, ratings and more**, in the background, with no account needed: the libretro
  database recognises your exact files by checksum without unpacking them, libretro's thumbnails add box art and
  screenshots, and Wikidata and Wikipedia add summaries.
- **More with a free key:** IGDB, ScreenScraper, RAWG and TheGamesDB, each in the setup wizard and in **Settings, Metadata
  agents**, with a link to get a key, what it costs and a test button. Choose which are on and which is believed first.
- **Age ratings count.** ESRB, PEGI, CERO and others reach Kids and Guest profiles, and a game above a profile's limit is
  hidden exactly as a film would be. Critic and player scores show who gave them.
- **Right matches, or none.** Checksums first, then disc codes, then the name and console. Each game shows how it was matched
  and how sure that is, and can be fixed with a search, locked, or looked up again.
- **The Games page** sorts by title, rating, year or date added, and filters by genre, decade, players, rating and console.
  A game's page looks like a film's, with a backdrop, scores, screenshots and credits.
- **Light on phones and TVs.** Lists use smaller copies of covers, made the first time they're asked for.
- **Find what's missing.** The Games page says how many games have no match, and shows them with one click, to fix.
- **Kind to free services.** Requests are batched and paced to each source's rules, nothing is asked twice, and a restart carries on
  where it stopped. Nothing is written next to your games unless saving next to your media is on.

### Games in archives

- **Nothing needs unpacking first.** Game libraries read `.7z`, `.zip`, `.rar`, `.tar`, gzip, bzip2, xz and Zstandard, and split
  `.7z.001` and `.part1.rar` sets, for every console. That includes disc games: an archive holding a `.cue` and its tracks, a `.gdi`, or several discs, unpacks everything the
  game needs and opens the right file.
- **Scans stay quick.** Only an archive's headers are read, once, until the file changes. About 2,000 archives take about a second.
- **Starting a game** shows how far unpacking has got. It's kept for next time in a space you set in Settings, Gaming (16 GB
  by default), and the longest unplayed goes first. Nothing is written beside your files, and saves carry on as they were.
- **Folders** named like `Super Nintendo (SNES)` are now recognised, and a `.cue` hides its tracks even when the case of the names
  differs.
- **Safe with untrusted files.** Archives are read in a separate, boxed-in process. Ones that try to climb out of their folder, hold
  links, unpack to absurd sizes or are password-protected are refused or skipped with a clear message.
### Games tested with real games

- **`wittyplay-check`** is a new tool that plays real games on the real sandboxed runner with no browser, and reports
  on each. It found everything below. [GAMING.md](docs/GAMING.md) explains it and gives the results.
- **Fixed, games that wouldn't start:** Mesen crashed on load, as the frontend set things up in the wrong order. PCSX ReARMed and Stella broke the connection by printing to it. bsnes crashed on Linux, as did PCSX ReARMed under
  the memory limit, and clownmdemu couldn't load there at all. Supafaust ran at 1.5 frames a second, pinned to two CPUs.
- **Fixed, resuming:** BlastEm started over without saying so, and SMS Plus refused its states. Leaving before the first
  picture no longer overwrites a good resume point with a fresh start.
- **Fixed, playing:** a very short tap of a button now reaches the game. The picture no longer goes wrong when a game
  changes its frame rate or asks to be rotated. Nestopia no longer blurs its picture.
- **Fixed, security:** on Linux the seccomp filter now covers every thread a core starts. It only covered one before.
- **Fixed, connecting:** the WebSocket fallback no longer fails for a device signed in by token, and state pictures now
  load there too.
- **New:** a game a core won't load tries the console's other cores. Crashes say so in plain words. Games on several discs
  can change disc in the player. Encrypted 3DS games are refused up front with a clear reason, and nothing tries to
  decrypt them. Satellaview `.bs` files go to Mesen-S and say if `BS-X.bin` is missing.
- **Changed:** the container image is now based on Debian rather than Alpine, because the runner couldn't start any core
  on Alpine. It is about 800 MB, up from about 150 MB.

### Games: controllers

- **Controllers just work:** common pads are recognised by model, including cheap USB ones and pads on Linux whose buttons a
  browser doesn't put in order. D-pads reported as axes, triggers reported as axes, and hats are all handled. Sticks have a proper
  round dead zone and can be calibrated, triggers reach games as how far they're held, and each console's controller maps
  sensibly, with the Nintendo 64's C buttons on the right stick and a DualShock plugged in for PlayStation where the emulator offers one.
- **Your own profiles:** make, name, copy, edit, delete, export and import mappings, for one controller, console or game. They're kept on
  the server, so they follow you between devices. Remapping is by pressing, conflicts are shown, a stick can press a button and a
  button can push a stick, and there's turbo and a reset.
- **A picture of the controller:** the controller screen, in the game menu and in your account menu, shows a drawing suited to the console, with each
  button labelled, clickable, and lighting up as you press it. Every mapping is also a plain list. All of it works from a remote or controller.
- **Your own background** behind the picture, with position, fit and dimming. Only you can see it.
- **An on-screen analog stick** for consoles with one, and a one-press swap of the face buttons for people who'd rather go by the letters on their pad than by position.
- **Several players:** a pad goes back to its own player when reconnected, and moving onto a taken player swaps the two.
- **Fixed:** unplugging a pad, or switching tabs, could leave buttons held. A new pad at the same place took over the old one's layout. The PlayStation's stick also pressed its D-pad. Pads that aren't
  in standard order couldn't open the menu. Rumble events went out every frame, and only ever ran one motor's strength on both.
- **Plain http:** browsers hide gamepads from such pages. The controller screen now says so, and what to do about it.
### Game PCs: play on any PC running Sunshine, in your browser

- **A Moonlight client in the page.** **Game PCs** lists your own PCs, yours or a friend's: find one on the network or type its address, pair it with a PIN, and its games
  are there. Press Play, and it streams right in the page with a controller, keyboard, mouse or touch. Nothing to install on the device. Your pairings are yours alone.
- **It's played for real:** a real Sunshine, Moonlight's real streaming core, a real browser. Real games on real gaming PCs are next.
- **WittyCouch** (Apache-2.0) is the client and relay. It uses **WittyCouch Host**, a separate **GPL-3.0** program built on Moonlight's own code, which is installed separately.
- **Who may use it:** adult profiles with no limits by default, and admins. Admins choose who, and whether it can reach the server's network or the internet. A PC can also be shared with everyone
  as a library, where Kids and Guest profiles see only the games they're given.
- **WittyRigs** is an advanced option, off by default, for hosting demanding games on the server's own hardware.
- **Handing off to the Moonlight app** stays as the fallback, and is what Android TV and Apple TV use.
- The design, licenses and what's really been tested: `docs/GAME-RIGS.md`.

### Interface audit

- **Fixed:** the TV guide went blank with no channels. The home banner could be an empty box when the newest
  titles had no art. The phone's bottom bar wrapped onto two rows. A folder typed without pressing Enter left
  Add greyed out in setup. A remote's Back button did nothing in a dialog. Home videos under 5 MB were skipped.
- **Games:** large libraries load a page at a time (they stopped at 2,000 before), with search and console filters.
  Files a scan can't use are now reported instead of quietly missing.
- **Wording:** movies, shows and ads throughout. Sign in a TV is clearer, and search asks for a word instead of
  listing everything.

### Modules become their own projects

- **A module store.** Settings, Modules is now grouped by what each module does, and shows its status, versions, whether it updates by itself or with the server, what it needs, its repository, and its recent activity and failures.
- **Choose what's on your server.** First-run setup asks before you make libraries, and what's installed decides which libraries can be made. Add library shows the rest greyed out, with a button to add what each needs, there and then.
- **Choose a version.** Every release is listed with its notes, and older ones can be installed, with a warning, and stay pinned until you go back to the latest. One that doesn't fit this server is shown and explained.
- **Automatic updates, module by module.** Each module that updates on its own has a switch to leave its updates to you, while still showing what's available.
- **Kinds of library shared by modules.** The store says when two modules bring the same kind, and a kind can be given a name of your own, which Add library then uses.
- **Withdrawn versions.** A module can withdraw a version with a signed list. One withdrawn for security reasons is refused, and stops being used even if pinned.
- **More can update apart from the server:** WittyDVD and WittyBluray run in a helper per open disc, WittyTuner's network sources and WittyPlay's game runner update too, along with the file browser and all four reader modules. [MODULES.md](docs/MODULES.md) says exactly what each can and can't do, and each module's built-in copy stays the default and the fallback.
- **Using someone's fork.** An admin can point a module at a fork, with a key they trust for that module alone. It runs under stricter limits and never updates by itself, and the server still refuses encrypted media whatever it says. WittyTuner can't be forked. Brand-new custom modules are marked as in development.
- **Stronger limits on helpers.** On Linux a helper can no longer change any file and has a memory limit, applied so that nothing in it can skip them.
- **Each module can release from its own repository**, with its own workflows, and the steps and script for moving over are in [MODULES-CUTOVER.md](docs/MODULES-CUTOVER.md). Nothing has been published yet.
- **Versions in one command:** `wittymodule bump` and `scripts/bump-server.sh`, with a check that a changed module has a new number.

### Modules update on their own

- Every module now has its own version and states the oldest and newest server it works with.
  **Settings, Modules** lists them with their status, and a version that doesn't fit is refused with a
  plain reason while the built-in copy carries on.
- **WittyPhotos** is the first that can update without a new server: signed like server updates, installed
  with no restart, and taken back out by itself if it keeps failing. Other modules follow. Looking for
  updates follows the existing update setting, and installing them automatically is off unless turned on.
  See [MODULES.md](docs/MODULES.md).

### New libraries

- **WittyPhotos**: a gallery with a timeline, albums, favourites, folders and a map. JPEG, PNG, GIF, WebP,
  BMP, TIFF, HEIC, AVIF, JPEG XL, camera RAW (shown by the preview the camera stored) and clips, with live
  photos and RAW+JPEG pairs grouped. Scanning reads only file headers; thumbnails are made when first looked
  at. Uploads from any device, slideshows, and a clearly marked placeholder for the local AI. The map needs
  nothing online; an online map is an opt-in setting. See [PHOTOS.md](docs/PHOTOS.md).
- **WittyShorts**: vertical videos, one at a time, with likes, comments, sharing, creators, custom metadata
  and a For You page worked out from what each profile did on this server, and nothing else. See
  [SHORTS.md](docs/SHORTS.md).
- **WittyChannels**: creator videos already on disk, with a home, subscriptions, channel pages, playlists and a
  watch page with comments and up next. Reads the sidecar files video archivers leave, including saved
  comments. See [CHANNELS.md](docs/CHANNELS.md).
- **Music videos and concerts**: by artist, with recordings of 20 minutes or more under Concerts and their
  chapters as a setlist.
- **Internet radio**: stations by stream address or M3U/PLS playlist, relayed by the server, which contacts a
  station only while someone is listening. See [COLLECTIONS.md](docs/COLLECTIONS.md).

### New everywhere

- **Collections and smart playlists** across libraries, with Watch later and Play all.
- **Links for people without an account**, for pictures, clips and albums: with an end date, showing only
  what they name, with EXIF and locations left out unless downloads are allowed. Only a fingerprint of a link
  is stored. Listed and ended in Settings, Sharing & photos.
- **Your year**: time, days, and top titles, shows, artists, creators and genres, from each profile's own record
  on the server. **Recommended for you** on the home page, from the same record.
- **Playback speed on every media type**: music, audiobooks, podcasts and vertical videos join video and discs,
  up to the server's limit. Each kind of listening keeps its own speed, and < and > step through them.
- **Chapters from video files** in the player, for every video library.
- The **WittyPhotos** engine (`modules/wittyphotos`): EXIF, picture sizes and RAW previews in pure Go, fuzz-tested.

Everything new has JSON routes, plays through standard HLS or direct files, and works with a D-pad, for the
coming TV apps.
### Security

A full security review and its fixes; the details are in `docs/SECURITY-REVIEW.md`. Things you may
notice:

- First-time setup from another network asks for a code from the server's log.
- New passwords for full accounts need 8 characters (Kids and Guest PINs stay at 4; passwords
  already set keep working).
- `maxTranscodes` is now enforced: beyond it, viewers are told the server is busy.
- Files that are links leading out of their library are no longer indexed or served.
- The expert FFmpeg fields accept only options that tune encoding.
- Accounts other than the administrator no longer see server folders or podcast feed addresses.
- An HLS playlist opened with a token in its address names its segments with a token for that stream
  alone, so the session's own token doesn't end up in playlists.
Everything found in a full audit of the code; the details are in the project's history.

### Security

- Nothing the API answers can run as a page on this site; a podcast host could have answered with
  a script. Podcast episodes are only passed on as sound.
- Saving metadata can't be made to copy another kind of file beside your media, or to fetch from
  your local network.
- Accounts with limits no longer see hidden shows' artwork, genres, networks or podcasts, and only
  administrators see where files are on the server.
- Sign-in slows down one address trying many accounts, and a flood of attempts can't take the
  whole processor.
- Releases are built with the newest Go patch release.

### Faster, on small machines above all

- The first page loads a third of the script it did; the player, readers and Settings come when
  opened, and scripts travel compressed.
- The home page and library pages ask far less of the database (home: 146 ms to 33 ms on a test
  library of 25000 items, with a thirtieth of the memory churn). Libraries load 120 items at a time.
- FFmpeg rests when it is well ahead of the viewer, *maxTranscodes* is really enforced, and the
  player says when it is done with a stream.

### Fixed

- A film paused for over 10 minutes hung when resumed.
- A library folder that is away no longer marks everything in it as missing.
- An interrupted database upgrade could stop the server from ever starting again.
- One DVD in 256 opened as "expired". Library pages flickered while scanning.
- Starting the server deleted the whole transcode folder, whatever else was in it.
### Getting ready for the TV apps

- **Client API**: `/api/v1`, `GET /api/capabilities`, and [a guide](docs/CLIENT-API.md) and
  [OpenAPI file](docs/openapi.json) covering every route. A test fails when a route is left out, or
  when what they say about sign-in isn't what the server does.
- **One way to play anything**: `POST /api/items/{id}/playback` takes what a device can play and
  chooses direct play, remux or transcode. Built-in profiles for Apple TV and Android TV; apps can
  add what their device reports. Remuxing keeps the picture untouched, in fMP4 for HEVC, for almost
  no work on the server.
- **Stream tokens**: short-lived, read-only addresses for one item's media, for players that can't
  send headers. They end with the sign-in they came from.
- **Apps can find the server on the home network** (mDNS, with an off switch), and check it is really
  your server with a key they remember, so a look-alike is noticed. Only its name, a random ID and
  its version are announced.
- **TV sign-in with a code**, either way round: the TV shows a code, and someone signed in approves it at
  *Sign in a TV*, after seeing which device is asking; or that page shows 6 digits to type on the device.
- **Subtitles for native players**: text tracks as WebVTT, by HLS rendition or as a separate file.
- **Themes as data**: `GET /api/themes/{id}/tokens.json` gives a theme's colours, lengths and fonts.
- The resume rule and how a disc starts now come from the server, for every client.

### Fixed

- **Text subtitles (SRT, ASS) that are burned in now show.** Choosing one converted the video but drew
  nothing.
- **TV remotes and controllers**: no more getting stuck in the search box, a drop-down list or the
  seek bar, and lists no longer change value as focus passes over them (OK opens them). OK brings up
  the player's buttons, and reaches Skip intro and Play when those are waiting. Disc menus hand over to
  the player's buttons at their edges. Episode cards and a disc's list of all titles can be reached.
  In books, OK brings up the toolbar and walking along it no longer turns the page.
### Remote access

Reach your server from outside your home ([REMOTE.md](docs/REMOTE.md)). Everything is off until you
turn it on in the new **Settings → Remote Access**, and nothing phones home.

- **A clear status:** whether the server can be reached, from where and how, based on what it has
  really seen.
- **Guided steps** for Tailscale, NetBird, forwarding a port, your own reverse proxy and a rented
  server. A hosted relay is shown greyed out as a possible future paid service; it doesn't exist.
- **A test with your own phone:** a QR code and a secret address; no outside service is involved.
- **HTTPS** without a restart, with Let's Encrypt or your own certificate (renewals are picked up by
  themselves), beside the home port.
- **Automatic port mapping** with UPnP or NAT-PMP, off by default. It only ever opens the HTTPS port,
  and closes it again. It also says when your provider shares addresses, so forwarding can't work.
- **Limits for viewers away from home:** highest resolution and most data per stream. Every encoder is
  told in the way it really obeys, direct play and direct downloads keep to them too, and playback at
  home is never affected.
- **The apps** ask for a server address: the mobile app has a screen for it, and the desktop app warns
  about unencrypted addresses outside a private network.

### Security

- **Connections from outside your home network are now refused until Remote Access is on.** Servers
  with `publicUrl` set count as on. Behind a reverse proxy, list it under the new trusted proxies
  (Settings, `trustedProxies` or `WMS_TRUSTED_PROXIES`). Forwarded headers are believed from nobody else.
- **Unencrypted connections from the internet are refused** or sent to the HTTPS address.
- **Two-factor sign-in:** optional codes from an authenticator app, with recovery codes, under the new
  **Settings → Sign-in & security**. `wms reset-2fa` is the way back for an administrator.
- **Your devices:** see where each account is signed in, and sign any of them out.
- **Sign-in limits** now also cover one address trying many names and one name tried from many
  addresses (away from home only, so nobody can lock you out at home), and how many passwords are
  checked at once. Changing a password counts too. Wrong tries and locks are shown in Settings.
- **Weak administrator passwords** are warned about when remote access is on.
- **Other websites** can no longer read the server's answers without a sign-in token, such as the list
  of profiles of a server on your network.
- Fixed: with *Show profiles outside your home* on, password-free profiles could sign in from outside.
- Players that can only sign in with `?token=` can now play HLS: the playlist carries it on to its
  segments, when that is how the playlist was fetched.
- Over HTTPS, the web app no longer puts its sign-in token in media addresses, where a reverse proxy
  would log it. The version is hidden from visitors on the internet who aren't signed in.
### TV channels and live TV

See [CHANNELS.md](docs/CHANNELS.md).

- **Channels from your library** that run all the time: tune in part-way through a programme, as on
  real TV. One click makes one from a TV network, an era, a genre or a show. Rules, day parts and a
  full weekly timetable are there for whoever wants them, and moving between them loses nothing.
- **As it aired**: shows in the day and time they first aired, where the metadata knows it.
- **Commercials libraries**, as many as you like. Each channel uses a whole library or only the videos
  you tick. Each library can be shown in the menus or kept out of the way.
- **A guide with a look of your choosing**: a bulky classic cable or satellite box, or clean streaming
  rows, with a line at the current time down all channels and a live preview you can put anywhere.
- **Live TV** through the new **WittyTuner** module: IPTV playlists and streams, network tuners, Linux
  DVB, capture cards, SAT>IP, external backends and a custom command. Most drivers have not been tried
  on real hardware yet, and say so.
- **Encrypted and copy-protected channels are detected and refused**, never decrypted. Every channel
  shows whether it is clear or encrypted, with a help page, and the same station from a clear source
  is offered when there is one.
- **Unencrypted ATSC 3.0** tunes, plays and records. The bundled FFmpeg has no AC-4 decoder, so channels
  with AC-4 sound play silently for now, and say so.
- **Guide data** from XMLTV and from the broadcasts themselves, and **recording** of live channels.
- One stream per channel however many are watching, and FFmpeg runs only while someone is.
### Games

- **Games libraries**: add one for a console, or for a folder of many consoles, with several libraries of
  one console allowed. Zipped games and multi-file games are understood. Bring your own game files.
- **Play in the browser**: games run on the server, each in its own sandboxed process, and stream with
  low latency over WebRTC. Where WebRTC can't connect, the player falls back to a WebSocket with WebCodecs,
  and then to JPEG, without restarting the game.
- **Every controller**: gamepads with hot-plug and up to four players, a keyboard, and a screen for
  changing what every button does.
- **Touch controls**: an on-screen controller per console, with an editor to move, resize, fade or hide
  each control, saved per console, device and orientation. Android gets haptics.
- **Saves**: battery saves and nine save-state slots per account with pictures, and a resume state written
  when you stop.
- **Settings, Gaming**: install cores on request with each licence shown first, and set up streaming.
- **Ready for TV apps**: controller layouts live on the server, and `docs/GAMING-API.md` documents everything a client
  needs. The player works with a TV remote's D-pad.
### Interface

- A new look across every page, built on one design system (see `docs/DESIGN.md`): one type scale,
  a spacing grid, shared buttons, fields, dialogs, cards and empty and loading states. All 20 themes
  are updated; a theme's `--radius` now shapes the whole interface.
- Backdrops run behind the top bar, rows of cards have arrows for the mouse, and dialogs are bottom
  sheets on phones.
- Faster on low-end devices: no blur on anything that scrolls or covers video, and long libraries
  skip the cards that are off screen.
- Accessibility: the featured banner can be paused, field outlines and switches reach 3:1, every
  target is at least 24px, and a focused control is never hidden under the bars.
- Rows show an album once instead of once per song; books no longer say "0 min".

## 0.4.0-alpha

0.3.0-alpha was never published as a release, so this is the first release with everything in it
as well (see its section below).

### Player

- **Scenes**: the Scenes button opens every scene as a picture with its time and name, like the
  disc page, with the one playing now marked.
- **Next moves on**: on the last scene, or in a short clip such as a warning or a trailer, Next goes
  on to what the disc plays next instead of starting the clip again.
- **No black flash** when a disc moves from one clip to the next (a menu to the next screen, Far Far
  Away Idol's show to its vote, a vote to its answer): the last picture stays up until the next
  clip has one.
- **Forced captions** can be turned off in the viewing settings, for one disc or everything.
- The disc remote fits its own edges with every engine name (WittyBluray was cut off).

### DVDs

- **Researched against the DVD-Video specification and the quirks real discs have**
  ([DVD-QUIRKS.md](docs/DVD-QUIRKS.md)): random and shuffle program chains, parental blocks, menu
  language fallback, junk cells used as structural protection, cells without NAV packs, every
  sub-picture control sequence with colour regions (CHG_COLCON), buttons that end when the disc ends
  them, and reserved bits in jump targets. Copy protection is still never decrypted.
- Clips that hold their last picture (menus, games, credits) are never restarted part-way, which
  left empty segments: Far Far Away Idol's credits and their music no longer race by.

### Libraries and setup

- **Built-in folder browser** (WittyIntegratedFileBrowser): choose the server's folders from any
  device, with drives and free space, your folders, a path you can type, filtering, several folders
  at once and full keyboard and screen-reader support. Read-only.
- **Folders of the same kind** join that kind's library or become a new, named one: set the default
  in setup and in *Settings → Libraries*, and choose either way when adding. Setup and Settings
  show how many libraries there are.
- **Server name and logo**: a Rename window with a live preview and a logo upload.
- Setup: a centred welcome, and no Continue button while you choose a folder.

### Accessibility

- All 20 themes meet WCAG 2.2 AA contrast for text and buttons; a new `--accent-text` colours
  links and highlighted words (see [THEMES.md](docs/THEMES.md)).
- Dialogs keep the keyboard while open and give focus back; page changes are announced; settings
  controls are named by their rows; the player says Play/Pause, reads its timeline, speaks what
  changes and lists its keys under **?**.

### Fixes

- Pictures load when the browser holds another server's (or an expired) sign-in cookie.
- The docked music player no longer moves the profile button; the profile's name shows beside it.


### Updates

- **One-click updates**: *Settings → About & legal → Updates* shows the installed and the newest
  version with its release notes. **Update now** downloads it, verifies it, installs it and
  restarts; the page reloads by itself. Admins see a dot on *Settings* when an update is out.
- **Only verified releases install**: each release's checksum list is signed with the project's
  Ed25519 key, built into the server. The server checks that signature and the package's
  SHA-256, downloads only from GitHub over HTTPS, and refuses anything that doesn't match.
- **Your data is never touched**: the new files are swapped in next to the running ones (old
  programs are set aside and removed on the next start); the data folder, `wms.json` and the other
  data folders are never written, and a failed install puts every file back.
- The server looks for new versions twice a day (it only asks GitHub for its list of releases);
  *Check for updates automatically* turns that off.
- Copies that can't replace their own files (macOS, Docker, `install.sh` installs owned by root, a
  read-only program folder) say why and link to the download.

### Windows

- **A setup program** (`…-windows-amd64-setup.exe`): installs for your account without
  administrator rights, adds a Start menu entry, starts at sign-in and appears in *Installed apps*
  with an uninstaller that keeps your library. Installed this way, the server updates itself.
  `wms-setup.exe` in the zip installs an unpacked folder the same way.

### For maintainers

- `cmd/wms-sign` makes the release signing key and signs checksum lists;
  `scripts/build-release.sh` signs each release (`WMS_SIGNING_KEY`, default
  `~/.wms-signing/release.key`) and builds the Windows setup. The release workflow signs with the
  `WMS_SIGNING_KEY` repository secret. See *Signing releases* in `docs/PUBLISHING.md`.

## 0.3.0-alpha

### Home

- **A home page that makes sense at a glance**: the featured carousel, then *Your libraries* as
  tiles (with *Add a library* for admins), one **Jump back in** row for everything you are
  watching, reading or listening to, *Next up*, what's new in each library, and *Can't decide?*
  Saved layouts start fresh once; sections you hid stay hidden.
- **First run**: with no libraries yet, home welcomes you and adds one right there, instead of an
  empty page.
- **Offline**: when the server can't be reached, home says so plainly and reconnects by itself,
  instead of showing "Failed to fetch".

### DVDs and Blu-rays

- **A DVDs & Blu-rays library type**, for a shelf of disc images, films and TV series together.
  Discs still work in Movies and TV shows libraries; the descriptions explain the choice.
- **Film or TV series**: each disc is sorted from its name (season, series, box set…) or its
  library, and you can set it yourself in *Disc settings*. Libraries holding discs get
  *Films* / *TV series* (or *DVDs & Blu-rays*) filters; TV libraries list their discs below the
  shows. Discs of a series are no longer looked up as films.
- Blu-rays are offered by *Can't decide?* again.

### Podcasts

- Adding a podcasts library explains what it holds (shows you follow, downloads, your own files)
  and goes straight on to **following shows**: search the free Apple Podcasts directory, or paste
  any RSS feed. The folder is optional: without one, episodes stream.
- *Keep downloaded* now reads "newest 5 episodes"; an empty podcasts library shows the same
  search instead of a bare message.

### Settings

- **Version and modules**, in *About & legal* and at the foot of the settings menu: this server's
  version and those of WittyDVD, WittyBluray, WittyDisc and the readers inside it.


### Disc engines as Go modules

- **WittyDisc, WittyDVD and WittyBluray v0.1.0**: tagged on their own repositories, so
  `go install github.com/wittywilla/wittydvd/cmd/wittydvd@latest` (or `…/wittybluray/cmd/wittybluray`)
  works without a WittyMediaServer checkout. In this repository a `go.work` builds them from
  local source; `scripts/mirror.sh <Repository> <vX.Y.Z>` publishes and tags a release.

### WittyDVD

- **Angles work**: interleaved multi-angle blocks are read per angle, so intros no longer flicker
  between language versions. Pick a *Video version* while watching; playback continues
  from the same moment.
- **Disc memory**: the audio language, subtitles and angle you pick are remembered per disc and
  put back next time, like a DVD player's last-memory feature.
- Fixed features that were treated as menus (no seek bar, no audio/subtitle choice).
- Fixed playback stalling a few seconds into some titles: streams are now matched by their DVD
  stream id, so cells that list their streams in a different order keep their sound.
- Damaged audio frames no longer stop a transcode (which used to leave the player stuck on a
  warning screen); a watchdog also moves on when a clip stalls right before its end.
- No black flash after pressing a menu button.

### Disc page

- **Feature Presentation** first, with a thumbnail for every scene, then **Bonus Features** and
  shortcuts straight into the **Disc Menus**; the full title list is folded away underneath.
- Admins can name the feature, scenes and bonus features; the names show in the player too.
- Thumbnails that cannot be made show "Preview not available" after a few attempts.

### Player

- Big TV-style tile pickers for **Audio**, **Subtitles** and **Video version** (angles).
- **Viewing settings** (gear): seek bar, chapter marks, remote, remote style, hide delay,
  fit/fill picture, subtitle size — for this disc/video or for everything.
- **Playback speed** from 0.5× to 4× on every kind of media with the pitch of voices kept;
  admins can allow up to 16× in *Settings → Server* (with a hardware warning).
- **Mini player**: *Back* keeps the video playing in a corner window (a bottom bar on phones)
  with play/pause, chapter skip, disc Menu/Title buttons, expand and stop.
- The remote: labelled **Remote** button in the control bar, resizable from every edge and
  corner, see-through tooltips (*Eject the DVD*, *Close the Remote*), and an *In control bar*
  style that puts its buttons in a strip above the seek bar.

### First run and sign-in

- **Setup wizard**: welcome → administrator account → libraries → posters & info → done, with a
  step indicator. Libraries are added by kind (Movies, TV Shows, Home videos).
- **System folder picker**: on the server's own screen, *Choose folder…* opens the native
  dialog (the modern Explorer window on Windows, Finder on macOS, the desktop's dialog on
  Linux); other devices browse the server's folders in the page.
- **Metadata keys in setup**: step-by-step instructions for TMDB and TheTVDB with a *Test*
  button. TMDB accepts either the API key or the API Read Access Token.
- The browser opens the setup page by itself on first start (not on servers, containers or
  with `WMS_NO_BROWSER=1`).
- **"Who's watching?"** profile screen; *Settings → Users → Sign-in screen* switches back to
  the classic form. Profiles are only listed on the home network unless allowed for remote
  devices.
- Sign-in is rate-limited: after 5 wrong passwords for a name, each try waits longer (up to
  15 minutes). First-run account creation cannot be raced.

### Books, comics and manga

- New **Books** library type for EPUB, PDF and CBZ (CBR/CB7/MOBI/AZW3 are listed and can be
  downloaded). Files sharing a name are one book read from the best format; digital-library
  extras (OCR text, scans, DRM-locked copies, metadata databases) are ignored, while their
  `_meta.xml` and thumbnail are used.
- Metadata from the EPUB package (title, author, series, description, cover), `ComicInfo.xml`
  (series, number, writer, summary, manga reading direction, age rating) or the file name
  (`Series 145 (2023) (Digital)` → series, number, year). Folders named "Manga" read right to left.
- **Reader**: comics page by page, as two-page spreads or a vertical strip, left-to-right or
  right-to-left, with tap zones, swipes, keys and preloading; EPUBs paged or scrolling with
  font size and light/sepia/dark pages and the contents; PDFs in the browser's viewer.
  Progress is saved per account.
- Security: book HTML is served with a policy that blocks scripts, forms and outside requests,
  and is shown in a sandboxed frame.

### Podcasts

- New **Podcasts** library type: follow shows by searching the Apple Podcasts directory or by
  pasting a feed address; feeds are checked every 2 hours.
- Episodes play through the server (private; old http:// feeds work; seeking supported) and can
  be **downloaded** into the library folder, by hand or automatically (keep newest N).
- Episodes resume where you stopped; explicit episodes are hidden from Kids and Guest profiles.
- Security: feeds and episodes may only be fetched from the internet — never from the server
  itself or the local network.

### Music and audiobooks

- New **Music** and **Audiobooks** library types: tags (artist, album, album artist, track,
  disc, year, genre), folder or embedded covers (the largest image wins), folder-name fallbacks.
  Nothing is written into music folders.
- Albums / Artists / Songs views (Books / Authors for audiobooks), album and book pages with
  Play, Shuffle, Add to queue and Continue.
- **Audio player bar** that keeps playing while browsing, full *Now playing* view with the
  queue, shuffle and repeat; audiobooks resume on every device, speed up to 3× with pitch kept,
  ±15/30 s, sleep timer (incl. end of chapter); lock-screen and media-key controls.
- MP3/AAC/FLAC/Opus/Vorbis/WAV play directly; other formats (ALAC, WMA, APE…) are converted to
  AAC while playing, with seeking.
- Audit: every FFmpeg/ffprobe process now starts without a console window on Windows (one
  shared helper replaces two copies).

### Anime

- New **Anime** library type with **AniList** information (no key): titles in English, rōmaji
  or Japanese, synopsis, studio, score, cover, banner, episode names; adult titles are rated 18.
- Anime in TV libraries (genre *Anime*) also shows in the Anime library; a setting can show it
  **only** there. An Anime library may have no folders and just collect anime from TV.
- **Absolute episode numbers** (`[Group] Show - 012 [1080p].mkv`, `Show - 05v2`) are parsed;
  release tags ignored, years never mistaken for episodes.

### Layout and home page

- **Navigation where you want it**: side menu, top bar, or both (per device, *Settings →
  Appearance → Layout*). Phones always get a bottom tab bar (Home, Search, Libraries, You) with
  room for the mini player above it.
- **Home page widgets**: *Customize* to drag sections into any order (or use the arrow buttons
  on touch screens, TV remotes and keyboards), hide and show them, or reset. The layout is saved
  to your profile and follows you to every device.
- New widgets: **Your libraries** (big shortcut tiles) and **Shuffle** ("Can't decide?" — a
  random movie or disc with Play and Another).
- Account menu in the corner: Appearance, Settings, **Switch profile**, Sign out.
- Larger touch targets on touch screens; the interface scales up on 4K screens and TVs.

### Metadata without keys

- **TV shows work out of the box** with **TVmaze** (no key): titles, summaries, networks,
  status, posters, backdrops, season posters and episode names. TheTVDB is used instead when a
  key is set; shows switch over on their next scan.
- **Movies get facts without a key** from **Wikidata** and Wikipedia: year, genres, US rating
  and a plot summary (posters still need TMDB). *Refresh* re-reads a title.
- Each title credits its source; both can be switched off in *Settings → Server*.
- **Content rating** can be set by hand on movies and shows (for parental controls).
- The setup wizard shows what is built in before asking for optional keys.

### Profiles and parental controls

- **Kids** and **Guest** profiles next to adult accounts. Kids and Guest profiles may have no
  password: they then sign in with one tap, but only on the home network.
- **Limits per account**: an age level (worked out from US, UK, German and TV ratings), titles
  without a rating on or off, allowed libraries, hidden genres, and *always allow* / *never
  allow* exceptions for single movies and shows. Live TV switches (all Live TV, channels made
  from your libraries, tuner/IPTV channels) are stored for the coming Live TV feature.
- Limits are enforced by the server everywhere (lists, home, search, show pages, playback,
  artwork, recommendations); hidden titles answer "not found". Unrestricted accounts pay no
  extra cost; unreadable limits fail closed.
- Adults may add and manage Kids profiles unless the admin keeps that to themself; Guest and
  adult accounts are admin-only. The setup wizard offers to add a Kids profile.
- The Insert DVD sound can be switched off separately from the animation.

### Config file, portable mode and logs

- **`wms.json` config file** for launch settings (port, folders, FFmpeg, transcode limit, log
  level), found next to the program, in the user config folder, or via `-config`. Typos are
  reported. `wms config path` / `wms config init [--portable]`. See [docs/CONFIG.md](docs/CONFIG.md).
- **Portable mode**: a `wms.json` next to the program keeps all data in that folder.
- **Custom folders** for metadata artwork, cache, transcodes and logs.
- **Logs**: daily files with opt-in categories (playback, discs, FFmpeg, library, web
  requests, app errors), secrets always removed, kept 14 days. *Settings → Logs & privacy*
  explains that logs never leave the server, and has the folder, an *Open folder* button and
  downloads. The web app reports its errors to the server only when allowed there.

### TV, controllers and accessibility

- Arrow keys / D-pad move between controls on every page; Back returns. Game controllers
  (Xbox, PlayStation, Nintendo, generic) work through the Gamepad API.
- Screen readers: labelled landmarks, live announcements, DVD menu options as real buttons,
  visible focus, reduced-motion and high-contrast support.

### Interface

- Fixed missing borders in the default look (a circular colour variable).
- Theme cards: the whole card is clickable and switching themes is reliable (the new theme
  replaces the old one only once it has loaded).

## 0.2.0-alpha

### WittyDVD (the DVD engine, formerly `internal/disc/dvd`)

- Renamed the DVD engine to **WittyDVD** (`internal/wittydvd`).
- **Interactive titles**: buttons inside titles (games, quizzes, voting, such as one disc's
  *Far Far Away Idol*) now work; such titles play cell by cell with their buttons and timers.
- **Insert DVD** button with an optional disc-tray animation and synthesized drive sound
  (per disc and server-wide), and a per-disc start mode (menu or main feature).
- **On-screen DVD remote** (top menu, menu, d-pad, return, audio, subtitle, chapter, transport
  and number keys); movable, resizable and translucent; also available for regular videos with
  disc-only keys greyed out.
- **Audio** tracks map through the disc's audio control table (the right language plays), are
  labelled with their purpose (*Commentary*, …), and switching keeps your position.
- **Subtitles** are rendered from the disc's own sub-pictures in the player: instant switching,
  forced subtitles, purpose labels.
- **Menu** and **Resume** remember the exact frame; *Return* key; *Play movie* works on discs
  that guard their feature.
- Fixes: menus that are a single still frame no longer stall; a still first-play no longer
  hangs; highlights parsed for "same as previous" NAV packs; forced-select/forced-action buttons;
  navigation timer; no black flashes or highlight flicker between menu clips.
- Faster on small machines: cached menu scans and decoded commands, chunked disc reads, shared
  file handles (fixes a handle leak), next-clip prefetch, low-power mode.
- New `wms dvd crawl` command presses every button of every menu.

### TV shows

- New **TV shows** library type: show → season → episode, multi-episode files, specials.
- **TheTVDB** metadata: overview, network, genres, status, content rating, posters, season
  posters, episode titles/overviews/air dates/stills; manual matching by TheTVDB id.
- Show pages with **Cards** or **Playlist** layout (per show + server default), **Next Up**,
  **Skip intro** (chapters or per-show range), **Watch next**, recommendations.
- Networks/genres API (`/api/shows/networks`, `?network=`, `?genre=`) for future TV channels.

### Library and metadata

- Metadata and artwork are **saved next to media** in the standard NFO + artwork layout other media software reads
  (`.nfo`, `poster.jpg`, `fanart.jpg`, …), with backups of hand-written NFOs.
- **Artwork picker** (local files, TMDB, TheTVDB) for movies, discs, episodes and shows.
- Movie recommendations at the end of a film (sequels first).

### Transcoding

- **Settings → Transcoding**: direct-play toggle; one checkbox per hardware encoder (NVENC,
  Quick Sync, VideoToolbox, AMF, VAAPI), each test-encoded at start-up and greyed out when
  missing; **Easy / Advanced / Expert** quality modes with tooltips;
  [transcoding guide](docs/TRANSCODING.md) with GPU/CPU capability tables.
- VAAPI and Quick Sync now upload frames correctly (nv12 + hwupload).

### Interface

- **Themes**: 20 built-in themes (dark, light, OLED and colour variants), custom themes as ZIPs
  in the `themes` folder, per-device or server-wide; [theme guide](docs/THEMES.md).
- Custom server name in the top-left corner.
- Player controls auto-hide outside fullscreen too.
- Version shown at the bottom of the sidebar and the sign-in page.

### Builds and releases

- Release archives for **10 platforms**: Windows x64/ARM64, macOS Intel/Apple Silicon, Linux
  x64/ARM64/ARMv7/x86, FreeBSD x64/ARM64, plus a SHA256 checksum file; Docker images for
  amd64, arm64 and armv7.
- Fixed the FreeBSD FFmpeg build (x264's configure needs bash) and moved it to FreeBSD 14.4;
  a failed FreeBSD FFmpeg build no longer blocks the release.
- macOS FFmpeg downloads fall back to snapshot builds when release links are missing.
- Archives built on Windows now keep the executable permission for Linux/macOS/FreeBSD files.
- CI now runs on `master`; GitHub Actions updated to their Node 24 versions.

### Documentation

- New guides: [Getting started](docs/GETTING_STARTED.md), [DVDs](docs/DVD.md),
  [TV shows](docs/TV.md), [Metadata](docs/METADATA.md), [Transcoding](docs/TRANSCODING.md),
  [Themes](docs/THEMES.md), [Contributing](CONTRIBUTING.md), [Publishing](docs/PUBLISHING.md).

## 0.1.0-alpha

- First public build: Go media server with native DVD-Video menu navigation, libraries,
  metadata, HLS transcoding with instant seeking, users, tray launcher, installers, Docker and
  Kubernetes manifests.
