<p align="center">
  <img src="web/public/icon-192.png" width="88" alt="">
</p>

<h1 align="center">Your media. Right at home.</h1>

<p align="center">
  <b>WittyMediaServer</b> puts your films, shows, discs, music, books and games on every screen in your home.<br>
  A self-hosted media server that plays DVDs and Blu-rays <i>with their real menus</i>, in any browser.
</p>

<p align="center">
  <a href="https://github.com/wittywilla/WittyMediaServer/releases/latest"><img alt="Download" src="https://img.shields.io/badge/Download-0.8.0--alpha-6d4aff?style=for-the-badge"></a>
  <a href="https://wittymediaserver.com"><img alt="Website" src="https://img.shields.io/badge/Website-wittymediaserver.com-1f1d2b?style=for-the-badge"></a>
  <a href="docs/GETTING_STARTED.md"><img alt="Get started" src="https://img.shields.io/badge/Get%20started-guide-1f1d2b?style=for-the-badge"></a>
</p>

<p align="center">
  <a href="LICENSE"><img alt="Licence: AGPL-3.0-or-later" src="https://img.shields.io/badge/licence-AGPL--3.0--or--later-blue.svg"></a>
  <img alt="Runs on Windows, macOS, Linux, FreeBSD and Docker" src="https://img.shields.io/badge/runs%20on-Windows%20·%20macOS%20·%20Linux%20·%20FreeBSD%20·%20Docker-8b6cff.svg">
  <a href="CHANGELOG.md"><img alt="What's new" src="https://img.shields.io/badge/what's%20new-changelog-orange.svg"></a>
</p>

<p align="center">
  <a href="docs/images/home.webp"><img src="docs/images/home.webp" alt="The home page, with a large picture from a film and a row of libraries" width="74%"></a>
  <a href="docs/images/phone-home.webp"><img src="docs/images/phone-home.webp" alt="The home page on a phone" width="20%"></a>
  <br><sub>Home on a computer and a phone. <a href="https://commons.wikimedia.org/wiki/File:WING_IT!_-_Blender_Open_Movie-full_movie.webm">Wing It!</a>, Blender Studio, <a href="https://creativecommons.org/licenses/by/4.0/">CC BY 4.0</a>. Everything shown is <a href="docs/images/CREDITS.md">openly licensed</a>.</sub>
</p>

<table align="center">
<tr>
<td align="center"><b>Streaming at home</b><br>free</td>
<td align="center"><b>Streaming from anywhere</b><br>free</td>
<td align="center"><b>Every feature, for everyone</b><br>free</td>
</tr>
</table>

> [!NOTE]
> **Alpha.** 0.8.x-alpha is still changing quickly. It can fail or lose data. Keep backups, and back up your data folder before you upgrade.

## What it does

<table>
<tr>
<td width="33%" valign="top"><a href="docs/images/film-page.webp"><img src="docs/images/film-page.webp" alt="A film's page"></a><br><b>Films and TV</b><br>Posters, plots and ratings, <i>Continue watching</i>, skip intro and instant seeking. Details arrive with no API key.</td>
<td width="33%" valign="top"><a href="docs/images/dvd-menu.webp"><img src="docs/images/dvd-menu.webp" alt="A DVD's own menu in a browser"></a><br><b>DVDs and Blu-rays</b><br>Disc images play like a disc player: intros, animated menus, scene selection, extras and the disc's own subtitles.</td>
<td width="33%" valign="top"><a href="docs/images/wittytv-guide.webp"><img src="docs/images/wittytv-guide.webp" alt="A TV guide with a film playing in the preview"></a><br><b>Live TV and channels</b><br>Channels made from your library, a guide, tuners, IPTV and recording.</td>
</tr>
<tr>
<td width="33%" valign="top"><a href="docs/images/music-visualizer.webp"><img src="docs/images/music-visualizer.webp" alt="A light show behind the music player"></a><br><b>Music, audiobooks and podcasts</b><br>Albums and artists, lyrics, light shows, books that remember your place, and shows you follow.</td>
<td width="33%" valign="top"><a href="docs/images/reader-comic.webp"><img src="docs/images/reader-comic.webp" alt="A comic open in the reader"></a><br><b>Books and comics</b><br>EPUB, PDF, comics and manga, with highlights, notes and a pen.</td>
<td width="33%" valign="top"><a href="docs/images/photos.webp"><img src="docs/images/photos.webp" alt="A timeline of photos"></a><br><b>Photos and videos</b><br>A timeline, albums and an offline map, a feed of vertical videos, and creators' videos by channel.</td>
</tr>
<tr>
<td width="33%" valign="top"><a href="docs/images/games.webp"><img src="docs/images/games.webp" alt="A library of games"></a><br><b>Games</b><br>Your own game files run on the server and stream to the screen in front of you, with a gamepad, keyboard or touch.</td>
<td width="33%" valign="top"><a href="docs/images/theme-light-home.webp"><img src="docs/images/theme-light-home.webp" alt="The home page in the Light theme"></a><br><b>Your look</b><br>Fourteen layouts and fifteen themes, the same on a TV, a phone and a computer, or a theme of your own.</td>
<td width="33%" valign="top"><a href="docs/images/module-store.webp"><img src="docs/images/module-store.webp" alt="The module store"></a><br><b>Only what you want</b><br>Discs, games, photos, readers and tuners are modules you add from a store inside the app.</td>
</tr>
</table>

