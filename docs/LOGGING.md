# How every part of the project logs

WittyLog is the server's logging; people meet it in *Settings → WittyLog*.

For anyone adding code to the server or a client. What people see of this is in
[Config](CONFIG.md#logs) and [WittyDiagnoser](DIAGNOSER.md).

The aim: **if something goes wrong for someone, the log shows it, tied to what they were doing,
without showing their secrets, and without costing anything when all is well.**

- [The short version](#the-short-version)
- [Categories and levels](#categories-and-levels)
- [Sessions: tying things together](#sessions-tying-things-together)
- [Errors that vanish](#errors-that-vanish)
- [Goroutines and panics](#goroutines-and-panics)
- [Child processes](#child-processes)
- [Secrets, titles and paths](#secrets-titles-and-paths)
- [Modules: their own Go modules](#modules-their-own-go-modules)
- [Module logging: levels and options per module](#module-logging-levels-and-options-per-module)
- [Clients](#clients)
- [Crash reports and the startup summary](#crash-reports-and-the-startup-summary)
- [What logging promises](#what-logging-promises)
- [Plain-language explanations](#plain-language-explanations)
- [Checklist for merged code](#checklist-for-merged-code)

## The short version

```go
log := s.Log.With("cat", "transcode", "session", streamID) // once, when the work starts

log.Info("transcode started", "height", 1080)              // the message never changes...
log.Warn("hardware encoder failed", "encoder", enc, "err", err) // ...the details are attributes

// In a request handler: pass the context, so the record carries the request's ID.
s.Log.ErrorContext(r.Context(), "could not save progress", "cat", "playback", "item", id, "err", err)

logging.Go("epg refresh", func() { ... }, "cat", "tv") // not a bare `go func()`

var stderr logging.Tail                                // not bytes.Buffer
cmd.Stderr = &stderr
err := cmd.Run()
logging.ChildExit(log, "ffmpeg", err, stopped, &stderr, started)
```

## Categories and levels

Every message belongs to a category. Pass `"cat", id` as an attribute, or `.With("cat", id)` when the
work starts. No category means `server`.

| Category | For |
|---|---|
| `playback` | what is played and how: direct play, codecs, tracks, sessions opening and closing. Names titles |
| `transcode` | FFmpeg: command lines, its stderr, how it ended, hardware encoders |
| `discs` | the DVD and Blu-ray engines: navigation, menus, BD-J; WittyDiagnoser on discs |
| `games` | runners, cores, controllers, the game stream |
| `tv` | tuners, channels, the guide, recordings |
| `library` | scans, metadata lookups, matching, photos and books |
| `modules` | helpers starting and stopping, module updates and installs |
| `update` | server updates |
| `remote` | HTTPS, port mapping, discovery, reachability |
| `auth` | sign-ins, lock-outs, sessions, pairing, two-factor |
| `clients` | what clients report. Only `POST /api/logs/client` writes this |
| `http` | web requests. Only the `s.logging` middleware writes this |
| `storage` | folders and disks: free space, permissions; WittyDiagnoser on storage |
| `server` | starting, stopping, the database, settings, anything else |

The owner sets each category's level in *Settings → WittyLog*, live. They start at `warn` for the ones
that name what people watch or who signs in (`playback`, `discs`, `auth`, `clients`, `http`), `info` for the
rest. So choose the level by what the message is:

| Level | Means | Example |
|---|---|---|
| Error | it failed and someone will notice, or data is at risk | a transcode that won't start; the database can't be written |
| Warn | it failed, but the server coped; or it will fail soon | a hardware encoder fell back to software; a disk is nearly full |
| Info | a milestone: something started or finished. **A few per action, no file names in `library`** | scan finished, 3 errors; game session started |
| Debug | each step, for chasing a problem | every file a scan looked at; every cell the disc navigator played |

A loop that runs per file, per segment, per frame or per button logs at Debug. **Call `log.Debug` freely:** when
the category is quiet it returns before formatting anything (see the numbers below).

Messages are fixed text with details in attributes, so they group in the viewer and the flood limit can count
them: `log.Warn("could not read file", "file", p, "err", err)`, not `fmt.Sprintf`. Use `err` as the key for an
error, `stderr` for what a process said, `stack` for a stack: the viewer and the explanations look for these.

## Sessions: tying things together

A session ID is what lets someone jump from an error to *everything about that playback*: the request that
started it, FFmpeg's output, and what the player said in the browser.

| Work | Session ID | Set where |
|---|---|---|
| A web request | `r-…`, automatic | `s.logging` middleware. Log with `…Context(r.Context(), …)` to carry it |
| HLS playback | the stream ID | `hls.Session.log` |
| A disc | the `sid` from `POST /items/{id}/dvd` | `dvdsession`, `bdsession`; their HLS streams use `Spec.LogSession` |
| A game | the game session ID | `stream.Session.log` |
| TV | `tv-…` for a stream, `rec-…` for a recording | `livetv` |
| A scan | `scan-…` | `library.Scanner` |
| A module install or update | `mod-…`, `update-…` | `modules`, `update` |
| A WittyDiagnoser job | the job's ID | `diagnose` |

Add it with `.With("session", id)` when the work starts, or put it in a context with `logging.WithSession`. New
long-running work gets its own: `id := logging.NewID("scan")`. The IDs are not secrets: every route using one
also needs a sign-in. Never use a token as a session ID.

Requests are tied to the media session automatically when the address has it in it (`sessionFromPath` in
`internal/server/server.go`). Add new media routes there.

**Tell the client its session.** Whatever a playback route returns, the ID in it is what the client reports its
errors under. See [Clients](#clients).

## Errors that vanish

`_ = thing()` is fine when failure truly doesn't matter, and nothing is worse off:

- closing something you only read from; removing a temp file; a write to a client who has gone (`writeJSON`).

It is not fine for: saving anything, starting anything, a child process exit, a scan step, a network call
something depends on, or any `SetSetting`. Log those. Add a short comment when the reason to ignore isn't obvious.

Don't log what you return: whoever receives the error decides whether it's a problem. Log at the point where the
error stops travelling. A handler that answers 5xx is logged by the middleware with the body of `writeErr`, so
just answering is enough, unless the detail you have (a path, a stderr) isn't in the response.

Database errors are logged by the driver wrapper in `internal/db`, with the statement (never its values), so
nothing needs adding at call sites for that. Context cancellations and "no rows" are left out.

## Goroutines and panics

A panic in a goroutine takes the whole server with it. So:

```go
logging.Go("name", fn, "cat", "tv")             // recovers, logs with a stack, writes a report; fn ends
logging.Loop("hls sweep", m.sweep, "cat", "transcode") // the same, and starts fn again after a while
defer logging.Recover("runner reader", "session", id)  // for a goroutine you start yourself
```

Use `Loop` for anything that must keep running. Don't recover where carrying on would be unsafe: a transaction
half done, or a lock held across the panic (use `defer` for locks). HTTP handlers are already covered by the
middleware.

## Child processes

Every process the server starts leaves a record of how it ended, tied to its session:

- **`logging.Tail`** as `cmd.Stderr`: it keeps the first 2 KB and the last 14 KB, whatever the process prints.
  Never `bytes.Buffer` (a file can make FFmpeg print without end) and never let stderr go nowhere.
- **`logging.ChildExit(log, "ffmpeg", err, intended, &tail, started)`** after `Wait`. `intended` is true when the
  server stopped it on purpose: that's a Debug line, and anything else a Warn with the reason in words
  (`logging.ExitReason`: *killed, perhaps because memory ran out*; *a DLL it needs is missing*).
- Log the command line at Debug with `"args", strings.Join(args, " ")`. Addresses and headers in it are redacted.

## Secrets, titles and paths

**Everything is redacted centrally** (`internal/logging/redact.go`), so a mistake at a call site is caught:
attribute names like `token`, `password`, `*key`, `cookie`, `setupCode`; patterns in free text like `?token=`,
`st=`, `Bearer …`, `Authorization: …`, `"password":"…"`, `user:pass@`, share links, long hex; and the exact values
of secrets the server holds (`logging.AddSecret`).

Still, **never log**: passwords, tokens, API keys, cookies, recovery or setup codes, or **file contents**. If a
secret has a new shape, add a pattern and a case to `TestRedactPlantedSecrets`. Anything printed for a person on
purpose (the setup code) goes to the console directly, not the logger.

Attacker-controlled text (file names, request paths, what a client reports, what a program prints) is safe to log
as an attribute: values are quoted and escaped, so it can't start a new line or pass for a field, and control
characters and text-direction tricks are shown as escapes. The viewer shows it as text only.

**Titles and paths are fine in local logs**, where they're needed. They come out of *shared* bundles if the owner
asks, by attribute name. So put them in attributes with a name that says what they are: `title`, `game`, `show`,
`file`, `path`, `dir`, `url`. Never put a title in the message text.

## Modules: their own Go modules

`modules/*` can't import `internal/logging`: they are separate projects. They take a `*slog.Logger` (or a callback)
from the host, and the same conventions work through it: `"cat"` and `"session"` are ordinary attributes.

```go
cfg.Log = log.With("cat", "games")                 // the host
s.log = cfg.Log.With("session", id, "game", title) // the module
```

In a module use `defer func() { if p := recover(); p != nil { log.Error(..., "stack", string(debug.Stack())) } }()`
for goroutines that might meet bad input.

## Module logging: levels and options per module

Every module has its own controls in *Settings → WittyLog*: a level (or *same as its area*) and advanced options
called **topics**, each a kind of detail that can be turned on by itself. Nothing needs registering: the server
compares the modules it has with the ones it has seen, every 30 seconds and whenever someone opens the page. A new
one gets a line in the log, a **New** badge and its controls; a changed version and a module that went away are logged too.

To be judged by a module's settings, a line carries the attributes `module` and, for optional detail, `topic`:

```go
log := cfg.Log.With("cat", "discs", "module", "wittydvd")
log.Debug("cell played", "topic", "nav", "pgc", n) // shown if the nav option is on, or the module is at Debug
log.Warn("menu unreadable", "err", err)            // judged by the module's level, or else the area's
```

A topic that's on lets its lines through whatever the level, and turning a module to `off` silences everything but
crashes. When no module has settings of its own this costs nothing more than an ordinary quiet line.

Where the list of topics comes from:

- **Any helper module** gets `calls`, `output` and `lifecycle`, done by the server around the helper.
- **The built-in engines** are listed in `engineTopics` in `internal/server/api_logmodules.go`.
- **Any other module** declares its own in its manifest, under `capabilities.logTopics`, each as
  `"id|Label|What it adds"`. See [Modules](MODULES.md). A helper is told what's on through the environment, as
  `WITTYMODULE_LOG` and `WITTYMODULE_LOG_TOPICS`, the next time it starts, and prints what it likes to stderr.

## Clients

Clients report to `POST /api/logs/client` ([CLIENT-API](CLIENT-API.md)), signed in, and only that. It's limited by
size (64 KB) and by rate (60 events a minute per account, 600 overall); past that it answers 429. Batch, and group
repeats with `count`.

| Field | |
|---|---|
| `app`, `version`, `device` | what is reporting |
| `events[].level` | `error`, `warn`, `info` |
| `events[].kind` | `js`, `request`, `player`, `game`, `crash`, `network` |
| `events[].message`, `stack`, `page` | what happened. No tokens, no file contents |
| `events[].session` | **the stream, disc or game session in play** |
| `events[].request` | the `X-Request-Id` of a failed request |

- **The web app:** `reportClient(...)` and `setLogSession(id)` in `web/src/errorReport.ts`. Failed API requests,
  uncaught errors and React errors are already captured. A player calls `setLogSession` when a stream starts and
  reports its fatal errors.
- **Desktop app:** `clients/wittymediaclient/desktop/clientlog.js`, which also keeps a local log file. It reports only
  what its main process sees; the server's page reports its own errors.
- **Android and Apple:** `ClientLog` in the app (`diag/ClientLog.kt`, `WittyKit/ClientLog.swift`), also with a local
  file. Set its `session` when a stream starts and clear it when the stream stops.
- What each client's log must hide is tested from one list: `clients/wittymediaclient/shared/data/redaction-cases.json`.

A client must never let reporting cause more errors: no retries in a loop, and a report that fails is dropped.

## Crash reports and the startup summary

Handled in one place, nothing to add at call sites:

- **`logging.StartCrashGuard`** (in `cmdServe`): a fatal Go error is written by the runtime to a file, and the next
  start turns it into a report. A start that finds the last run left without saying so writes an "unclean" report.
  Recovered panics write one too. They're in `<data>/crash/`, listed in the settings page and the bundle.
- **`Server.LogStartup`** writes version, modules, system, GPU and encoders, disk space and settings at each start.
  Settings appear only if listed in `summarySettings`, so a new secret can't slip in. Add any setting that changes
  how the server behaves to that list.

## What logging promises

- **It never blocks a caller.** Records go in a ring in memory and a bounded queue; one goroutine writes the file.
  When the disk is slow, the queue drops with a count, and the log says how many.
- **It can't take the server down.** A full disk or an unwritable folder means the log stays in memory and on the
  console, says so on the settings page, and tries again every 30 seconds.
- **It can't fill the disk.** Files are 20 MB at most, 200 MB and 14 days in all. With under 100 MB free, only
  warnings and errors are written and old logs are removed.
- **A flood can't bury it.** One message repeating is written 40 times in 10 seconds, then counted.

What it costs, measured with `go test -bench` on a 14-core laptop that was busy with other builds (a small box
will be several times slower, but the proportions hold):

| | Time | Memory |
|---|---|---|
| A `Debug` line in a quiet category | about 40 ns | none |
| The same, while some other module has settings of its own | about 60 ns | none |
| A line that is written | about 3.3 µs | 350 bytes |
| A web request, for its ID, status and panic catching | about 0.8 µs | 512 bytes |

Direct play still sends files with the system's own copy: `statusWriter` passes `ReadFrom` through.

## Plain-language explanations

`internal/logging/explain.go` turns common failures into a sentence and a suggested fix, shown in the viewer.
When you add something that can fail in a way a person can fix, add a rule (the first match wins, so specific ones
go first), and a case in `TestExplain`. Say what to do, not what the code did.

## Checklist for merged code

For the coordinating session, for code merged after this landed:

1. `go func()` → `logging.Go`, `logging.Loop`, or a `defer logging.Recover` at its top.
2. `cmd.Stderr = &bytes.Buffer{}` or no stderr → `logging.Tail`, with `logging.ChildExit` after `Wait`.
3. New long-running work: an ID with `logging.NewID`, and `.With("cat", …, "session", id)`.
4. `log.Info` in a per-file or per-frame loop → `log.Debug`.
5. `_ = s.DB.SetSetting(...)`, `_ = cmd.Start()` and the like → logged.
6. A new media route → add its prefix to `sessionFromPath`.
7. A new secret-shaped value → a redaction pattern, or `logging.AddSecret`, and a test case.
8. A new setting that changes behaviour → `summarySettings`, if it isn't secret.
9. A new module or engine: `"module", id` on its logger; optional detail with `"topic", id`, listed in `engineTopics`
   or its manifest.
