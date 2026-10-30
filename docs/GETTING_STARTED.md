# Getting started

New to media servers? This page takes you from download to watching, one step at a time. No
command line needed on Windows or macOS.

- [1. What you need](#1-what-you-need)
- [2. Download and start](#2-download-and-start)
- [3. Create your account](#3-create-your-account)
- [4. Organise your files](#4-organise-your-files)
- [5. Add a library](#5-add-a-library)
- [6. Optional: posters and descriptions](#6-optional-posters-and-descriptions)
- [7. Watch](#7-watch)
- [8. Watch on other devices](#8-watch-on-other-devices)
- [Glossary](#glossary)
- [Where next](#where-next)

## 1. What you need

- A computer that stays on while you watch: Windows 10/11, macOS 11+, Linux, FreeBSD, a NAS
  with Docker, or a Raspberry Pi 4/5.
- Your videos: movie files (`.mkv`, `.mp4`, …), TV episodes, or **DVD images** (`.iso`) /
  `VIDEO_TS` folders that you made from discs you own.
- A web browser (Chrome, Edge, Firefox, Safari) on whatever you watch on.

You do **not** need to install FFmpeg, a database or anything else: the download contains it all.

## 2. Download and start

1. Open the **Releases** page of the project on GitHub and download the archive for your system,
   e.g. `wittymediaserver-0.4.0-alpha-windows-amd64.zip`.
2. Unzip it anywhere (for example `Documents\WittyMediaServer`).
3. Start it:
   - **Windows**: double-click `WittyMediaServer.exe`. If SmartScreen warns about an unknown
     publisher, choose *More info → Run anyway* (alpha builds are not code-signed yet).
   - **macOS**: right-click `WittyMediaServer.app` → *Open* → *Open* (only the first time).
   - **Linux**: run `./WittyMediaServer` (tray) or `./wms` (no tray).
4. A small icon appears next to the clock (system tray / menu bar). Click it → **Open
   WittyMediaServer**. Your browser opens `http://localhost:9797`.

> Prefer Docker or want it to run as a background service? See
> [Installing](INSTALL.md).

## 3. Create your account

The first page asks you to create the **administrator** account. Pick a name and a password
(at least 4 characters). This account can change every setting; you can add accounts for family
members later under *Settings → Users*.

## 4. Organise your files

The server can find videos in any folder layout, but a tidy layout gives the best titles and
artwork. Keep movies and TV shows in separate folders:

```
Movies/
  Shrek (2001)/Shrek (2001).mkv
  Shrek 2 (2004).mp4
DVDs/
  SHREK.ISO
  SHREK_2.ISO
TV Shows/
  Codename Kids Next Door/
    Season 02/
      Codename - Kids Next Door - S02E01 - Operation C.A.T.S.mkv
```

Tips:

- Put the **year** in brackets after movie titles: `Title (Year)`.
- Name TV episodes with **S01E01** (season 1, episode 1). Details: [TV shows](TV.md).
- DVD images can keep their original names; the disc label is used as the title.

## 5. Add a library

1. Go to **Settings → Libraries → Add library**.
2. Give it a name ("Movies").
3. Choose the type:
   - **Movies** — films and DVD images, with movie posters from TMDB.
   - **TV shows** — series organised by show and season, with data from TheTVDB.
   - **Other videos** — home videos and anything else; no online lookups.
4. Click **Browse…**, open the folder with your videos, and click **Use this folder**.
5. **Save & scan**. Titles appear within seconds; posters and details fill in as the scan goes.

Repeat for each kind of media. Libraries are rescanned every 6 hours, or press **Scan** on the
library page after adding files.

## 6. Optional: posters and descriptions

Without any setup you get titles from file names and a still frame as the poster. For real
posters, descriptions and ratings, add free API keys under **Settings → Metadata agents**:

- **Movies**: a TMDB key — sign up at [themoviedb.org](https://www.themoviedb.org/signup), then
  *Settings → API* → create a key and copy the "API Key (v3 auth)".
- **TV shows**: a TheTVDB key — create an account at [thetvdb.com](https://thetvdb.com) and
  request a key on their [API information](https://thetvdb.com/api-information) page.

Paste the key, **Save**, and rescan the library. You can change any poster afterwards with
**Artwork** on the movie or show page.

## 7. Watch

- Click a poster → **Play** (or **Resume** where you stopped).
- For DVDs, click **Insert DVD**: the disc starts like it would in a DVD player — warnings,
  intros, then its real menu. Use the mouse, the arrow keys or the on-screen **Remote**.
  **Play movie** skips straight to the film. More: [DVDs](DVD.md).
- For TV shows, open the show and pick an episode; **Skip intro** and **Watch next** appear on
  their own.

Keyboard: `Space` pause · `←`/`→` skip 10 s · `f` fullscreen · `r` remote · `Esc` back.

## 8. Watch on other devices

Any phone, tablet, laptop or smart-TV browser on the **same home network** (the same Wi-Fi or
cable network) can open the server:

1. Find the server computer's address. The last step of the setup wizard and *Settings → About & legal*
   show it; or on Windows run `ipconfig` and look for *IPv4 Address* (e.g. `192.168.1.20`); on macOS:
   *System Settings → Network*.
2. On the other device open `http://192.168.1.20:9797`.
3. If it doesn't load, allow WittyMediaServer through the computer's firewall (Windows asks the
   first time it starts — tick *Private networks*).

On phones you can choose *Add to Home Screen* to get an app-like icon.

> **Home network only, until you turn on remote access.** The server runs on your own computer, and
> refuses connections from outside your home by default. To watch away from home, go to
> *Settings → Remote Access* and follow the steps: see [Remote access](REMOTE.md).

## Glossary

| Term | Meaning |
|---|---|
| **Library** | a group of folders of the same kind (movies, TV, other) |
| **Scan** | the server looking through library folders for new or changed files |
| **Direct play** | the browser plays the original file; nothing is converted |
| **Transcoding** | converting a video on the fly so the browser can play it; uses CPU or GPU |
| **ISO / DVD image** | one file containing a whole DVD, made from a disc you own |
| **NFO** | a small text file next to a video holding its title, plot, year… (a standard format most media servers and Kodi understand) |
| **WittyDVD** | the part of WittyMediaServer that runs DVD menus like a real player |

## Where next

- [DVDs and the WittyDVD engine](DVD.md)
- [TV shows](TV.md)
- [Metadata, NFO files and artwork](METADATA.md)
- [Transcoding and hardware encoders](TRANSCODING.md)
- [Themes and the custom name](THEMES.md)
- [Troubleshooting](TROUBLESHOOTING.md)

---
<sub>WittyMediaServer 0.4.0-alpha documentation</sub>
