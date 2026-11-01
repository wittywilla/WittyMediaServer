# Backup and restore

A backup is one ZIP file holding your server's data and metadata. Make one before a reset, before moving to another
machine, or on a schedule, and bring it back when you need to. Your media files are never in it: they stay where they are.

- [Making a backup](#making-a-backup)
- [What's in it, and what isn't](#whats-in-it-and-what-isnt)
- [Passwords and secrets](#passwords-and-secrets)
- [On a schedule](#on-a-schedule)
- [Bringing one back](#bringing-one-back)
- [Starting again from a reset](#starting-again-from-a-reset)
- [What happens to sign-ins, keys and modules](#what-happens-to-sign-ins-keys-and-modules)
- [Moving to another machine or drive](#moving-to-another-machine-or-drive)
- [Why a backup might be refused](#why-a-backup-might-be-refused)
- [From the command line](#from-the-command-line)
- [How it stays safe](#how-it-stays-safe)

## Making a backup

*Settings → Backup*, tick what to include, set a password, and press **Make a backup now**. The page says about how large it
will be before you start. It runs in the background, so you can leave the page, and it shows in the activity panel. Nobody
has to stop watching: the database is copied at one moment in time while the server keeps running.

Backups are kept in the `backups` folder inside the data folder. **Download** one to keep it somewhere else: a copy on the same
drive doesn't help if that drive fails.

## What's in it, and what isn't

| Part | What it covers | Normally |
|---|---|---|
| **Libraries and metadata** | Your libraries and everything known about what's in them: titles, matches, edits, locks and history | Included |
| **Settings** | Everything under Settings, your server's name and logo, your own themes | Included |
| **Accounts and profiles** | Everyone's name, profile and password, as a hash | Included |
| **Watch history and personal data** | Progress, what's been watched, favourites, ratings, playlists, collections, notes, share links | Included |
| **Channels and TV** | WittyTV channels, tuners, the guide, scheduled recordings, bumpers, channel logos | Included |
| **Artwork** | Posters, backdrops and logos the server saved. They can be fetched again, so leave this out if space is tight | Included |
| **Game saves** | Saves, saved states, game settings, bezels and backgrounds, for every account | Included |
| **Module settings** | Which modules are on, and how each updates | Included |
| **Metadata files** | The [folder of text files](FIXING-METADATA.md) for fixing titles by hand, if you've made one | Included |
| **Lyrics corrections** | Lyrics someone corrected on this server. Left out unless you ask, because they're the words of songs | Left out |
| **TV recordings** | Recordings kept in the data folder. These can be very large | Left out |

Some parts need others, because their numbers refer to each other: watch history and game saves need the library and the accounts,
and channels need the library. The page ticks those for you.

**Never in a backup, whatever you tick:** your media files; modules and their programs; game cores and BIOS files; logs and reports; every
sign-in; HTTPS certificates and game PC pairings. Programs are left out so that a backup can never put code on your machine. They install
again from their signed releases.

## Passwords and secrets

**With a password** (the page's default), everything inside the ZIP is sealed with AES-256-GCM, with the key made from your password by
Argon2id. File names can't be read either. It also proves nobody has changed the backup, since that can't be done without the password. All of
your secrets come along: API keys, two-factor sign-in, share links and this server's identity. **If you lose the password, nobody can open the
backup, including us.** ZIP's own passwords aren't used: the old kind can be broken in minutes.

**Without a password**, the ZIP can be opened with anything, so secrets are left out: API keys, two-factor secrets, share links, this server's
identity, and logins inside addresses (a TV source's, for instance). Account passwords are in it as bcrypt hashes, which can't be turned back into
a password but can be guessed at, and titles and file names can be read. Keep it private.

## On a schedule

*Settings → Backup → Schedule*: every day or every week, at a time you choose, keeping the newest few. If the server was off at that time, the backup is
made soon after it next starts. Older scheduled backups are removed when a new one is made; ones you made yourself never are.

To protect scheduled backups, set a password there. The server keeps a key made from it rather than the password, so it can make backups with nobody
there. That key is a secret like your API keys: anyone with your server's database could open those backups, as they could read everything in them anyway.

## Bringing one back

*Settings → Backup → Restore*. Choose a file, or one already on the server, and enter its password if it has one. Then:

1. **Every file is checked** before anything changes. See [what gets refused](#why-a-backup-might-be-refused).
2. **It shows you what's in it:** each part, with sizes; who could sign in afterwards, and which of them are administrators; which folders the server would use;
   any web addresses it would contact; and what's been left out and why. Untick what you don't want.
3. **You agree**, with your password and the word RESTORE.
4. **What's on the server now moves to a safety copy** in the `backups` folder, the backup goes in, and the server restarts. If anything goes wrong on the way, even a
   power cut, what was there goes back by itself.

**Only some parts?** Anything you untick stays as it is on the server. The exception is what depends on something you're replacing: if you replace the library, watch history
can't stay, because its numbers would point at other titles. The page says so.

**Changed your mind?** The page has **Undo the last restore**, and `wittymediaserver backup undo` does the same. The newest three safety copies are kept.

## Starting again from a reset

Make a backup, then *Settings → Reset*. A reset that keeps a copy leaves the `backups` folder where it is; one that deletes everything deletes those too, so download first.
First-time setup has **Restore from a backup** on its first page, so you never have to set anything up again. From another device it asks for the setup code on the
server's console, just as making the first account does.

## What happens to sign-ins, keys and modules

| | After a restore |
|---|---|
| **Every sign-in, on every device** | Ended. Everyone signs in again with the name and password in the backup. If you restore accounts, your current password here no longer applies |
| **Accounts** | Replaced by those in the backup, which the page lists first, administrators included. If you don't recognise one, stop: whoever made the backup could sign in with it |
| **API keys, two-factor, share links** | Back as they were if the backup has a password. Otherwise gone: add the keys in Settings, set two-factor up again, and share links stop working |
| **Apps on your devices** | Connect again. With a password this server keeps its identity, and without one it gets a new one, so an app may ask you to trust it again |
| **Remote Access** | Off until you turn it on again from home. It's never turned on by a file |
| **Settings naming a program or folder** | Such as a Java path or a log folder. Left out, since they name something on the old machine: set them again |
| **Modules** | Their programs aren't in a backup. Install them again from *Settings → Modules*. Which were on and how they update comes back |
| **Game PCs, custom cores, BIOS files** | Pair and add them again |

## Moving to another machine or drive

Restore as usual, on the new machine. The library's folders are kept as they were, so if the drive letters or paths differ, the next step is *Settings → Repair → Moved a
drive?*, or `wittymediaserver doctor remap --from <old> --to <new>`, which changes every path after checking the files are there. Watch history, matches and edits stay. A
backup from an older version is brought up to date as it's restored. One from a newer version is refused: update this server first.

## Why a backup might be refused

Anyone can write a file and call it a backup, so each one is treated as hostile, even one you made. It's refused, with the file named and the reason in words, when:

- it isn't a ZIP, has no manifest, or is from a newer version of the format or database;
- a path tries to leave its folder, or has a name that means something else on Windows or anywhere else (`..`, a drive letter, `CON`, a colon, a hidden character);
- it holds a link or a special file, or the same name twice;
- it would unpack to far more than it's holding, which is how a ZIP bomb works, or has an impossible number of files;
- a file isn't what its folder says: artwork that isn't a real picture, a theme that isn't valid, a program or web page anywhere at all;
- a file isn't listed in the manifest, or isn't the size or hash it lists (metadata files are the exception, [see below](#how-it-stays-safe));
- the database has been tampered with: views, triggers it shouldn't have, computed columns, or damage.

A file refused for damage may be saved by an earlier backup. If you're sure it's yours and it's refused, a support bundle from *Settings → WittyLog* says what was seen.

## From the command line

Everything works with no server running, which is the point of it. `wittymediaserver backup help` lists it all.

```bash
wittymediaserver backup create -o /mnt/usb/server.zip --ask-password      # everything normally included, with a password
wittymediaserver backup parts                                             # what can be included, and how large each is
wittymediaserver backup create --parts library,settings,users,history     # only these, with secrets left out
wittymediaserver backup list
wittymediaserver backup inspect server.zip --ask-password                 # check it, and say what's in it; nothing changes
wittymediaserver backup restore server.zip --ask-password                 # with a server running, it happens as that server restarts
wittymediaserver backup undo                                              # put back what the last restore replaced
```

A password can't be given on the command line, where other programs could see it. Use `--ask-password`, `--password-file <file>`, `--password-stdin`, or the
`WMS_BACKUP_PASSWORD` environment variable. Add `-data <folder>` to work on a data folder other than the usual one.

## How it stays safe

For anyone who wants to check. The code is `internal/backup`.

- **Nothing is unpacked until everything has been read.** Names and modes are checked from the ZIP's directory first, then each file is read once, by a reader that stops
  at the size the archive claims, with a hash from the manifest and a check of what it really is.
- **Names are judged as text, for every platform,** whichever the server runs on, then again when the path is built with `filepath.IsLocal`. Files are created with `O_EXCL`, so nothing
  can already be there, least of all a link.
- **The database in a backup is never opened as your database.** It's opened read-only with `immutable=1` and `trusted_schema=OFF`, its `integrity_check` is run, and a clean database is
  built from this program's own migrations at the version the file says it has. Only the rows are copied, as values, by column name. So a trigger, a view, a check or an odd index has nowhere
  to go, and an older database then comes forward by the usual migrations.
- **The rows are judged too.** An item outside every library folder is dropped, as it would let a row serve any file. A library pointed at a system folder or the data folder is dropped.
  Artwork references must be a cached picture, an `http(s)` address, or a file inside a library. Stored passwords must look like a bcrypt hash. Text has any markup and hidden characters taken
  out, the same as [metadata files](FIXING-METADATA.md). Sign-ins are never copied.
- **Settings that start programs or write files aren't taken from a file**, password or not. Web addresses are taken only from a backup with a password, since nobody can have changed one of those
  without it. Module settings are reduced to on or off and who chooses updates, so a file can't name a place to install a module from.
- **Metadata files may be edited by hand** inside a ZIP with no password, which is what they're for: a changed one isn't refused, it's listed, and read through the same checks as the folder.
- **The password protects against the file being changed.** Chunks can't be reordered, dropped or cut off. Its cost settings are limited before any work is done, so a file can't ask for a terabyte
  of memory.
- **Admin only**, and restoring asks for the password and any second step again. Downloads use a ticket good for one file for ten minutes, so no sign-in goes in an address.

One thing no check can do: a backup with no password can be rewritten by anyone, with a new administrator who has a known password. That's why the page shows who could sign in before you agree. A saved
game state also goes straight to a game core, like any file you'd load; a save from someone you don't trust is worth leaving out.

---
<sub>WittyMediaServer documentation · [Back to the README](../README.md)</sub>
