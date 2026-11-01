# Server statistics

See what your server's machine, streams and libraries are doing, on a screen you arrange yourself. The screen comes from a module called WittyStats, and works on phones, TVs and computers.

- [What it needs](#what-it-needs)
- [What it shows](#what-it-shows)
- [Customising the layout](#customising-the-layout)
- [Resetting the layout](#resetting-the-layout)
- [Who may see it](#who-may-see-it)
- [What each system can show](#what-each-system-can-show)
- [What it costs your server](#what-it-costs-your-server)
- [For developers](#for-developers)

## What it needs

WittyStats is a module, and it isn't part of the server until you add it.

1. Open **Settings → Modules**, and find **WittyStats** in the store. See [Modules and the store](MODULE-STORE.md).
2. Install it.
3. **Statistics** appears in the main menu, once the app has reloaded.

The numbers belong to the server. It collects them and decides who sees what, with or without this module. Removing WittyStats takes the screen away, and saved layouts stay with each person's account.

## What it shows

| Group | What's there |
|---|---|
| **Machine** | Processor and its cores, temperature, memory, swap, graphics and encoder sessions, disk space, disk speed and how busy, network per connection |
| **This server** | The program itself, uptime, modules and updates, background work, the database, caches, warnings and errors |
| **Playing now** | Who is playing what on which device, whether it's direct, remuxed or transcoded and why, its rate and picture size, how much converting is going on, and game sessions with frame rate and encoding time |
| **Libraries** | Items and sizes by library and type, video formats, picture sizes, duplicates and possible savings, scan times |
| **History** | Most watched, data by person, the busiest moment, playing over time |

A few things to read carefully:

- **Not available here** means this server can't provide that number, and it says why. It's never drawn as a zero.
- **Duplicates and possible savings are estimates**, and are marked so. Duplicates means the same title and year held more than once. Savings means older video in a newer format, at about 40% smaller, and isn't a promise for any one file. Nothing here changes or removes anything.
- **Encoding time** is the server's part of a game's delay. The network's part is known only to the player.

At the top of the page:

- **Pause** holds the live numbers, and **Resume** starts them again.
- **Time** chooses how far back the history charts go.
- **Show** turns every widget into **Charts** or **Tables**. Each widget can also switch by itself.

Each widget's details button opens more of it with every row, and **Save as CSV** and **Save as JSON**. Those files are made in your browser from what it already holds.

## Customising the layout

Each person has their own layouts.

1. Press **Customise**.
2. Each widget gets buttons to move it earlier or later, make it wider, narrower, taller or shorter, and remove it. With a mouse or finger you can also drag its handle and its corner.
3. **Add widgets** adds more.
4. Press **Done**.

**Undo** takes back the last change, and works until you leave the page.

**Start from** offers three presets, **Simple**, **Standard** and **Advanced**. A preset replaces what's on the layout on screen with no question asked, and **Undo** brings it back.

**This layout** manages your layouts. You can have up to eight:

- **New** makes a copy of what's on screen.
- **Rename** and **Delete** act on the one on screen. You can't delete your last one.
- Outside **Customise**, the **Layout** chips switch between them.

The grid has four columns on a wide screen, two on a medium one and one on a phone. A widget is 1 to 4 wide and 1 to 3 tall, and asks for no more than the screen has.

Everything can be reached with a keyboard, a TV remote or a controller, because everything is a real button.

## Resetting the layout

Under **Customise**, press **Reset to stock**, then **Reset to stock** again to confirm. All your layouts go, and you get one layout with the **Standard** preset. No statistics are lost, and **Undo** still brings back what was there.

## Who may see it

Administrators always see everything. Who else may see it is the `statsViewers` setting: `admins` (the default), `adults` or `everyone`.

There's no switch for this in Settings yet. An administrator can change it through the API, with `PUT /api/settings` and `{"statsViewers": "adults"}`.

Anyone it lets in sees only part of it:

| | Administrator | Anyone else allowed |
|---|---|---|
| **The machine**, its history, this server, modules | Yes | No |
| **Streams and game sessions** | Everyone's | Only their own |
| **Libraries, codecs and resolutions** | All | Only libraries they can see |
| **Most watched** | Across everyone | Only what they watched |
| **Duplicates, savings, scan times, database and cache sizes, log counts, bandwidth by person, peak streams** | Yes | No |

There's no empty frame for what isn't theirs, and every preset still works. Someone who isn't allowed at all gets a message saying so.

## What each system can show

What a system can't provide says why on the screen.

- **Linux:** everything, where the kernel has sensors for temperature. Graphics for NVIDIA with `nvidia-smi`, and AMD.
- **Windows:** everything but temperature, which Windows only tells administrators, and often wrongly. Graphics for NVIDIA with `nvidia-smi`. The page file counts as swap.
- **macOS:** memory (free memory is an estimate), swap, disk space and this server. No processor use, temperature, graphics, disk speed or network traffic. This has not been run on a real Mac yet.
- **FreeBSD:** processor, memory, disk space and this server. No temperature, swap, graphics, disk speed or network traffic. This has been built for FreeBSD but not run there yet.

Disk health is whatever the system will tell without administrator rights, as in [WittyDiagnoser](DIAGNOSER.md#checking-storage).

## What it costs your server

- **While someone has the page open,** the server reads the machine every two seconds, and disk space every 30 seconds. Graphics cards are asked at most every 5 seconds, which for an NVIDIA card means running `nvidia-smi`, the one program this ever starts. It stops when the page is hidden or left.
- **All the time,** one small sample a minute goes into memory (a day of them), and an hour's average and peak go into the database (90 days). This starts no programs and wakes no disks.

## For developers

The numbers are the server's, so any app can use them. WittyStats is the screen, in its own repository, and asks only for the sections of widgets on the layout, only while the page is showing. Paths below are relative to `/api/v1`.

### The API

| Route | What it gives |
|---|---|
| `GET /system/metrics` | `access`; `sections`: what can be shown here, each with `available` and, if not, `why`; `series` and `ranges` for the history; `everySec`. Always answered |
| `GET /system/metrics/stream?sections=cpu,memory` | Server-sent events: `event: sample` every two seconds with `MetricsNow`, for as long as it's open. Read it as in [Background tasks](TASKS.md#the-api). **This is what starts the sampler, so close it when it isn't on screen.** At most 16 per account |
| `GET /system/metrics/now?sections=` | One `MetricsNow`, for an app that wants a number now and then. Takes about a quarter of a second if nobody is watching, as a speed needs two readings |
| `GET /system/metrics/history?range=1h&series=cpu,memory` | `History`: `times`, and `series` by name. `peak` too when each point stands for several readings. Admins only |
| `GET /system/metrics/report` | `MetricsReport`: everything that takes a look through the database. Made only when asked and kept for a minute. `?fresh=1` skips the wait |
| `GET /system/metrics/drives` | Each disk's health, where the system will say, or `note` saying why not. Admins only, and kept for ten minutes: on Windows, asking starts PowerShell, and the first answer can take seconds |

`statsViewers` is enforced by the server, and a client follows `access` in `GET /system/metrics`. No answer holds a file path, except `volumes[].mount` for administrators, which is where a disk is mounted.

**Sections:** `server`, `cpu`, `temperature`, `memory`, `gpu`, `disks`, `diskio`, `network`, `process`, `modules`, `streams`, `games`, `libraries`, `logs`. Leaving out `sections` gives all this account may have. A section a system can't provide is left out of the answer rather than sent as zero, so check before showing it. Lists with nothing in them, such as `streams` with nobody watching, are left out too. The fields are in `docs/openapi.json`.

**Ranges:** `15m`, `1h`, `6h`, `24h`, `7d`, `30d`, `90d`. At most 240 points come back. Up to ten minutes uses the two-second samples if someone has been watching, up to a day the minute samples, and longer the hourly rows.

**Series:** `cpu`, `memory`, `swap` and `processCpu` are percent. `netRx`, `netTx`, `diskRead` and `diskWrite` are bytes a second. `rss` and `heap` are bytes. `goroutines`, `streams`, `transcodes` and `games` are counts.

**Numbers worth a note**

- A stream's `decision` is `direct`, `remux` or `transcode`, or what `POST /items/{id}/playback` answered, and `reasons` say why. `sendingKbps` is what's really going out, and `fileKbps` is the file's own average.
- A game session's `encodeMs` is the server's part of the latency.
- `wastedBytes` under `duplicates` is what would go if only the largest of each stayed.
- A cache count stops after two seconds on a very large folder and says `partial`.

---

<sub>WittyMediaServer documentation</sub>
