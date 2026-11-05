# Performance

How heavy the server is on the machine it runs on, measured the same way for each release so that one can be compared with the next. This page is for people working on the server: running it doesn't need any of this.

- [What is measured](#what-is-measured)
- [Running the benchmark](#running-the-benchmark)
- [Reading the numbers](#reading-the-numbers)
- [Other benchmarks](#other-benchmarks)
- [Results](#results)
- [What was found, and where it went](#what-was-found-and-where-it-went)

## What is measured

`scripts/perf` starts a server on a new, empty data folder and measures, in this order:

1. **Start-up**, five times: from starting the program until `GET /api/health` answers, on a new data folder (the database is made) and again on the same one.
2. **Idle, with no libraries**, after first-time setup: processor use over two minutes, memory (what the system counts, what Go's heap holds live, and everything Go holds), goroutines, garbage collections, and on Linux how often the server's threads woke. It also asks Go to hand back all the memory it can at once, to show how much was only waiting to go back.
3. **A scan of each library** given with `-lib`, one at a time: how long it took, the server's processor time and allocations, and its highest memory. FFmpeg's own time isn't in the processor column, since FFmpeg is a program of its own.
4. **The same scans again**, with nothing changed, as the six-hourly scan does.
5. **Idle again**, after the scans.
6. **The API**: each route asked 30 times in turn, one at a time, with how much it sent, allocated, and how many database statements it ran per request.
7. **Every database statement** the server ran during all of that, with how often and for how long, and SQLite's plan for each (`EXPLAIN QUERY PLAN`), marking the ones that read a whole table or sort without an index.

For runs to compare, the benchmark turns off what would reach the internet (update checks, TVmaze, Wikidata, MusicBrainz), and turns off `saveMetadata` so nothing is written into the media folders. The media is only read.

## Running the benchmark

The server it measures is built with the `wmsprofile` tag, which adds a profiling port. Only that build has it, and it listens only on this machine, at the address in `WMS_PPROF`. Release builds have none of this in them. The port serves Go's `pprof` profiles, `/debug/wms` (the process as the system sees it, and Go's own counters) and `/debug/wms/sql` (every statement run).

On Windows, from the repository:

```bash
go build -tags wmsprofile -o .build/wms-prof.exe ./cmd/wittymediaserver
```

```bash
go run ./scripts/perf -exe .build/wms-prof.exe -ffmpeg <ffmpeg.exe> -ffprobe <ffprobe.exe> -lib "movies=D:\WittyDemoMedia\Movies" -lib "music=D:\WittyDemoMedia\Music" -out <folder>
```

On Linux (or WSL), the same with the Linux build: `GOOS=linux go build -tags wmsprofile -o wms-prof ./cmd/wittymediaserver` and `GOOS=linux go build -o perf ./scripts/perf`, then `./perf -exe ./wms-prof -lib "movies=/mnt/d/WittyDemoMedia/Movies" … -out <folder>`. Idle wake-ups are only counted on Linux.

- `-lib type=folder` adds a library of that type (repeat it). Types that need a module (books, photos, discs, games) are refused without the module, and the report says so.
- `-port` is the server's port, and the profiling port is the one above it. The benchmark refuses a port that's in use, so it never measures someone else's server.
- `-starts`, `-settle`, `-idle` and `-repeat` change how many start-ups, how long it waits before an idle reading, how long the reading is, and how often each route is asked.
- `-report` with `-out` of an earlier run prints its report again, with the plans made again: after adding an index, say.

The report is printed, and `results.json`, goroutine lists, heap profiles and the server's log are left in the `-out` folder.

The numbers below were measured on the demo media: openly licensed films, TV, music, podcasts and more, credited item by item, kept outside the repository (`D:\WittyDemoMedia` on the owner's machine). A run only reads it.

## Reading the numbers

Many things run on a development machine at once, and times move with them: the same start-up took 2 s and 18 s on one busy afternoon. So:

- **Trust counts over times.** Allocations, objects, statements per request, goroutines, wake-ups, query plans and bytes sent don't change with a busy machine. They are what a change should be judged on first.
- **Report times with their spread.** Start-up is given as the median of five, with the fastest and slowest. Route times are the median (p50) and the 95th percentile of 30.
- **Compare runs on the same machine**, as close together as possible, with the same media and flags.

What the columns mean:

- **CPU %** is of one processor: 100 % is one processor fully busy.
- **RSS** is the memory the system counts against the server. **RSS after returning memory** is the same after Go has handed back all it can; the difference is memory Go would have given back by itself, over the next few minutes.
- **Go heap live** is what the server's data actually takes. **Go total** is everything the Go runtime holds from the system, including what it keeps ready to reuse.
- **Wake-ups a minute** is how many times the server's threads were switched in while idle (Linux counts every thread's context switches). An idle server should wake as little as possible, so that the processor can stay in its low-power states.
- **Statements** is database statements per request. Every signed-in request costs two (its session and its account), so a route at 2 or 3 does almost nothing else.

## Other benchmarks

Smaller ones live beside the code, for `go test -bench`:

| Benchmark | What |
|---|---|
| `internal/library`: `BenchmarkStore`, `BenchmarkHomeQueries` | Listing queries on a large library |
| `internal/library`: `BenchmarkHasDjVu` (with #146) | Whether DjVuLibre is installed, which is kept for a minute |
| `internal/db`: `BenchmarkQueryThroughLoggedDriver` | What the statement logging costs |
| `internal/server`: `BenchmarkRequestLogged` | What request logging costs |
| `internal/logging`, `internal/metrics`, `internal/sysinfo`, `internal/tasks` | The log, the statistics, readings of the machine, and progress with nobody watching |

FFmpeg's own benchmark, for transcoding, is separate (#81).

## Results

All on one laptop: an Intel Core i7-12700H (14 cores, 20 threads) with 64 GB of memory, Windows 11, and Ubuntu in WSL on the same machine. The demo media was read from `D:` (on Linux through WSL's `/mnt/d`, which is much slower to look through than a Linux disk). The machine was running a dozen other builds and tests the whole time, at 100 % of its processors much of it, so **every time below is slower and more spread out than on a quiet machine**: compare the counts.

### 0.7.0-alpha, before the work in #89

The server as it was at the start of 0.7.0-alpha (0.6.2-alpha), 4 November 2026.

**Windows**, with 11 libraries of the demo media (books and photos need modules, so they weren't scanned):

| | |
|---|---|
| Start-up | median 3.4 s (2.1 to 18.9) on a new data folder, 4.0 s (1.6 to 9.6) again: the machine was at its busiest |
| Idle, no libraries | 0.05 % of one core, RSS 34 MB, Go heap 3.5 MB live, 27 goroutines |
| Idle, after the scans | 0.00 % of one core, RSS 46 to 48 MB, Go heap 4.1 MB live, 27 goroutines |
| Scanning, peak RSS | 49 MB |
| First scan of music, 521 files | 174 to 236 s, 5.6 to 6.3 s of the server's processor time (the rest is FFmpeg reading each file) |
| `GET /api/media-types` | **p50 14.4 s, p95 145 s**: DjVuLibre looked for on PATH every time |
| `GET /api/settings` | **102 statements** a request |
| `GET /api/home` | 33 statements, 1.2 MB allocated a request |
| A poster asked `?w=300` | **the whole picture: 1.6 MB** for one demo poster, 503 KB for another |

**Linux** (WSL), with 7 libraries (films, TV, music, podcasts, home videos, music videos and commercials: 764 files):

| | |
|---|---|
| Start-up | median 212 ms (202 to 251) on a new data folder, 139 ms (113 to 253) again |
| Idle, no libraries | 0.010 % of one core, RSS 43 MB (41.5 MB once Go has given back what it can), 27 goroutines |
| Idle, after the scans | 0.007 % of one core, RSS 45 MB (42.8 MB), 27 goroutines |
| Scans with nothing changed | 0.3 to 43 s a library, under 1.2 s of processor each: looking through folders on `/mnt/d` |
| `GET /api/settings` | 102 statements, 138 KB allocated a request |
| `GET /api/home` | 23 statements, 1,032 KB allocated a request |
| `GET /api/media-types` | 1.4 ms: looking through PATH is quick on Linux |

**Query plans:** the statements a request makes on `items`, `progress` and the photo, game and creator tables use an index. What reads a whole table:

- **Once at start-up:** upkeep such as filling in rating ages and the games' queue (two of these read all of `items`), clearing expired sessions, and marking interrupted jobs.
- **Small tables, on requests:** `libraries` (a row for each library), `collections` and `listings`.
- **`shows`, in Browse's queries:** they ask for `(? = 0 OR s.library_id = ?)`, which stops SQLite using the index on `library_id`, so every show in every library is looked at. That costs little at a few thousand shows; it's the first thing to look at if Browse slows down on a large TV library.

The sorts without an index are of the few rows already found (a page of a list, a library's genres). No index was missing for the demo media.

### 0.7.0-alpha, after

The same, with #146, #199, #214, #218 and #273 merged locally. Linux, back to back with the run above:

| | Before | After |
|---|---|---|
| Goroutines while idle | 27 | 25 |
| Wake-ups a minute while idle (the server's own, two runs of each, 3 minutes each) | 89 to 144 | 85 to 141: no change that can be told from the noise. The two disc sweeps woke at the same moments as the HLS sweep, which still runs |
| `GET /api/settings` | 102 statements, 138 KB allocated | 3 statements, 20 KB |
| `GET /api/home` | 23 statements, 1,032 KB | 19 statements, 914 KB |
| A poster asked `?w=300` (item 226) | 1,660,855 bytes, 1 open of the media drive | 25,255 bytes, 1 open of the media drive |
| Its time, five requests | 74 ms to 2.8 s | 18 to 34 ms |
| `GET /api/media-types` on Windows | 2 to 12 s per lookup of DjVuLibre | about 100 ns after the first (`BenchmarkHasDjVu`) |
| Idle CPU and memory | 0.010 %, RSS 43 MB | 0.028 %, RSS 40 MB: the same, within what the busy machine moves them |
| Start-up again | median 139 ms | median 124 ms: the same, within the noise |

Totals of statements over a whole run don't compare between runs: the benchmark looks at a scan's progress every quarter of a second, so a scan that takes longer on a busy machine adds statements of its own.

## What was found, and where it went

| What | Where it went |
|---|---|
| The formats page and DjVu books looked for DjVuLibre on PATH every time: seconds on Windows | #118, PR #146 |
| Posters asked for smaller came whole for everything but games; then, checking a poster was inside a library looked at the media drive 37 times | #178, PR #199 (posters, backdrops up to 1920, show and season art, remote art, video frames at least 480) |
| Settings took a statement for each of a hundred settings | #200, PR #214 |
| DVD and Blu-ray sessions were swept every minute from start-up, with no disc open | #215, PR #218 |
| Home read progress once for each library | #252, PR #273 |
| A minimum hardware guide, from these figures and FFmpeg's | #113, PR #269 |
| Start-up ran FFmpeg twice, one after the other, before the database opened | #164, PR #226 (by #81's work) |
| `internal/hls` sweeps every 30 s from start-up, and its live sweep every 5 s once a live stream has been watched; `GameMeta` wakes every 5 minutes with no games | Noted in #215, for the chats changing `hls.go` and for later |
| Go's memory settings (`GOGC`, `GOMEMLIMIT`) | Left as they are: the server's own heap is about 4 MB live while idle and its RSS peaked at 49 MB while scanning, and only 1 to 3 MB is ever waiting to go back to the system. The memory that matters is FFmpeg's (hundreds of MB a conversion, see #81), which these settings don't touch |
| Missing indexes | None found for the demo media; the benchmark's plan list is there to catch one in a later release. Browse's `(? = 0 OR …)` on shows is noted above, not changed |
| Scans with nothing changed, on a slow drive | Up to 43 s a library on `/mnt/d` with under a second of processor: the time is in looking through the folders, as each unchanged file is passed over without reading it. Not changed |
| Not measured yet | Libraries that need a module (books, comics, photos, discs, games), and how long module helpers take to start: the benchmark doesn't install modules. Next time, with `WMS_TEST_CATALOG` |

---

<sub>WittyMediaServer documentation</sub>
