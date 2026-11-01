# Background tasks

Everything the server does in the background goes through one registry, `internal/tasks`. The activity
button in the top bar reads it, and so can WittyStats, the native apps and anything else with the API below.

- [Reporting a task from the server](#reporting-a-task-from-the-server)
- [Reporting a task from a module](#reporting-a-task-from-a-module)
- [Who sees what](#who-sees-what)
- [The API](#the-api)
- [What it costs](#what-it-costs)

## Reporting a task from the server

Every server job gets the registry as `s.Tasks` (or a `Tasks` field on its service). A nil registry and a nil
handle both do nothing, so a job and its tests never have to check.

```go
ctx, cancel := context.WithCancel(context.Background())
t := s.Tasks.Start(tasks.Spec{
    Type:     tasks.TypeBackup,      // a Type* constant, or a short name of your own
    Title:    "Backing up",          // no file paths: non-admins may read it
    Unit:     "files",               // what Done and Total count
    By:       byUser(userFrom(r)),   // who asked; leave it out for the server's own work
    Cancel:   cancel,                // offers Cancel
    Pausable: true,                  // offers Pause and Resume; you must call Wait
    RetryKey: "all",                 // offers Retry, with OnRetry below
})
logging.Go("backup", func() {
    var err error
    defer func() { t.Finish(err) }()         // nil: done. context.Canceled: cancelled. Else: failed.
    for i, f := range files {
        if err = t.Wait(ctx); err != nil {   // holds here while paused; ctx's error once cancelled
            return
        }
        t.Progress(int64(i), int64(len(files)))
        t.Item(filepath.Base(f), f)          // name and path: only admins see them
        if err := copyFile(f); err != nil {
            t.Problem(f, "This file couldn't be copied. Check that it can still be read.")
        }
    }
    t.Result(fmt.Sprintf("Backed up %d files.", len(files)))
}, "cat", "server")
```

| Call | What it does |
|---|---|
| `Start(spec)`, `Queue(spec)` | A task running now, or one waiting its turn, which `Begin()` then starts |
| `Phase(s)` | Says which stage it's at, such as "Finding files" |
| `Progress(done, total)`, `Add(n)` | How far along. A total of 0 means nobody knows yet, and the app shows it as busy. The speed and time left come from these |
| `Item(name, path)` | What it's working on now |
| `Problem(item, message)`, `ProblemN` | One thing went wrong and the task carries on. The first 50 are kept and the count goes on. Write the message for a person: what happened and what to do |
| `Wait(ctx)`, `PauseWanted()` | `Wait` holds while paused, and returns `ctx`'s error, so it's also the check for cancelling. Use `PauseWanted` to let go of something first, as a scan does with its turn at the disk |
| `Result(s)` | What came of it, in a sentence, for the history |
| `Finish(err)` | Ends it. A failure is put in words by `tasks.Explain`, and the error as it came is kept for admins |
| `Fail(message, fix, cause)` | Ends it as failed, in your own words. Better than `Finish` whenever you know what went wrong |

Calling these often is fine: see [what it costs](#what-it-costs). Report as you go, and don't add timers of your own.

**Spec options**

| Field | Use it for |
|---|---|
| `LibraryID`, `LibraryName` | Work on one library. It's listed under that library, and hidden from anyone who can't see it |
| `Private` | One person's activity, such as their transcode. Only they and admins see it |
| `AdminOnly` | Never shown to anyone else, whatever the settings say |
| `NoHistory` | Left out of the history. For things that come and go constantly |
| `Quiet` | Dropped when it ends having done nothing, with nothing wrong: a service that woke up and found no work |

**Retry.** Say once how to start a kind of task again. It works from the history too, even after a restart, so it
can only use what's in `RetryKey`:

```go
s.Tasks.OnRetry(tasks.TypeBackup, func(old tasks.Task, by tasks.By) error {
    return s.startBackup(old.RetryKey, by)
})
```

**After a restart.** A task that was going when the server stopped shows in the history as failed, saying so.

**Kinds of task.** `scan`, `analysis`, `metadata`, `thumbnails`, `intros`, `games`, `module`, `backup`, `transcode`, `convert` (library transcoding), `tdarr`,
`update`, `guide`, `recording`, `diagnose`, `other`. A new kind needs nothing but its name.

## Reporting a task from a module

A helper calls `wittymodule.ReportTask` (module API 3), and the task shows beside the server's own:

```go
wittymodule.ReportTask(wittymodule.TaskReport{Key: "reindex", Title: "Reindexing", State: wittymodule.TaskRunning, Done: 3, Total: 10, Item: "a file"})
```

A helper may be someone's fork, so the server believes it only about itself:

- Every report goes through `TaskReport.Validate`. A task's name is `<module id>/<key>`, with the id from the
  manifest and never from what the helper says, so a module can't touch another's tasks.
- Each module can have 16 tasks going, and each task's reports apply at most 4 times a second, keeping the latest.
- Module tasks are for admins only, and offer no Pause or Cancel.
- When the process stops, whatever it left going ends: failed if the process failed, and otherwise cancelled.

Most helpers are stopped after a minute idle and report only during a call. Long work needs a process that stays
running and never streams (`keep` and `background` on its `proc`). Don't start a process just to report.

## Who sees what

Two settings, in **Settings, Activity**, and each person's own choice:

| Setting | Values | Default |
|---|---|---|
| `activityEnabled` | `1` on, `0` off for everyone. The owner's switch | `1` |
| `activityViewers` | Who gets the read-only view besides admins: `admins`, `adults`, `everyone` | `admins` |
| `activityKeepCount`, `activityKeepDays` | How much history is kept, at most | `500`, `30` |
| `activityHidden` in `/me/ui` | This person has hidden the button for themselves | not set |

The server enforces these, and a client only follows `access` in the answer.

| | Admin | Anyone else allowed to see it |
|---|---|---|
| Tasks | All | Those on libraries they can see, their own private ones, and general ones. Never `AdminOnly` |
| File names and paths (`item`, `path`) | Yes | **No** |
| Who started it (`by`, `byId`) | Yes | Only if it was them |
| The words of an error (`detail`, `fix`, `problems`, `result`) | Yes | **No**: these often hold paths. `error` becomes a general sentence, and the number of problems stays |
| Pause, Resume, Cancel, Retry, clearing the history | Yes | **No** |

## The API

Paths are relative to `/api/v1`. All of it needs a signed-in account. A task's fields are the `Task` schema in
[openapi.json](openapi.json).

| Route | Who | What |
|---|---|---|
| `GET /system/tasks` | user | `access`, `tasks` (going on now), `recent` (finished lately) and `summary`. If the account may not see activity, the lists are empty and `access.visible` is false. It's always safe to ask |
| `GET /system/tasks/stream` | user | Server-sent events. See below. 403 if the account may not see activity |
| `GET /system/tasks/history` | user | Finished tasks, newest first. Query: `limit` (at most 200), `before` (the last row's `finishedAt`), `type`, `state`, `library` |
| `DELETE /system/tasks/history`, `DELETE /system/tasks/history/{id}` | admin | Forget the history, or one task of it |
| `POST /system/tasks/{id}/pause`, `/resume`, `/cancel` | admin | 404 if it has finished, 409 if this task can't do that. Offered by `canPause`, `canResume`, `canCancel` |
| `POST /system/tasks/{id}/retry` | admin | Starts a failed or cancelled task again, under a new id. Offered by `canRetry` |

**States.** `queued`, `running`, `paused`, then one of `done`, `failed` or `cancelled`. Pausing and cancelling take effect
when the job next checks, usually within a moment, so wait for the state to change rather than assuming it. Between asking and
stopping, a task is `running` with `canResume` true: show that as pausing. A cancelled
library scan can be retried, and it picks up where it stopped because files it has already seen, unchanged, take almost
no time.

**The stream.** Send the token in a header. `EventSource` can't, so a browser or an app reads the response body as it arrives:

```
GET /api/v1/system/tasks/stream
Authorization: Bearer <token>

event: snapshot
data: {"access":{...},"tasks":[...],"recent":[...],"summary":{"active":1,"queued":0,"paused":0,"percent":25}}

event: update
data: {"tasks":[{"id":"...","state":"running","done":10,"total":40,...}],"gone":["..."],"summary":{...}}

: still here
```

- `snapshot` comes first and holds everything. Each `update` holds the tasks that changed, whole, including ones that just
  finished. Replace by `id`. Drop any listed in `gone`, and move finished ones from your current list to your history.
- Updates come at most twice a second, and not at all when nothing is changing. A comment line every 25 seconds keeps
  the connection open, so treat 60 seconds of silence as a lost connection.
- On losing it, connect again with a growing wait. The new `snapshot` replaces all you had, so nothing needs replaying.
- **Close it when you aren't showing it.** A stream is one of the few connections a browser allows per server, and the
  web app closes its own whenever its tab is hidden. Signing out, losing access or the owner turning this off ends it.
- At most 16 streams per account (429 past that).

## What it costs

With nobody watching, reporting takes a short lock and sets a few fields: no timer, no goroutine and no copying
(`BenchmarkProgressNobodyWatching`). With someone watching, one goroutine sends what changed at most twice a second,
however often jobs report, and a slow watcher gets the latest state rather than a backlog. History rows are written by a
goroutine that exists only while there's something to write, because a job may be holding the database when it reports.
