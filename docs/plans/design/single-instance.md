# Design: one WittyMediaServer at a time

> **Status on 2026-11-04**
>
> **In progress:** [#92](https://github.com/wittywilla/WittyMediaServer/issues/92). **Done already:** WittyMediaClient and WittyReader's desktop apps bring the open window forward on a second start.

For 0.6.3-alpha. The owner: "Prevent the ability to open multiple instances of wittymediaserver (with a pop up asking if you want to start another instance on another port, or to just open the web UI, or exit)."

Not built. Not tested.

---

## What happens today

From `docs/TROUBLESHOOTING.md`: a second copy fails to bind port 9797, and the tray icon says **Stopped**, "address already in use". The fix offered is to stop the other copy or set another port. Nothing stops a second copy from opening the same data folder if it's given another port, and nothing tells a beginner that their server is in fact already running.

## Goals

1. A second start of the **same data folder** never runs a second server on it.
2. The person is asked, in words they understand, what they want.
3. Nothing ever blocks a start because of a lock left by a crash.
4. Services, Docker and scripts never get a dialog: a clear line and an exit code instead.
5. Light: no polling, no extra process, no network call except one local health check.

## Not goals

- Stopping two servers with **different data folders**: that's two servers on purpose, and allowed.
- Stopping a server on another machine on the network.

---

## How

### The lock

- On start, before the database opens, the server takes an exclusive OS lock on `<data>/instance.lock`:
  - Windows: `LockFileEx` with `LOCKFILE_EXCLUSIVE_LOCK | LOCKFILE_FAIL_IMMEDIATELY`, the handle kept open for the life of the process.
  - Linux, macOS, FreeBSD: `flock(LOCK_EX | LOCK_NB)`.
- An OS lock is released by the kernel when the process dies, however it dies. That's goal 3: no stale lock files, no PID guessing.
- Beside it, `<data>/instance.json` (written after the listener is up, replaced atomically): `{ "pid", "started", "version", "urls": [...], "instance": "<random id>" }`. No secrets in it: it's only how a second start finds the first.

### The second start

```
take lock ──ok──► normal start
    │
  busy
    │
read instance.json ──► GET <url>/api/health, expect the same "instance" id (1 s timeout)
    │                         │
    │                  answers: it's alive
    │                         │
    │            interactive? ──yes──► ask (below)
    │                         └─no──► print "WittyMediaServer is already running at <url> (pid N)." exit 3
    │
 no answer within 10 s (a server still starting, or hung)
    └──► "WittyMediaServer is starting or not answering (pid N). Try again in a moment." exit 3
```

`/api/health` gains an `instance` field (public, no secrets: a random id per run) so a second start can't be fooled by some other program on the port.

### The question

Asked by whichever part of the product is on screen: the tray app on Windows and macOS, the desktop launcher on Linux. Wording:

> **WittyMediaServer is already running**
> It's at http://localhost:9797 and has been running since 14:02.
>
> [ **Open WittyMediaServer** ] [ Start another server… ] [ Exit ]

- **Open WittyMediaServer** (default, focused): opens the running server's address in the default browser, then exits. If the running server is the installed service and this is the tray app, it attaches the tray to it instead of exiting.
- **Start another server…**: explains that a second server needs **its own data folder** (its own libraries, accounts and settings), and offers:
  - a new folder beside the current one, `WittyMediaServer-2`, or one the person chooses (typed path or "Browse folders"; never the native picker during tests);
  - the next free port from 9798 up, shown and editable.
  It then starts with `-data <folder> -listen :<port>`.
- **Exit**.

Keyboard: Enter opens, Escape exits, Tab moves. Screen readers get a real dialog with a title and description (native message boxes on each system already are).

### A decision for the owner

The owner's line says "start another instance on another port". Two servers on one data folder would share one SQLite database and one transcode folder: scans would fight, caches would be cleared under each other, and the database could be damaged. **This plan only offers another port with another data folder.** If the owner wants a second server on the *same* libraries (say, for testing), the safe version is a read-only second server, which is a bigger feature: it belongs in a later release, not 0.6.3.

### Other entry points

| Start | Behaviour |
|---|---|
| Double-click on the server program or tray app | The question |
| Windows service, systemd, launchd, rc.d | Log line and exit 3. The service manager's own restart policy must treat 3 as "don't retry": set `RestartPreventExitStatus=3` in the systemd unit, and the equivalent on Windows (no restart on that code) |
| Docker | Log line, exit 3. Two containers on one volume is the same fault, so the lock works there too (flock works on bind mounts on Linux; **to verify** on Docker Desktop's file sharing on Windows and macOS, where it may not, in which case `instance.json` plus the health check is the fallback) |
| `wms` commands that open the database (`wittymediaserver backup`, `wittymediaserver doctor`, repair tools) | Refuse while a server holds the lock, unless they're the read-only kind, with "Stop the server first, or use the same action in Settings" |
| Network shares as the data folder | Locks on SMB and NFS are unreliable. Refuse a data folder on a network share at start (the server already warns about this kind of path in places; **to verify**) or warn that single-instance can't be guaranteed there |

### The other apps

Same idea, smaller:

- **WittyMediaClient desktop** (Electron): `app.requestSingleInstanceLock()`, and a second launch focuses the existing window. **To verify** whether it already does.
- **WittyMediaPlayer**: a second launch with a file passes the file to the running player (Qt `QLocalServer`), unless the person chose "new window".
- **WittyReader desktop**: as the client.

---

## Tests

- Unit: lock taken, lock busy, lock released after `kill -9` (Linux) and `TerminateProcess` (Windows).
- Integration on Linux: two starts on one folder (second exits 3 with the line); two folders on two ports (both run); a fake program on 9797 that isn't us (health id mismatch: the second start says the port is taken by another program).
- Windows: the question appears (owner clicks through; no native windows during automated tests on the owner's machine).
- The service unit doesn't restart-loop on exit 3.

## Costs

One file handle for the life of the process. One local HTTP call on a second start. Nothing else.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
