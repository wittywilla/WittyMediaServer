# Backup and restore

Keep a copy of your server's data and metadata in one ZIP file, and bring it back after a reset, a failed drive or a move to another machine. Your media files are never in a backup: they stay where they are.

Backups are for administrators. They have not been run on a real Mac or on FreeBSD yet.

- [Making a backup](#making-a-backup)
- [What's in a backup](#whats-in-a-backup)
- [What isn't in a backup](#what-isnt-in-a-backup)
- [The password](#the-password)
- [Backing up on a schedule](#backing-up-on-a-schedule)
- [Restoring a backup](#restoring-a-backup)
- [Undoing a restore](#undoing-a-restore)
- [Restoring during first-time setup](#restoring-during-first-time-setup)
- [What happens to sign-ins, keys and modules](#what-happens-to-sign-ins-keys-and-modules)
- [Moving to another machine or drive](#moving-to-another-machine-or-drive)
- [Why a backup might be refused](#why-a-backup-might-be-refused)
- [From the command line](#from-the-command-line)
- [For developers](#for-developers)

## Making a backup

1. Open **Settings → Backup**, on the **Back up** tab.
2. Under **What to include**, tick what you want. The page says about how large each part is.
3. Leave **Protect with a password** on, and type a password of at least 8 characters.
4. Press **Make a backup now**.

It runs in the background, so you can leave the page. It also shows on the [activity button](TASKS.md), and **Stop** ends it. Nobody has to stop watching: the database is copied at one moment in time while the server keeps running.

The finished backup is listed under **Backups on this server**, in the `backups` folder inside the data folder. **Download** it to keep a copy somewhere else, because a copy on the same drive doesn't help if that drive fails. **Download** and **Delete** each ask for your own password, since a backup holds your accounts.

## What's in a backup

| Part | What it covers | Normally |
|---|---|---|
| **Libraries and metadata** | Your libraries and everything known about what's in them: titles, matches, edits, locks and history | Included |
| **Settings** | Everything under Settings, your server's name and logo, and your own themes | Included |
| **Accounts and profiles** | Everyone's name, profile and password, kept only as a hash | Included |
| **Watch history and personal data** | Progress, what's been watched, favourites, ratings, playlists, collections, notes and share links | Included |
| **Channels and TV** | WittyTV channels, tuners, the guide, scheduled recordings, bumpers and channel logos | Included |
| **Artwork** | Posters, backdrops and logos the server saved. They can be fetched again, so leave this out if space is tight | Included |
| **Game saves** | Saves, saved states, game settings, bezels and backgrounds, for every account | Included |
| **Module settings** | Which modules are on, and how each one updates | Included |
| **Metadata files** | The [metadata folder](METADATA-FOLDER.md), if you've made one | Included |
| **Lyrics corrections** | Lyrics someone corrected on this server. Left out unless you ask, because they're the words of songs | Left out |
| **TV recordings** | Recordings kept in the data folder. These can be very large | Left out |

Some parts need others, because their numbers refer to each other. Watch history and game saves need the library and the accounts, channels need the library, and recordings need channels. The page ticks those for you.

## What isn't in a backup

These are never in a backup, whatever you tick:

- **Your media files.**
- **Programs:** modules and their helpers, game cores and BIOS files. A backup can never put a program on your machine. After a restore, install modules again under **Settings → Modules**, and add cores and BIOS files again under **Settings → Consoles & cores**. Which modules were on and how they update does come back.
- **Sign-ins.** Everyone signs in again after a restore.
- **Game PC pairings and custom cores.** Pair and add them again.
- **HTTPS certificates.**
- **Logs, reports and measurements:** crash reports, statistics history and the list of finished background work.

## The password

**With a password,** which is the page's default, everything inside the ZIP is encrypted with AES-256, using a key made from your password. File names can't be read either, and nobody can change the backup without the password. All your secrets come along: API keys, two-factor sign-in, share links and this server's identity.

**If you lose the password, nobody can open the backup.**

**Without a password,** the ZIP can be opened with any program, so a plain archive drops what shouldn't be left lying around:

- API keys
- two-factor secrets
- share links
- this server's identity
- logins inside addresses, such as a TV source's

Account passwords are in it as hashes. A hash can't be turned back into a password, but it can be guessed at, and titles and file names can be read. Keep a plain archive private.

Anyone could also have changed a file with no password, so restoring one leaves out a few more things:

- settings that hold web addresses
- libraries set to convert new files by themselves, which you turn on again if you want them
- recordings kept outside the data folder (the files themselves aren't touched)

The page lists whatever it left out before you agree.

## Backing up on a schedule

On the **Schedule** tab:

1. Under **How often**, choose **Every day** or **Every week**.
2. Choose the time, and the day for a weekly backup.
3. Under **Keep the newest**, say how many to keep, from 1 to 365. The default is 7.
4. Choose what to include.
5. Set a password if you want one. With no password, secrets are left out.
6. Press **Save**.

If the server was off at that time, the backup is made soon after it next starts. Older scheduled backups are removed when a new one is made, and ones you made yourself never are. Keeping fewer than before asks for your own password, because it removes backups.

The server keeps a key made from the schedule's password, not the password itself, so it can make backups with nobody there. Anyone with this server's database could open those backups, as they could read everything in them anyway.

If a scheduled backup fails, the tab says why.

## Restoring a backup

1. Open **Settings → Backup**, on the **Restore** tab.
2. Press **Choose a backup file**, or pick one under **Or one already on this server**. You can also press **Restore** beside a backup on the first tab.
3. If it's protected, type its password and press **Open it**. Every file is checked before anything changes.
4. Read **What's in it**. It lists each part with its size, **Who can sign in afterwards**, **Folders the server will use**, **Addresses it would contact**, and what was **Left out, on purpose**. If you don't recognise an administrator, stop: whoever made this backup could sign in.
5. Untick anything you don't want.
6. Type your own password, and `RESTORE`, and press **Restore this backup**.

What's on the server now moves to a safety copy in the `backups` folder, and the server restarts with the backup in place. Anything playing is interrupted. If something goes wrong on the way, even a power cut, what was there goes back by itself.

If the server can't restart itself, the page says **A restore is waiting**. Stop the server and start it again, and it happens then. **Cancel it** takes the request back.

**Restoring only some parts.** Anything you untick stays as it is on the server, unless it depends on something you're replacing. If you replace the library, watch history can't stay, because its numbers would point at other titles.

## Undoing a restore

On the **Restore** tab, under **The last restore**, press **Undo the last restore**, type your password, and press **Put it back**. The server restarts with what it had before. The command `wittymediaserver backup undo` does the same.

The newest three safety copies are kept.

## Restoring during first-time setup

After a reset, or on a new machine, you don't have to set anything up again.

1. On the first page of setup, press **Restore from a backup**.
2. From any device but the server's own machine, type the setup code. It's shown on the server's console, and is in `setup-code.txt` in the data folder.
3. Choose the file and carry on as in [Restoring a backup](#restoring-a-backup). There's no account yet, so it doesn't ask for your own password.

A reset that keeps a copy leaves the `backups` folder where it is. One that deletes everything deletes those backups too, so download one first. See [the Reset page](REPAIR.md#starting-again-the-reset-page).

## What happens to sign-ins, keys and modules

| | After a restore |
|---|---|
| **Every sign-in, on every device** | Ended. Everyone signs in again with the name and password in the backup. If you restored accounts, your current password here no longer applies |
| **Accounts** | Replaced by those in the backup, administrators included |
| **API keys, two-factor, share links** | Back as they were if the backup has a password. Otherwise gone: add the keys in Settings, set two-factor up again, and make new share links |
| **Apps on your devices** | Connect again. With a password this server keeps its identity. Without one it gets a new one, so an app may ask you to trust it again |
| **Remote Access** | Off until you turn it on again from home. A file never turns it on |
| **Settings naming a program or folder** | Left out, such as a Java path or a log folder, since they name something on the old machine. Set them again |
| **Modules** | Install them again under **Settings → Modules** |
| **Game PCs, custom cores, BIOS files** | Pair and add them again |

## Moving to another machine or drive

1. Install the server on the new machine, and restore during first-time setup.
2. The library's folders are kept as they were. If the drive letters or paths are different, open **Settings → Repair**, and use **Moved a drive?**. See [Moved a drive?](REPAIR.md#moved-a-drive).

Watch history, matches and edits stay. A backup from an older version is brought up to date as it's restored. One from a newer version is refused, so update this server first.

## Why a backup might be refused

Anyone can write a file and call it a backup, so each one is treated with suspicion, even one you made. It's refused, with the file named and the reason in words, when:

- it isn't a ZIP, has no list of its contents, or comes from a newer version;
- a path tries to leave its folder, or has a name that means something else on some system (`..`, a drive letter, `CON`, a colon, a hidden character);
- it holds a link or a special file, or the same name twice;
- it would unpack to far more than it's holding, or has an impossible number of files;
- a file isn't what its folder says: artwork that isn't a real picture, an invalid theme, or a program or web page anywhere at all;
- a file isn't listed, or isn't the size or hash it should be;
- the database has been tampered with or damaged.

Nothing changes when a backup is refused. If it's refused for damage, an earlier backup may still be good. If you're sure it's yours, press **Download logs as ZIP** under **Settings → WittyLog** and pass that on to whoever is helping you.

A changed file in the [metadata folder](METADATA-FOLDER.md) is the one exception. In a backup with no password, those files may be edited by hand on purpose. A changed one isn't refused: it's listed under **Metadata files changed by hand**, and read through the same checks as the folder.

Two things no check can do. A backup with no password can be rewritten by anyone, with a new administrator who has a known password, which is why the page shows who could sign in before you agree. And a saved game state goes straight to a game core, like any file you load, so leave out saves from someone you don't trust.

## From the command line

These work with no server running, which is the point of them.

```
wittymediaserver backup create -o /mnt/usb/server.zip --ask-password
wittymediaserver backup parts
wittymediaserver backup create --parts library,settings,users,history
wittymediaserver backup list
wittymediaserver backup inspect server.zip --ask-password
wittymediaserver backup restore server.zip --ask-password
wittymediaserver backup undo
wittymediaserver backup cancel
wittymediaserver backup help
```

| Command | What it does |
|---|---|
| `create` | Makes a backup. With no password, secrets are left out. `--parts` chooses what goes in |
| `parts` | Says what can be included, and how large each part is |
| `list` | Lists the backups in the `backups` folder |
| `inspect` | Checks a backup from end to end and says what's in it. Nothing changes |
| `restore` | Brings one back, after you type `RESTORE` (`--yes` skips that). With a server running, it happens as that server restarts |
| `undo` | Puts back what the last restore replaced. You can name an older safety copy |
| `cancel` | Takes back a restore waiting for the next start |

The part names are `library`, `settings`, `users`, `history`, `channels`, `artwork`, `saves`, `modules`, `metadatafiles`, `lyrics` and `recordings`.

A password can't be given on the command line, where other programs could see it. Use `--ask-password`, `--password-file <file>`, `--password-stdin`, or the `WMS_BACKUP_PASSWORD` environment variable. Add `-data <folder>` to work on a data folder other than the usual one.

## For developers

The code is `internal/backup`, the routes are `/api/backup/...` ([API](CLIENT-API.md#backup)), and the page is `web/src/pages/settings/Backup.tsx`.

- **Nothing is unpacked until everything has been read.** Names and modes are checked from the ZIP's directory first. Then each file is read once, by a reader that stops at the size the archive claims, with a hash from the manifest and a check of what it really is.
- **Names are judged as text, for every platform,** whichever the server runs on, then again when the path is built with `filepath.IsLocal`. Files are created with `O_EXCL`, so nothing can already be there, least of all a link.
- **The database in a backup is never opened as your database.** It's opened read-only with `immutable=1` and `trusted_schema=OFF`, and `integrity_check` is run. A clean database is built from this program's own migrations at the version the file says it has, and only the rows are copied, as values, by column name. So a trigger, a view, a check or an odd index has nowhere to go, and an older database then comes forward by the usual migrations.
- **The rows are judged too.** An item outside every library folder is dropped, as it would let a row serve any file. A library pointed at a system folder or the data folder is dropped. Artwork references must be a cached picture, an `http(s)` address, or a file inside a library. Stored passwords must look like a bcrypt hash. Text has markup and hidden characters taken out, the same as [metadata files](METADATA-FOLDER.md#how-its-kept-safe). Sign-ins are never copied.
- **Settings that start programs or write files aren't taken from a file**, password or not. Web addresses are taken only from a backup with a password. Module settings are reduced to on or off and who chooses updates, so a file can't name a place to install a module from.
- **Encryption.** AES-256-GCM in 1 MB chunks, with the key from Argon2id. Chunks can't be reordered, dropped or cut off, and the cost settings in a file are limited before any work is done.
- **Access.** Admin only, except during first-time setup, when from any other device it takes the setup code, as making the first account does. Restoring asks for the password and any second step again. Downloads use a ticket good for one file for ten minutes, so no sign-in goes in an address.
- **New tables** must be listed in `tableParts` or `neverTables` in `internal/backup/backup.go`. A test fails until they are.

---

<sub>WittyMediaServer documentation</sub>
