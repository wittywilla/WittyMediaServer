# Getting started

This guide takes you from nothing to a film playing on your screen, one step at a time. On Windows and on a Mac you won't need the command line.

- [What you need](#what-you-need)
- [Install it and start it](#install-it-and-start-it)
- [What first-time setup asks](#what-first-time-setup-asks)
- [Put your files in order](#put-your-files-in-order)
- [Add your first library](#add-your-first-library)
- [Play your first film](#play-your-first-film)
- [Posters and descriptions](#posters-and-descriptions)
- [Watch on other devices](#watch-on-other-devices)
- [Add the family](#add-the-family)
- [Words you'll see](#words-youll-see)
- [Where next](#where-next)

## What you need

- **A computer that stays on while you watch.** Windows 10 or 11, macOS 11 or newer, Linux, FreeBSD 14 or newer, or a NAS that runs Docker.
- **Your media.** Films and TV episodes in nearly any format, or images of DVDs and Blu-rays you own. [Supported formats](SUPPORTED-FORMATS.md) has the full list. Encrypted discs are detected and refused with a clear message.
- **A web browser** on whatever you watch on. Nothing else has to be installed there.

You don't need to install FFmpeg, the program that converts video, or a database. The download has them.

## Install it and start it

Download the installer for your computer from the [Releases page](https://github.com/wittywilla/WittyMediaServer/releases), then:

| Your computer | File | What to do |
|---|---|---|
| Windows | `.msi` | Open it. If SmartScreen warns about an unknown publisher, choose *More info*, then *Run anyway*. Choose **Just me** unless you want it always on |
| Mac | `.pkg` | Right-click it, choose *Open*, and *Open* again |
| Debian, Ubuntu | `.deb` | `sudo apt install ./wittymediaserver-….deb` |
| Fedora, RHEL | `.rpm` | `sudo dnf install ./wittymediaserver-….rpm` |
| FreeBSD | `.pkg` | `pkg add wittymediaserver-….pkg`, `sysrc wittymediaserver_enable=YES`, `service wittymediaserver start` |

This is an alpha, and some of these haven't been run on the real thing yet. The Windows installer has only been tested under Wine, the Mac installer has never been run, and the FreeBSD build has been compiled but not run.

**On Windows and a Mac,** open **WittyMediaServer** from the Start menu or Applications. A small icon appears by the clock or in the menu bar, and your browser opens the setup page. To get back to it later, click the icon and choose **Open WittyMediaServer**, or go to `http://localhost:9797`.

**On Linux and FreeBSD,** the server runs as a background service. Open `http://<this computer>:9797/` in a browser.

**With Docker:**

```bash
docker run -d --name wittymediaserver --restart unless-stopped \
  -p 9797:9797 \
  -v wms-config:/config \
  -v /path/to/movies:/media/movies:ro \
  ghcr.io/wittywilla/wittymediaserver:alpha
```

[Installing](INSTALL.md) has every download, background services, Docker details and a NAS.

## What first-time setup asks

Setup takes a few minutes. Everything after your account can be skipped, and each page says where in **Settings** to change it later. Each answer is saved as you go, so you can go back, or stop and carry on tomorrow. A page only appears if this server can use what it asks about: there's no page about games until a games module is here.

- **Set up a new server, or restore from a backup.** These are the two buttons on the first page. Moving from another machine, or coming back from a reset? [Restoring](BACKUP.md) brings your accounts, libraries and settings back, and you won't see the rest of this list. The first page is also where you can rename your server and give it a logo.
- **Your account.** A name and a password of at least 8 characters, typed twice, and a tick to say you've read the legal notes. This first account looks after libraries, people and settings. Keep the password safe: [forgetting it is hard to undo](ACCOUNTS.md#if-you-forget-your-password).
- **What you want on this server.** Movies, TV shows and music work straight away. [Modules](MODULE-STORE.md) add discs, games, photos, books and more. Tick what you want and it downloads, signed, from the official list, so this takes an internet connection. The folder browser is ticked for you, as it's how you choose a folder from a TV or a phone. No internet? Carry on without them, and add them later.
- **When a module has a new version.** Tell you and let you choose, install it by itself, or look only when you ask.
- **Where your media is.** Pick a kind of media and the folder it's in, as many times as you like. See [Add your first library](#add-your-first-library). A folder on another computer has [its own steps](LIBRARIES.md#network-shares), which the page lays out if you choose one from another device.
- **Your games**, if you added the games module. Point it at your games folder and leave it. Anything it can't prove is listed for you to look at. Optional sources for more details can have their keys added here.
- **What a disc does when it starts**, if you added a disc module. Show its menu, or play the film straight away.
- **Posters and details.** TVmaze, Wikidata, Wikipedia, AniList and libretro work with no key, and names taken from your files and folders are sent to them to look things up. You choose whether posters are saved beside your files or kept on the server. Free keys for TMDB and TheTVDB are optional.
- **Logos.** Real logos for consoles, studios and networks, instead of plain names. They aren't part of this software: they download from Wikimedia Commons only if you say yes, and they're still trademarks of their owners.
- **Lyrics and artists**, where music can be played. Lyrics online come from LRCLIB, and artists' biographies and photos from Wikipedia. Both are free, with no account and no key, and both stay off unless you turn them on. See [Lyrics and karaoke](LYRICS-KARAOKE.md).
- **Streaming service lists.** Which of your own titles a streaming service lists right now. Nothing streams from those services. It stays off unless you choose it, accept four statements, and have a free TMDB key.
- **How it looks.** A theme for everyone, whether the menu sits along the top, down the side, or both, and whether this screen is across the room.

The last page says what you chose, as the server has it now, and where to change each one. It shows the address other devices can use, and offers to add a Kids profile. [Library transcoding](LIBRARY-TRANSCODING.md) and TV channels are mentioned there too: there's nothing to set up for them now. Press **Start watching** and you're in.

**With a remote or the keyboard.** The arrow keys reach everything and OK presses it. Back goes back a page. On a list of choices, the arrows only move: press OK on the one you want.

### If it asks for a setup code

On the computer running the server, setup doesn't ask for one. From any other device it does, so that nobody else on your network can claim the server first. Find it on the server's console when it starts (`docker logs wittymediaserver` for Docker, `journalctl -u wittymediaserver` for a Linux service), or in `setup-code.txt` in the server's data folder. See [Setting up from another device](ACCOUNTS.md#setting-up-from-another-device).

## Put your files in order

The server can find videos in any folder layout, but a tidy one gets you the right titles and posters. Keep films and TV shows in separate folders, put the year after a film's title, and number episodes like `S02E01`:

```
Movies/
  The Long Harbour (2001)/The Long Harbour (2001).mkv
  Paper Lanterns (2014).mp4
TV Shows/
  Night Ferry/
    Season 02/
      Night Ferry - S02E01 - The Crossing.mkv
```

[Libraries](LIBRARIES.md#how-to-lay-out-and-name-your-files) covers every kind of media.

## Add your first library

A library is one or more folders holding one kind of media. Setup offers this, or go to **Settings → Libraries → Add library** at any time.

1. Choose the kind of media, such as **Movies**.
2. Give it a name if you'd like a different one.
3. Choose the folder:
   - On the computer running the server, **Choose folder…** opens your computer's own folder window.
   - **Browse folders** on that computer, or **Browse folder…** on any other device, shows the server's drives in the page. Open the folder you want and press **Use this folder**.
   - Or type the folder's full path.
4. Press the button that adds it, such as **Add Movies**.

Scanning starts right away in the background. Titles appear as they're found, and posters and plots fill in after them. The activity button at the top of the screen shows how it's going.

## Play your first film

1. Open **Home**, or your library from the menu, and select a poster.
2. Press **Play**. If you've started it before, you'll see **Resume from** and the time, with **Play from start** beside it.

For a disc image, **Insert DVD** or **Insert Blu-ray** starts it like a disc player, with its intros and real menu, and **Play movie** goes straight to the film. See [DVDs](DVD.md) and [Blu-rays](BLURAY.md).

| Key | What it does |
|---|---|
| `Space` | Pause and play |
| `←` `→` | Back or forward 10 seconds |
| `↑` `↓` | Volume |
| `f` | Full screen |
| `m` | Mute |

With a TV remote or game controller, the arrows move around every page. [Watching and listening](PLAYING.md) has everything the player does.

If it doesn't play, see [Troubleshooting](TROUBLESHOOTING.md).

## Posters and descriptions

Films and shows get information with no key, from the services named above. For the best matches and more artwork, add a free key from TMDB or TheTVDB under **Settings → Metadata agents**, then press **Refresh** on a title. If a title has the wrong details, press **Edit** on its page, or use **Fix matches**. See [Metadata](METADATA.md).

## Watch on other devices

Any phone, tablet, computer or smart TV browser on the same home network can open your server.

1. Find the address. It's on the last page of setup, and under **Settings → About** at *Open it on other devices at home*. It looks like `http://192.168.1.20:9797`.
2. Type it into the browser on the other device.
3. If it doesn't load, the firewall on the server's computer may be in the way. On Windows, allow WittyMediaServer on private networks when asked, or choose **Let other devices on my home network connect** in the installer. See [Troubleshooting](TROUBLESHOOTING.md#connecting).

**The apps.** WittyMediaClient finds a server on your home network by itself, or you can type its address. A TV can be signed in with a code from **your name → Sign in a TV**: see [Signing in with a code](ACCOUNTS.md#signing-in-with-a-code). The Android app has been run on emulators only, not on real phones or TVs, and the iOS and Apple TV apps have never been built here. See [Apps and devices](APPS.md).

**Away from home.** The server refuses connections from outside your home until you turn on **Settings → Remote Access** and follow its steps. See [Remote access](REMOTE.md).

## Add the family

Under **Settings → Profiles**, add an **Adult**, **Kids** or **Guest** profile for each person. Everyone gets their own progress, and a Kids profile only shows titles up to the age you choose. See [Accounts and profiles](ACCOUNTS.md).

## Words you'll see

| Word | What it means |
|---|---|
| **Library** | One or more folders of one kind of media |
| **Scan** | The server looking through a library's folders for new or changed files |
| **Module** | A part of the server you can add or remove, such as disc menus or games |
| **Direct play** | The original file is sent as it is, with nothing converted |
| **Transcoding** | Converting a video as it plays, so a device can play it. It uses the server's processor or graphics card |
| **Disc image** | One file holding a whole disc, usually an `.iso` |
| **NFO** | A small text file beside a video that holds its title, plot and year |

## Where next

- [Libraries](LIBRARIES.md): every kind of library, and how to name your files
- [Accounts and profiles](ACCOUNTS.md): people, passwords and limits
- [Metadata](METADATA.md): posters, plots and fixing wrong matches
- [Live transcoding](TRANSCODING.md): hardware encoders and quality
- [Backup and restore](BACKUP.md): keeping your server safe
- [Troubleshooting](TROUBLESHOOTING.md): when something goes wrong

All of these are in the app too: open the menu under your name and choose **Help**.

---

<sub>WittyMediaServer documentation</sub>
