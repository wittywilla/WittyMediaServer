# The data folder

WittyMediaServer keeps everything it makes in one folder: your library's details, accounts, settings, artwork, modules and logs. Your films, music, photos and other media are never in it; they stay where they are.

This page says where that folder is, what each part of it is for, and how to move it.

## Where it is

On a computer you use (the Windows installer and tray app, the Mac app, a Linux desktop), first-time setup asks where to keep it, on the page after you make your account:

| Choice | Windows | macOS | Linux |
|---|---|---|---|
| **Documents** (the default) | `Documents\WittyMediaServer` | `~/Documents/WittyMediaServer` | `~/Documents/WittyMediaServer`, or the Documents folder your desktop names |
| **The install folder** | `data` beside `WittyMediaServer.exe` | Not offered: an app can't change what's inside it | `data` beside the program, for a `.tar.gz` you unpacked |
| **Somewhere else** | Any folder you type, or choose with **Browse folders** | | |

The install folder is offered only when the server's account can write there. A copy installed for everyone, in `C:\Program Files`, usually can't, and setup says so. Install for your account only, or choose Documents. A `.deb`, `.rpm`, AppImage, Flatpak or Snap never offers it.

**Synced folders.** When a sync program keeps Documents in the cloud (OneDrive's backup of Documents, iCloud Drive's Desktop & Documents, Dropbox, Google Drive, Nextcloud and others), setup warns you and offers a folder on the computer instead. The server's database is open the whole time it runs, and a sync program copying it, or another computer changing it, can damage it. You can still choose the synced folder, but it isn't a good idea.

Setup checks each choice before it's made: that the folder is empty or new, that the server can write there, and that there's room. Choosing a place moves what the server has made so far (only a few megabytes), while it restarts, and setup carries on.

**Before setup asks, and for older installs,** the folder is the computer's usual place for an app's data:

| How it runs | Data folder |
|---|---|
| Windows, the app with the tray icon | `%LOCALAPPDATA%\WittyMediaServer` |
| macOS, the menu-bar app | `~/Library/Application Support/WittyMediaServer` |
| Linux or BSD, in your account | `~/.local/share/wittymediaserver`, or under `$XDG_DATA_HOME` |

An install from before 0.8.1-alpha keeps using that folder after the update. Nothing moves unless you move it.

**These stay as they always were,** and setup doesn't ask: the Windows service (`C:\ProgramData\WittyMediaServer`), the Linux packages' service (`/var/lib/wittymediaserver`), FreeBSD's (`/var/db/wittymediaserver`), the Mac's background service, Docker (`/config`), and the Snap. Neither does a server started with `-data`, `WMS_DATA_DIR` or a `dataDir` in [wms.json](CONFIG.md): the folder you named is the one it uses.

### How the server finds it

The choice is kept in a small file, `data-location.json`, in the folder for your account's settings:

| System | The file |
|---|---|
| Windows | `%APPDATA%\WittyMediaServer\data-location.json` |
| macOS | `~/Library/Application Support/WittyMediaServer/data-location.json` |
| Linux or BSD | `~/.config/wittymediaserver/data-location.json` |

It holds the folder's full path and nothing else of note. The server, the tray or menu-bar app, the installer and the uninstaller all read it. When the server starts, the first of these that's set decides the folder:

1. `-data` on the command line
2. the `WMS_DATA_DIR` environment variable
3. `dataDir` in a [config file](CONFIG.md), or a `data` folder beside a portable copy's `wms.json`
4. a container's `/config`, or a package's service folder when run as root
5. `data-location.json`
6. a `data` folder with a database in it beside the program (the install folder, on a copy carried to another computer without its `data-location.json`)
7. the computer's usual place, above

**If the folder isn't there,** for example on a drive that isn't plugged in, the server doesn't start, and says so: plug the drive in and start it again. It never makes a new, empty server in its place. To start again with no data instead, delete `data-location.json`.

## What's in it

The server writes a `README.txt` at the top of the folder that says the same as this, in short.

**Settings and your library.** Back these up; Settings → Backup does it for you ([Backups](BACKUP.md)).

