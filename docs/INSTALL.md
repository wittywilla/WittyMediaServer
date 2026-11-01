# Installing WittyMediaServer

Everything is in the download, FFmpeg included, with nothing else to install first. New to all this?
[Getting started](GETTING_STARTED.md) walks through it with no command line.

- [Downloads](#downloads)
- [Windows](#windows)
- [macOS](#macos)
- [Linux](#linux)
- [FreeBSD](#freebsd)
- [Removing it](#removing-it)
- [What the programs are](#what-the-programs-are)
- [The tray app](#the-tray-app)
- [Updating](#updating)
- [Docker](#docker)
- [A NAS](#a-nas)
- [Kubernetes](#kubernetes)
- [At home and away](#at-home-and-away)
- [Behind a reverse proxy](#behind-a-reverse-proxy)

## Downloads

From the **Releases** page. Most people want the first file in their row.

| Platform | Installer | Without installing |
|---|---|---|
| Windows 10/11 (x64, ARM64) | `…-windows-amd64.msi`, `…-windows-arm64.msi` | `…-windows-amd64.zip`, `…-windows-arm64.zip` |
| macOS 11+ (Apple Silicon, Intel) | `…-macos-arm64.pkg`, `…-macos-amd64.pkg` | `…-darwin-arm64.tar.gz`, `…-darwin-amd64.tar.gz` |
| Debian, Ubuntu (x64, ARM64, ARMv7, x86) | `…-linux-amd64.deb`, and `arm64`, `armv7`, `386` | `…-linux-amd64.tar.gz`, and the same |
| Fedora, RHEL, openSUSE (x64, ARM64, ARMv7, x86) | `…-linux-amd64.rpm`, and the same | `…-linux-amd64.tar.gz`, and the same |
| FreeBSD 14+ (x64, ARM64) | `…-freebsd-amd64.pkg`, `…-freebsd-arm64.pkg` | `…-freebsd-amd64.tar.gz`, `…-freebsd-arm64.tar.gz` (ARM64 uses FreeBSD's FFmpeg: `pkg install ffmpeg`) |
| Docker | `ghcr.io/wittywilla/wittymediaserver` (amd64, arm64, armv7) | |

Each release has a `…-SHA256SUMS.txt` file to check downloads, and its signature
(`…-SHA256SUMS.txt.sig`), which the server checks before it installs an update.

## Windows

Open the `.msi`. If SmartScreen warns about an unknown publisher, choose *More info*, then *Run anyway*:
alpha builds aren't code-signed yet. It asks two things.

**Who is it for?**

| | Just me | Everyone on this computer |
|---|---|---|
| Needs an administrator | No | Yes |
| Program | `%LOCALAPPDATA%\Programs\WittyMediaServer` | `C:\Program Files\WittyMediaServer` |
| Library, accounts and settings | `%LOCALAPPDATA%\WittyMediaServer` | each person's own, or with the service, `C:\ProgramData\WittyMediaServer` |
| Update from Settings | Yes | Run the new installer |
| Can run as a service | No | Yes |

**How should it run?** Each of these is optional.

- **Start when I sign in**: the tray app, starting by itself. You can turn this off from its icon. If it's
  installed for everyone, turn it off in Windows Settings, Apps, Startup.
- **A shortcut on the desktop**, and **the `wittymediaserver` command** in every new terminal.
- **Run as a background service, always on** (for everyone only): it runs even when nobody is signed in,
  under the limited `LOCAL SERVICE` account. Give that account permission to read your media folders:
  a folder's *Properties*, *Security*, *Edit*, *Add*, type `LOCAL SERVICE`. It can't reach a network share by a
  mapped drive letter, so use `\\server\share`, on a share that lets it in.
- **Let other devices on my home network connect** (for everyone only): adds a Windows Firewall rule for
  networks you've marked private. It opens nothing to the internet. Installed just for you, Windows asks
  when the server first starts: choose *Allow*.

Running a newer installer over an installed copy updates it in place and keeps your choices. It takes over
from a copy that 0.5.2-alpha or older installed, as well: the old program, boot task and firewall rule go, and your
library stays where it is.

**For admins.** Every choice is a property, and `/qn` asks nothing:

```powershell
msiexec /i wittymediaserver-….msi /qn MSIINSTALLPERUSER=1 AUTOSTART=1 DESKTOPSHORTCUT=1 ADDTOPATH=1
msiexec /i wittymediaserver-….msi /qn MSIINSTALLPERUSER="" INSTALLSERVICE=1 ADDFIREWALL=1 AUTOSTART="" PORT=9797
```

**Without installing.** Unzip the `.zip` anywhere, and open `WittyMediaServer.exe`. Add a
[`wms.json`](CONFIG.md) to keep everything in that folder, on a USB stick for instance.

## macOS

Right-click the `.pkg`, choose *Open*, and *Open* again: alpha builds aren't signed yet. It puts
**WittyMediaServer** in Applications, and the `wittymediaserver` command in `/usr/local/bin`. *Customize* lets you leave
out **Run in the background**, which otherwise starts the server when the Mac starts, as you, even when nobody
is signed in. Your library is in `~/Library/Application Support/WittyMediaServer`.

Running a newer `.pkg` updates it in place, and takes over from what the `install.sh` of 0.5.2-alpha or older put in `/opt`,
keeping its library.

## Linux

```bash
sudo apt install ./wittymediaserver-….deb    # Debian, Ubuntu and relatives
sudo dnf install ./wittymediaserver-….rpm    # Fedora, RHEL and relatives
```

That's all: the service is running. Open `http://<this computer>:9797/` to finish setting up.

| | |
|---|---|
| Its status and log | `systemctl status wittymediaserver`, `journalctl -u wittymediaserver` |
| Program | `/usr/lib/wittymediaserver`, with the command at `/usr/bin/wittymediaserver` |
| Library, accounts and settings | `/var/lib/wittymediaserver` |
| Runs as | its own account, `wms`, which needs read permission on your media folders |
| Change the port, data folder or media access | `sudo systemctl edit wittymediaserver`, and see `/usr/lib/systemd/system/wittymediaserver.service` |

Installing the same way updates it in place. It takes over from what the `install.sh` of 0.5.2-alpha or older installed, keeping
its data folder, port and account. A desktop gets **WittyMediaServer** in its app menu: the tray app shows the
service, and doesn't start a second server.

**Another distribution.** Unpack the `.tar.gz`, and:

```bash
sudo ./install.sh                      # /opt/wittymediaserver, data in /var/lib/wittymediaserver, port 9797
sudo ./install.sh --prefix /srv/wms --data /tank/wms --port 8080 --user media
```

Or just run `./WittyMediaServer`, or `./bin/wittymediaserver serve` with no tray icon.

Run either a service or the tray app on a port, not both by hand: the tray app notices a server already
answering, and only shows it.

## FreeBSD

```sh
pkg add wittymediaserver-…-freebsd-amd64.pkg
sysrc wittymediaserver_enable=YES
service wittymediaserver start
```

Then open `http://<this computer>:9797/` to finish setting up. As with FreeBSD's own packages, it doesn't start
until you enable it.

| | |
|---|---|
| Program | `/usr/local/lib/wittymediaserver`, with the command at `/usr/local/bin/wittymediaserver` |
| Library, accounts and settings | `/var/db/wittymediaserver`, and the log is `wms.log` in there |
| Runs as | its own account, `wms`, which needs read permission on your media folders |
| Change the account, data folder or port | `sysrc wittymediaserver_user=…`, `wittymediaserver_data=…`, `wittymediaserver_listen=:8080` |
| A settings file | `/usr/local/etc/wittymediaserver/wms.json` |

`pkg add -f` with a newer package updates it in place, and `service wittymediaserver start` brings it back. It takes over from what `install.sh` installed, keeping
its data folder, port and account. A package made for FreeBSD 14 needs `env IGNORE_OSVERSION=yes pkg add …` on 15.

Prefer not to use a package? Unpack the `.tar.gz` and run `./install.sh`, with the same options as on Linux. It keeps
everything inside its folder, and starts the service straight away.

## Removing it

It asks in plain words whether to keep your library, accounts and settings. Keep them, and they're there if you
install it again. A full clean-up removes **everything WittyMediaServer made**: program files, services, scheduled tasks,
firewall rules, startup entries, shortcuts, registry keys, the library database, settings, caches, logs, downloaded
modules and game cores. Then it lists what it removed, and what it couldn't.

| | How |
|---|---|
| **Windows** | *Settings, Apps, Installed apps*, WittyMediaServer, *Uninstall*. For scripts: `"Uninstall WittyMediaServer.exe" /S` keeps your data, and `/S /RemoveData` is a full clean-up. It finishes in the background: `--report <file>` says what it did. `msiexec /x` also works, and keeps your data |
| **macOS** | Open **Uninstall WittyMediaServer** in Applications |
| **Debian, Ubuntu** | `sudo wittymediaserver --uninstall` asks. Or `sudo apt remove wittymediaserver` keeps your data, and `sudo apt purge wittymediaserver` removes it |
| **Fedora, RHEL** | `sudo wittymediaserver --uninstall` asks. `sudo dnf remove wittymediaserver` keeps your data |
| **FreeBSD** | `wittymediaserver --uninstall`, as root, asks. `pkg delete wittymediaserver` keeps your data |
| **Anywhere**, even if the installer has gone missing | `wittymediaserver --uninstall`, from the program's `bin` folder if the command isn't found |

The command line can also skip the questions: `--keep-data` or `--remove-data`, `--remove-recordings`,
`--remove-backups`, `--yes`, and `--dry-run` to see what it would do first.

**What it never touches.** Your media: it reads your library folders from the database, and a folder that holds one
stays, whatever it's called. Anything it didn't make, such as a file you put in the data folder: that stays, and it
tells you. Recorded TV and backups have a question each, and a silent or `apt purge` removal leaves them.
Artwork and details saved beside your media files stay too, as they're yours to keep with those files.
A folder you chose yourself in [`wms.json`](CONFIG.md) or Settings, such as a cache on another drive, is listed for you to delete.

**Other people's settings.** Run as an administrator or with `sudo`, it also clears what other accounts on the computer have.

## What the programs are

You only ever need to open the first one.

| Program | What it does |
|---|---|
| `WittyMediaServer.exe` (Windows), `WittyMediaServer.app` (Mac), **WittyMediaServer** in the app menu (Linux) | **The app you open.** Puts an icon in the tray or menu bar, starts the server and opens the web app |
| `Uninstall WittyMediaServer.exe`, `Uninstall WittyMediaServer.app` | Removes it, asking first |
| `bin/wittymediaserver` | The server itself, and the command line for repairs and support: try `wittymediaserver help` |
| `bin/ffmpeg`, `bin/ffprobe` | FFmpeg: converts video when a device needs it, and reads what's in a file |
| `bin/wittyplay-game-runner` | Runs one game, in a process of its own |
| `wms` | Only in the `.zip` and `.tar.gz`, so a copy of 0.5.2-alpha can update to this one. Safe to delete otherwise |

On Windows each says what it is in its Properties, and in Task Manager's *Details*. On a Mac the same programs are
inside the app, under `Contents/MacOS` and `Contents/Helpers`.

## The tray app

An icon appears in the system tray or menu bar:

| Menu item | What it does |
|---|---|
| **● Running at http://localhost:9797** | live status |
| **Open WittyMediaServer** | opens the web app (the setup wizard the first time) |
| **Stop / Start / Restart server** | not when a background service is running it: the service looks after itself |
| **Open data folder**, **View log** | |
| **Start at login** | Windows Run key, macOS LaunchAgent or XDG autostart |
| **Quit** | stops the server, including any FFmpeg jobs, and exits |

The app restarts the server if it crashes, and stops trying after three quick failures so a bad configuration
doesn't loop. The icon turns grey while the server is stopped.

## Updating

Admins see a dot on **Settings** when a newer version is out. *Settings → About → Updates*
shows the installed and the newest version with its release notes; **Update now** installs it:

1. **Downloading**: the release's checksum list and its signature come from GitHub (only from
   github.com and its download servers, over HTTPS).
2. **Verifying**: the checksum list must be signed with the WittyMediaServer release key built into
   the server, and the downloaded package must match its checksum. Anything else is refused and
   nothing changes.
3. **Installing**: the new files are unpacked next to the program and swapped in. Running programs
   are renamed to `*.old` (Windows allows that) and deleted on the next start. The `data` folder,
   `wms.json` and any other folder the server keeps data in are never written.
4. **Restarting**: the tray app starts the new version (systemd does it for the Linux service;
   otherwise the server starts its successor itself). The page reloads by itself.

The server looks for new versions twice a day (*Check for updates automatically*, on by default;
**Check now** asks at once). It only fetches GitHub's list of releases: nothing about you or your
library is sent, and nothing is installed until an admin clicks **Update now**.

In-place updates need a copy installed from a release package in a folder the server can write.
Otherwise Settings says why and links to the download:

| Installed with | How to update |
|---|---|
| Windows installer, *Just me*, or an unpacked folder | **Update now** |
| Windows installer, *Everyone on this computer* | run the new installer, as Windows protects Program Files |
| `.deb`, `.rpm`, FreeBSD `.pkg` | install the new package the same way, as your package manager owns these files |
| `install.sh` (`/opt/…`, owned by root) | unpack the new archive and run `sudo ./install.sh` again |
| macOS | run the new `.pkg` |
| Docker / Kubernetes | pull the new image and recreate the container |

A copy of 0.5.2-alpha or older takes the update as well, from a folder it can write. The server moves to `bin/`
at the same time, and a small `wms` stays where the old server was, so whatever started it still can.

## Docker

```bash
docker run -d --name wittymediaserver --restart unless-stopped \
  -p 9797:9797 \
  -v wms-config:/config \
  -v /path/to/movies:/media/movies:ro \
  -v /path/to/discs:/media/discs:ro \
  ghcr.io/wittywilla/wittymediaserver:alpha
```

Or `docker compose up -d` with [`docker-compose.yml`](../docker-compose.yml). For hardware
transcoding pass `--device /dev/dri` (VAAPI) or use the NVIDIA container runtime, then tick the
encoder under *Settings → Live transcoding*. The image runs as UID 1000: `chown` your media or add
`--user`.

The command line is there too: `docker exec wittymediaserver wittymediaserver doctor`. Remove the container and its
`wms-config` volume, with `docker compose down -v`, for a full clean-up. It holds no media, as those are mounted from your disks.

## A NAS

Most NAS systems run containers, and that's the way to do it: a packaged app for each make isn't offered.

| | |
|---|---|
| **Synology** (Container Manager), **QNAP** (Container Station), **TrueNAS SCALE**, **Unraid** | Create a project or stack from [`docker-compose.yml`](../docker-compose.yml) |
| The `/config` volume | Keep it on a fast disk, an SSD if you have one, in a shared folder of its own. It holds the database and artwork |
| Your media | Mount each share read-only at a folder under `/media`, as the file shows |
| Permissions | The image runs as UID 1000. Set `user:` in the compose file to your NAS account's UID and GID, which `id` shows over SSH, and give that account read access to your media and write access to `/config` |
| Hardware transcoding | Add `devices: ["/dev/dri:/dev/dri"]` on Intel and AMD, and tick the encoder under *Settings → Transcoding* |
| An older or 32-bit NAS | `linux/arm/v7` and `amd64` images exist, without a 32-bit x86 one: the `…-linux-386.tar.gz` archive runs on those by hand |

If your NAS runs Debian underneath, as OpenMediaVault does, the `.deb` works too.

## Kubernetes

```bash
kubectl apply -k deploy/k8s
```

Edit [`deploy/k8s/deployment.yaml`](../deploy/k8s/deployment.yaml) for your media volume (NFS,
hostPath, CSI) and [`ingress.yaml`](../deploy/k8s/ingress.yaml) for your hostname and TLS. The
deployment uses `Recreate` (SQLite has one writer) and an in-memory `emptyDir` for transcoding; the
config volume holds the database and artwork.

## At home and away

WittyMediaServer runs on your own computer, in your home. **Out of the box it is available on your
home network only**: every device on the same Wi-Fi or cable network can open
`http://<this computer's address>:9797` (the setup wizard and *Settings → About* show the address).

**Setting up from another device.** Until the first account exists, whoever makes it owns the server. On
the computer running it, setup just works. From any other device, your phone included, setup asks for a
**setup code**, because someone else on your network could otherwise get there first. Find it in either place:

- on the server's console when it starts (`This server has no accounts yet...`, in `docker logs` or in
  `journalctl -u wittymediaserver`);
- in `setup-code.txt` in the server's data folder, which goes away once setup is done.

It is never kept in the log files or shown in the log viewer. If you trust everyone on your network and would
rather not, start the server with `-setup-from-home` (or `WMS_SETUP_FROM_HOME=1`).

A server nobody can reach from a home network first (a rented one, say) also needs `remoteAccess` turned on
in its configuration ([CONFIG.md](CONFIG.md)), and HTTPS.

**Windows firewall rule from the installer?** It covers networks you've marked private, not public ones.
If other devices can't connect, set your network to *Private* in Windows' network settings.

Away from home it works once you turn on **Remote Access** in *Settings* and follow its steps. Until
then the server refuses connections from outside your home, even if your router lets them through.
**[REMOTE.md](REMOTE.md)** explains each way in: a private VPN (Tailscale, NetBird), which is the
easiest and safest; forwarding a port with HTTPS; your own reverse proxy; or a rented server.

The server refuses unencrypted connections from the internet, so use HTTPS, and strong passwords for
every adult profile.

## Behind a reverse proxy

WittyMediaServer serves plain HTTP; put Caddy, nginx or Traefik in front for HTTPS. Video segments are
a few MB each and long requests are normal, so turn off body-size limits and raise read timeouts (the
Kubernetes ingress has example annotations). Set `WMS_PUBLIC_URL` (or `publicUrl` in the
[config file](CONFIG.md)) to the outside address, and list the proxy in `WMS_TRUSTED_PROXIES` or in
*Settings → Remote Access*: forwarded headers are believed from nobody else. More in
[REMOTE.md](REMOTE.md#a-reverse-proxy-you-already-run).

```caddyfile
media.example.com {
    reverse_proxy 127.0.0.1:9797
}
```

---
<sub>WittyMediaServer documentation · [Back to the README](../README.md)</sub>
