# Config file, portable mode, folders and logs

Most settings live in the web app (*Settings*). A few things have to be known **before** the
server starts — which port to use, where to keep its data — and those go in a small config
file called `wms.json`. You never *need* one; without it the server uses sensible defaults.

- [Where the config file goes](#where-the-config-file-goes)
- [What you can set](#what-you-can-set)
- [Portable mode](#portable-mode)
- [Folders the server uses](#folders-the-server-uses)
- [Logs](#logs)

## Where the config file goes

The server looks for `wms.json` in this order and uses the first one it finds:

| # | Where | When to use it |
|---|---|---|
| 1 | the file given with `-config <file>` or the `WMS_CONFIG` environment variable | services, scripts, several servers on one machine |
| 2 | **next to the program** (`wms.exe` / `wms`) | portable mode — see below |
| 3 | your user config folder: | a normal install |
| | Windows: `%APPDATA%\WittyMediaServer\wms.json` | |
| | macOS: `~/Library/Application Support/WittyMediaServer/wms.json` | |
| | Linux/BSD: `~/.config/wittymediaserver/wms.json` | |
| 4 | Linux/BSD only: `/etc/wittymediaserver/wms.json` | system services |

Create a starter file with:

```bash
wms config init
```

and check which file and folders are in use with:

```bash
wms config path
```

## What you can set

Every setting is optional. Leave out what you don't need.

```json
{
  "listen": ":9797",
  "dataDir": "D:\\WittyMedia\\data",
  "metadataDir": "E:\\Artwork",
  "cacheDir": "",
  "transcodeDir": "R:\\transcode",
  "logDir": "",
  "logLevel": "info",
  "ffmpeg": "",
  "ffprobe": "",
  "maxTranscodes": 4,
  "publicUrl": "",
  "openBrowser": true,
  "portable": false
}
```

| Setting | What it does | Default |
|---|---|---|
| `listen` | address and port the server listens on (`:9797` = every network card, port 9797) | `:9797` |
| `dataDir` | the server's own folder: database, settings, users, watch history | per OS, see below |
| `metadataDir` | downloaded posters, backdrops and generated thumbnails | `<cacheDir>/images` |
| `cacheDir` | other cached files (disc playlists, menu stills) | `<dataDir>/cache` |
| `transcodeDir` | temporary video while transcoding — put it on a fast SSD or RAM disk | `<dataDir>/transcode` |
| `logDir` | log files (can also be changed in *Settings → WittyLog*) | `<dataDir>/logs` |
| `logLevel` | `error`, `warn`, `info` or `debug` | `info` |
| `ffmpeg`, `ffprobe` | use these FFmpeg programs instead of the bundled ones | bundled, then `PATH` |
| `maxTranscodes` | how many videos may be transcoded at once; lower it on small machines. When all are in use, a stream nobody has asked anything of for 20 seconds gives way, otherwise the next viewer waits | `4` |
| `publicUrl` | the address people use from outside your home, e.g. `https://media.example.com`. Setting it also turns remote access on | — |
| `remoteAccess` | allow connections from outside your home without using *Settings → Remote Access*, for servers nobody can reach from a home network first ([REMOTE.md](REMOTE.md)) | `false` |
| `trustedProxies` | reverse proxies whose `X-Forwarded-For` is believed: addresses or networks, separated by commas; added to those in Settings | — |
| `openBrowser` | open the setup page in your browser the first time the server starts | `true` |
| `portable` | keep everything next to the program | `false` |

**Relative paths** (like `"data"`) are relative to the folder the config file is in.

A setting can also be given as an environment variable or a command-line flag, which win over
the file: `WMS_LISTEN` / `-listen`, `WMS_DATA_DIR` / `-data`, `WMS_METADATA_DIR`,
`WMS_CACHE_DIR`, `WMS_TRANSCODE_DIR`, `WMS_LOG_DIR` / `-log-dir`, `WMS_LOG` / `-log`,
`WMS_FFMPEG` / `-ffmpeg`, `WMS_FFPROBE` / `-ffprobe`, `WMS_MAX_TRANSCODES` / `-max-transcodes`,
`WMS_PUBLIC_URL` / `-public-url`, `WMS_NO_BROWSER=1` / `-open-browser=false`.

`WMS_TUNER_COMMANDS=1` allows the custom command tuner, which runs a program on the server. There is no
setting for it on purpose: a web session alone must not be able to turn it on.

A typo in a setting name stops the server with a clear message instead of being ignored.

## Portable mode

Portable mode keeps **everything** — program, database, artwork, logs — in one folder, so you
can run the server from a USB drive or an external disk, or keep several independent copies.

To make any folder portable, put a `wms.json` next to `wms.exe` (or `wms`):

```json
{ "portable": true }
```

or run `wms config init --portable`. The data then lives in a `data` folder beside the
program. The portable download already contains this file.

> Keep a portable server on a drive that stays plugged in while it runs, and back up the whole
> folder to keep your libraries, users and watch history.

## Folders the server uses

Without a config file the data folder is:

| System | Data folder |
|---|---|
| Windows | `%LOCALAPPDATA%\WittyMediaServer` |
| macOS | `~/Library/Application Support/WittyMediaServer` |
| Linux, BSD | `~/.local/share/wittymediaserver` (or `$XDG_DATA_HOME/wittymediaserver`) |
| Docker | `/config` |

Moving the **metadata** folder: stop the server, move the folder's contents to the new place,
set `metadataDir`, start the server. Artwork is referenced by file name, so nothing has to be
rescanned. The same works for `transcodeDir` (it only holds temporary files).

## Logs

Logs are plain text files named `wms-<date>.log`, one line per message. A new file starts after 20 MB. They
are kept for 14 days and 200 MB in all, whichever comes first. If the disk has under 100 MB free, only warnings
and errors are written and old logs are removed; if the folder can't be written, the log stays in memory and the
settings page says so.

**Logs never leave your server.** Nothing is uploaded or sent anywhere; nobody sees them
unless you send a file yourself, for example with a bug report.

*Settings → WittyLog*, the server's logging, has:

- **The viewer** — a live tail you can filter by level, area, session and text. Each failure has an explanation
  in plain words and a suggested fix where there is one, and one press shows everything from the same session:
  the request that started a playback, what FFmpeg said, and what the player reported.
- **Modules** — every module is listed with a level of its own and, under **Advanced**, the kinds of detail
  it can add, such as a helper's every call or a disc's navigation. A newly installed module is noticed and
  listed by itself, marked **New**. Changes take effect at once.
- **Catch a problem** — one button records everything for up to two hours, then goes back to your usual settings
  by itself, so you can try to make a problem happen and not have to remember to turn it off.
- **What to log** — each area (playback, transcoding, discs, games, Live TV, library, modules, updates, remote
  access, sign-ins, app errors, web requests, storage) has its own level, which changes at once with no restart.
  Areas that name what you watch or who signs in (playback, discs, sign-ins, app errors, web requests) keep to
  warnings and errors until you ask for more. Each one says what it adds.
- **Where the logs are** — the folder, an **Open folder** button (when the browser runs on the
  server itself), and a picker to choose another folder.
- **Crash reports** — if the server crashes, or stops without shutting down (a power cut, say), the next start
  writes a report and points it out here. They're in `crash/` inside the data folder.
- **Support bundle** — a zip of recent logs, crash reports and a summary of your system, to send with a bug
  report. It shows you what's inside first, and can hide titles and file paths. It's made when you ask and
  downloaded by you: nothing is uploaded.

Passwords, sign-in tokens, stream tokens, API keys, PINs, cookies and setup codes are removed from every log line
automatically. Apps report their errors only to your own server, and you can turn that off.

For developers: [how every part of the project logs](LOGGING.md).

---
<sub>WittyMediaServer 0.5.0-alpha documentation</sub>