**Everyone at home** gets a *Who's watching?* profile with their own progress, and Kids profiles only show titles for their age. **It's yours:** it runs on your computer, with no accounts with anyone, no telemetry and no cloud. No media comes with it.

## Where can I use it?

The server runs on a computer you already have. Every other screen reaches it in a web browser, or with the optional app, **WittyMediaClient**.

| The server runs on | Watch, listen and read on |
|---|---|
| **Windows** 10 and 11 | **Any web browser:** computers, phones, tablets and TVs. Nothing to install |
| **macOS** 11 and newer | **Android:** phones and tablets |
| **Linux:** Debian, Ubuntu, Fedora and most others | **Android TV and Fire TV:** an app made for the remote, with game controllers and QR sign-in. Google TV too |
| **FreeBSD** 14 and newer | **Apple:** iPhone, iPad and Apple TV |
| **Docker and NAS:** any NAS or server that runs Docker | **Desktop:** Windows, macOS, Linux and Plasma Bigscreen |

> [!TIP]
> The apps aren't in the app stores yet: for now, each one is installed by hand (sideloaded) from the [WittyMediaClient releases page](https://github.com/wittywilla/WittyMediaClient/releases/latest). App store listings are coming soon.

## Install

Install the server on the computer or NAS that holds your media. Downloads are on the **[Releases page](https://github.com/wittywilla/WittyMediaServer/releases)**, and FFmpeg comes with them. When it's running, open `http://<that computer>:9797/` in a browser to set it up.

| On | Do this | Full guide |
|---|---|---|
| **Windows** | Open `windows-amd64.msi` (`arm64` for ARM PCs) and choose who it's for. Setup opens in your browser | [Windows](docs/INSTALL.md#windows) |
| **macOS** | Open `macos-arm64.pkg` (Apple silicon) or `macos-amd64.pkg` (Intel), then **WittyMediaServer** in Applications | [macOS](docs/INSTALL.md#macos) |
| **Debian, Ubuntu** | `sudo apt install ./wittymediaserver-<version>-linux-amd64.deb`. The service starts by itself | [Linux](docs/INSTALL.md#linux) |
| **Fedora, RHEL** | `sudo dnf install ./wittymediaserver-<version>-linux-amd64.rpm`. The service starts by itself | [Linux](docs/INSTALL.md#linux) |
| **Any other Linux** | The AppImage, Flatpak or snap, or unpack `linux-amd64.tar.gz` and run `sudo ./install.sh` for a service | [Linux](docs/INSTALL.md#linux) |
| **FreeBSD** | `pkg add wittymediaserver-<version>-freebsd-amd64.pkg`, `sysrc wittymediaserver_enable=YES`, `service wittymediaserver start` | [FreeBSD](docs/INSTALL.md#freebsd) |
| **Docker** | `docker run -d -p 9797:9797 -v wms-config:/config -v /path/to/media:/media:ro ghcr.io/wittywilla/wittymediaserver:alpha`, or a [ready compose file](deploy/docker/compose.yml) | [Docker](docs/DOCKER.md) |
| **A NAS** | Docker, with a guide for each: [UGREEN](docs/DOCKER-UGREEN.md) · [Synology](docs/DOCKER-SYNOLOGY.md) · [QNAP](docs/DOCKER-QNAP.md) · [Unraid](docs/DOCKER-UNRAID.md) · [TrueNAS SCALE](docs/DOCKER-TRUENAS.md) · [Asustor](docs/DOCKER-ASUSTOR.md) | [A NAS](docs/INSTALL.md#a-nas) |

Then go through setup: your account, a name, your language and time zone, a layout and theme, the modules you want, and your folders. [Getting started](docs/GETTING_STARTED.md) walks through it with no command line, and [First-time setup](docs/FIRST-TIME-SETUP.md) shows every page.

## Your server, from anywhere

**New in 0.8.0-alpha.** Turn on peer-to-peer connections in **Settings → Remote Access**, and the app on your phone, TV or laptop finds its own way home: no port forwarding, no rented server, and no account with anyone.

- Encrypted from end to end, to a key only your server holds.
- Only devices you approved can connect, and they still sign in.
- A TV that has never been home joins with a code it shows.

Until you choose a way in, the server answers on your home network only. See [Remote access](docs/REMOTE.md).

## Karaoke night

<p>
  <a href="docs/images/karaoke.webp"><img src="docs/images/karaoke.webp" alt="Karaoke: the line being sung fills word by word over a light show" width="74%"></a>
  <a href="docs/images/phone-karaoke.webp"><img src="docs/images/phone-karaoke.webp" alt="Karaoke on a phone" width="20%"></a>
</p>

Any song in your library, ready to sing. The singer is turned down in seconds, on your own server, and the words fill in as they're sung, big enough to read from across the room. <sub>"Take Me Out to the Ball Game", 1908, public domain, over WittyVisualizer's Northern Veil.</sub> See [Lyrics and karaoke](docs/LYRICS-KARAOKE.md).

## A look around

A test server, filled with openly licensed media to show what it can do. Every work on it is named in [the credits](docs/images/CREDITS.md), with its licence.

<table>
<tr>
<td width="33%"><a href="docs/images/movies.webp"><img src="docs/images/movies.webp" alt="A wall of film posters"></a><br><sub><b>Films.</b> <a href="docs/images/CREDITS.md">Credited here</a></sub></td>
<td width="33%"><a href="docs/images/bluray-menu.webp"><img src="docs/images/bluray-menu.webp" alt="A Blu-ray playing in a browser"></a><br><sub><b>Blu-rays.</b> <a href="https://archive.org/details/SitaSingsTheBlues_201812">Sita Sings the Blues</a>, Nina Paley, <a href="https://creativecommons.org/publicdomain/zero/1.0/">CC0</a></sub></td>
<td width="33%"><a href="docs/images/tv-show-page.webp"><img src="docs/images/tv-show-page.webp" alt="A show's page with its episodes"></a><br><sub><b>TV shows.</b> <a href="docs/images/CREDITS.md">Credited here</a></sub></td>
</tr>
<tr>
<td width="33%"><a href="docs/images/music-lyrics.webp"><img src="docs/images/music-lyrics.webp" alt="Lyrics beside the player"></a><br><sub><b>Lyrics.</b> "Take Me Out to the Ball Game", 1908, public domain</sub></td>
<td width="33%"><a href="docs/images/reader-epub.webp"><img src="docs/images/reader-epub.webp" alt="A book open in the reader"></a><br><sub><b>Books.</b> <a href="https://standardebooks.org/">Standard Ebooks</a>, <a href="https://creativecommons.org/publicdomain/zero/1.0/">CC0</a></sub></td>
<td width="33%"><a href="docs/images/podcasts.webp"><img src="docs/images/podcasts.webp" alt="A page of podcasts"></a><br><sub><b>Podcasts and audiobooks.</b> <a href="docs/images/CREDITS.md">Credited here</a></sub></td>
</tr>
<tr>
<td width="33%"><a href="docs/images/shorts.webp"><img src="docs/images/shorts.webp" alt="A feed of vertical videos"></a><br><sub><b>Vertical videos.</b> Clips from open films, <a href="docs/images/CREDITS.md">credited here</a></sub></td>
<td width="33%"><a href="docs/images/gameplay-ucity.webp"><img src="docs/images/gameplay-ucity.webp" alt="A city-building game"></a><br><sub><b>Games.</b> <a href="https://github.com/AntonioND/ucity">µCity</a>, Antonio Niño Díaz, GPL-3.0-or-later and CC BY-SA 4.0</sub></td>
<td width="33%"><a href="docs/images/settings-libraries.webp"><img src="docs/images/settings-libraries.webp" alt="The Libraries page in Settings"></a><br><sub><b>Plain settings.</b> Settings, Libraries</sub></td>
</tr>
</table>

## Guides

**Everything here is also inside the app, under Help,** so you don't need this page once you're running.

<details>
<summary><b>Getting going</b></summary>

| | |
|---|---|
| [Getting started](docs/GETTING_STARTED.md) | From install to your first film |
| [First-time setup, step by step](docs/FIRST-TIME-SETUP.md) | Every page of setup, with a picture of each |
| [Installing](docs/INSTALL.md) | Downloads, the tray app, services, updating and removing it |
| [Docker](docs/DOCKER.md) | The image, compose files, graphics cards, and a guide for each make of NAS |
| [Libraries](docs/LIBRARIES.md) | Adding your media, and naming your files |
| [Accounts and profiles](docs/ACCOUNTS.md) | People, passwords, limits and sign-in |
| [Profiles and parental controls](docs/PROFILES.md) | Switching profile on a shared screen, limits, screen time and reports |
| [Apps and devices](docs/APPS.md) | Every device, and how far each has been run |
| [Settings tour](docs/SETTINGS.md) | Where every setting lives |
| [Troubleshooting](docs/TROUBLESHOOTING.md) | Common problems, by what you're seeing |

</details>

<details>
<summary><b>Your media</b></summary>

| | |
|---|---|
| [Watching and listening](docs/PLAYING.md) | The player, keys, tracks and remotes |
| [What each media type supports](docs/MEDIA-TYPES.md) · [Supported formats](docs/SUPPORTED-FORMATS.md) | Which library to make, and every file type |
| [DVDs](docs/DVD.md) · [Blu-rays](docs/BLURAY.md) | Menus, the remote, audio and subtitles |
| [TV shows](docs/TV.md) · [Channels and live TV](docs/CHANNELS.md) | Show folders, channels, tuners and the guide |
| [Music](docs/MUSIC.md) · [Lyrics and karaoke](docs/LYRICS-KARAOKE.md) | Music, audiobooks, podcasts and singing along |
| [Books, PDFs and comics](docs/BOOKS.md) · [Photos](docs/PHOTOS.md) | Reading and looking |
| [Vertical videos](docs/SHORTS.md) · [Creator videos](docs/CREATOR-VIDEOS.md) | Feeds, channels and concerts |
| [Collections, radio and Your year](docs/COLLECTIONS.md) | Smart playlists and internet radio |
| [Games](docs/GAMES.md) | Your own games, from a folder to playing |

</details>

<details>
<summary><b>Running a server</b></summary>

| | |
|---|---|
| [Metadata](docs/METADATA.md) · [Fixing wrong details](docs/FIXING-METADATA.md) · [The metadata folder](docs/METADATA-FOLDER.md) | Posters, plots and wrong matches |
| [Live transcoding](docs/TRANSCODING.md) · [Library transcoding](docs/LIBRARY-TRANSCODING.md) · [Using Tdarr](docs/TDARR.md) | Converting video |
| [Modules and the store](docs/MODULE-STORE.md) | Adding, updating and trusting modules |
| [Remote access](docs/REMOTE.md) | Watching away from home, safely |
| [Backup and restore](docs/BACKUP.md) · [Repair](docs/REPAIR.md) · [WittyDiagnoser](docs/DIAGNOSER.md) | Keeping it healthy |
| [Themes](docs/THEMES.md) · [Pages and the menu](docs/PAGES.md) | Layouts, colours and what's on each page |
| [Config file](docs/CONFIG.md) · [Logs](docs/LOGGING.md) · [Statistics](docs/STATS.md) · [Command line](docs/REFERENCE.md) | Fine tuning |

</details>

<details>
<summary><b>For developers</b></summary>

[Contributing](CONTRIBUTING.md), [Repositories](docs/REPOSITORIES.md), [Client API](docs/CLIENT-API.md), [Server-driven screens](docs/SDUI.md) and [a module's screens](docs/SDUI-MODULES.md), [Making a module](docs/MODULES.md) and [Module API 5](docs/MODULES-API5.md), [Peer-to-peer connections](docs/P2P.md), [the interface design](docs/design-2/README.md), [Security model](docs/SECURITY-REVIEW.md) and the [Changelog](CHANGELOG.md).

</details>

## Where it has been run

The server is written for Windows, Linux, macOS and FreeBSD, and runs in Docker.

- **macOS:** 0.8.0-alpha was built and tested on a Mac with Apple silicon. The `.pkg` has been installed on GitHub's Mac machines, not yet by a person on a real Mac.
- **Windows and Linux:** run on a real PC for 0.8.0-alpha. The Linux packages are installed, upgraded and removed on Debian, Ubuntu and Fedora before each release.
- **FreeBSD:** the package and its service in virtual machines.
- **Docker:** the image in WSL, not yet on a NAS.

WittyMediaClient has run on real Android phones, tablets and TVs, a real Apple TV, and desktops. The iPhone app has run in Apple's simulator only. [What has been tested](docs/INSTALL.md#what-has-been-tested) and [Apps and devices](docs/APPS.md) have the details.

## Privacy and security

- **No telemetry, no accounts with anyone, no cloud.** Logs stay on your computer and never contain passwords or session tokens.
- **Your files stay here.** Names taken from your file and folder names go to the metadata services you keep switched on, and the server asks GitHub for updates twice a day. **Settings → Legal → Privacy** lists every outside service, what it receives and what controls it.
- **Limits by default.** Cross-site requests are refused, feeds and agents can't reach other devices on your network, and modules run in a sandbox where the system offers one.
- **Copy protection is never bypassed.** Encrypted discs, broadcasts and game files are detected and refused.

To report a security problem, see [SECURITY.md](SECURITY.md).

## Contributing

Issues and pull requests are welcome. [CONTRIBUTING.md](CONTRIBUTING.md) has how to build, test and send a change. Disc engines must stay free of other players' code, because they're written from the public disc formats.

**Working on a Mac?** Start with [MAC-FOLLOW-UPS.md](MAC-FOLLOW-UPS.md), newest section first, and the apps' own list in WittyMediaClient's [MAC-FOLLOW-UPS.md](https://github.com/wittywilla/WittyMediaClient/blob/v0.8.1-alpha/MAC-FOLLOW-UPS.md).

## Licence and legal

**[GNU Affero General Public License v3.0 (or later)](LICENSE).** Use, change and share WittyMediaServer, commercially or not, under its terms; please keep the attribution in **[NOTICE](NOTICE)**: *"This product includes WittyMediaServer, developed by WittyWilla"*, in your distribution, documentation or About screen. Every Witty module is AGPL-3.0-or-later too. The client apps (WittyMediaClient, WittyMediaPlayer, WittyReader and the smaller apps) are under the GNU General Public License v3.0 or later. Code forked or derived from another project keeps that project's licence and is labelled with it; for example, WittyCouch Host is AGPL-3.0-or-later with its Moonlight-derived parts under the GPL-3.0. A few separate programs have licences of their own, and each is listed in the app and in [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md).

<details>
<summary><b>Your media, third parties and warranty</b></summary>

The app's own **Legal** page (*Settings → Legal*, or `/legal` on any server) has the full text. This is plain-language information, not legal advice.

**Your media, your responsibility.** WittyMediaServer organises and streams media you already have the right to use. It doesn't provide, host, own or check any of it. Server operators and users are responsible for the files they add and share, and for following the laws where they and their viewers live. The authors and contributors accept no liability for media stored on, played from or shared through a server, nor for how a server is used.

**Web results and third-party content.** Search results, details, artwork, podcasts, radio stations, TV channels and guides, and game details come from third parties. They aren't created by, associated with, endorsed by or checked by the WittyMediaServer project, its contributors or affiliates.

**Games.** No games, ROMs or BIOS files come with this software, and the project doesn't link to them. Use only files you've obtained legally.

**No circumvention.** Copy protection is never decrypted or bypassed: CSS-scrambled DVDs and AACS-encrypted Blu-rays are refused. Encrypted media isn't supported, and no plugin or setting changes that.

**Third parties.** Video is converted by **FFmpeg**, run as a separate program. The bundled builds are GPL-licensed, with sources linked from [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md). This product uses the **TMDB** API but is not endorsed or certified by TMDB. TV information is provided by **[TheTVDB](https://thetvdb.com)** and **TVmaze** (CC BY-SA 4.0), movie and game facts by **Wikidata** (CC0) and **Wikipedia** (CC BY-SA 4.0), anime by **AniList**, definitions by the **Free Dictionary API**, and podcast search by the **Apple Podcasts** directory. Each service's terms apply to its data.

**No warranty.** Provided "as is", without warranty of any kind.

**No affiliation, and trademarks.** WittyMediaServer is not affiliated with any other media software project, studio, console maker, service or rights holder. Third-party names only describe compatibility or where information came from, and belong to their owners. "DVD" is a trademark of DVD Format/Logo Licensing Corporation; "Blu-ray" and "Blu-ray Disc" are trademarks of the Blu-ray Disc Association. This software isn't a licensed or certified DVD or Blu-ray product.

</details>

---

<p align="center"><sub>WittyMediaServer 0.8.0-alpha · AGPL-3.0-or-later · Made by <a href="https://github.com/wittywilla">WittyWilla</a> · <a href="https://wittymediaserver.com">wittymediaserver.com</a></sub></p>