| Name | What it is |
|---|---|
| `wms.db`, `wms.db-wal`, `wms.db-shm` | The database: libraries, accounts, watch history and settings. The three go together |
| `settings.ini` | The port, as you or Settings → Network set it. A plain file you can change by hand |
| `ui_config.json`, `ui_config.history` | The layout of pages and menus, and earlier ones |
| `certs` | HTTPS certificates, for Remote Access |

**Things you made or added.** Keep these.

| Name | What it is |
|---|---|
| `branding` | Your server's logo |
| `themes` | Themes you added ([Themes](THEMES.md)) |
| `bumpers`, `channel-logos` | WittyTV's bumpers and channel logos |
| `lyrics-corrections` | Lyrics you corrected |
| `metadata` | Text files for fixing titles by hand ([The metadata folder](METADATA-FOLDER.md)) |
| `recordings` | Recorded TV, when it's kept here |
| `backups` | Backups the server made |

**Modules.** `modules` holds the modules you added, and what each one keeps for itself.

**Logs and reports.** Safe to delete, but useful when something goes wrong: `logs`, `crash` and `diagnostics` ([Logs](LOGGING.md)).

**Safe to delete while the server is stopped.** The server makes these again when it needs them.

| Name | What it is |
|---|---|
| `cache` | Artwork, subtitles, lyrics and lists downloaded or made. They're fetched or made again, which takes a while for a big library |
| `transcode` | Temporary video for players that need a file converted. Emptied as it goes |
| `separator` | Songs split into parts for karaoke, and the separator program. Slow to make again |

**The server's own bookkeeping.** Leave these alone: `instance.lock` and `instance.json` (the running server), `setup-code.txt`, `update-withdrawn.json`, and the `*-request.json` and `*-result.json` files of a reset, restore, repair or move that's waiting or done. `before-reset-*` folders are copies kept by a reset ([Repair](REPAIR.md)).

[Uninstalling](UNINSTALL.md#what-goes-and-what-stays) removes only the names on this page, so anything else you keep in the folder stays.

### Keeping big or temporary files elsewhere

The cache and temporary video can live on another drive, a bigger or faster one, while the rest stays put. Set `cacheDir`, `metadataDir` (artwork) or `transcodeDir` in [wms.json](CONFIG.md#folders-the-server-uses), or `WMS_CACHE_DIR`, `WMS_METADATA_DIR` and `WMS_TRANSCODE_DIR`. Settings → Data folder lists them under **Kept apart**, and a move leaves them where they are.

## Moving it

**Settings → Data folder** shows where the folder is now and how big it is, and moves it:

1. Choose **Documents**, **The install folder** or **Somewhere else**. The page checks the place as you choose: room, whether the server can write there, and whether it's synced.
2. Press **Move data here**, and confirm. The server stops what it's doing (anyone watching is interrupted) and starts again.
3. While it starts, it copies everything except temporary video, then checks the copy: every file is there at the same size, and the database and settings are the same byte for byte. The page shows how far it is.
4. Only then does it switch: `data-location.json` names the new folder, and the server opens it.
5. The old copy stays, with a `MOVED.txt` saying where everything went. Once you're happy, **Delete old copy** removes it. That asks for your password, and deletes only what the move copied: anything else you kept in that folder stays.

If anything goes wrong before the switch (no room, a file that can't be read, a copy that doesn't match), the half-made copy is cleared, nothing changes, and the page says why. If the computer turns off in the middle, the next start finds the move unfinished and does it again from the beginning.

The page can't move a folder set by `-data`, `WMS_DATA_DIR`, `wms.json` or a service, and says where it was set instead. To move one of those by hand: stop the server, copy the folder, change the setting, and start it again.

In first-time setup there's nothing worth keeping in the old folder yet, so it's cleared once the copy is checked.

## Copying it to another computer

Stop the server and copy the whole folder. On the other computer, start the server with `-data` and the folder's path. Setup and Settings → Data folder only move data into an empty folder, so they won't take over a folder that already holds a server's data. A copy kept in the install folder needs nothing: a server started from that folder finds its `data` folder by itself.

---

<sub>WittyMediaServer documentation</sub>
