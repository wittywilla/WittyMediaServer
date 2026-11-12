# Games: the API for clients

For developers writing a client: everything needed to play a game, on a TV app or anything else. To play games, see [Games](GAMES.md).

The web player uses exactly these routes and has no private ones. Conventions (sign-in, errors, versions) are those of
[the client API](CLIENT-API.md), which also lists these routes under `/api/v1`, with the game's details and matching. Paths below are written without `/api`.

The server runs the game. A client shows a video stream, plays an audio stream and sends controller
states, so there is no emulator or game logic to write. The routes are in `internal/server/api_games*.go`,
`api_game_options.go` and `api_game_profiles.go`. What runs behind them is the WittyPlay module: [the design](GAMING.md) explains it.

## Playing a game

1. List games with `GET /items?library=<id>&kind=game`. A library's `console` and an item's `console` say which
   console it is. A game without a picture answers 404 on its poster: draw a placeholder, as for any item.
2. `GET /games/consoles` says what each console has (`pad`) and whether it can be played now (`ready`, and when
   it can't, `reason` and `message`). See [Games this server can't draw](#games-this-server-cant-draw).
3. `GET /consoles/{console}/controls?device=<your device id>` gives the controller as data (below).
4. `GET /games/{id}/saves` lists saved states. If slot 0 exists, offer Resume.
5. `POST /games/{id}/play` with `{"resume": true}` or `{"resume": false}` starts it and answers the session,
   including `modes`.
6. Connect with the first mode in `modes` you can use. The game starts once a viewer is connected. A mode with
   `"stream": 2` takes **version 2 of the stream**, one route and the same messages for every device:
   [the stream](GAMING-STREAM.md). New apps should use it; what follows here is version 1, which keeps working.
7. Send controller states. Show a menu of your own for saving, loading and quitting.
8. `DELETE /play/{sid}` saves and stops it. The answer comes once everything is written.

An account plays one game at a time: starting another saves and ends the first, on any device. A game
is stopped and saved after 60 seconds with nobody connected, and after 30 minutes with no controller
changes. The picture comes back if you reconnect within the first limit, so a brief network drop or a
trip through the app switcher loses nothing.

## Playing with a standalone emulator

A server whose `features` include `standalone-emulators` may offer a game in a standalone emulator as well as on a core. For Xbox, PlayStation 3 and Wii U it's the only way: a game's `check` names the emulator, and each console in `GET /games/consoles` carries `standaloneEmulators` (as below) and `standalone`, the one Play starts.

1. `GET /games/{id}/emulators` answers `{"emulators": [{"id", "name", "version", "stream", "ready", "why", "experimental", "needsBios"}]}`. The list is empty when nothing is set up for the game's console: offer nothing then. When `ready` is false, `why` is a sentence to show beside a disabled button.
2. `POST /games/{id}/play/emulator` with `{"emulator": "<id>"}` starts it: `201` and

   ```json
   {"id": "…", "emulator": "pcsx2", "name": "PCSX2", "backend": "docker+webrtc",
    "connect": "/api/v1/modules/wittyplay/docker/sessions/…/connect",
    "stop": "/api/v1/modules/wittyplay/docker/sessions/…",
    "pause": "…/pause", "resume": "…/resume",
    "width": 1280, "height": 720, "fps": 60, "players": 4, "notices": []}
   ```

   `409` with `{"error", "code"}` is a refusal to show as it is (not installed, needs firmware, this server is too small for it …); `503` means as many are running as are allowed. Posting again for the same game while it runs answers the same session, so a reloaded page rejoins it.
3. `backend` ending in `+webrtc`: connect exactly as to a core's game over WebRTC ([the stream](GAMING-STREAM.md#webrtc)): a `recvonly` video and audio transceiver, the `input` and `events` data channels, then `POST` `{"hello", "offer"}` to `connect` and apply the answer's SDP. Send pad states for players 0 to `players - 1`, and pointer or touch messages for the mouse. There are no seats, save states, core options or timing messages: build only picture, sound, input, pause and stop.
4. `backend` `docker+kasmvnc`: the answer has `kasmUrl` instead of `connect`. Show it in a frame sandboxed with `allow-scripts allow-pointer-lock`, **never** `allow-same-origin`, and send no sign-in with it: the ticket in the address is the whole of the way in.
5. `POST pause`, `POST resume`, `DELETE stop`. On `events`, `{"type": "ended", "reason", "text"}` says the game is over and why. A game with nobody connected for two minutes is stopped.

Each emulator's settings are data: `GET /modules/wittyplay/docker/emulators/{id}/settings?game=<id>` gives a `schema` to draw a form from and the values in three layers (`admin`, `user`, `game`); `PUT` the same address with `{"scope": "user"|"game"|"admin", "game", "values"}`. A field's `scope` says who may set it.

## Routes

| Route | Sign-in | What it does |
|---|---|---|
| `GET /games/home?library=` | user | The games home screen in one answer: `hero`, `continue` (`{item, plays, lastPlayed, save}`), `recent`, `favourites`, `favouriteIds`, `mostPlayed`, and `shelves` (`{kind: "genre" or "series", name, count, items}`) |
| `GET /games/{id}/check` | user | What a game needs and whether this server has it: `{ready, reason, message, core, emulator, needsGpu, gpuMissing, software, notice, encrypted, bios: [{file, for, required, found, using, from}]}`. For a BIOS a core takes under any name (PlayStation 2), `using` is the file this game starts with, and `from` is where the server found it by its checksum. `reason` is `core`, `bios`, `gpu`, `emulator` (no core plays it, and no standalone emulator is set up) or `unavailable`, and `message` says it for people. `emulator` (`{id, name, version}`), with `ready` true, means no core plays it here and that standalone emulator does: start it as [Playing with a standalone emulator](#playing-with-a-standalone-emulator) says. See [Games this server can't draw](#games-this-server-cant-draw) |
| `GET /games/consoles` | user | Every console, with `pad`, `ready`, `unavailable`, `cores` and `installed`, and its `era`, years, `media`, `capabilities` and `shelf` ([below](#what-a-console-can-do)). When it isn't ready, `reason` (`unavailable`, `core` or `gpu`) and `message`. `gpuMissing`, `software` and `notice` as for a game's check, with `softwareWhy`, `playsOn` and `biosMissing`. `coreSupport`: [which platforms each core runs on](#which-platforms-a-core-runs-on). With `?library=<id>`, also `count`: that library's games for it |
| `GET /games/playing` | user | The game this account is playing now: `{id, itemId, title, console, device, startedAt, seats}`, or 204. What a device needs to offer "Continue here" |
| `GET /games/modes?client=native` | user | What this server can stream, best first |
| `GET /consoles/{console}/controls?device=` | user | The controller as data |
| `PUT /consoles/{console}/controls` | user | Saves one layout, or removes it |
| `POST /games/{id}/prepare` | user | Gets a game ready without waiting: `{state, done, total}`, see below |
| `GET /games/{id}/versions` | user | `{versions: [{id, label, current}], inside?: [{entry, label, current, altered}], discs?: [{disc, name}]}`: its other versions as games of their own, and for an administrator the versions inside its archive, and its discs |
| `PUT /games/{id}/version` `{entry}` | admin | Which file in its archive to play, or `""` to choose by the rules |
| `POST /games/{id}/split` | admin | A version becomes a game of its own, and stays one |
| `PUT /games/{id}/console` `{console}` | admin | This file is for another console. A scan follows |
| `POST /games/{id}/not-a-game` | admin | Takes it off the list. Its file stays, and it can be brought back from the review list |
| `GET /libraries/{id}/games/review?kind=needs\|blocked\|left\|ignored` | admin | Files that didn't become games, or can't play: `{items: [{id, path, name, size, kind, reason, suggest: [{console, name}], itemId}], counts, total}` |
| `POST /libraries/{id}/games/review/apply` `{ids, action, console}` | admin | `add` says they're games for a console, `ignore` that they aren't games, `forget` takes that back. Up to 5,000 at a time. A scan follows |
| `GET /libraries/{id}/games/import` | admin | How an import is going: `{scanning, scan, lookup, review, games}` |
| `GET /game-profiles`, `POST /game-profiles` | user | This account's controller profiles; makes one |
| `GET`, `PUT`, `DELETE /game-profiles/{id}`; `POST /game-profiles/{id}/copy`, `/active` | user | One profile, only the account's own |
| `GET /game-profiles/resolve?kind=&controller=&console=&game=&raw=` | user | What a controller should do here: `{profile, map or keys, options}` |
| `GET /game-profiles/export?id=&id=`, `POST /game-profiles/import` | user | A file of profiles out, and in |
| `GET /game-controllers` | user | Controllers known by vendor and product. Browsers need it; an app usually doesn't |
| `GET`, `PUT`, `DELETE /game-background`; `GET`, `PUT /game-background/image` | user | The account's own picture behind the controller screen |
| `POST /games/{id}/play` | user | Starts a game. Body: `{"resume": bool, "client": "native"}` |
| `GET /games/{id}/saves` | user | This account's saved states: `[{slot, updatedAt, size, hasThumb, core}]` |
| `GET /games/{id}/saves/{slot}/thumb` | user | A state's picture (PNG) |
| `DELETE /games/{id}/saves/{slot}` | user | Deletes a state |
| `GET /play/{sid}` | owner | The session |
| `POST /play/{sid}/webrtc` | owner | Connects over WebRTC. Body: `{"sdp": offer, "prefer": ""}`; answers `{"sdp": answer}` |
| `GET /play/{sid}/ws?video=h264` | owner | Connects over a WebSocket; `video` is `h264` or `jpeg` |
| `POST /play/{sid}/stream` | owner | Version 2: connects over WebRTC. Body `{"hello": {...}, "sdp": offer}`; answers `{"stream": {...}, "sdp": answer}`. [The stream](GAMING-STREAM.md) |
| `GET /play/{sid}/stream` | owner | Version 2's WebSocket, the same messages: the hello first, as text |
| `POST /play/{sid}/action` | owner | `{"type": "save" or "load", "slot": n}`, `{"type": "reset"}`, `{"type": "pause", "on": bool}`, `{"type": "disc", "slot": n}` to change disc, from 0, in a game whose session says it has `discs` above 1, `{"type": "device", "port": n, "device": id}`. [More while a game is running](#while-a-game-is-running) |
| `DELETE /play/{sid}` | owner | Saves and stops |
| `POST /play/{sid}/invite` | owner | A code others can join this game with: `{code, expiresAt}`, four letters, ten minutes. See [Seats](#seats) |
| `POST /play/join` | user | `{code}` joins someone's game: `{sid, seat, itemId, title, console, host}` |
| `GET /play/{sid}/seats` | owner or joined | `{host, seats, code?}`: the host, who joined and their seats; the code for the host only |
| `DELETE /play/{sid}/seats/{user}` | owner or that person | Frees a seat |
| `GET /gaming`, `POST /gaming/cores/{id}/install`, `DELETE /gaming/cores/{id}`, `DELETE /gaming/unpacked` | admin | Server setup. An app can leave this to the web interface |
| `/gaming/custom/...`, `/gaming/consoles/{console}/core`, `/gaming/games/{id}/core` | admin | An administrator's own cores and custom consoles, listed in [the client API](CLIENT-API.md). An app can leave this to the web interface too. A custom console is like any other console to every route here: its ID starts with `custom-`, and `pad` may be a whole RetroPad |

A session belongs to the account that started it; to anyone else its routes answer 404. One game takes at most 8 viewers and 4 connecting at once: past that the oldest goes, and a connection may answer 429 with `Retry-After`.

Games in a shared PC's library (`console: rig`) go another way: see [the client API](CLIENT-API.md#shared-pcs-and-wittyrigs).

Starting a game can answer:

| Status | Meaning |
|---|---|
| 201 | Started: the session |
| 404 | No such game for this account, or its file has gone |
| 409 | Can't be played yet. `needCore: true` with `console` means an admin has to install a core. `needBios: true` with `console` and `file` means a BIOS is missing, and `error` says where to put it. `reason: "gpu"` with `needsGpu: true`, `gpuMissing` and `console` means it can't play on this server at all: its console is drawn on a graphics card, this server has none it can use, and no core for it can do without. Nothing was started. `rig: true` means this game plays on a PC. Otherwise `error` says why |
| 422 | The file can't be used. `encrypted: true` means it's an encrypted 3DS file, which will never play: `error` says so. Nothing here decrypts a file. A game's check says the same first: `reason: "encrypted"`, `encrypted: true`, and `message` is this `error`, before any reason this server could fix |
| 503 | The server is running as many games as it allows, or as many as its graphics card is allowed, or its game runner is missing |

### Games this server can't draw

Nintendo 64, PlayStation 2, PSP, Dreamcast, 3DS and the faster PlayStation cores draw on a graphics card. A server
without one it can use (a Mac, FreeBSD, or a PC whose only OpenGL is drawn by its processor) says so before anyone
presses Play, in the same words everywhere, and refuses to start such a game rather than letting the emulator crash:

| Field | In | Meaning |
|---|---|---|
| `ready: false`, `reason: "gpu"` | check, consoles, play's 409, prepare's 409 | Not on this server. Show `message` (`error` on a 409) instead of Play. An admin can't fix it in Settings |
| `message` | check, consoles | Not on this server, and why, for people. Play's `error` is the same text |
| `gpuMissing` | check, consoles, play's 409 | Only the reason, such as `hardware-rendered games run on Windows, Linux and macOS servers`, for a sentence of your own |
| `needsGpu` | check | The core it starts on draws on this server's graphics card |
| `software: true` | check, consoles | It plays, but the server's processor draws it, as there's no card here, or, on a Mac, because its core draws wrongly on the card there. Show `notice` before it starts: it can run slower and keeps several of the server's cores busy. The session's `notices` say it again once it starts |
| `notice` | check, consoles | That speed caveat, in words for people |
| `softwareWhy` | consoles | With `software`: `no-card` (no card here games can use) or `card-fault` (a card, and the core draws wrongly on it on this system) |
| `playsOn` | consoles | The core Play starts its games on, unless a game has a core of its own: not always the first in `installed` (on a Mac, PlayStation 2 plays on Play!) |
| `biosMissing` | consoles | When the core it wants can't start for want of a BIOS: a file it would take |

On a server with a card, none of these appear except `needsGpu`. A console whose cores can't draw without a card
(Dreamcast, 3DS) shows `reason: "gpu"` even with no core installed, since installing one wouldn't help. Nintendo 64,
PSP and PlayStation 2 have cores that can have the processor draw, so with none installed they say `reason: "core"`.
With `reason: "gpu"`, `runsOn` lists the server platforms its games would play on (see below).

### Which platforms a core runs on

Every device plays every game: the server runs it and streams it. What decides whether a game plays is the server's
platform: whether its core is published there, and how it draws there. `GET /games/consoles` and `GET /gaming` give,
for each console, `coreSupport`: for each core on offer, `{here, platforms}`.

| Field | Meaning |
|---|---|
| `platforms` | One entry for each server platform: `{os, arch, build, draws, api, note}`. `os` is `windows`, `linux`, `macos` or `freebsd`; `arch` is `x64`, `arm64` or `arm32` (none for FreeBSD, which has no prebuilt cores) |
| `here` | The same for this server, with this server's graphics card taken into account |
| `build` | A download is published for that platform. Without one, the core isn't offered to install there |
| `draws` | `gpu`: on the server's graphics card. `cpu`: by the processor, as the core always is. `cpu-fallback`: a graphics card's core drawn by the processor because the server has no card it can use, slower. `none`: it can't play there |
| `api` | With `draws: "gpu"` only: how the runner reaches the card. `opengl` today, everywhere; `vulkan` and `metal` are reserved for when the runner offers them |
| `note` | In words: what that means, or the speed measured, such as "Full speed on an Apple M3 Pro, on about one processor core." |

Where it shows: Settings → Consoles & cores, for each console and each core; the core a console or a game plays with
(`GET /gaming/games/{id}/core` gives each option's `here`); and a game's page, where `GET /games/{id}/check` adds:

| Field | When | Meaning |
|---|---|---|
| `runsOn` | `reason: "gpu"` or `"core"` | `[{os, arch, build, draws, api, note}]`: the platforms where one of its console's cores plays it, with the best way it draws there |
| `alternative` | `reason: "core"` | `{id, name, console}`: a core that would play it on this server, once installed |
| `noBuild: true` | `reason: "core"` | Not one of its console's cores is published for this server's platform, so installing can't help: show it as not on this server, with `runsOn` |

**Downloads.** `internal/server/corebuilds.txt` says which platforms each core is published for, from libretro's build
server; CI checks every listed download weekly (`scripts/corebuilds`). `POST /gaming/cores/{id}/install` for a core
with no build for this server answers 409 `{error, noBuild: true, platform, alternative}`, where `platform` is like
`macos/arm64` and `alternative` is `{id, name, console}`: a core that plays the same console here. If the download
server answers 404 anyway, the answer is 502 with the same fields.

### Choosing a core

What a core picker needs, for an administrator: a console's default (`GET /gaming/consoles/{console}/core`) or one
game's own core (`GET /gaming/games/{id}/core`). Both answer:

| Field | Meaning |
|---|---|
| `console` | The console's id |
| `choice` | The core chosen at this level (the game's own, or the console's default), or `""` for none |
| `consoleDefault` | For a game: its console's default, or none |
| `effective` | The core that plays now, once a missing BIOS or graphics card is taken into account; `""` when none would |
| `canDownload` | This server's platform has prebuilt cores to install |
| `choices` | Every core that can play the console, in the server's order of preference, then the administrator's own |

Each choice is `{id, name, license, installed, source, canInstall, recommended, strength, about, support, bios}`:

| Field | Meaning |
|---|---|
| `installed`, `source` | It's on this server, and from where: `installed`, `retroarch`, `system` or `custom` |
| `canInstall` | It isn't installed, and installing it here would work: published for this platform, able to draw here, and this platform has downloads |
| `recommended` | The one this server would pick: the first published for this platform that can draw here |
| `strength` | What it's chosen for beside the console's other cores: `accuracy`, `speed` or `balanced`. None for a console's only core, or a custom one |
| `about` | One line on what it's good at, such as "The most accurate, and plays every game. Uses more of the processor." |
| `support` | `{here, platforms}`, as `coreSupport` above |
| `bios` | When it won't start without a BIOS file of the owner's own: `{files, found}`, any one of `files` |
| `starts`, `failures`, `seconds`, `lastError`, `best` | For a game: how it has gone on that game |

`GET /games/consoles` gives the same `strength` and `about` for each core as `coreAbout`, for anyone.

To choose a core that isn't installed: `POST /gaming/cores/{id}/install`, then the `PUT` beside the `GET` with
`{core}`, which only takes an installed core. While the install runs, `GET /gaming/cores/{id}/install` answers
`{installing, received, total}` in bytes (`total` 0 when the download server gave no size; a core with support files,
such as PPSSPP, counts again from 0 for those). The website asks about four times a second. An older WittyPlay module
always answers `installing: false`; show the install as under way without a size.

A save state is made by one core and only loads in that core; in-game saves (battery saves and memory cards) stay with
the game. `GET /games/{id}/saves` says which core made each state (`core`, its name).

**A game in an archive** has to be unpacked before it starts, which can take minutes for a large disc on a small server.
`POST /games/{id}/play` takes that long, which is fine as long as your HTTP client's timeout allows it. To show
progress, call `POST /games/{id}/prepare` about once a second until `state` is `ready`, and then call play:

| `state` | Meaning |
|---|---|
| `ready` | Nothing to wait for. Every game not in an archive answers this straight away |
| `waiting` | Other games are unpacking first |
| `reading`, `unpacking` | `done` of `total` bytes so far. `total` is what the archive says, so it can be 0 |

It can answer 404, 409 and 422 the way play does. An unpack nobody has asked about for two minutes is stopped, so leaving
the screen needs nothing else.

**The session** is `{id, itemId, title, console, core, width, height, aspect, fps, sampleRate, resumed, encoder, turns, disc, discs, renderer, modes, devices, safeMode, maxSpeed, notices}`. `renderer` names the graphics card, for a game drawn on one. `turns` is quarter turns to draw the picture by. `disc` and `discs` are which disc is in the drive, from 0, and how many the game has.
Show the picture at `aspect` (width over height), not at the video's own shape: the video is often
doubled, and consoles' pixels often aren't square.

`devices`, when there, lists the kinds of controller the game's emulator lets each port choose from:
`[{"port": 0, "current": 261, "types": [{"id": 1, "name": "Standard"}, {"id": 261, "name": "DualShock"}]}]`. The console's
best choice is already plugged in, so a PlayStation game starts with sticks and rumble where its emulator has them. To let
someone change it, send a `device` action. Anything the emulator didn't offer answers 422.

**Save slots:** 0 is the automatic one, written when a game stops, and can only be loaded. 1 to 99 are manual. 100 is the game from before the last load, and 101 the automatic save from before starting over: both can only be loaded.

## What a console can do

`GET /games/consoles` gives every console what a client needs to lay out its games and build its game menu, so a new
console needs data, not new screens:

- `era` (`first`, `8-bit`, `16-bit`, `early-3d`, `disc`, `pocket`), `launched` and `ended` (years, roughly), and `media`
  (`cart`, `pocket`, `disc`).
- `capabilities`: `screens` (`count`, which one takes `touch`, `layouts` and, by core, the core option and values that
  set each layout and swap the screens; and `frame`, where each screen is in a frame of the stacked layout the core starts
  in, as `[{x, y, w, h}]` fractions of the frame, for drawing each screen where someone placed it), `input` (`ports`, `multitap`, `sticks`, `analogTriggers`, `touch`, `lightGun`,
  `mouse`, `motion`, `keyboard`, `mic`), `media` (`discs`, `memoryCards`, `bios`: none, optional or required, and by core
  the option that sets the `region`), `play` (`rumble`, `rewindByDefault`) and `picture` (`aspect`, `integer`, and the
  `look`: the display preset it starts with). What a running core really has is in the session (`discs`, `devices`).
- `shelf`: how its games stand on a shelf, for a games home. `case` and `cart`, each `{kind, aspect, height}`: the case is
  a `box`, `clamshell`, `jewel` or `keep` case; the cartridge a `cart`, `card` or `disc`; `aspect` is the front's width over
  its height, and `height` how tall it stands beside others of its kind (a full-size box is 1). Draw them as plain
  shapes around the game's own cover: never a console maker's banners, logos, colours or outlines.

Rewind is on by default for consoles with `rewindByDefault` (the 8-bit, 16-bit and older handheld ones, whose states fit
the default rewind memory): it shows as the everyone-on-this-console setting until an administrator says otherwise, and
a person's own choice wins over both.

## Seats

Playing one game together from several devices. The account that starts a game is its host, and its own devices can
always watch and play it. Anyone else signed in to this server needs a code: the host asks for one
(`POST /play/{sid}/invite`), four letters that last ten minutes, and the other device sends it to `POST /play/join`.
Adults, Kids and Guest profiles join the same way, and only with a code; a profile whose limits wouldn't let it open
the game can't join it (403). Wrong codes are limited per account (429 with `Retry-After`). Someone who joined may get
the session, connect over WebRTC or the WebSocket and send their seat's controller states (`seat`, from 1; -1 means
every seat is taken and they can only watch). Seats are numbered as [the stream](GAMING-STREAM.md)'s players, the host
being 0. Over version 2's WebRTC (`POST /play/{sid}/stream`) the server sets a joined device's hello to its own seat, or
to none for a watcher, whatever it asked for. Over version 2's WebSocket the hello goes straight to WittyPlay, so the
app sends `seats: [seat]` itself. Saving, loading, settings and stopping stay the host's. A seat ends when
the host frees it, its holder leaves (`DELETE /play/{sid}/seats/{user}`) or the game ends.

Playing with someone on another server is reserved and not built: clients show **Multiplayer** greyed out.

## Playing together

WittyPlay Online's data (Design 2, `docs/plans/DESIGN-2-SDUI-GAPS.md` section 6). Its native views and shell
(`gamesHome`, `quickMenu`, `lobby`, `controllers`, `memories`, the status bar and the system dock) are drawn by the
apps from these sources; nothing here is a page of its own.

**Friends** are the other profiles of this server. A profile makes two choices (`GET` and `PUT /games/online/me`;
yes to both until it says otherwise): whether friends see what it plays (`sharePlaying`; off, it's shown online with no
game) and whether it appears online at all (`appearOnline`; off, it's offline to everyone, playing or not). A **Kids**
profile sees, and is seen by, only the profiles an administrator chose for it (`GET` and
`PUT /games/online/kids/{user}`, `{friends}`; the `GET` also lists every profile that could be one, as `candidates`
with `approved`), and gets no open rooms. A **guest** has no friends and is
nobody's. Presence is `playing` (in a game on this server, as its host or in a seat), `online` (used this server in
the last five minutes), `away` (in the last hour) or `offline`; the game is named only to someone whose limits would
let them open it.

A **room** is a game being played on this server with seats for others ([Seats](#seats)): rooms are shared on this
server, not across servers, so a room's mode is "Shared". The host opens it to the friends who may see it
(`POST /play/{sid}/room`, `{open}`), who then join with no code (`POST /play/join` with `{room: sid}`); it answers the
host's code, which a phone's QR join link carries (`/games?join=CODE`). The host moves players
(`PUT /play/{sid}/seats/{user}`, `{seat}`; a held seat is swapped). Kids and guest profiles can't open their games or
join an open room; they still join with a code.

**Memories** are a profile's own save states and the screenshots it kept (`POST /play/{sid}/screenshot`, at most 200 a
game; `GET` and `DELETE /games/{id}/shots/{name}`). There are no clips and no voice chat.

### Its sources

WittyPlay declares them in its `sdui.json` and the server answers them, through the profile's limits, age ratings and
hidden libraries, and cleans them as any module's items. Nothing is given when games are off for the profile.

| Source | Gives | Each item |
|---|---|---|
| `wittyplay.recent` | The games this profile played, newest first | `game`: its era as a badge ("16-bit"), the home row's line as `subtitle` ("16-bit classics · 1 to 2 players · Played 3 hr 12 min · Save state 3 ready"), seconds played as `runtimeSec` |
| `wittyplay.collections` | Without `era`, the eras with games, in order; with `era` (`first`, `8-bit`, `16-bit`, `early-3d`, `disc`, `pocket`), that era's games by title | `collection` (`era-<era>`, `count`, route `/games?era=<era>`), or `game` |
| `wittyplay.friends` | The profile's friends, playing first, then online, away, offline | `person` (`profile-<id>`), a person card: name, presence as the badge ("Playing", "Online", "Away", "Offline") and the line ("Playing Star Tide"); icon `user` (the app draws the profile's avatar or initials); no route, so not a button |
| `wittyplay.rooms` | Friends' open rooms with a seat free, of games this profile may play; none for Kids or guests | `view` (the session): "Moth Racer · 3 of 4", "Shared · Sam", `count` players; its action and its one button (A, Join) are `wittyplay.room.join` |
| `wittyplay.room` | The room this profile is in: the room, then each player | The room (`view`: game, host, the code as `tagline` and the join route for the host only, badge "Open" when open; for the host, `actions`: A Play (rejoins the running game), X Invite, Y Open to friends or Close the room, the last not for Kids or guests), then a `person` per player: the host ("Host", badge "P1"), a player ("Player n", badge "Pn") or a free seat ("Waiting…", "Free", badge "Free"); no route |
| `wittyplay.saveStates` | One game's saves (`item`, required), the profile's own first, newest first | `view` `slot-<n>`: "Save state 3", when, its picture as `backdrop` |
| `wittyplay.memories` | Saves and kept screenshots of every game, newest first | `view` (badge "Save") or `photo` (badge "Shot"): the game's title, what and when, picture as `backdrop` |
| `wittyplay.controllers` | Each player of the game this profile is in | `view` `player-<n>`: "Player n", who (and the host's device), or "No controller" (badge "Free"). Battery and the controller's kind are the device's own (the Gamepad API): the server never sees them |

### Its commands

`rewind.open` and `controller.remap` open the player's own panels and need no server. The rest are the catalogue's
`wittyplay.` commands, each one of these routes:

| Command | Route |
|---|---|
| `wittyplay.room.create` | `POST /play/{sid}/room` `{open: true}` |
| `wittyplay.room.join` | `POST /play/join` `{room}`, or `{code}` |
| `wittyplay.room.invite` | `POST /play/{sid}/invite` |
| `wittyplay.state.save`, `wittyplay.state.load` | `POST /play/{sid}/action` `{type: "save" or "load", slot}` |
| `wittyplay.state.delete` | `DELETE /games/{id}/saves/{slot}` |
| `wittyplay.controller.assign` | `PUT /play/{sid}/seats/{user}` `{seat}` |

The action's `item` is the session (`{sid}`), or the game's ID for `wittyplay.state.delete`; the rest come in the
action's `params` (`slot`, `user`, `seat` and, for `wittyplay.room.join`, `code`). [SDUI](SDUI.md#action) says how a
client fills them.

## Modes

`GET /games/modes?client=native` answers `{client, modes}`, and the session's `modes` is the same list: what you can use, best first. Use the first one
you support, and move to the next if it doesn't show a picture within about 12 seconds. Ignore ids you don't
know, because more may appear. `"stream": 2` on a mode means it can be reached through version 2 of the stream,
[one way for every device](GAMING-STREAM.md), which is what the web player uses. A mode with `browserOnly` set is never listed for a native client, and leaving
out `client` counts as native. No such mode exists today.

| `id` | Video | Audio | Input | Needs |
|---|---|---|---|---|
| `webrtc` | `video`: `h264` (Constrained Baseline, `profile-level-id=42e01f`) and/or `vp8` | Opus | Data channel | A WebRTC stack: the Google library on Android, or WebRTC.framework on tvOS |
| `ws-h264` | H.264 Annex B | PCM | WebSocket | A decoder that takes raw frames, such as `MediaCodec` or `VTDecompressionSession` |
| `ws-jpeg` | One JPEG per frame | PCM | WebSocket | Only a JPEG decoder. Works when the server has no video encoder |

### WebRTC

Standard offer and answer with no trickle, over HTTPS or HTTP, using your sign-in:

1. Create a peer connection. Add **receive-only** video and audio transceivers.
2. Create the data channels: `input` (unordered, no retransmits, binary), `events` (reliable, text), and if you want it,
   `timing` (unordered, no retransmits, binary: see [Measuring and timing](#measuring-and-timing)).
3. Create an offer, set it, and wait for ICE gathering to finish (a second or two is enough).
4. `POST /play/{sid}/webrtc` with the SDP, then set the answer. The answer holds all of the server's
   candidates.

The server picks H.264 if your offer has it and VP8 otherwise. Send `"prefer": "vp8"` to ask for VP8. Offer the
`http://www.webrtc.org/experiments/rtp-hdrext/playout-delay` header extension: the server uses it to say a frame may not wait, and a
frame then goes to your decoder as soon as it's whole. If your stack doesn't honour it, ask for the smallest receive buffer it allows
(`jitterBufferTarget` or the same on your platform). Latency is the point. Sound and video are separate streams, so as not to
hold one back for the other.

A connection that fails or stalls should be made again with a new offer: the game waits, paused, for a minute. The old picture
can stay up meanwhile.

Out of your home network, WebRTC needs a route to the server's UDP port. Admins set that up in Settings, Consoles & cores.
If it fails, the WebSocket modes work anywhere HTTP does.

### WebSocket

Connect to `GET /play/{sid}/ws?video=h264` or `video=jpeg` with your sign-in in the `Authorization` header.
Browsers can't set that header, and use the sign-in cookie or `?token=`. The first message is text: `{"type":"hello","video":"h264"}`. It
says what you'll get, which may be `jpeg` if H.264 isn't available. After that:

Binary messages, whose first byte says what they are:

| First byte | Rest |
|---|---|
| 1 | H.264: one byte (1 for a keyframe, else 0), then one frame in Annex B with its parameter sets before each keyframe. The first frame is a keyframe |
| 2 | One JPEG |
| 3 | PCM: u32 little-endian sample rate, then signed 16-bit little-endian stereo. The rate may be anything a console uses, such as 32040 |

Text messages are events, as below. Send controller states as binary messages. If you fall behind, frames are
dropped, and you carry on from the next keyframe.

## Measuring and timing

All optional, and all binary, on the `timing` data channel, or the WebSocket beside everything else. The first byte says what it is.

| First byte | From | Rest |
|---|---|---|
| `0xF1` | You | A ping: an f64 of your own clock |
| `0xF2` | Server | The answer: that f64, then an i64 of the server's Unix nanoseconds. The fastest of these lines the clocks up |
| `0xF3` | You | One byte: 1 to have a record of every frame sent, 0 to stop |
| `0xF4` | Server | One frame's record: a byte, 1 for a keyframe; then u32 frame number, u32 newest input it saw, u32 RTP timestamp, i32 encoded size; then i64 Unix nanoseconds for when that input reached the server, when it reached the runner, when the frame began, when its picture was ready, when it reached the server, when it went to the encoder, when it came out, and when it had been sent. 0 is "not known" |
| `0xF5` | You | One byte: 0 fastest, 1 balanced, 2 steadiest. See [the design](GAMING.md#speed-or-steadiness) |
| `0xF6` | You | An i16 of tenths of a millisecond to move when frames are made, once. Later is positive. It's held to a frame's length, and a few a second, and only with one viewer |
| `0xF7` | You | You need a keyframe, and you don't have your stack's own way to say so |
| `0xF8` | You | You're decoding frames yourself, so your stack's own requests for keyframes mean nothing |

Your side of a frame is when you received it, when it was decoded and when it reached the display, and your RTP timestamp or frame
number ties it to its record. `web/latency.ts` in WittyPlay is the browser's version.

## Controller states

A 13-byte binary message, little-endian, on the `input` data channel or the WebSocket. The last two bytes may be left
off if you can't tell how far triggers are held, which makes it 11. Four more may follow a 13-byte state, making it 17: a u32 you choose that names this state, which comes back in
the `0xF4` record of the first frame that saw it, so that a press can be followed to the screen:

| Bytes | Field | Meaning |
|---|---|---|
| 0 | port | The player, 0 to 3. Others are ignored |
| 1-2 | buttons | u16, a bit per RetroPad button |
| 3-4, 5-6 | left stick | X, Y: i16, -32768 to 32767, with up negative |
| 7-8, 9-10 | right stick | X, Y |
| 11, 12 | L2, R2 | How far each trigger is held, 0 to 255. Also set their button bits, past your press point |

| Bit | Button | Bit | Button |
|---|---|---|---|
| 0 | B (bottom) | 8 | A (right) |
| 1 | Y (left) | 9 | X (top) |
| 2 | Select | 10 | L |
| 3 | Start | 11 | R |
| 4 | Up | 12 | L2 |
| 5 | Down | 13 | R2 |
| 6 | Left | 14 | L3 |
| 7 | Right | 15 | R3 |

Send the whole state when anything changes, and again every 250 ms or so. A lost packet then corrects itself,
which is why the channel doesn't retransmit. Repeats don't count as playing. Release everything when your
app goes to the background or a menu opens.

## Touch screens

A console whose `pad` has `"pointer": true` has a touch screen. Send its state as a 6-byte binary message, on the same
channel as controller states:

| Bytes | Field | Meaning |
|---|---|---|
| 0 | mark | Always 255, which a controller state never starts with |
| 1 | flags | Bit 0: pressed |
| 2-3 | X | i16, -32767 at the picture's left edge to 32767 at its right |
| 4-5 | Y | i16, -32767 at the top to 32767 at the bottom |

Positions are across the whole picture, whatever the screens' layout. Send it at once when anything changes, and
every 250 ms or so while pressed. Send a release a few times, because a lost one would hold the stylus down.

## Events

JSON text on the `events` data channel or the WebSocket. Ignore types you don't know.

| `type` | Fields | What to do |
|---|---|---|
| `rumble` | `port`, `effect`, `strength` (0 to 65535) | Rumble that player's controller, if it can |
| `message` | `text` | Show it for a few seconds |
| `geometry` | `width`, `height`, `aspect` | Change the picture's shape |
| `rotation` | `turns` | Rotate the picture by this many quarter turns |
| `avinfo` | `width`, `height`, `aspect`, `fps`, `rate` | The game's timing changed |
| `shutdown` | | The game ended itself: leave the player |
| `ended` | `text` | The game stopped: it crashed, stopped responding, or was stopped from elsewhere. `text` says which, for showing to a person |
| `disc` | `index`, `count` | The disc in the drive changed |
| `frame` | `width`, `height` | What the core really draws changed. See [While a game is running](#while-a-game-is-running) |

## The controller as data

`GET /consoles/{console}/controls?device=<id>` answers:

```json
{
  "console": "snes",
  "pad": { "face": [{ "id": 1, "label": "Y" }], "shoulder": [], "meta": [], "sticks": false },
  "controls": [{ "key": "dpad", "id": -1, "label": "D-pad", "kind": "dpad" }, { "key": "b8", "id": 8, "label": "A", "kind": "face" }],
  "defaults": {
    "gamepad": { "8": { "b": 1 } },
    "keyboard": { "8": "KeyX" },
    "remote": { "8": "Enter" },
    "touch": { "landscape": { "dpad": { "x": 0.13, "y": 0.62, "size": 0.36, "opacity": 0.55 } }, "portrait": {} }
  },
  "saved": { "gamepad": { "Some Pad Model": { "8": { "b": 5 } } }, "keyboard": null, "touch": {}, "layouts": {} }
}
```

Numbers used as keys are RetroPad buttons, the bit numbers above. Labels are what that console calls them, so
draw `Y` on a Super Nintendo and `□` on a PlayStation from the same data.

- **Gamepads** map each button to `{"b": n}` (a button) or `{"a": n, "d": 1 or -1}` (one direction of an axis).
  The numbers are those of the [W3C standard gamepad](https://www.w3.org/TR/gamepad/#remapping), which is laid out by
  position, so translate from your platform's names. Bottom face button is 0, right 1, left 2, top 3, L1 4, R1 5,
  L2 6, R2 7, View/Select 8, Menu/Start 9, stick clicks 10 and 11, D-pad up, down, left, right 12 to 15. With no
  saved layout, the left stick steers the D-pad as well. Saved layouts are filed under the pad's model name.
- **Keyboards** map to `KeyboardEvent.code` names, such as `KeyX`.
- **Remotes** map to `KeyboardEvent.key` names. Translate from your platform:

  | Name | Means | tvOS | Android TV |
  |---|---|---|---|
  | `ArrowUp`, `ArrowDown`, `ArrowLeft`, `ArrowRight` | D-pad | Swipes or clickpad | `KEYCODE_DPAD_*` |
  | `Enter` | A | Select | `KEYCODE_DPAD_CENTER` |
  | `MediaPlayPause` | B | Play/Pause | `KEYCODE_MEDIA_PLAY_PAUSE` |
  | `MediaRewind` | Select | Not on all remotes | `KEYCODE_MEDIA_REWIND` |
  | `MediaFastForward` | Start | Not on all remotes | `KEYCODE_MEDIA_FAST_FORWARD` |

  **Back opens your game menu; it isn't a game button.** A remote can't reach every button, so the menu should
  offer "Press Start" and "Press Select", which send those bits for about 200 ms. Offer a gamepad for games that
  need more.
- **Touch** layouts place each control by its `key`: `x` and `y` are its centre, as fractions of the play area,
  and `size` is a fraction of the shorter side. A D-pad is a square of that size; `meta` buttons are about half as
  tall as wide. A control of kind `stick`, or a D-pad with `"stick": true`, is an analog stick: send how far a finger
  pushes it as the left stick, measured from its centre to its edge.
- **Layouts** (`saved.layouts`) are what Edit layout keeps: where everything sits while playing, for each shape of device
  (`phone-portrait`, `phone-landscape`, `tablet-portrait`, `tablet-landscape`, `desktop`, `tv`). They follow the account.
  - `controls` places controls by `key` as a touch layout does.
  - `menu` and `touchbar` are centres, `{x, y}`.
  - `screens` is the picture, `[{x, y, size}]`, with `size` its height as a fraction of the play area's. A two-screen
    console can have two, one per screen in the core's order, drawn from where `pad.screens` (or
    `capabilities.screens.frame`) says each sits in the frame. Left out is the player's own fit.
  - `faceTogether` and `screensTogether` are how the editor was left.
  - With no layout for a shape, use `defaults.touch` for its orientation (desktop and TV are sideways), and an older
    touch layout saved for this device, if there is one.
- `pad.screens`, on a two-screen console, is each screen's part of the frame, `[{x, y, w, h, touch}]`. Over the screen
  with `touch`, a press is the stylus except exactly on a control drawn over it.

More in the same answer: `targets` lists everything this console can have mapped, in the order to show it, and `pad.art` says
what shape of controller to draw (`one`, `two`, `four`, `six`, `handle` or `dual`). `defaults.options` is how sticks and turbo
behave until someone changes it. `profiles` is this account's profiles in use here; add `&game=<id>` to include those for that
game. `background` is below.

Numbers from 16 to 23 used as keys are one direction of a stick, so a mapping treats them like buttons, and a stick can press
a button or a button can push a stick: 16 left stick left, 17 right, 18 up, 19 down, 20 right stick left, 21 right, 22 up,
23 down. On a console whose stick is how games steer (`pad.stickFirst`: the Nintendo 64), the arrow keys and on-screen D-pad
push the stick. Its C buttons are the right stick, as the emulators have them.

**Saving:** `PUT /consoles/{console}/controls` with one of these. `"data": null` goes back to the default. This is the simple
way, with one layout per model per console. Profiles, below, do more, and win when both exist.

```json
{ "kind": "gamepad", "key": "Some Pad Model", "data": { "8": { "b": 5 } } }
{ "kind": "keyboard", "data": { "8": "KeyK" } }
{ "kind": "touch", "key": "my-device-id", "orientation": "landscape", "data": { "dpad": { "x": 0.2, "y": 0.6, "size": 0.3, "opacity": 0.6 } } }
{ "kind": "layout", "key": "phone-landscape", "data": { "controls": { "dpad": { "x": 0.13, "y": 0.66, "size": 0.32, "opacity": 0.55 } }, "menu": { "x": 0.96, "y": 0.06 }, "screens": [{ "x": 0.5, "y": 0.5, "size": 1 }] } }
```

Bad data answers 400 and saves nothing. Layouts follow the account to every device, except touch layouts, which
belong to the `device` that saved them. A `layout` is checked by the server, whatever version of WittyPlay is installed.
`"data": null` for a layout is Reset to default. A device id is yours to choose: up to 64 letters, digits, `.`, `_` or `-`.

## Controller profiles

A profile is a named mapping that belongs to one account, and follows it to every device.

```json
{
  "id": 12, "name": "Couch", "kind": "gamepad", "active": true,
  "controller": "054c:0ce6", "controllerName": "DualSense", "console": "snes", "game": 0,
  "map": { "0": { "b": 5 }, "8": { "a": 2, "d": 1 }, "17": { "b": 15 }, "2": null },
  "options": { "deadzone": 0.2, "threshold": 0.5, "stickDpad": true, "turbo": [0], "turboHz": 12, "noRumble": false }
}
```

- **Scope:** `controller` is `vendor:product` in lowercase hexadecimal, `id:<name>` for a pad with no numbers, or empty for any
  controller. `console` and `game` are empty or 0 for any. A game needs its console. Keyboard profiles have `"kind": "keyboard"`,
  `keys` in place of `map`, and `"controller": "keyboard"`.
- **Which applies:** the most exact one in use wins: this game and this controller, this game, this console and this controller,
  this console, this controller, then one for anything. With none, the console's defaults. `GET /game-profiles/resolve` works this out,
  and answers what to use. Of profiles with the same scope, one is `active`; making one active takes the others out of use.
- **`map`** says what changes: a target left out keeps its default, and `null` means nothing presses it. A value is `{"b": n}` (a
  button), `{"a": n, "d": 1 or -1}` (a direction of an axis, with `"r": -1 or 1` for a trigger that rests at that end), or
  `{"h": n, "d": 1 to 4}` (a D-pad reported as one axis: up, right, down, left). Numbers are W3C standard ones. Translate
  from your platform as for `saved.gamepad` above, and never set `raw`, which is for browsers that can't tell a pad's order.
- **`options`:** `deadzone` up to 0.6 is how far a stick moves before it counts, measured as a circle. `threshold`, 0.1 to 0.9,
  is how far an analog input goes to press a button. `stickDpad` lets the left stick steer the D-pad as well. `turbo` lists buttons
  that repeat while held, `turboHz` times a second, from 2 to 30. `noRumble` turns rumble off. `calibration` is per axis,
  `[lowest, at rest, highest]`. Applying these is your job: the server only keeps them.
- **Limits:** 200 profiles an account, 16 KB each, names of up to 60 characters. Bad data answers 400 and saves nothing.
- **Files:** `GET /game-profiles/export` gives `{"format": "wittyplay-controller-profiles", "version": 1, "profiles": [...]}`, as an
  attachment. `POST /game-profiles/import` takes that file as its body, up to 256 KB and 100 profiles, and answers
  `{imported, rejected: [{index, name, error}], notes}`. Good profiles are kept. A profile for a game this server doesn't have
  becomes one for its console. An imported profile is only put in use if nothing with its scope already is.

Copying (`POST /game-profiles/{id}/copy`) gives a separate profile that isn't in use. Nobody can see, change or address
another account's profiles: to them, the routes answer 404.

## The picture behind the controller screen

`GET /game-background` answers `{"version": 0, "fit": "cover", "x": 50, "y": 50, "dim": 50}`. `version` is 0 with no picture. Otherwise,
show `GET /game-background/image?v=<version>` behind your controller screen. `fit` is `cover` or `contain`, `x` and `y` are
which part of the picture shows (0 to 100), and `dim` is how far to fade it towards your background colour (0 to 90), with a
solid backing under any text on top.

`PUT /game-background` changes those. `PUT /game-background/image`, with the picture as the body, sets it: PNG, JPEG or WebP, up to 10 MB
and 24 megapixels. The server decodes it and stores a new JPEG of up to 2560 pixels, so nothing but the picture survives, and
answers the setting. A file that isn't one of those answers 400. `DELETE /game-background` removes it. All of these are for
the signed-in account only, as they take no account number.

## Settings

Everything in the game menu is on the server, so an app doesn't have to invent any of it.

| Route | What it does |
|---|---|
| `GET /games/settings/schema` | Every setting: `key`, `group`, `section`, `label`, `help`, `caution`, `kind` (`toggle`, `choice`, `slider`, `color`, `bezel`), `default`, `min`, `max`, `step`, `unit`, `choices`, `shader`, `motion`. Also `presets` to offer, and the save slot numbers |
| `GET /games/settings?item=<id>` or `?console=<id>` | `resolved` (`values`, `sources`, `coreOptions`, `coreSources`, `controls`, `presets`), `layers` by level, and `limits` (`maxSpeed`, `rewindBytes`, `runAhead`) |
| `PUT /games/settings` | Change one level, and get the same view back |
| `POST /games/settings/validate` | Check a settings file: `{"document": "...", "item": 12}` gives `{"ok", "report": {"errors", "ignored"}, "layer"}`. Each issue has `line`, `column`, `path` and `message` |

A `PUT` has a `level` (`person`, `console`, `game`, or for an administrator `server`, `serverConsole`), the `item` or
`console` it's about, and one of:

```json
{"level": "console", "item": 12, "settings": {"display.scanlines": 0.4, "display.curve": null}, "coreOptions": {"fceumm_sndquality": "High"}}
{"level": "game", "item": 12, "document": "{ \"wittyplay\": 1, ... }"}
{"level": "game", "item": 12, "reset": true}
```

`null` removes a value, so the level below shows through. What can't be used comes back in `ignored`, and the rest is
applied. A `document` replaces the whole level, or changes nothing and answers 422 with its `report`. A running game of
this person's hears about changes straight away.

What an app has to apply itself is the `display.*`, `osd.*` and `audio.*` groups, because they happen on the device. The
`emulation.*` group and core options take effect on the server. `states.autoSave` does too; `states.resume` is for the
app to act on when opening a game.

## While a game is running

`POST /play/{id}/action` takes these as well, as `{"type", "slot", "on", "value", "key", "text"}`:

| `type` | With | What it does |
|---|---|---|
| `speed` | `value` | 1 is normal, down to 0.1 and up to the server's limit. Answers with the `speed` it was held to. Sound stops while it isn't 1 |
| `rewind` | `on` | Go backwards while on. Refused with a message when rewind isn't turned on |
| `advance` | `value` | Run this many frames while paused |
| `undo` | | Go back to before the last load. Pressing it again goes forward |
| `option` | `key`, `text` | Set one core option now, without saving it |

Save and load take slots 1 to 99, and load also takes 0, 100 and 101, as under [Routes](#routes). `GET /games/{id}/saves` lists what exists, with the `core` that made each.

Events also bring `frame`, with the `width` and `height` of what the core really draws, whenever that changes. `GET
/play/{id}` says the same once a picture exists. What a client draws on top needs this, because the video may be
scaled on the way and the size in the first answer is only what the core expected.

`GET /play/{id}/stats` gives `gameFps`, `sentFps`, `skipped`, `encodedFps`, `encodeMs`, `kbps`, `droppedFrames`, `width`,
`height`, `core`, `encoder`, `speed`, `paused`, `rewindBytes` and `rewindFrames`. Ask at most once a second, and only
while showing them. `GET /play/{id}/screenshot` gives a PNG with no effects on it.

`GET /games/{id}/core-options` lists each option with `key`, `label`, `info`, `category`, `choices`, `labels` (the name of each choice),
`default` and `hidden` (the core says it doesn't apply just now, so leave it out of a menu).

`POST /games/{id}/play` also takes `"safeMode": true`: defaults only, no cheats, no patches. Its answer has `safeMode`,
`maxSpeed`, and `notices` to show, such as a patch that wasn't applied.

## A person's own files

All of these are private to the person. Files go as the request body. A loader that can't add a header, such as an image
tag or a download link, can put the sign-in in the address as `?token=` on these reading routes only: a save's `thumb` and
`file`, `battery`, one image from `game-bezels`, `game-background/image`, and `play/{id}/screenshot`.

A Guest profile gets 403 from every route that uploads here. A Kids profile can put its own saves and pictures, but gets 403 for a patch.

| Route | Limits |
|---|---|
| `GET`, `PUT /games/{id}/saves/{slot}/file` | 256 MB. Slots 1 to 99 to put. A file with our header is checked, a plain one gets a `warning` |
| `GET`, `PUT /games/{id}/battery` | 64 MB. Refused with 409 while that person is playing it |
| `GET`, `POST /games/{id}/patches?name=`, `PUT`, `DELETE /games/{id}/patches/{patch}` | IPS or BPS, 64 MB, 16 to a game. They take effect when the game next starts. Adult profiles only |
| `GET`, `PUT /games/{id}/cheats`, `POST /games/{id}/cheats/import` | 500 codes. Import reads a file and says what's in it, and only `PUT` keeps anything |
| `GET`, `POST /game-bezels?name=`, `GET`, `DELETE /game-bezels/{bezel}` | PNG, JPEG, WebP, 16 MB, 16 MP, 16 images. Decoded and made again |
| `GET`, `PUT /games/{id}/meta`, `GET /games/recent` | `favourite`, `note` (8 KB). `recent` takes `limit` and `favourites=1`, and gives `item`, `plays`, `lastPlayed`, `favourite` |

## What a TV app must build itself

- The player screen, showing the stream and playing the sound.
- Reading controllers and the remote, and turning them into the states above using the data above.
- The game menu: Resume, Press Start, Press Select, Save, Load, Reset, Quit. Focus handling is the platform's.
  Remember that Back goes up, and that the picture owns the D-pad until the menu is open.
- Choosing a mode from `modes`, and falling back.
- Stopping cleanly with `DELETE /play/{sid}`, including when the app leaves the screen for good.

**Moved to the server for this**, so apps don't have to do it: what every console's controller has, its names
for its buttons, default layouts, saved layouts, controller profiles and which applies, sessions, saves, resume and play time.
Nothing about a game lives only in the web player, except this device's own choices: which connection mode to force,
whether to show touch controls, and which controller is on which player.

**With the rest of the API:** these routes are under `/api/v1` as well. `GET /capabilities` lists `games` in `libraryTypes`, and `game-metadata`, `game-settings`, `game-controller-profiles`, `custom-game-cores` and `game-rigs` in `features`. `POST /items/{id}/playback` is for videos and answers 400 for a game: start one with `POST /games/{id}/play`.

---

<sub>WittyMediaServer documentation</sub>
