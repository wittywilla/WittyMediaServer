# Design: a reliability audit, an overhaul, and a server that can explain itself

> **Status on 2026-11-04**
>
> **In progress:** the music and lyrics stop [#17](https://github.com/wittywilla/WittyMediaServer/issues/17), the code audit [#82](https://github.com/wittywilla/WittyMediaServer/issues/82), faults found so far ([#101](https://github.com/wittywilla/WittyMediaServer/issues/101), [#102](https://github.com/wittywilla/WittyMediaServer/issues/102)), and a soak run before each release [#115](https://github.com/wittywilla/WittyMediaServer/issues/115). The whole audit is [an idea](https://github.com/wittywilla/WittyMediaServer/discussions/25).

The owner, 2026-11-02: "the crash I'm reporting was during normal use. I was trying to play music, it was trying to download lyrics, and the full server crashed. yet, the logs show no evidence of what I experienced. This need to be fixed and more robust. Make design docs noting the need for a massive audit, overhaul, and better logging capabilities."

This document is the case for that audit, its scope and method, the overhaul it should lead to, and the logging the server needs so that the next failure explains itself. The log-format details and stop reasons are in [logging-improvements.md](logging-improvements.md); this is the larger programme around them.

Not built. Not tested. No attempt was made to reproduce the failure for this document.

---

## 1. Why a massive audit, now

**The incident.** On 0.6.0-alpha, on the owner's Windows PC (i7-12700H, 64 GB, RTX 3070): playing music with online lyrics, the whole server stopped working. The log has no line about the music, the lyrics, an error or a crash; the last lines before the stop are an update check at 14:26 and `shutting down` at 14:33. Windows recorded no crash. See [logging-improvements.md §1](logging-improvements.md#1-what-happened-on-2026-11-02).

**What it shows:**

1. **One feature can take the whole server down,** or make it stop answering, and nothing contains it.
2. **When that happens, the server can't say what it was doing.** Default log levels hide normal activity; a hang leaves no trace; a fatal runtime error on a tray-started server loses its message.
3. **0.6.0-alpha grew fast:** in two days, dozens of parallel chats added the module separation, a store, stats, Tdarr-style transcoding, WittyTV themes, lyrics and karaoke, game streaming changes, backups, installers and more. Each chat tested its own piece. Nobody has looked at the server as one running system, for a whole day of ordinary use, on the owner's kind of machine.
4. **The test record admits it:** flaky tests under load (`TestRealSubtitles/embedding_files`, `TestRealDolbyVision`, `TestRealHEVCInMP4IsTaggedForApple`, a format row that timed out, a lyrics test and a stream token test that failed in CI on 2026-11-02), the optimise pass stopped early, Windows `go test` unchecked since Application Control started blocking test binaries, and the final screen reader, keyboard and controller pass skipped.

**The owner's bar** for 0.6.0 was "an audit by the top developers and designers: no half-baked things". The reliability side of that hasn't been done.

---

## 2. What the audit must answer

For every part of the server and every module:

1. **Can it take the server down?** A panic, a fatal runtime error (concurrent map write, stack overflow, out of memory), a deadlock, a hang that holds something everyone needs (a lock, a database connection, a worker slot, the HTTP server's connection limit).
2. **Can it stall others?** A network call, an FFmpeg or ffprobe run, a file read on a slow or sleeping drive, or a module helper, made while holding a lock or a database transaction, or with no time limit, or on a path every request shares.
3. **Does it leak?** Goroutines, file handles, processes (FFmpeg, helpers, game runners), memory, temp files, transcode folders.
4. **Does it fail in a way a person understands?** A message on screen, a line in the log with the cause, a task in activity that shows failed instead of spinning.
5. **Does it leave evidence?** Enough in the log, at the default levels, to tell afterwards what happened.

---

## 3. Where to start: the incident's path

The audit starts with everything that happens when someone plays a song with online lyrics, because that's what failed. **Hypotheses to test, none confirmed:**

| Suspect | Why | How to test |
|---|---|---|
| Reading lyrics from the file's own tags | `localLyrics` runs ffprobe (15 s limit) and reads ID3. A huge or damaged tag, or a file on a sleeping drive | Songs with 50 MB embedded art, broken tags, on a USB drive that sleeps |
| The online lookup | 12 s per request to LRCLIB or the mirror, several sources in turn, retried? From many tabs or devices at once? | Point the sources at a test server that accepts and never answers, answers slowly, sends a huge body, sends invalid JSON |
| Saving what was found | `os.WriteFile` beside the music when `saveMetadata` is on (it was on), into `D:\ServerMedia\Music`. A read-only file, a locked file, an antivirus scan holding it | Read-only folder, file open in another program |
| Database writes | Lyrics saved, offsets, play history, all on SQLite with one writer. Anything holding a write transaction while doing network or disk work blocks every other write, and with a busy timeout, every request waiting on it | Trace transaction lengths under load; look for I/O inside transactions |
| The audio stream itself | Transcoding a song while lyrics load; FFmpeg's pipe filling if nobody reads it | Play while the lookup hangs |
| The browser side | The music page polling lyrics or position fast enough to exhaust the server's connection limit (`max-connections 2048`) or a per-user limiter | Count requests per second from the player; watch with two tabs |
| The visualizer or karaoke | The page itself freezing (heavy canvas work) looks like the server going down | Check whether other devices could still reach the server at the time |
| A runtime crash | Concurrent map access in a cache added in 0.6.0, under the race detector only seen in some runs | Run the music and lyrics paths under `-race` for hours |

The first job is to **reproduce it**, on Windows, with the owner's settings (online lyrics on, `saveMetadata` on, Music on drive D:), for long enough, with the new evidence collection (section 5) turned on.

## 4. Scope of the whole audit

Each row is a work package with its own issue, owner chat and report.

| Area | Focus |
|---|---|
| **Server core** | Start, stop, signal handling, every background loop (scans, update checks, discovery, module checks, task registry, schedulers), every shared lock, the database layer (transaction lengths, busy timeouts, connection pool), the HTTP server (time limits on reads, writes and idle; per-route limits; streaming routes) |
| **Playback** | Direct play, HLS sessions and their FFmpeg runs, live TV sessions, disc sessions, clean-up when viewers leave, what happens when FFmpeg hangs or dies |
| **Music** | Lyrics (local, sidecar, online, saving), karaoke, radio streams (external, slow, endless), visualizer data, play history |
| **Library and metadata** | Scans on big and slow libraries, online lookups (TMDB, TVmaze, Wikidata, MusicBrainz, logo service) with rate limits, retries and time limits, saving metadata beside media |
| **Modules** | Helpers and services: start, idle stop, crash, hang, version change while in use, the 424 path when a module is missing, notifications flood |
| **Games** | Runners, the service, encoders, sessions that end badly, the sandbox's failure modes (the log showed `sandbox: job limits err="The parameter is incorrect."` on Windows for two games on 2026-11-02, which needs explaining) |
| **Remote and clients** | Remote access, discovery, the client API under many devices, reconnects |
| **Windows specifics** | The tray app, the service, paths with odd characters, drives that sleep, antivirus holding files, Smart App Control |
| **Data** | Backups, restore, reset (it left the cache folders missing), migrations, disk full |

## 5. Making the server explain itself

The logging work in [logging-improvements.md](logging-improvements.md) (stop reasons, start summaries, crash reports, timeline) plus these, which turn "the logs show nothing" into "the logs show exactly what it was doing":

### 5.1 A flight recorder

- **Always on, in memory:** the last 10 minutes of everything at **debug** level (all categories), in a ring buffer of a fixed size (for example 16 MB). Nothing is written to disk unless something goes wrong, so it costs almost nothing.
- **Written to `crash\` when:** the server stops uncleanly, a hang is detected (5.2), a panic is caught, or an admin presses **Save what just happened** in Settings → WittyLog (a button for exactly the owner's situation: "something just went wrong").
- **Go's own flight recorder** (`runtime/trace.FlightRecorder`, in Go since 1.25) for the same window: an execution trace of the last seconds, showing what every goroutine was blocked on.
- Included in support bundles, sanitised like the rest.

### 5.2 A hang detector

- A **watchdog** that checks, every few seconds, that the server's own heartbeat ticks, that a request to its own `/api/health` answers within 2 seconds, and that the database answers a trivial query.
- When any of those fails for 10 seconds: **dump every goroutine's stack** (`runtime.Stack(all)`) to `crash\hang-<time>.txt`, write the flight recorder out, log `level=ERROR msg="the server stopped answering"` with what was blocked, and keep watching. A second dump 30 seconds later shows whether it's stuck or slow.
- Optional (admin setting, off by default): **restart itself** after a hang longer than a minute, saying so in the log and on Home.

### 5.3 Normal activity is visible

- **Every request** gets one line at info in a separate, size-capped `requests` log (method, route pattern, status, time, size, who, which device), so "what was happening at 14:30" has an answer. The main log stays readable.
- **Every outgoing call** (lyrics, metadata, logos, updates, the store, IPTV, radio) logs at info: source, what was asked (no secrets), status, time, bytes. Failures log at warn with the cause.
- **Every child process** (FFmpeg, ffprobe, helpers, runners): start with its purpose, end with exit code, time and the last lines of stderr. (Partly there today for FFmpeg and helpers.)
- **Slow things are named:** anything over a threshold (a database transaction over 1 s, a lock held over 1 s, a request over 10 s, a file read over 5 s) logs at warn with what it was.

### 5.4 The browser tells its side

- The web app and WittyMediaClient keep a small log of their own errors and of when the server stopped answering, and send it to the server when it answers again (the client log and `errorReport.ts` exist; this extends them). Then "the server crashed while I played music" shows in the server's log as "the music page lost the server at 14:31:07, last request GET /api/music/lyrics/123".

### 5.5 Stderr is never lost

- From the first line of `main`, stderr goes to `crash\stderr-<run>.txt` as well as wherever it went, so a fatal runtime error on a tray-started or service-started server is kept.

## 6. The overhaul the audit leads to

The audit will produce its own list. These rules are already clear and should be applied everywhere as they're audited:

1. **Nothing slow while holding something shared.** No network call, child process or disk scan inside a lock or a database transaction. Gather, then lock briefly, then write.
2. **Every outside call has a deadline**, from the request's context or a set limit, and a **circuit breaker**: a source that fails five times in a row is left alone for a while, and says so, instead of every request waiting 12 s on it.
3. **Every background job is a task** in the registry, with a time limit, a cancel and a clear failed state.
4. **Every goroutine has an owner and an end.** Tests check for leaks (`goleak` in package tests).
5. **Panics are caught at every boundary** (HTTP handler, task, background loop, module call) with the stack logged, and turned into a clear error; a fatal runtime error is prevented by the race detector in tests, not caught.
6. **Bounded everything:** queues, caches, buffers, concurrent lookups per source, concurrent FFmpeg runs (exists), request bodies (exists).
7. **Isolation for risky work:** heavy parsing of untrusted files (tags, subtitles, images) in a helper process with limits, as modules already do, so a parser bug can't take the server down. The cost is measured first.
8. **Features fail alone:** lyrics failing shows "Lyrics couldn't be loaded" and the music plays on.

## 7. How the audit is run

- **Reproduce first:** a long session on Windows playing music with online lyrics, with the flight recorder and hang detector from section 5 built first (they're small, and without them a reproduction might again show nothing).
- **Soak tests:** 24 hours of scripted ordinary use on Windows and Linux (browse, play films, music with lyrics, live TV, a game, scans, an update check), with memory, handles, goroutines, processes and response times recorded every minute. Nothing may grow; nothing may stall.
- **Fault injection:** slow and dead network sources, a drive that disappears, a full disk, a module helper that hangs, FFmpeg that hangs, the database locked by another process, the clock jumping.
- **Race detector** on the whole server under the soak scripts, not only in unit tests.
- **Static checks:** `go vet`, staticcheck, and a lint for "I/O under lock" patterns where practical.
- **Code review by area** against section 2's five questions, with findings as issues ([../GITHUB-PRACTICES.md](../GITHUB-PRACTICES.md)), labelled `area:reliability`.
- **The owner's machine is the reference:** what failed there must be reproduced there or in a matching setup before it's called fixed.

## 8. Where it goes

| Release | Work |
|---|---|
| **0.6.3-alpha** | The evidence first: stop reasons and start summaries ([logging-improvements.md](logging-improvements.md)), stderr kept, the flight recorder, the hang detector with stack dumps, outgoing calls and child processes logged, **Save what just happened**. Then the reproduction of the music and lyrics failure, and its fix. Open the audit's milestone and one issue per area |
| **0.7.0-alpha** | Audit of the server core, playback, music and library; the overhaul rules (1 to 3, 5, 8) applied there; the requests log; the browser telling its side; 24-hour soak tests in CI on Linux, and on Windows before each release |
| **0.8.0-alpha** | Audit of modules, games, remote, Windows specifics and data; isolation of risky parsing; optional self-restart after a hang; the timeline and summaries in WittyLog for all of it |
| Before beta | The whole audit closed or each finding written down as accepted; soak tests green on every server platform; a second reader for every sandbox and supervision change |

## Done when

The owner's case, repeated (music with online lyrics, with the lookup made slow, failing or huge), never takes the server down, and lyrics failing shows a message while the music plays on. And when anything does go wrong, **Settings → WittyLog** shows, without anyone opening a file, what the server was doing at that moment and why it stopped.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
