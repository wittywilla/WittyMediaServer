<p align="center">
  <img src="web/public/icon-192.png" width="96" alt="">
</p>

<h1 align="center">WittyMediaServer</h1>

<p align="center">
  <b>Your films, shows, discs, music, books and games, on every screen in your home.</b><br>
  A self-hosted media server that plays DVDs and Blu-rays <i>with their real menus</i>, in any browser.
</p>

<p align="center">
  <a href="LICENSE"><img alt="Licence: Apache-2.0" src="https://img.shields.io/badge/licence-Apache--2.0-blue.svg"></a>
  <img alt="Runs on Windows, macOS, Linux, FreeBSD and Docker" src="https://img.shields.io/badge/runs%20on-Windows%20·%20macOS%20·%20Linux%20·%20FreeBSD%20·%20Docker-8b6cff.svg">
  <img alt="Version 0.6.0-alpha" src="https://img.shields.io/badge/version-0.6.0--alpha-orange.svg">
</p>

<p align="center">
  <a href="docs/GETTING_STARTED.md"><b>Get started</b></a> ·
  <a href="docs/INSTALL.md">Install</a> ·
  <a href="#guides">Guides</a> ·
  <a href="CHANGELOG.md">What's new</a>
</p>

> **Alpha.** Version 0.6.0-alpha is still changing quickly and can fail or lose data. Keep backups, and back up your data folder before you upgrade.

## What it does

- **Films and TV.** Posters, plots and ratings, *Continue watching*, skip intro and instant seeking. Information arrives with no API key, and you can add free keys or your own agents for more.
- **DVDs and Blu-rays.** Disc images play like a disc player: intros, animated menus, scene selection, extras and the disc's own subtitles.
- **Live TV and channels.** Channels made from your library, a guide, tuners, IPTV and recording.
- **Music, audiobooks and podcasts.** Albums and artists, lyrics and karaoke, books that remember your place, and shows you follow.
- **Books and comics.** EPUB, PDF, comics and manga, with highlights, notes and a pen.
- **Photos and videos.** A timeline, albums and an offline map, a feed of vertical videos, and creators' videos by channel.
- **Games.** Your own game files, played in a browser with a gamepad, keyboard or touch.
- **Everyone at home.** *Who's watching?* profiles with their own progress, and Kids profiles that only show titles for their age.
- **Yours.** It runs on your computer. No accounts with anyone, no telemetry and no cloud. No media comes with it.

Discs, games, photos, readers and tuners are modules: separate programs you add from a store inside the app, each with its own [repository](docs/REPOSITORIES.md). The server works without any of them.

## Get started

1. Download the installer for your computer from the [Releases page](https://github.com/wittywilla/WittyMediaServer/releases).
2. Open **WittyMediaServer**. Your browser opens first-time setup.
3. Create your account, point it at your folders and press **Start watching**.

Those downloads are the server, for the computer that holds your media. **To watch on another device,** open the server's address in a web browser, or use the app, [WittyMediaClient](https://github.com/wittywilla/WittyMediaClient/releases), which has its own Releases page. It's for phones, TVs and other computers, and it's early: see [Where it has been run](#where-it-has-been-run).

The full walkthrough is in [Getting started](docs/GETTING_STARTED.md), and [Installing](docs/INSTALL.md) covers services, Docker and a NAS:

```bash
docker run -d -p 9797:9797 -v wms-config:/config -v /path/to/media:/media:ro ghcr.io/wittywilla/wittymediaserver:alpha
```

The server is on your home network only until you turn on **Settings → Remote Access** and follow its steps. See [Remote access](docs/REMOTE.md).

## Where it has been run

The server is written for Windows, Linux, macOS and FreeBSD. Linux gets the most testing. The Windows installers have been tested under Wine, not on real Windows. Nothing has been run on a real Mac yet. FreeBSD has been cross-compiled, not run.

WittyMediaClient adds Android, Android TV, iOS, tvOS and Plasma Big Screen. Android has been run on emulators only, and iOS and tvOS have never been built. A web browser is the way to reach everything today. See [Apps and devices](docs/APPS.md).

## Guides

**Everything here is also inside the app, under Help,** so you don't need this page once you're running.

| Getting going | |
|---|---|
| [Getting started](docs/GETTING_STARTED.md) | From install to your first film |
| [Installing](docs/INSTALL.md) | Downloads, the tray app, services, Docker, updating and removing it |
| [Libraries](docs/LIBRARIES.md) | Adding your media, and naming your files |
| [Accounts and profiles](docs/ACCOUNTS.md) | People, passwords, limits and sign-in |
| [Apps and devices](docs/APPS.md) | Every device, and how far each has been run |
| [Settings tour](docs/SETTINGS.md) | Where every setting lives |
| [Troubleshooting](docs/TROUBLESHOOTING.md) | Common problems, by what you're seeing |

| Your media | |
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

| Running a server | |
|---|---|
| [Metadata](docs/METADATA.md) · [Fixing wrong details](docs/FIXING-METADATA.md) · [The metadata folder](docs/METADATA-FOLDER.md) | Posters, plots and wrong matches |
| [Live transcoding](docs/TRANSCODING.md) · [Library transcoding](docs/LIBRARY-TRANSCODING.md) · [Using Tdarr](docs/TDARR.md) | Converting video |
| [Modules and the store](docs/MODULE-STORE.md) | Adding, updating and trusting modules |
| [Remote access](docs/REMOTE.md) | Watching away from home, safely |
| [Backup and restore](docs/BACKUP.md) · [Repair](docs/REPAIR.md) · [WittyDiagnoser](docs/DIAGNOSER.md) | Keeping it healthy |
| [Config file and logs](docs/CONFIG.md) · [Statistics](docs/STATS.md) · [Themes](docs/THEMES.md) | Fine tuning |

**For developers:** [Contributing](CONTRIBUTING.md), [Repositories](docs/REPOSITORIES.md), [Reference](docs/REFERENCE.md), [Client API](docs/CLIENT-API.md), [Making a module](docs/MODULES.md), [Security model](docs/SECURITY-REVIEW.md) and the [Changelog](CHANGELOG.md).

## Privacy and security

- **No telemetry, no accounts with anyone, no cloud.** Logs stay on your computer and never contain passwords or session tokens.
- **Your files stay here.** By default, names taken from your file and folder names go to the metadata services you keep switched on, and the server asks GitHub for updates twice a day. **Settings → Legal → Privacy** lists every outside service, what it receives and what controls it.
- **Limits by default.** Cross-site requests are refused, feeds and agents can't reach other devices on your network, and modules run in a sandbox where the system offers one.
- **Copy protection is never bypassed.** Encrypted discs, broadcasts and game files are detected and refused.

To report a security problem, see [SECURITY.md](SECURITY.md).

## Contributing

Issues and pull requests are welcome. [CONTRIBUTING.md](CONTRIBUTING.md) has how to build, test and send a change. Disc engines must stay free of other players' code, because they're written from the public disc formats.

## Licence and legal

**[Apache License 2.0](LICENSE).** Use, change and share WittyMediaServer, commercially or not, keeping the licence and the attribution in **[NOTICE](NOTICE)**: *"This product includes WittyMediaServer, developed by WittyWilla"*, in your distribution, documentation or About screen. A few separate programs have licences of their own, and each is listed in the app and in [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md).

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

<p align="center"><sub>WittyMediaServer 0.6.0-alpha · Apache-2.0 · Made by <a href="https://github.com/wittywilla">WittyWilla</a></sub></p>
