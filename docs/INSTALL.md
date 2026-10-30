# Installing WittyMediaServer

One self-contained download per platform: the server, a tray launcher and FFmpeg, with nothing else
to install first. New to all this? [Getting started](GETTING_STARTED.md) walks through it with no
command line.

- [Downloads](#downloads)
- [Desktop: the tray app](#desktop-the-tray-app)
- [Always on: as a service](#always-on-as-a-service)
- [Docker](#docker)
- [Kubernetes](#kubernetes)
- [At home and away](#at-home-and-away)
- [Behind a reverse proxy](#behind-a-reverse-proxy)

## Downloads

Download the archive for your platform from the **Releases** page.

| Platform | Archive | Notes |
|---|---|---|
| Windows 10/11 (x64, ARM64) | `…-windows-amd64.zip`, `…-windows-arm64.zip` | |
| macOS 11+ (Apple Silicon, Intel) | `…-darwin-arm64.tar.gz`, `…-darwin-amd64.tar.gz` | includes `WittyMediaServer.app` (menu-bar app) |
| Linux x64 / ARM64, incl. Raspberry Pi 4/5 (64-bit) | `…-linux-amd64.tar.gz`, `…-linux-arm64.tar.gz` | static, works on any distribution |
| Linux ARMv7, Raspberry Pi 2/3/4 (32-bit) | `…-linux-armv7.tar.gz` | static |
| Linux x86 32-bit | `…-linux-386.tar.gz` | static, for old PCs |
| FreeBSD 14+ (x64) | `…-freebsd-amd64.tar.gz` | static FFmpeg built in CI |
| FreeBSD 14+ (ARM64) | `…-freebsd-arm64.tar.gz` | uses FreeBSD's FFmpeg: `pkg install ffmpeg` |
| Docker | `ghcr.io/wittywilla/wittymediaserver` | amd64, arm64, armv7 |

Each release has a `…-SHA256SUMS.txt` file to check downloads. Every archive looks like this:

```
wittymediaserver-<version>-<os>-<arch>/
├── WittyMediaServer(.exe)    tray / menu-bar launcher (desktop use)
├── wms(.exe)                 the server (command line, services, Docker)
├── bin/ffmpeg, bin/ffprobe   bundled FFmpeg
├── install.sh / install.ps1  service installers
└── README, LICENSE, NOTICE, THIRD_PARTY_LICENSES
```

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

Away from home it works only once you set up remote access yourself, for example:

- a **VPN** into your home network (WireGuard, Tailscale and similar), the safest option;
- a **reverse proxy** with HTTPS on a domain you own (below), with the port opened on your router.

Never expose plain HTTP to the internet: always put HTTPS in front, and use strong passwords for
every adult profile.

## Behind a reverse proxy

WittyMediaServer serves plain HTTP; put Caddy, nginx or Traefik in front for HTTPS. Video segments are
a few MB each and long requests are normal, so turn off body-size limits and raise read timeouts (the
Kubernetes ingress has example annotations). Set `WMS_PUBLIC_URL` (or `publicUrl` in the
[config file](CONFIG.md)) to the outside address.

```caddyfile
media.example.com {
    reverse_proxy 127.0.0.1:9797
}
```

---
<sub>WittyMediaServer documentation · [Back to the README](../README.md)</sub>
