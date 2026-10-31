# WittyDiagnoser

WittyDiagnoser is the disk diagnoser, for people who'd rather not open a terminal. It's in *Settings →
WittyDiagnoser*, for administrators. "Disk" can mean two things, so it has two tabs: **A disc** (a DVD or
Blu-ray, as an image or a folder) and **Storage** (the drives and folders the server uses).

It looks, and it tells you. It changes nothing.

- [What it never does](#what-it-never-does)
- [Checking a disc](#checking-a-disc)
- [Checking storage](#checking-storage)
- [Reading the results](#reading-the-results)
- [Reports](#reports)
- [How it treats the server](#how-it-treats-the-server)
- [For developers](#for-developers)

## What it never does

- **It never repairs or changes anything.** Discs and media are opened to read. The one thing it ever writes
  is the small temp file of the write test, if you turn that on, and its own reports.
- **It never decrypts anything or gets around copy protection.** It detects protection by reading a flag in
  the stream, and says so. It reads no keys, and doesn't name or suggest any tool.
- **It never runs a disc's code.** BD-J (Java) programs are read as files; they're not run.
- **It never looks outside your library folders and the data folder**, and storage checks take a folder from
  a list, not a path. Links that lead out of those places are refused.
- **It never asks for administrator rights.** What needs them is reported as not checked.
- **Its reports hold nothing from the disc:** no pictures, no menu art, no subtitle text, no file contents, and
  no full paths. Only structure, counts, base names of files and the commands of menu buttons.
- **It sends nothing anywhere.** A report leaves the server only if you download it and pass it on.

## Checking a disc

Pick a disc with the page's own browser, which only shows places it may look, or from the discs your libraries
already know. Then **Start the check**.

| It checks | What it looks at |
|---|---|
| **What it is** | Image or folder, UDF or ISO 9660, DVD or Blu-ray, size. A file that's no disc at all, and why it might be (too small, a raw 2352-byte dump) |
| **Index files** | Each DVD IFO and its backup, by name; a Blu-ray's index, movie objects, playlists and clip information and their backups |
| **Scrambling** | DVD: samples of sound and picture packs across the main title, the other titles and the menus, since some discs are scrambled only in part. Blu-ray: whether the streams are encrypted, the AACS folder, BD+ |
| **What's on it** | Titles or playlists with lengths, chapters, angles and track counts; the main title guess; region; first play and top menu; 3D, 4K and HDR |
| **Files it needs** | Video files the index names that aren't there; titles that run past the end; clips shorter than their information says |
| **BD-J** | Objects, applications, JAR files, class counts, which platform classes their code names that the server's runtime lacks, whether Java is found and BD-J is on |

Two choices take longer:

**Press every menu button.** Walks every menu it can reach, breadth first, and reports each button that leads
nowhere, with the path of buttons that got there: the disc stopped, a picture held for ever with no buttons, a
run of cells that never reached anything, scrambled video. It stops by itself at 300 screens or 10 minutes.

**Check for damaged or unreadable sectors.** Reads everything once from start to end. Parts that fail are read
again sector by sector, to count exactly what can't be read and say which file and title it falls in. It also
finds:

- an image shorter than its own file system says, which means an incomplete copy;
- files running past the end of the image;
- long stretches of zeros in the video, which copying tools leave where they couldn't read the disc;
- DVD sectors that don't start as MPEG packs;
- Blu-ray packets that have lost their rhythm (not checked on encrypted discs, where it means nothing).

The first 50 places are listed, with totals for the rest. A few of these can be deliberate: filler that never
plays, and decoy files with impossible sizes, are forms of copy protection. The report says which title a place
belongs to, so you can tell.

## Checking storage

Tick some library folders and server folders, choose what to try, and **Start the check**.

| It checks | Notes |
|---|---|
| **Is it there** | Exists, is a folder, can be listed, a few files can be opened |
| **What drive** | NTFS, exFAT, FAT32, ReFS, ext4, NFS and so on; fixed, removable, SD card, network |
| **Free space** | Warns under 5%, and under 10 GB where video gets converted, with how many hours of 1080p that is |
| **Known trouble** | Database on a network drive; transcode folder on a network drive or SD card; a mapped drive letter, which services can't see; read-only mounts; FAT32's 4 GB limit; a container's own layer |
| **Go through every file** | Up to 200,000 entries or 2 minutes a folder: files that can't be opened, empty files, paths over 260 characters on Windows, names FFmpeg and others may trip over, and for libraries, items the database knows that have gone from the disk |
| **Read speed** | Up to 256 MB or 15 seconds from random places in the largest files, and 32 random 1 MB reads, with how many 4K, Blu-ray and 1080p streams it could keep up with. The system's cache can flatter this |
| **A small write** | A 32 MB temp file, flushed to the drive, read back, compared and removed. Only in the server's own folders, and never in or around a library folder |
| **Drive health** | What the system tells without administrator rights. Windows: `Get-PhysicalDisk`. Linux: `/sys/block`, including temperature where shown. macOS: `diskutil info`. Otherwise "not checked" |

## Reading the results

Each finding has a word as well as a colour, worst first:

| Word | Means |
|---|---|
| **Problem** | Something won't work, or data is at risk. It says what to do |
| **Warning** | It may cause trouble, or may be on purpose. Read why |
| **Note** | Worth knowing; nothing is wrong |
| **Fine** | Checked, and good |

**Details** opens the numbers and file names behind a finding.

"The engine crashed on this disc" means a bug in the server's own disc engine. The rest of the check carries
on. It's a useful thing to report, because the same disc would probably cause trouble in the player.

## Reports

The last 20 reports stay on the server, in `<data>/diagnostics/`, under **Past reports**. **Download report**
gives plain text or JSON. Turn on **Hide disc name and file names** for a copy to send to someone: the disc's
name, its label and every file name come out. The newest three go in the
[support bundle](CONFIG.md#logs), always with names hidden.

To report a disc problem, attach a report. Never attach the disc image.

## How it treats the server

- One check at a time, with up to 8 waiting. You can leave the page; it keeps going. **Cancel** stops it, and
  keeps what it found.
- While someone is watching something, it rests after each read for as long as that read took. A check can't
  take more than half of what a disk can do while a stream is playing. (A file sent to a player as it is, with
  no conversion, isn't counted: the server doesn't track those.)
- Memory stays small: one 1 MB buffer for reading, one JAR at a time, and menus are kept as buttons, without
  their pictures.
- Its log lines are in the `discs` and `storage` categories, with the check's ID as the session, so *Settings →
  WittyLog* can show one check's whole story.

## For developers

The code is `internal/diagnose`, using the exported API of WittyDVD, WittyBluray and WittyDisc, and no changes to
them: it has to build against their published versions. File systems and drives are in `internal/sysinfo`. The
routes are `/api/diagnose/...` ([API](CLIENT-API.md)). The command-line tools, `wms disc`, `wms dvd` and
`wms bluray`, stay for engine work: [DVD](DVD.md#diagnosing-a-disc), [Blu-ray](BLURAY.md#diagnosing-a-disc).

---
<sub>WittyMediaServer documentation · [Back to the README](../README.md)</sub>
