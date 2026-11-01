# Repair

Repair finds and fixes problems with the server's own data and install. It's in *Settings → Repair*, for administrators,
and on the command line as `wittymediaserver doctor`, which works with the server stopped, when it won't start, and with nothing but the
program and its data folder. Both are the same code and find the same things.

**It looks first.** Nothing changes until you press a fix, or add `--fix`. Your media files are never changed or removed.

- [Using it in Settings](#using-it-in-settings)
- [Using it on the command line](#using-it-on-the-command-line)
- [What each word means](#what-each-word-means)
- [What it checks](#what-it-checks)
- [Moved a drive?](#moved-a-drive)
- [A damaged database](#a-damaged-database)
- [The safety copy](#the-safety-copy)
- [With the server running](#with-the-server-running)
- [For developers](#for-developers)

## Using it in Settings

Press **Check this server**. You can leave the page while it runs. Each thing it finds says what it found, what the fix does,
and how safe that is. **Fix everything that's safe** does what can't lose anything; anything that changes data asks for your
password and goes one at a time. Afterwards it looks again and shows what was done, and where the safety copy is.

To check a disc, or a drive's speed and health, use [WittyDiagnoser](DIAGNOSER.md), which only reads.

## Using it on the command line

```bash
wittymediaserver doctor                         # look, and say what's wrong. Changes nothing
wittymediaserver doctor --fix                   # fix what can be fixed. Asks before what changes data
wittymediaserver doctor --fix --safe-only       # only what can't lose anything
wittymediaserver doctor --fix --yes             # don't ask
wittymediaserver doctor --only data             # one area: data, metadata, modules or install
wittymediaserver doctor --only ffmpeg,port      # or particular checks. wittymediaserver doctor list shows them
wittymediaserver doctor --json                  # for a program
wittymediaserver doctor --try-port              # also try the port, listening on this machine for a moment
wittymediaserver doctor -data /path/to/folder   # a data folder other than the usual one
wittymediaserver doctor remap --from <old> --to <new> [--fix]
wittymediaserver doctor reinstall <module>      # the only thing here that goes online
```

It finds the folders the way the server does: `wittymediaserver config path` shows which. A `wms.json` with a mistake in it doesn't stop it, and
it says what the mistake is. It doesn't start a server, open a port or need FFmpeg.

| Exit code | Means |
|---|---|
| 0 | Nothing needs attention. Notes may still be listed |
| 1 | Something does |
| 2 | It couldn't run |

Run it as the same account that runs the server, so it can reach the same files. For a system service, that's `sudo -u wms wittymediaserver doctor`.

## What each word means

How much it matters, in the same words as [WittyDiagnoser](DIAGNOSER.md#reading-the-results): **Problem**, **Warning**, **Note**, **Fine**.

How safe the fix is:

| In Settings | On the command line | Means |
|---|---|---|
| **Safe** | safe | Nothing can be lost by it. A safety copy is made anyway |
| **Backed up first** | backed up first | It changes data, so it asks first, and a safety copy is made |
| **You'll need to do this yourself** | you'll need to do this yourself | Only a person can, and it gives the steps for this system |

## What it checks

**The database and data folder**

| Check | Finds | Its fix |
|---|---|---|
| `database` | Damage, using SQLite's own check; indexes out of step; a file that isn't a database | Rebuilds indexes, or [recovers what can be read](#a-damaged-database) |
| `database-links` | Rows pointing at things that no longer exist | Removes those rows |
| `database-size` | A large journal; a lot of empty space | Folds it in; compacts the file |
| `temp-files` | Temporary files left by something that stopped | Moves them to the safety copy. The folders of old streams and unpacked games are removed, as they're made again when needed. In a conversion folder you chose, anything that isn't a stream's folder is yours, and stays |
| `reset-state` | A reset or repair waiting from a while ago, an unreadable request, a reset that stopped halfway, copies taking up space | Takes the request back, or brings the copy back. It never deletes copies: *Settings → Reset* does that |

**Metadata**

| Check | Finds | Its fix |
|---|---|---|
| `library-folders` | A library's folder that can't be reached or is empty | Nothing is touched, in case the drive comes back. [If it moved, say where](#moved-a-drive) |
| `missing-files` | Titles whose files have gone, in a folder that's there | Hides them, as a scan would. They come back if the files do |
| `artwork-refs` | Titles pointing at artwork that has gone | Clears the reference, so a refresh fetches a new one. If *every* picture is missing, it's the folder that's away, and nothing changes |
| `artwork-orphans` | Artwork nothing uses | Moves it to the safety copy. Only pictures nothing else can reach: not `logos/`, and not web-address or small copies, which look unused and aren't. Anything under a day old is left, as its row may be on the way |
| `matches` | A match with no id, an id with no source, or an id that can't exist | Forgets the match, so it's looked up afresh. What you typed and locked stays |
| `duplicates` | The same place listed twice; a file in two libraries; two shows with one name | Merges only what is truly the same place, moving watch progress and episodes. The others are for you to decide |
| `nfo-files` | NFO and `.metadata.json` files that can't be read, with the file and line | You fix it. Nothing here writes beside your media. The [metadata guide](METADATA.md) has examples |

**Modules**

| Check | Finds | Its fix |
|---|---|---|
| `module-state` | A damaged list of what's installed | Moves it aside. Modules use their built-in copies until the next install |
| `module-versions` | A version with a failed signature, missing files, or not for this server | Goes back to the version before or the built-in copy, and moves its files to the safety copy. **Reinstall** also downloads it again, only if you choose that |
| `module-updates` | An update that stopped partway; installed files nothing uses | Moves them to the safety copy |
| `module-helpers` | Helpers still running with no server | Ends them. Only programs running from this data folder's modules |

**The install**

| Check | Finds | Its fix |
|---|---|---|
| `config-file` | A mistake in `wms.json`, or a path in it that isn't there | You fix it, with steps |
| `ffmpeg` | FFmpeg or ffprobe missing or not running | You install it, with steps for your system |
| `folders` | Folders missing, not folders, or that can't be written | Makes missing ones. It never changes permissions, and gives the steps |
| `disk-space` | Under 5 GB where the server keeps things | You make room, with suggestions |
| `port` | The port taken, and by what if the system says; a server of another version already on it | You choose, with steps |
| `firewall` | Nothing it can see: a note on what to allow if other devices can't connect | You do it, with steps for your system. Firewall settings are never changed |
| `leftover-processes` | FFmpeg still converting for a server that stopped | Ends them. Only an FFmpeg whose command line names a stream's folder in this server's conversion folder |
| `layout` | A data folder inside the program's folder, which a reinstall can remove | You move it, with steps |

## Moved a drive?

If a library's folder is now somewhere else, the server only needs to be told. Watch progress, matches, ratings and your edits all stay.

```bash
wittymediaserver doctor remap --from "D:\Movies" --to "E:\Movies"         # says what would change
wittymediaserver doctor remap --from "D:\Movies" --to "E:\Movies" --fix   # does it
```

Or *Settings → Repair → Moved a drive?*. Either way:

- It looks for the files in the new place first, up to 2,000 spread through the library. Unless at least 60% are there, it says so and stops: that's
  not the same files.
- Every path that starts with the old place changes, in every table, in one transaction: it all happens or none of it does. A path that only starts
  with the same letters doesn't change (`D:\Movies2` for `D:\Movies`), and the history of past edits stays as it was.
- A move from Windows to Linux or back works: the rest of the path takes the new system's separators.

If you've already added the new folder and scanned it, the titles are there twice, and this says so instead of changing anything. Take that folder out
of the library first.

## A damaged database

A power cut, a failing drive, or a copy taken while the server was running can damage the file. If that's what the check finds:

1. A new database is made by this version of the server, and every row that can still be read is copied into it, table by table. What can't be read, or
   isn't valid, is counted, and it says which tables were affected.
2. Rows pointing at something lost are left out, as a delete would have taken them.
3. The new file is checked. With the server stopped it takes over at once; from Settings, at the next start, which **Restart now** does for you.
4. The damaged file is kept in the safety copy, as it was.

If nothing can be read, nothing is replaced. If you have a recent backup, restoring it may save more than this does, so it's worth comparing first.

## The safety copy

Before the first fix of a run, the database is copied to `<data>/backups/before-repair-<date>-<time>/`: a clean copy when it's sound, and its files as they are when it's
damaged. If the copy can't be made, for lack of space for example, nothing is changed.

Whatever a fix would remove goes into that folder's `removed` folder, with its place in the data folder kept, instead of being deleted. A replaced database goes in `replaced`.
The one exception is converted video and unpacked game files, which can be many gigabytes and are never anyone's only copy of anything.

The newest five are kept, and older ones are removed by the next run. To undo a repair, stop the server, and copy `wms.db` back from the folder.

## With the server running

- **In Settings**, the server repairs itself. Anything needing the database file to itself waits for the next start, and the page offers a restart. This uses the same
  mechanism as [Reset](TROUBLESHOOTING.md): a request is written, and the next start carries it out before anything opens the database. If anything can't move, every file goes back.
- **On the command line**, `wittymediaserver doctor` can look while a server is running. `--fix` still works for what SQLite can do safely alongside it. It refuses what needs the server
  stopped, says so and points at Settings. It knows a server is running from the marker each run leaves in `<data>/crash/`, and checks that the process is alive.
- Helpers, FFmpeg and working files are only called leftovers when no server is running: with one, they're its own.

## For developers

The code is `internal/diagnose/doctor*.go`. A check is a `Check` with an id, an area, `Run` and optionally `Fix`. `Run` returns `Issue`s, and has to work with no server, on a database that may be damaged:
open it with `e.metaDB(...)` or `e.readDB()`, and if a read fails, say *not checked*, never *nothing wrong*. `Fix` moves files with `f.Remove` and writes through `withWrite`, in a transaction. Add a test with a fixture for each check.

The database is read with `db.OpenExisting`, which doesn't migrate or create anything. `db.RecoverInto` is the recovery. The next-start request is `reset.AskRepair` and `reset.ApplyRepair`, which `cmdServe` calls right after `reset.Apply`. What it knows of modules is in `internal/modules/doctor.go`, which reads `state.json` and version folders with no `Manager`.

The routes are `/api/repair/...` ([API](CLIENT-API.md)), and the page is `web/src/pages/settings/Repair.tsx`, sharing `FindingCard` with WittyDiagnoser.

---
<sub>WittyMediaServer documentation · [Back to the README](../README.md)</sub>
