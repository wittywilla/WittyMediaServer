# Design: logs that explain every stop, start and fault

> **Status on 2026-11-04**
>
> **In progress:** stop reasons and how the last run ended [#93](https://github.com/wittywilla/WittyMediaServer/issues/93), the cache folders after a reset [#66](https://github.com/wittywilla/WittyMediaServer/issues/66), the support bundle [#73](https://github.com/wittywilla/WittyMediaServer/issues/73).
>
> **Done already:** the tray app restarts a server that stopped by itself, gives up after three quick failures, and keeps the server's own output in a file while it runs it (`cmd/wittymediaserver-tray`).
>
> **Planned:** the rest, [the idea](https://github.com/wittywilla/WittyMediaServer/discussions/24).

The owner, 2026-11-02: "Server crashed. can't find reason in UI logs. take a look. Maybe improve the log module?" With the support bundle `wittymediaserver-logs-2026-11-02-145800.zip`.

Not built. Not tested. Section 1 is what was found by reading that bundle and Windows' own event log; the rest is the plan.

---

## 1. What happened on 2026-11-02

| Time (local) | Source | What |
|---|---|---|
| 11:55:44 | server log | The server was reset ("keep-copy") and started fresh. The `cache` and `cache\images` folders were missing: `err="The system cannot find the path specified."` |
| 11:55 to 14:26 | server log | Modules installed, libraries scanned, a channel started, update 0.6.1-alpha found |
| **14:33:43** | server log | `msg="shutting down"`. A clean stop, with **no reason given**. No crash report was written, so the server didn't crash |
| 14:34:10 | Windows Application log | The 0.6.1-alpha MSI starts; error **2613**; rolled back (status 1603) |
| 14:34:25 | Windows Application log | Tried again; 2613 again; rolled back |
| 14:57:11 | server log | Started again by hand |

**What the owner saw (their correction, 2026-11-02):** "the crash I'm reporting was during normal use. I was trying to play music, it was trying to download lyrics, and the full server crashed. yet, the logs show no evidence of what I experienced."

**What the evidence says, read against that:**

- Nothing between 14:26:23 (update found) and 14:33:43 (`shutting down`) was logged at all: no music playback, no lyrics request, no error. With **playback** and **http** at `warn`, a song playing and lyrics being fetched leave no line.
- No Windows Error Reporting or Application Error event for the server that day, and no leftover crash marker from the earlier run, so the process didn't die from a runtime crash: it reached its own shutdown at 14:33:43.
- The most likely story is a **hang**: the server stopped answering while music and lyrics were loading, which from the browser looks exactly like a crash, and then it was quit (tray or installer). A hang writes nothing, so the log has nothing. A crash the log missed is also possible (a fatal runtime error on a tray-started server goes to a stderr nobody keeps).
- The cause is **not known** and can't be found from what was recorded. That is the failure to fix. See [reliability-audit.md](reliability-audit.md) for the audit and the overhaul this calls for.

**What we can't say:** what the server was doing, and *who* asked it to stop. The log line has no reason.

**Also found:**

- **A real bug:** after the reset, `cache` and `cache\images` didn't exist, and the server didn't create them. Every thumbnail failed all day with `Could not open file ... cache\images\thumb-*.jpg` (in the morning log too). They only came back when the server restarted. The server must create every folder it writes to, when it needs it, not only at start.
- The support bundle has no `crashes/` folder when there are none. It should say "no crash reports", not leave the reader to wonder.
- `summary.json` says `"log": {"since": "0001-01-01T00:00:00Z"}`, an empty time.
- The reset moved the earlier logs into `before-reset-20261102-165543`, so the bundle holds nothing from before 11:55. The bundle should include them, or at least say they exist and where.
- With **playback**, **discs** and **http** at `warn` by default, a person reporting "it stopped playing" gives us almost nothing about what was playing.

---

## 2. Every stop says why

**Each stop logs one line with a reason**, from a fixed list, plus what was running:

```
level=WARN cat=server msg="stopping" reason=tray-quit by=user uptime=2h38m sessions=1 transcodes=0 games=0 tasks="scan:Games" lastRequest=14:33:12
```

| `reason` | When |
|---|---|
| `tray-quit` | Quit in the tray icon (says which account's tray) |
| `service-stop` | The Windows service, systemd, launchd or rc.d was told to stop |
| `os-shutdown`, `os-logoff`, `os-sleep` | Windows `WM_QUERYENDSESSION`/`WM_ENDSESSION` and power broadcasts; systemd/launchd equivalents |
| `installer` | Windows Restart Manager asked (the app registers with it, so the request is recognisable), or the MSI's own stop step |
| `update` | The server's own updater restarting it |
| `restart-setting` | A setting that needs a restart |
| `signal` | SIGINT/SIGTERM (Ctrl+C, `kill`), with the sender's process id where the system says (Linux `SA_SIGINFO`) |
| `console-closed` | The console window was closed |
| `api` | An admin pressed Restart or Stop in Settings (with who) |
| `second-instance` | (with the single-instance work) a second start asked it to |
| `panic`, `out-of-memory`, `watchdog` | Faults: see section 3 |

**Starts say how the last run ended.** A small `run.json` in the data folder is written at start (`{pid, started, version}`) and marked `{ended, reason}` at a clean stop. At the next start:

- clean: `msg="started" previous="stopped 14:33 (tray-quit), ran 2h38m"`
- not clean: `level=ERROR msg="the last run ended without stopping cleanly" lastLog="14:33:41 ..." ` and a crash report, see below.

(The 0.6.0 `crash\run-*.json` and `pending-*.txt` files already do part of this. Fold them into one mechanism, and check what they record today.)

## 3. Faults leave evidence

- **Panics:** Go writes them to stderr, which a Windows tray-launched or service process loses. Send stderr to `crash\stderr-<run>.txt` from the first line of `main`, and keep the last 500 log lines in memory, written into the crash report on a panic.
- **Out of memory and hard kills** leave no panic. The `run.json` marker catches them at the next start ("ended without stopping cleanly"). On Windows, the report adds any `Application Error` or `Windows Error Reporting` event for `wittymediaserver.exe` around that time (read from the event log at the next start; read-only).
- **Local crash dumps (opt-in, off by default):** Windows Error Reporting's `LocalDumps` for `wittymediaserver.exe`, kept in `crash\dumps`, three at most. The page says what a dump holds (memory, which can include file names and settings) before it's turned on.
- **Module helpers** already log their stderr when they end. Add: a helper that crashed is listed in the next crash report with its last lines.

## 4. Someone watches the server

On Windows and macOS the **tray app becomes the server's supervisor** for desktop installs:

- If the server exits **without** a stop reason it asked for, the tray shows a notification ("WittyMediaServer stopped unexpectedly. It's starting again."), restarts it (at most three times in five minutes, then stops trying and says so), and the next log says `reason=restarted-by-tray after=...`.
- If an installer stopped it and then failed, the tray (which an MSI rollback leaves in place) notices the server isn't running after the installer exits, and offers **Start WittyMediaServer**.
- Services already have restart rules (`util:ServiceConfig` restart on failure); log those restarts too.

## 5. Easier to read in the app

**Settings → WittyLog** (or **Logs**) gets:

- **A timeline at the top:** starts, stops (with reasons), crashes, updates, resets, as a strip of the last 7 days. Clicking one jumps to that moment in the log.
- **Plain-words summaries** for the common events, above the raw line: "Stopped at 14:33 because Quit was chosen in the tray icon."
- **Filters** by category, level and time, and **search**; "show 5 minutes around this line".
- **"More detail for an hour"** per category (playback, discs, http), which turns itself off, so a person can reproduce a problem with debug logs without leaving them on.
- **A notice for admins** after an unclean stop, on Home: "WittyMediaServer stopped unexpectedly at 14:33. [What happened] [Make a support bundle]".

## 6. A support bundle that answers the question

- Include the logs from before a reset (they're in the `before-reset-*` copy), or name them.
- Include `run.json` history (the last 20 starts and stops with reasons).
- **On Windows**, the Application log's events from `MsiInstaller`, `Application Error`, `Windows Error Reporting` and `Microsoft-Windows-RestartManager` for the same days, filtered to WittyMediaServer. On Linux with systemd, `journalctl -u wittymediaserver` for the same days. Read-only, and sanitised like the rest.
- `crashes/` always present, with a `none.txt` when empty.
- Fix `log.since` (the time of the oldest log line included).
- The sanitise switch (0.6.0) applies to all of it.

## 7. Bugs to fix on the way

- **Create folders when writing, not only at start:** thumbnails, disc stills, transcodes and every other cache write call `MkdirAll` for their folder first (or the cache layer does it once, and again when the folder vanishes). Found here: `cache\images` missing after a reset broke every thumbnail until a restart.
- A reset that removes the cache must leave the server able to carry on without a restart.

---

## Where it goes

| Release | Work |
|---|---|
| **0.6.3-alpha** (small) | Stop reasons on every stop; `previous=` on every start; "ended without stopping cleanly" detection; the cache folder bug; bundle fixes (`crashes/` note, `log.since`, before-reset logs named) |
| **0.7.0-alpha** | Tray as supervisor; installer-rollback restart offer; panics and last lines in crash reports; Windows event log and journal in the bundle; the timeline and plain-words summaries in **Settings → WittyLog**; "more detail for an hour" |
| **0.8.0-alpha** (next major) | Opt-in crash dumps; the Home notice after unclean stops; logs reachable from the activity panel and notifications; the same views in WittyMediaClient for admins |

## Done when

Quitting from the tray, stopping the service, shutting Windows down, running an installer, killing the process and forcing a panic each leave a log line that says exactly that, and the next start says how the last run ended. A person who reads only **Settings → WittyLog** can tell which one happened.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
