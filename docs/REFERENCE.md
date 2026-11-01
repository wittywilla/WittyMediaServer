# Command line reference

Look up every `wittymediaserver` command, and how to script the server through its API. Everyday settings are in the web app: this is what sits underneath.

- [Commands](#commands)
- [Flags](#flags)
- [Tools that come with modules](#tools-that-come-with-modules)
- [HTTP API](#http-api)
- [For developers](#for-developers)

## Commands

The command is `wittymediaserver`. With a package it's on your `PATH`. Otherwise it's in the program's `bin` folder, and on a Mac in `Contents/Helpers` inside the app. Each command works on the data folder the server would use: add `-data <dir>` to name another, as [Folders the server uses](CONFIG.md#folders-the-server-uses) explains. Most work with the server stopped.

**Running and version:**

```text
wittymediaserver [serve] [flags]        start the server (the default)
wittymediaserver version                the version
wittymediaserver help                   this list, in short
```

**Config:**

```text
wittymediaserver config path            the config file and folders in use
wittymediaserver config init            write a starter wms.json in your own config folder
wittymediaserver config init --portable write it next to the program, for portable mode
```

**Sign-in:**

```text
wittymediaserver reset-2fa [-data <dir>] <name>
                                        turn off an account's two-factor sign-in, when the phone
                                        and the recovery codes are both gone
```

**Repair**, the same checks as **Settings → Repair**. See [Repair](REPAIR.md).

```text
wittymediaserver doctor [-data <dir>]   look, and say what's wrong and what a fix would do
  --fix                                 fix what can be fixed. A safety copy is made first
  --safe-only                           with --fix, only fixes that can't lose anything
  --yes                                 with --fix, don't ask before the ones that change data
  --only <id,id or area>                only these checks. Areas: data, metadata, modules, install
  --json                                a report for a program
  --try-port                            also try the port by listening on it for a moment
wittymediaserver doctor list            every check
wittymediaserver doctor remap --from <old> --to <new> [--fix]
                                        a library's drive or folder moved: rewrite the paths
wittymediaserver doctor reinstall <module>
                                        download a module again from its publisher
wittymediaserver doctor help            more about these
```

`doctor` returns 0 when nothing needs attention, 1 when something does, and 2 when it couldn't run.

**Backup**, the same as **Settings → Backup**. See [Backup](BACKUP.md).

```text
wittymediaserver backup create [-data <dir>] [-o out.zip] [--parts a,b,c] [password option]
                                        a ZIP of this server's data and metadata
wittymediaserver backup parts           what can be included, and how large each part is
wittymediaserver backup list [-data <dir>]
wittymediaserver backup inspect <file> [-data <dir>] [password option]
                                        check a backup from end to end, and say what's in it
wittymediaserver backup restore <file> [-data <dir>] [--parts a,b,c] [--yes] [password option]
                                        bring a backup back
wittymediaserver backup undo [<safety copy>] [-data <dir>]
                                        put back what the last restore replaced
wittymediaserver backup cancel [-data <dir>]
                                        take back a restore waiting for the next start
wittymediaserver backup help            more about these
```

The password options are `--password-file <file>`, `--password-stdin` and `--ask-password`. `WMS_BACKUP_PASSWORD` also works. A password is never taken on the command line, where other programs could see it.

**Fixing titles by hand.** See [Fixing metadata by hand](FIXING-METADATA.md).

```text
wittymediaserver metadata export [-data <dir>] [-library <id>]
                                        write a plain text file for each title into <data>/metadata
wittymediaserver metadata check [-data <dir>]
                                        say what's wrong with any file there, by file and line
wittymediaserver metadata apply [-data <dir>] [--dry-run | --apply]
                                        say what the files would change, and with --apply change it
wittymediaserver metadata undo [-data <dir>] <batch>
                                        take back everything one apply changed
```

**Logs,** with no server needed. See [Logs](LOGGING.md).

```text
wittymediaserver logs zip [-data <dir>] [-o out.zip] [-days N] [--raw]
                                        the logs as a ZIP, sanitized for sharing unless --raw
wittymediaserver logs sanitize [-data <dir>] [-o out.txt] <file>
                                        a sanitized copy of any text log
```

**Starting again:**

```text
wittymediaserver reset [-data <dir>] [-delete] [-yes]
                                        back to a new install, the next time the server starts.
                                        It keeps a copy unless -delete
wittymediaserver reset -list            the copies kept
wittymediaserver reset -restore <copy>  bring one back
wittymediaserver reset -cancel          take back a reset that's waiting
```

**Removing it.** See [Uninstalling](UNINSTALL.md).

```text
wittymediaserver --uninstall [--keep-data | --remove-data] [--remove-recordings] [--remove-backups]
                             [--yes] [--dry-run] [--json] [--report <file>]
```

The installers run three more themselves, and there's no reason to use them by hand: `--windows-service`, `--secure-service-data` and `--take-over-old-copy`.

## Flags

Flags go after `wittymediaserver serve`. Each of them, with its variable and its name in `wms.json`, is in [What you can set](CONFIG.md#what-you-can-set): `-config`, `-data`, `-listen`, `-ffmpeg`, `-ffprobe`, `-java`, `-log`, `-log-dir`, `-max-transcodes`, `-max-connections`, `-public-url`, `-remote-access`, `-trusted-proxies`, `-setup-from-home`, `-allow-network-paths`, `-gaming-custom-cores`, `-open-browser`, `-watch-stdin` and `-dev`.

Everything else, such as API keys, metadata agents, transcoding, disc behaviour, themes and profiles, is set in the web app and kept in `wms.db` in the data folder. [Back up](BACKUP.md) to keep your libraries, people and watch history.

## Tools that come with modules

The tools for looking inside discs and images come with their modules, and aren't part of `wittymediaserver`. They never print video or artwork, so their output is safe to attach to a bug report.

```text
wittydvd info    <image>                titles, menus, languages, first-play program
wittydvd walk    <image> [button...]    simulate inserting the disc and pressing buttons
wittydvd crawl   <image>                press every button of every menu, and report dead ends
wittydvd pgc     <image> <domain> <vts> <pgcn>
                                        the commands of one program chain
wittydvd render  <image> <out-dir>      save the first menu's highlight images
wittydvd disc ls <image>                list the files inside a disc image
wittybluray info     <disc>             titles, the disc's program, playlists, clips and streams
wittybluray playlist <disc> <number>    every item, stream and chapter of one playlist
wittybluray menu     <disc> <playlist>  a playlist's menu: pages, buttons and every command
wittybluray walk     <disc> [button...] simulate inserting the disc, then pressing buttons by id
wittybluray render   <disc> <out-dir>   save the first menu's pages as PNG files
wittycd info  [-json] <image>           tracks, indexes, CD-TEXT, flags and disc ID
wittycd check [-json] <image>           read the whole image and report what is wrong with it
```

Run one with no arguments for its full list. To go through what a disc does in the app, use **Settings → WittyDiagnoser**: see [WittyDiagnoser](DIAGNOSER.md).

## HTTP API

**Writing an app or a tool?** The [client API](CLIENT-API.md) is the guide, with every route. The newer libraries' routes are also in their own guides: [photos](PHOTOS.md#for-developers), [vertical videos](SHORTS.md#for-developers), and [collections, radio and Your year](COLLECTIONS.md).

Everything is under `/api` and answers JSON. Sign in with `POST /api/login` and send the token as `Authorization: Bearer <token>`, or `X-WMS-Token`. An account with two-factor sign-in answers `POST /login` with 401 and `{"twoFactor": true}` until the request also has `code`.

Media addresses that a page can't add headers to, such as images and video, take `?token=`. An HLS playlist fetched that way carries it on to its segments. Those addresses can end up in the logs of a reverse proxy, so players that can add a header should.

**Security rules for API clients:**

- Apps on other origins, such as the mobile app or your own tools, send the token in a header. The server answers them with CORS, but never honours the session cookie across origins, and refuses state-changing requests signed in by cookie that come from another site.
- A page elsewhere that sends no token gets no answer it can read. Only the mobile app's origins may sign in.
- Connections from outside the home network are refused until [remote access](REMOTE.md) is on.
- Changing your own password needs the current one (`{"password", "current"}`), and signs out your other devices.

| Area | Endpoints |
|---|---|
| Sign-in | `GET /status`, `POST /setup`, `POST /login`, `POST /logout`, `GET /me`, `GET /profiles` |
| Devices | `GET /sessions` (`?all=1`: everyone's, admin), `DELETE /sessions/{id}`, `DELETE /sessions` (all your others) |
| Two-factor sign-in | `GET /me/2fa`, `POST /me/2fa/setup`, `POST /me/2fa/confirm`, `POST /me/2fa/disable`, `DELETE /users/{id}/2fa` (admin) |
| Remote access (admin) | `GET/PUT /remote`, `POST /remote/test`, `POST /remote/selfcheck`, `POST /remote/router`, `POST /remote/obtain`, `DELETE /remote/lockouts`. Public: `GET /reach/{nonce}`, the outside test |
| Home | `GET /home`, `GET/PUT /me/ui` (home layout) |
| Libraries | `GET/POST /libraries`, `PUT/DELETE /libraries/{id}`, `POST/GET /libraries/{id}/scan` |
| Items | `GET /items?library=&search=&sort=&desc=&genre=&kind=&limit=&offset=`, `GET/PUT /items/{id}`, `GET /items/{id}/poster`, `.../backdrop`, `.../recommend`, `.../artwork` |
| Watch state | `POST/DELETE /items/{id}/progress` |
| Video | `GET /items/{id}/play?audio=&subtitle=&maxHeight=&transcode=1`, `GET /items/{id}/file`, `GET /hls/{id}/playlist.m3u8`, `.../seg/{n}.ts`, `DELETE /hls/{id}` |
| Discs (DVD and Blu-ray) | `POST /items/{id}/dvd` `{mode, ttn, chapter}` gives `{sessionId, presentation}`. `GET/DELETE /dvd/{sid}`, `POST /dvd/{sid}/action`, `GET /dvd/{sid}/subs`, `.../sub/{name}`, `.../menu/{serial}/overlay.png`, `.../highlight/{n}`, `.../still/{name}`. Blu-ray sessions start with `bd` |
| TV | `GET /shows`, `GET/PUT /shows/{id}`, `PUT /shows/{id}/view`, `GET /shows/networks`, `GET /shows/genres`, `GET /shows/{id}/art/{kind}` |
| Music and podcasts | `GET /music/albums`, `.../artists`, `.../tracks`, `GET /items/{id}/stream`, `GET/POST /podcasts`, `PUT/DELETE /podcasts/{id}`, `POST /podcasts/{id}/refresh`, `POST /items/{id}/download` |
| Books | `GET /items/{id}/book`, `.../page/{n}` (comics), `.../epub/{file}`, `.../search?q=`, `GET/POST /items/{id}/annotations`, `PUT/DELETE /annotations/{aid}`, `GET /dictionary?word=` |
| Metadata agents | `GET/PUT /agents`, `POST /agents/test`, `POST /metadata/test` |
| Administration | `GET/PUT /settings`, `GET /system`, `GET/POST /users`, `DELETE /users/{id}`, `PUT /users/{id}/access`, `PUT /users/{id}/password`, `GET /fs?path=`, `GET/PUT /logs`, `GET /logs/entries`, `GET /logs/bundle` |
| Themes | `GET /themes`, `GET /themes/{id}/{file}`, `GET /themes/{id}/download`, `POST /themes`, `DELETE /themes/{id}` |

A disc *presentation* tells a client exactly what to show:

- `kind`: `menu`, `title` or `stop`.
- `media`: an HLS address or a still picture, a loop flag, and a still time.
- `menu`: the frame size, button rectangles with their up, down, left and right links, the selected button, overlay and highlight pictures, and for Blu-ray pop-up menus `popup` and `closed`.
- For titles: duration, chapters and track lists.

A route of a module that isn't installed answers `424 Failed Dependency` with `module` and `moduleName`.

## For developers

### Architecture

The server is one program with no module's code in it. Everything a module does arrives at run time as a signed archive, and runs as a program of its own in a sandbox, or in the browser. A server with no modules starts and works, and a page whose module is missing says so and offers to add it. See [Modules](MODULES.md).

```text
Web app, WittyMediaClient, other apps
        |
        |  /api
        v
internal/server        routes, streaming, security, the built-in web app
   |-- internal/library     scanning, metadata, watch state  --> SQLite (wms.db)
   |-- internal/hls         on-demand HLS                    --> FFmpeg
   |-- internal/channels, internal/livetv    channels, the guide, recording
   |-- internal/dvdsession, internal/bdsession    one viewer's disc
   `-- internal/modules     installs, checks and runs modules
              |-- helpers and services:  WittyDVD, WittyBluray, WittyCD, WittyTuner,
              |                          WittyPhotos, WittyPlay, WittyCouch, ...
              |-- programs:              WittyCouchHost
              `-- web bundles:           WittyEbook, WittyPDF, WittyComics, WittyInk,
                                         WittyVisualizer, WittyStats
```

| Folder | What lives there |
|---|---|
| `cmd/wittymediaserver` | The server and its command line |
| `cmd/wittymediaserver-tray` | The app people open: the tray or menu-bar icon |
| `cmd/wittymediaserver-uninstall` | The uninstaller with windows and buttons, on Windows and macOS |
| `wms-forwarder` in `cmd` | Ships as `wms` in the `.zip` and `.tar.gz` only, so a copy of 0.5.2-alpha can update |
| `internal/server` | The HTTP API, streaming, security middleware and the built-in web app |
| `internal/modules` | The store, catalogue, signatures, sandboxes, and starting helpers |
| `internal/play`, `internal/couch` | Stubs written by a generator from each module's source: its types, and functions that call its service |
| `internal/dvd`, `internal/bluray`, `internal/cd`, `internal/tuner` | The types those modules answer in, kept as a copy |
| `internal/isofs`, `internal/tuner/mpegts` | The server's own disc image reader and encryption guard, so it refuses encrypted media itself, whatever a module says |
| `internal/library` | Libraries, scanning, shows, music, books, metadata, watch state |
| `internal/channels`, `internal/livetv` | WittyTV: channels, the guide, tuners and recording |
| `internal/dvdsession`, `internal/bdsession` | One viewer's DVD or Blu-ray: presentations, menus, subtitles, streams |
| `internal/hls`, `internal/ffmpeg`, `internal/playback`, `internal/subtitle` | On-demand HLS, finding and probing FFmpeg, how a file plays on each kind of client, subtitles |
| `internal/libtranscode`, `internal/tasks`, `internal/metrics` | Converting libraries ahead of time, background tasks, server statistics |
| `internal/gameid`, `internal/gamemeta`, `internal/gamearchive`, `internal/gameregion` | What a game file is, its details, its archive, and which version comes first |
| `internal/auth`, `internal/identity`, `internal/remote`, `internal/portmap`, `internal/discovery` | Accounts, this server's identity, remote access, router ports, and being found on the home network |
| `internal/backup`, `internal/reset`, `internal/metafolder`, `internal/diagnose` | Backups, resets, the metadata folder, repair and the diagnoser |
| `internal/update`, `internal/layout`, `internal/uninstall` | Updating, what a package holds and what each program is called, and removing everything |
| `internal/db`, `internal/config`, `internal/logging`, `internal/themes`, `internal/legal`, `internal/licenses` | Storage, configuration, logs, themes, legal wording, licences |
| `packaging` | The installers: MSI, `.deb`, `.rpm`, `.pkg`, and `install.sh` |
| `web` | The web app. What a module shows in the browser arrives as a web bundle at run time |
| `deploy`, `scripts` | Kubernetes, and developer tools. `scripts/README.md` lists them |

Each module and app, including **WittyDisc**, **WittyDVD**, **WittyBluray**, **WittyReader**, **WittyMediaPlayer** and **WittyMediaClient**, is a repository of its own, and this one holds none of their source. `docs/REPOSITORIES.md` lists them.

**How a disc plays, end to end:**

1. The scanner opens the image with its module, and records what the disc holds.
2. **Insert** starts a session. The module runs the disc's first-play program and decides what to show.
3. For a menu, the module decodes the buttons and their pictures, and the server returns rectangles and overlay images.
4. The stretch of video is handed to FFmpeg as exact byte ranges and streamed as HLS.
5. Key presses go back to the module, which runs the button's commands.

The cycle repeats until the film plays, with chapters, audio, subtitles and resume working as on a player. Before any of it, the server's own reader looks for encryption: an encrypted disc is refused with a clear message.

More: [Modules](MODULES.md), [DVDs](DVD.md), [Blu-rays](BLURAY.md).

### Building from source

You need **Go 1.26 or newer**, which fetches the toolchain `go.mod` names, and **Node 22 or newer**. FFmpeg is only needed to run it.

```bash
git clone https://github.com/wittywilla/WittyMediaServer && cd WittyMediaServer
(cd web && npm install && npm run build)                 # the web app, built into the program
go build -o wittymediaserver ./cmd/wittymediaserver      # the server
go build -o WittyMediaServer ./cmd/wittymediaserver-tray # the tray app (Windows: -ldflags -H=windowsgui)
./wittymediaserver serve -ffmpeg /path/to/ffmpeg -data ./devdata -log debug
go test ./...
```

- A build needs only one thing from the module repositories: WittyModule, at the version `go.mod` names. Until that's published, point at a checkout once in each checkout of the server: `node scripts/dev-modules.mjs WittyModule=<folder>`.
- Tests of what the server does with a real module need a catalogue: `WMS_TEST_CATALOG=<folder> go test ./...`, from `scripts/make-test-catalog.sh`. They skip without one.

Release archives for every platform:

```bash
scripts/build-release.sh 0.6.0-alpha                 # macOS, Linux, WSL
scripts\build-release.ps1 -Version 0.6.0-alpha       # Windows
```

These are unsigned, as a test build should be. `docs/PUBLISHING.md` covers real releases.

- The macOS tray app needs CGO (Cocoa), so it's built on a Mac. Everything else cross-compiles with `CGO_ENABLED=0`.
- FreeBSD has no public static FFmpeg. `scripts/build-ffmpeg-static.sh` builds one in CI, for amd64.
- `scripts/genicon` draws the app icon from code.

See `CONTRIBUTING.md` in the source before sending changes.

---

<sub>WittyMediaServer documentation</sub>
