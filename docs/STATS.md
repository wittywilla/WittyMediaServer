# Server statistics

The numbers behind the WittyStats screen. They belong to the server, not the module, so the server works without
WittyStats, and any app can use them. Paths below are relative to `/api/v1`.

- [What it costs](#what-it-costs)
- [Who sees what](#who-sees-what)
- [The API](#the-api)
- [What's available where](#whats-available-where)

## What it costs

There are two speeds:

- **While someone is watching** (a stream is open), a sampler reads the machine every two seconds, and space on the disks
  every 30. It starts with the first watcher and stops with the last. Graphics cards are asked at most every 5 seconds,
  which for an NVIDIA card means running `nvidia-smi`, the one program this ever starts.
- **All the time**, one sample a minute of a few numbers goes into a small ring in memory (a day of them), and an hour's
  average and peak go into the database (90 days). This starts no programs and wakes no disks.

`GET /system/metrics/report` looks through the database, so it's made only when asked and kept for a minute.

## Who sees what

`statsViewers` in Settings: `admins` (the default), `adults` or `everyone`. The server enforces it, and a client follows
`access` in `GET /system/metrics`.

| | An administrator | Anyone else allowed |
|---|---|---|
| The machine: processor, memory, graphics, disks, network, this server, modules | Yes | **No** |
| History of those | Yes | **No** |
| Streams and game sessions | Everyone's | Only their own |
| Libraries, codecs and resolutions | All | Only libraries they can see |
| Most watched | Across everyone | Only what they watched themselves |
| Duplicates, possible savings, scan times, database and cache sizes, log counts, bandwidth by person, peak streams | Yes | **No** |

No answer holds a file path, except `volumes[].mount` for administrators, which is where a disk is mounted.

## The API

| Route | What it gives |
|---|---|
| `GET /system/metrics` | `access`; `sections`: what can be shown here, each with `available` and, if not, `why`; `series` and `ranges` for the history; `everySec`. Always answered |
| `GET /system/metrics/stream?sections=cpu,memory` | Server-sent events: `event: sample` every two seconds with `MetricsNow`, for as long as it's open. Read it as in [TASKS.md](TASKS.md#the-api). **This is what starts the sampler, so close it when it isn't on screen.** At most 16 per account |
| `GET /system/metrics/now?sections=` | One `MetricsNow`, for an app that wants a number now and then. Takes about a quarter of a second if nobody is watching, as a speed needs two readings |
| `GET /system/metrics/history?range=1h&series=cpu,memory` | `History`: `times`, and `series` by name. `peak` too when each point stands for several readings. Admins only |
| `GET /system/metrics/report` | `MetricsReport`: everything that takes a look through the database. `?fresh=1` skips the minute's wait |

**Sections:** `server`, `cpu`, `temperature`, `memory`, `gpu`, `disks`, `diskio`, `network`, `process`, `modules`, `streams`, `games`,
`libraries`, `logs`. Leaving out `sections` gives all this account may have. A section a system can't provide is left out
of the answer rather than sent as zero, so check before showing it. The fields are in [openapi.json](openapi.json).

**Ranges:** `15m`, `1h`, `6h`, `24h`, `7d`, `30d`, `90d`. At most 240 points come back. Up to ten minutes uses the two-second samples
if someone has been watching, up to a day the minute samples, and longer the hourly rows.

**Series:** `cpu`, `memory`, `swap` and `processCpu` are percent. `netRx`, `netTx`, `diskRead` and `diskWrite` are bytes a second. `rss` and `heap`
are bytes. `goroutines`, `streams`, `transcodes` and `games` are counts.

**Numbers worth a note**

- A stream's `decision` is `direct`, `remux` or `transcode`, or what `POST /items/{id}/playback` answered, and `reasons` say why.
  `sendingKbps` is what's really going out, and `fileKbps` is the file's own average.
- A game session's `encodeMs` is the server's part of the latency. The network's part is only known to the player.
- `savings` is an estimate: older video in a newer format, at about 40% smaller. It isn't a promise for any one file.
- `duplicates` means the same title and year held more than once. `wastedBytes` is what would go if only the largest of each stayed.
  Nothing here changes or removes anything.
- A cache count stops after two seconds on a very large folder and says `partial`.

## What's available where

| | Linux | Windows | macOS | FreeBSD |
|---|---|---|---|---|
| Processor, per core | Yes | Yes | No | Yes |
| Temperature | Where the kernel has sensors | No: Windows only tells administrators, and often wrongly | No | No |
| Memory | Yes | Yes | Yes, with free memory an estimate | Yes |
| Swap | Yes | The page file | Yes | No |
| Graphics | NVIDIA with `nvidia-smi`; AMD | NVIDIA with `nvidia-smi` | No | No |
| Disk space | Yes | Yes | Yes | Yes |
| Disk speed | Yes, with how busy | Yes, by drive letter | No | No |
| Network, per interface | Yes | Yes | Names only | Names only |
| This server | Yes | Yes | Yes, with peak memory | Yes, with peak memory |
