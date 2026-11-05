# Config file and folders

Set what the server has to know before it starts: its port, where it keeps its data, and a few security switches that a web page can't change.

Everyday settings are in **Settings**, and you never need a config file. Without one the server uses its defaults.

- [The port](#the-port)
- [Where the config file goes](#where-the-config-file-goes)
- [What you can set](#what-you-can-set)
- [Security switches](#security-switches)
- [Variables with no setting](#variables-with-no-setting)
- [Setting a variable for a service](#setting-a-variable-for-a-service)
- [Portable mode](#portable-mode)
- [Folders the server uses](#folders-the-server-uses)
- [Logs](#logs)
- [For developers](#for-developers)

## The port

The port is the number at the end of the server's address: `9797` in `http://192.168.1.20:9797`. Change it if another program on the computer needs 9797, or you'd rather use a port of your own. You don't need a config file for it.

**The easy way: Settings → Network**, for administrators. Type the new port, and it's checked as you type: a number from 1 to 65535 that nothing else on the computer is using, with a warning for ports below 1024, and a refusal for the ports browsers won't open. Choose **Save**, then **Restart now**, and the page opens again at the new address. The last page of the setup wizard has the same, under **Change the port**.

**By hand: `settings.ini`.** A short text file beside the program, in the same folder as `WittyMediaServer.exe`. Open it in Notepad or any text editor, change the number after `port =`, save it, and restart the server. Lines starting with `#` are notes.

```ini
# The port browsers and apps connect to
port = 8096
```

Where the program's folder can't be changed (Program Files, `/usr`, an app on a Mac), Settings saves in a copy in the server's [data folder](#folders-the-server-uses) instead, and that copy wins over the one beside the program. You can change that one by hand too:

| How it's installed | The copy Settings saves in |
|---|---|
| Windows, the background service | `C:\ProgramData\WittyMediaServer\settings.ini` |
| Windows, the app with the tray icon | `%LOCALAPPDATA%\WittyMediaServer\settings.ini` |
| Windows, unzipped | `settings.ini` beside `WittyMediaServer.exe` |
| macOS | `~/Library/Application Support/WittyMediaServer/settings.ini`, never inside the app: a changed app breaks its signature |
| Linux `.deb` or `.rpm`, or `install.sh` | `/var/lib/wittymediaserver/settings.ini` |
| FreeBSD package | `/var/db/wittymediaserver/settings.ini` |
| [Portable mode](#portable-mode) | `data/settings.ini` |
| Docker | None: change the published port in `docker-compose.yml`, below |

From the command line, `wittymediaserver config port` says the port, where it comes from and where Settings saves it, and `wittymediaserver config port 8096` saves one. Run it as the account the server runs as, or as an administrator.

A line the server can't use, such as a typo, is skipped: the server still starts, on the port it would use without that line, and **Settings → Network** and the log say which line it was.

**What wins.** Strongest first: `-listen`, `WMS_LISTEN`, `listen` in `wms.json`, the data folder's `settings.ini`, the `settings.ini` beside the program, then 9797. When one of the first three is set, **Settings → Network** says where, and doesn't save somewhere that wouldn't count. The installers don't set any of them.

**Upgrading from 0.6.** A port you gave the service before carries over into the data folder's `settings.ini` by itself, so it goes on answering where it did: the Windows installer's `PORT` (the service's `-listen`), a `WMS_LISTEN` in a `systemctl edit` override (commented out there afterwards), `wittymediaserver_listen` in FreeBSD's `rc.conf` (removed afterwards), `install.sh --port`, and the Mac service's `WMS_LISTEN`. One with an address in front, such as `127.0.0.1:9797`, stays where it is and keeps winning. A port you changed by hand in the `settings.ini` beside a Linux or FreeBSD package's program is carried into the data folder's copy before an upgrade replaces that file.

**What follows the port.** Apps on the home network find the server again by themselves: they ask on UDP port 9797, which doesn't change. The tray or menu-bar icon opens the new address, and finds a background service on any port. The Windows installer's firewall rule is for the server program, so it covers any port. Remote Access keeps its own HTTPS port, so a router's port forwarding stays as it is. A firewall of your own, a reverse proxy, a VPN address you set up, bookmarks, and devices set up by typing the address need the new port.

**Docker.** The port inside the container stays 9797, and the image sets `WMS_LISTEN` to it. Change the first number of the published port in `docker-compose.yml`, such as `"8096:9797"`, then run `docker compose up -d`. **Settings → Network** says so.

## Where the config file goes

The file is called `wms.json`. The server uses the first one it finds:

| Order | Where | When to use it |
|---|---|---|
| 1 | The file named by `-config <file>` or `WMS_CONFIG` | Services, scripts, several servers on one machine |
| 2 | **In the program's top folder,** beside `WittyMediaServer.exe` and `bin` | [Portable mode](#portable-mode) |
| 3 | Your own config folder, listed below | A normal install |
| 4 | FreeBSD only: `/usr/local/etc/wittymediaserver/wms.json` | System services |
| 5 | Linux and BSD: `/etc/wittymediaserver/wms.json` | System services |

Your own config folder is:

| System | File |
|---|---|
| Windows | `%APPDATA%\WittyMediaServer\wms.json` |
| macOS | `~/Library/Application Support/WittyMediaServer/wms.json` |
| Linux, BSD | `~/.config/wittymediaserver/wms.json`, or under `$XDG_CONFIG_HOME` if that's set |

To write a starter file, and to see which file and folders are in use:

```bash
wittymediaserver config init
wittymediaserver config path
```

A wrong name in the file stops the server with a clear message, instead of being ignored. `wittymediaserver doctor` works even then: see [Repair](REPAIR.md).

**A service with a config file.** On Linux use `/etc/wittymediaserver/wms.json`, and on FreeBSD `/usr/local/etc/wittymediaserver/wms.json`. On a Mac the service runs as you, so it uses your own. On Windows, put it in `C:\Program Files\WittyMediaServer`.

**Packaged services and the image set the data folder themselves.** The Linux, FreeBSD and Mac services set `WMS_DATA_DIR`, the Windows service passes `-data`, and the Docker image sets `WMS_TRANSCODE_DIR` and `WMS_LISTEN` too. These win over the file, so `dataDir` in a config file does nothing there. The port is in [settings.ini](#the-port). See [Installing](INSTALL.md) for how to change them on each system.

## What you can set

Every setting is optional. Most can be given three ways, and the strongest wins: a flag, then a variable, then the file. Flags go after `wittymediaserver serve`.

| In `wms.json` | Flag | Variable | What it does, and its default |
|---|---|---|---|
| `listen` | `-listen` | `WMS_LISTEN` | The address and port. `:9797` is every network card on port 9797, and `127.0.0.1:9797` keeps it to this computer. Each of these wins over the port in [settings.ini](#the-port), which is where to change only the port, and what Settings changes. Default: the port in `settings.ini`, or `:9797` |
| `dataDir` | `-data` | `WMS_DATA_DIR` | The server's own folder: database, settings, users, watch history. Default: [see below](#folders-the-server-uses) |
| `metadataDir` | | `WMS_METADATA_DIR` | Downloaded posters, backdrops and generated thumbnails. Default: `images` in the cache folder |
| `cacheDir` | | `WMS_CACHE_DIR` | Other cached files, such as disc playlists and menu stills. It stays within limits: 4 GB of thumbnails, 1 GB of stills, 4 GB of unpacked comics, oldest first, and what's removed is made again when it's needed. Default: `cache` in the data folder |
| `transcodeDir` | | `WMS_TRANSCODE_DIR` | Temporary video while transcoding. Put it on a fast SSD or RAM disk. Default: `transcode` in the data folder |
| `logDir` | `-log-dir` | `WMS_LOG_DIR` | Log files. It can also be changed in **Settings → WittyLog**. Default: `logs` in the data folder |
| `logLevel` | `-log` | `WMS_LOG` | `error`, `warn`, `info` (the default) or `debug`. `debug` makes every area of the log as detailed as it goes, whatever **Settings → WittyLog** says |
| `ffmpeg`, `ffprobe` | `-ffmpeg`, `-ffprobe` | `WMS_FFMPEG`, `WMS_FFPROBE` | Use these FFmpeg programs. Default: the ones in `bin`, then `PATH` |
| `maxTranscodes` | `-max-transcodes` | `WMS_MAX_TRANSCODES` | How many videos may be transcoded at once. Lower it on small machines. When all are in use, a stream nobody has asked anything of for 20 seconds gives way, otherwise the next viewer waits. Default: `4` |
| `maxConnections` | `-max-connections` | `WMS_MAX_CONNECTIONS` | How many connections can be open at once, for the whole server. One address (an IPv6 network counts as one) can hold 128 of them, except this computer and trusted reverse proxies, and 64 more are kept for the home network. At least 16. Default: `2048` |
| `publicUrl` | `-public-url` | `WMS_PUBLIC_URL` | The address people use from outside your home, such as `https://media.example.com`. Setting it also turns remote access on |
| `openBrowser` | `-open-browser=false` | `WMS_NO_BROWSER=1` | Whether to open the setup page in your browser the first time the server starts. Default: yes |
| `portable` | | | Keep everything next to the program. Default: `false`. See [Portable mode](#portable-mode) |

These are in [Security switches](#security-switches):

| In `wms.json` | Flag | Variable |
|---|---|---|
| `setupFromHome` | `-setup-from-home` | `WMS_SETUP_FROM_HOME=1` |
| `allowNetworkPaths` | `-allow-network-paths` | `WMS_ALLOW_NETWORK_PATHS=1` |
| `remoteAccess` | `-remote-access` | `WMS_REMOTE_ACCESS=1` |
| `trustedProxies` | `-trusted-proxies` | `WMS_TRUSTED_PROXIES` |
| `javaPath` | `-java` | `WMS_JAVA` |
| `gamingCustomCores` | `-gaming-custom-cores` | `WMS_GAMING_CUSTOM_CORES=1` |

Each is off, or empty, until you set it. A switch that's on in the file can't be turned off by a variable: remove it from the file.

**Paths.** A relative path, like `"data"`, is relative to the folder the config file is in.

**An example:**

```json
{
  "dataDir": "D:\\WittyMedia\\data",
  "metadataDir": "E:\\Artwork",
  "transcodeDir": "R:\\transcode",
  "logLevel": "info",
  "maxTranscodes": 4
}
```

## Security switches

These are here, and not in Settings, on purpose. Each one changes what someone with a web session can do or reach, so it's set on the machine, by whoever runs it.

| Name | What it does | Why it's off |
|---|---|---|
| `setupFromHome` | Lets anyone on the home network make the first account without the [setup code](INSTALL.md#at-home-and-away) | Whoever makes the first account owns the server. Off, only the server's own desktop goes without the code |
| `allowNetworkPaths` | Lets settings changed from another device name a network share, such as `\\nas\media`, or a device path | Reaching a share signs in there with the server's own account, on Windows the machine's. Without it, choose a share on the server's own desktop, or use a mapped drive or a mount. Libraries already on a share go on working |
| `remoteAccess` | Allows connections from outside your home without using **Settings → Remote Access**, for a server nobody can reach from a home network first, such as a rented one. It also locks the switch on that page. See [Remote access](REMOTE.md) | Nothing answers from outside until you ask for it |
| `trustedProxies` | Reverse proxies whose `X-Forwarded-For` is believed: addresses or networks, separated by commas. These are added to the ones in **Settings → Remote Access** | Anyone else could claim to be at home |
| `javaPath` | The Java folder, or the `java` program in it, for Blu-ray menus. The server looks for it by itself if this is empty. See [Blu-ray](BLURAY.md) | A web account can't choose what the server runs. A location saved in Settings by an older version is ignored |
| `gamingCustomCores` | Lets custom game cores be turned on in **Settings → Consoles & cores**. See [Your own core](GAMES.md#your-own-core) | A core is a program, and can read what the server can |

Three more are variables only, with no setting and no flag, so a web session and the config file have no say:

| Variable | What it does |
|---|---|
| `WMS_NO_MODULE_FORKS=1` | Keeps modules from other publishers off, whatever an administrator asks, and makes the server forget one already chosen. See [Using someone's fork](MODULE-STORE.md#using-someones-fork) |
| `WMS_GAMING_NO_CUSTOM_CORES=1` | Keeps custom game cores off, whatever else is set |
| `WMS_TUNER_COMMANDS=1` | Allows the custom command tuner, which runs a program on the server |

## Variables with no setting

| Variable | What it does |
|---|---|
| `WMS_CONFIG` | The config file to use, the same as `-config` |
| `WMS_BACKUP_PASSWORD` | The password for `wittymediaserver backup`, as a password is never taken on the command line. See [Backup](BACKUP.md) |
| `WMS_DOCKER=1` | Says this is a container, so the server doesn't try to replace its own files. The image sets it |
| `XDG_DATA_HOME`, `XDG_CONFIG_HOME` | On Linux and BSD, the system's own settings for where a person's data and config folders are |

## Setting a variable for a service

Where there's a setting in `wms.json`, that's the simplest way. For a variable with none:

| How it runs | What to do |
|---|---|
| The tray app | Set it in your account's environment, then choose **Quit** and open the app again |
| Linux service | `sudo systemctl edit wittymediaserver`, add `[Service]` and `Environment=WMS_NO_MODULE_FORKS=1`, save, then `sudo systemctl restart wittymediaserver` |
| FreeBSD service | `sysrc wittymediaserver_env="WMS_NO_MODULE_FORKS=1"`, then `service wittymediaserver restart` |
| Windows service | As an administrator: `reg add HKLM\SYSTEM\CurrentControlSet\Services\WittyMediaServer /v Environment /t REG_MULTI_SZ /d WMS_NO_MODULE_FORKS=1`, then `sc.exe stop WittyMediaServer` and `sc.exe start WittyMediaServer` |
| Mac service | Add it under `EnvironmentVariables` in `/Library/LaunchDaemons/com.wittywilla.wittymediaserver.plist`, then `sudo launchctl bootout system <that file>` and `sudo launchctl bootstrap system <that file>`. A new `.pkg` writes this file again, keeping only the data folder and an address set with `WMS_LISTEN`, so add it again after updating |
| Docker | Under `environment:` in the compose file, or `-e` with `docker run` |

## Portable mode

Portable mode keeps **everything** in one folder: program, database, artwork and logs. You can run the server from a USB drive or an external disk, or keep several separate copies.

To make an unpacked folder portable, put a `wms.json` in its top folder, beside `bin`:

```json
{ "portable": true }
```

Or run `wittymediaserver config init --portable`. Any `wms.json` in that folder makes a copy portable, whatever it says. The data then lives in a `data` folder beside the program.

Keep a portable server on a drive that stays plugged in while it runs. Back up the whole folder to keep your libraries, users and watch history.

## Folders the server uses

Without a config file the data folder is:

| How it runs | Data folder |
|---|---|
| Windows, in your account | `%LOCALAPPDATA%\WittyMediaServer` |
| Windows, as the service | `C:\ProgramData\WittyMediaServer` |
| macOS | `~/Library/Application Support/WittyMediaServer` |
| Linux or BSD, in your account | `~/.local/share/wittymediaserver`, or under `$XDG_DATA_HOME` if that's set |
| The Linux package's service | `/var/lib/wittymediaserver` |
| The FreeBSD package's service | `/var/db/wittymediaserver` |
| Docker | `/config` |

**The command line uses the same rules,** so run it as the same account, or say which folder:

- As root, on a `.deb`, `.rpm` or FreeBSD package, it uses the service's folder.
- With a copy from `install.sh`, add `-data /var/lib/wittymediaserver`.
- For the Windows service, add `-data C:\ProgramData\WittyMediaServer`, in a terminal opened with **Run as administrator**.

**Inside the data folder:**

- `wms.db` is the database. `cache/images` holds the artwork, and `transcode` is temporary.
- `backups` holds the [backups](BACKUP.md) you make, and the safety copies from restores and repairs. A reset that keeps a copy leaves it where it is.
- `metadata` holds one plain text file for each title, to fix by hand. It's filled when you export it, and nothing reads it until you apply it. See [The metadata folder](METADATA-FOLDER.md). It isn't the same as `metadataDir`, the artwork folder.
- [Uninstalling](UNINSTALL.md#what-goes-and-what-stays) lists every name.

**To move the artwork folder:**

1. Stop the server.
2. Move the folder's contents to the new place.
3. Set `metadataDir`.
4. Start the server.

Artwork is referenced by file name, so nothing has to be scanned again. The same works for `transcodeDir`, which only holds temporary files.

## Logs

Logs are plain text files named `wms-<date>.log`, one line per message, in the log folder. A new file starts after 20 MB. They're kept for 14 days and 200 MB in all, whichever comes first. If the disk has under 100 MB free, only warnings and errors are written and old logs are removed. If the folder can't be written, the log stays in memory and the settings page says so.

**Logs never leave your server.** Nothing is uploaded, and nobody sees them unless you send a file yourself. Passwords, sign-in tokens, stream tokens, API keys, PINs, cookies and setup codes are removed from every line.

**Settings → WittyLog** has a live viewer with explanations in plain words, a level for each area and module, **Catch a problem**, **Crash reports**, and **Download logs as ZIP** with **Sanitize for sharing**. See [Logs](LOGGING.md).

With no server running, for a server that won't start:

```bash
wittymediaserver logs zip [-data <dir>] [-o out.zip] [-days N] [--raw]
wittymediaserver logs sanitize [-data <dir>] [-o out.txt] <file>
```

The first makes a ZIP, sanitized unless you add `--raw`. The second makes a sanitized copy of any text log. They only read. The database is looked at for the names to hide. If it's too broken to read they say so and carry on, replacing everything that looks like a name, address or path.

## For developers

| Variable or flag | What it does |
|---|---|
| `-dev`, `WMS_DEV=1` | Serves `web/dist` from disk instead of the built-in copy |
| `-watch-stdin`, `WMS_WATCH_STDIN=1` | Exits when stdin closes. The tray app uses it |
| `WMS_RESTARTED=1` | Set by the server for the copy it starts after an update, so that it keeps trying the port |
| `WMS_SUPERVISED=1` | Says a service manager starts the server again when it exits, so a restart asked for in Settings leaves that to it. The Windows service, the FreeBSD service and the Mac service set it; systemd is recognised by itself |
| `WMS_WITTYPLAY_RUNNER` | Runs games with this program, in place of what the module brought |
| `WMS_WITTYCOUCH_HOST` | Uses this program as the WittyCouch host |
| `WMS_GAMING_TESTCORE=1` | Offers the built-in test console |

Only a build with the `updatetest` tag reads `WMS_UPDATE_FEED`, `WMS_UPDATE_KEY`, `WMS_MODULE_FEED` and `WMS_MODULE_CATALOG`.

Build scripts read `WMS_SIGNING_KEY`, `WMS_STARTER`, `WMS_PREBUILT`, `WMS_MODULE_REPOS` and `WMS_FORBIDDEN_NAMES`. The rest are for tests, and do nothing in a running server: `WMS_TEST_*`, `WMS_REAL_*`, `WMS_FORMAT_*`, `WMS_MOCK_*`, `WMS_SANDBOX_*`, `WMS_CRASH_*`, `WMS_GAME_*`, `WMS_GAMES_FOLDER`, `WMS_HOSTILE_FULL`, `WMS_LOG_TEST_EXIT`, `WMS_TDARR_*`, `WMS_UPDATE_OPENAPI`, `WMS_NAME`, `WMS_PASS`, `WMS_SERVER` and `WMS_TMP`.

How every part of the project logs is in [Logs](LOGGING.md).

---

<sub>WittyMediaServer documentation</sub>
