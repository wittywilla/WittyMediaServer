# Installing WittyMediaServer

One self-contained download per platform: the server, a tray launcher and FFmpeg, with nothing else
to install first. New to all this? [Getting started](GETTING_STARTED.md) walks through it with no
command line.

- [Downloads](#downloads)
- [Windows: the setup](#windows-the-setup)
- [Desktop: the tray app](#desktop-the-tray-app)
- [Always on: as a service](#always-on-as-a-service)
- [Updating](#updating)
- [Docker](#docker)
- [Kubernetes](#kubernetes)
- [At home and away](#at-home-and-away)
- [Behind a reverse proxy](#behind-a-reverse-proxy)

## Downloads

Download the archive for your platform from the **Releases** page.

| Platform | Archive | Notes |
|---|---|---|
| Windows 10/11 (x64, ARM64) | `…-windows-amd64-setup.exe`, `…-windows-arm64-setup.exe` | installs for your account, no admin needed |
| | `…-windows-amd64.zip`, `…-windows-arm64.zip` | the same files to unpack and run anywhere |
| macOS 11+ (Apple Silicon, Intel) | `…-darwin-arm64.tar.gz`, `…-darwin-amd64.tar.gz` | includes `WittyMediaServer.app` (menu-bar app) |
| Linux x64 / ARM64, incl. Raspberry Pi 4/5 (64-bit) | `…-linux-amd64.tar.gz`, `…-linux-arm64.tar.gz` | static, works on any distribution |
| Linux ARMv7, Raspberry Pi 2/3/4 (32-bit) | `…-linux-armv7.tar.gz` | static |
| Linux x86 32-bit | `…-linux-386.tar.gz` | static, for old PCs |
| FreeBSD 14+ (x64) | `…-freebsd-amd64.tar.gz` | static FFmpeg built in CI |
| FreeBSD 14+ (ARM64) | `…-freebsd-arm64.tar.gz` | uses FreeBSD's FFmpeg: `pkg install ffmpeg` |
| Docker | `ghcr.io/wittywilla/wittymediaserver` | amd64, arm64, armv7 |

Each release has a `…-SHA256SUMS.txt` file to check downloads, and its signature
(`…-SHA256SUMS.txt.sig`), which the server checks before it installs an update. Every archive looks
like this:

```
wittymediaserver-<version>-<os>-<arch>/
├── WittyMediaServer(.exe)    tray / menu-bar launcher (desktop use)
├── wms(.exe)                 the server (command line, services, Docker)
├── bin/ffmpeg, bin/ffprobe   bundled FFmpeg
├── install.sh / install.ps1  service installers
├── wms-setup.exe             Windows: installs this folder for your account
└── README, LICENSE, NOTICE, THIRD_PARTY_LICENSES
```

## Windows: the setup

Run **`wittymediaserver-<version>-windows-amd64-setup.exe`** (or double-click `wms-setup.exe` in an
unpacked folder). It needs no administrator rights:

- the program goes to `%LOCALAPPDATA%\Programs\WittyMediaServer`, a folder your account can write,
  so the server can [update itself](#updating);
- your library, accounts and settings stay in `%LOCALAPPDATA%\WittyMediaServer`, which installing,
  updating and removing never touch;
- it adds **WittyMediaServer** to the Start menu, starts the tray app when you sign in (switch that
  off from the tray icon), and lists the app under *Settings → Apps → Installed apps*, where
  **Uninstall** removes the program and keeps your data.

Running a newer setup over an installed copy updates it in place. For scripts: `/S` installs
silently, `/noautostart` skips starting at sign-in, and `wms-setup.exe /uninstall /S` removes it.

Already running a copy from an unpacked folder with its library in `data\`? Quit it, run the setup,
and copy the contents of that `data` folder into `%LOCALAPPDATA%\WittyMediaServer` before the first
start to keep your library.

## Desktop: the tray app

Double-click **WittyMediaServer** (`WittyMediaServer.exe` on Windows, `WittyMediaServer.app` on
macOS). An icon appears in the system tray or menu bar:

| Menu item | What it does |
|---|---|
| **● Running at http://localhost:9797** | live status |
| **Open WittyMediaServer** | opens the web app (the setup wizard the first time) |
| **Stop / Start / Restart server** | |
| **Open data folder**, **View log** | |
| **Start at login** | Windows Run key, macOS LaunchAgent or XDG autostart |
| **Quit** | stops the server, including any FFmpeg jobs, and exits |

The launcher restarts the server if it crashes, and stops trying after three quick failures so a bad
configuration doesn't loop. The icon turns grey while the server is stopped.

> **macOS:** the first launch of an unsigned app needs **right-click → Open**, or
> `xattr -dr com.apple.quarantine WittyMediaServer.app`.
> **Linux:** copy `wittymediaserver.desktop` into `~/.local/share/applications/` for an app-menu entry
> (the installer does this system-wide).

## Always on: as a service

For servers, NAS boxes and always-on machines.

```bash
# Linux (systemd), macOS (launchd), FreeBSD (rc.d)
sudo ./install.sh                      # /opt/wittymediaserver, data in /var/lib/wittymediaserver, port 9797
sudo ./install.sh --prefix /srv/wms --data /tank/wms --port 8080 --user media
```

```powershell
# Windows, in an elevated PowerShell: a Scheduled Task that runs at boot, plus a firewall rule
Set-ExecutionPolicy -Scope Process Bypass
.\install.ps1 -Port 9797
.\uninstall.ps1 -KeepData     # remove the task, keep your library database
```

Run either the service or the tray app on a port, not both.

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
| Windows setup, an unpacked folder, `install.ps1` (runs as SYSTEM) | **Update now** |
| Tray app run from *Program Files* by a normal account | run the Windows setup, or `install.ps1` again as administrator |
| `install.sh` (`/opt/…`, owned by root) | unpack the new archive and run `sudo ./install.sh` again |
| macOS | download the new version and replace the app |
| Docker / Kubernetes | pull the new image and recreate the container |

## Docker

```bash
docker run -d --name wittymediaserver --restart unless-stopped \
  -p 9797:9797 \
  -v wms-config:/config \
  -v /path/to/movies:/media/movies:ro \
  -v /path/to/discs:/media/discs:ro \
  ghcr.io/wittywilla/wittymediaserver:latest
```

Or `docker compose up -d` with [`docker-compose.yml`](../docker-compose.yml). For hardware
transcoding pass `--device /dev/dri` (VAAPI) or use the NVIDIA container runtime, then tick the
encoder under *Settings → Transcoding*. The image runs as UID 1000: `chown` your media or add
`--user`.

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

**Setting up from another network.** Until the first account exists, whoever makes it owns the
server, so from this computer and your home network setup just works. A server nobody can reach from
a home network first (a rented one, say) needs `remoteAccess` turned on in its configuration
([CONFIG.md](CONFIG.md)) and HTTPS, and from there, through a reverse proxy, or under a domain name,
setup also asks for a **setup code**. The server prints the code on its console when it starts (`This server has no accounts yet...`, in
`docker logs` or in `journalctl -u wittymediaserver`). It is never kept in the log files or shown in the log viewer.

**Installed with `install.ps1`?** Its firewall rule covers home and work networks, not public ones.
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
