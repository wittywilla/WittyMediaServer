# Installing WittyMediaServer

Install the server on Windows, macOS, Linux, FreeBSD, a NAS or in a container, keep it up to date, and find out what each program does.

FFmpeg comes with the download, so there's nothing else to install first, apart from one FreeBSD case noted below. New to all this? [Getting started](GETTING_STARTED.md) walks through it with no command line. To remove it, see [Uninstalling](UNINSTALL.md).

- [Which file to download](#which-file-to-download)
- [What has been tested](#what-has-been-tested)
- [Windows](#windows)
- [macOS](#macos)
- [Linux](#linux)
- [FreeBSD](#freebsd)
- [Docker](#docker)
- [A NAS](#a-nas)
- [What the programs are](#what-the-programs-are)
- [The tray app](#the-tray-app)
- [Updating](#updating)
- [At home and away](#at-home-and-away)
- [Behind a reverse proxy](#behind-a-reverse-proxy)
- [Warnings you'll see](#warnings-youll-see)
- [For developers](#for-developers)

## Which file to download

Everything is on the project's [Releases page](https://github.com/wittywilla/WittyMediaServer/releases). Most people want the installer. Each file name starts `wittymediaserver-<version>-`, followed by what's in the table.

These files are the server, for the computer that holds your media. **For a phone, a TV or another computer, you don't install the server.** Open the server's address in a web browser, or use the app, [WittyMediaClient](https://github.com/wittywilla/WittyMediaClient/releases), which has its own Releases page. The app is written for Windows, macOS, Linux, FreeBSD, Android, Android TV, iOS, tvOS and Plasma Big Screen, but it's early: Android has been run on emulators only, and iOS and tvOS have never been built. A web browser is the way to reach everything today. [Apps and devices](APPS.md) has the state of each device.

| System | Installer | Without installing |
|---|---|---|
| Windows 10 or 11 | `windows-amd64.msi`, `windows-arm64.msi` | `windows-amd64.zip`, `windows-arm64.zip` |
| macOS 11 or later | `macos-arm64.pkg` (Apple silicon), `macos-amd64.pkg` (Intel) | `darwin-arm64.tar.gz`, `darwin-amd64.tar.gz` |
| Debian, Ubuntu and relatives | `linux-amd64.deb`, also `arm64`, `armv7`, `386` | `linux-amd64.tar.gz`, and the same |
| Fedora, RHEL and relatives | `linux-amd64.rpm`, also `arm64`, `armv7`, `386` | `linux-amd64.tar.gz`, and the same |
| FreeBSD 14 or later | `freebsd-amd64.pkg`, `freebsd-arm64.pkg` | `freebsd-amd64.tar.gz`, `freebsd-arm64.tar.gz` |

For containers, the image is `ghcr.io/wittywilla/wittymediaserver`: see [Docker](#docker).

Each release also has `SHA256SUMS.txt`, a list of every file's checksum, and `SHA256SUMS.txt.sig`, its signature. The server checks that signature before it installs an update.

## What has been tested

| System | How far it has been run |
|---|---|
| **Windows** | The `.msi` has been installed on Windows 11, and upgraded and removed under Wine. Before each release it's installed over the one before on GitHub's Windows machines, for everyone, for one account and as a service, with the server running. The ARM64 `.msi` was built once and hasn't been checked |
| **macOS** | Never built or run. This has not been run on a real Mac yet |
| **Linux** | The `.deb` and `.rpm` have been installed, upgraded and removed on Debian trixie, Ubuntu 26.04 and Fedora 43, on amd64, in containers with systemd running. The `arm64`, `armv7` and `386` packages haven't been run |
| **FreeBSD** | The amd64 package and its service have been installed, upgraded and removed in FreeBSD 14.4 and 15.1 virtual machines. Real hardware and arm64 haven't been tried, modules have only been built for it, and a film hasn't been played there |
| **Docker** | The amd64 image has been built and run in WSL, locked down, as several users, upgraded from 0.6.2-alpha, with NVENC through WSL. arm64 and armv7 have been built, not run. No NAS yet. See [Docker](DOCKER.md#what-has-been-tried) |

## Windows

1. Open the `.msi`. If Windows warns about it, see [Warnings you'll see](#warnings-youll-see).
2. Choose who it's for.

   | | Just me | Everyone on this computer |
   |---|---|---|
   | Needs an administrator | No | Yes |
   | Program | `%LOCALAPPDATA%\Programs\WittyMediaServer` | `C:\Program Files\WittyMediaServer` |
   | Library, accounts and settings | `%LOCALAPPDATA%\WittyMediaServer` | Each person's own, or with the service, `C:\ProgramData\WittyMediaServer` |
   | **Update now** in Settings | Yes | No: run the new installer |
   | Can run as a service | No | Yes |

3. Choose how it should run. Each of these is optional.
   - **Start when I sign in:** the tray app starts by itself. You can turn this off from its icon. If it's installed for everyone, turn it off in Windows Settings, under Apps, Startup.
   - **Put a shortcut on the desktop.** There's always a Start menu entry.
   - **Add the wittymediaserver command to the command line,** in every new terminal.
   - **Run as a background service, always on** (for everyone only): it runs even when nobody is signed in, under the limited `LOCAL SERVICE` account. See below for what it needs.
   - **Let other devices on my home network connect** (for everyone only): adds a Windows Firewall rule for networks you've marked private. It opens nothing to the internet. Installed just for you, Windows asks when the server first starts: choose **Allow**.
4. Leave **Open WittyMediaServer now** ticked to finish setting up in your browser.

Running a newer installer over an installed copy updates it in place and keeps your choices. It also takes over from a copy that 0.5.2-alpha or older installed: the old program, boot task and firewall rule go, and your library stays where it is.

**If you chose the service.** Give `LOCAL SERVICE` permission to read your media folders: open a folder's **Properties**, then **Security**, **Edit**, **Add**, and type `LOCAL SERVICE`. It can't reach a network share by a mapped drive letter, so use `\\server\share`, on a share that lets it in.

**For admins.** Every choice is a property, and `/qn` asks nothing. Without a choice of who it's for, it installs just for the person running it.

```powershell
msiexec /i wittymediaserver-<version>-windows-amd64.msi /qn MSIINSTALLPERUSER=1 AUTOSTART=1 DESKTOPSHORTCUT=1 ADDTOPATH=1
msiexec /i wittymediaserver-<version>-windows-amd64.msi /qn MSIINSTALLPERUSER="" INSTALLSERVICE=1 ADDFIREWALL=1 AUTOSTART="" PORT=9797
```

| Property | Means | If you don't say |
|---|---|---|
| `MSIINSTALLPERUSER` | `1` for this account, `""` for everyone | This account |
| `AUTOSTART` | `1` starts the tray app at sign-in, `""` doesn't | `1` |
| `DESKTOPSHORTCUT`, `ADDTOPATH` | `1` adds it | Off |
| `INSTALLSERVICE`, `ADDFIREWALL` | `1` adds it, for everyone only | Off |
| `PORT` | The port for the service and the firewall rule | `9797` |

**Without installing.** Unzip the `.zip` anywhere and open `WittyMediaServer.exe`. Add a [`wms.json`](CONFIG.md#portable-mode) beside it to keep everything in that folder, on a USB drive for instance.

## macOS

This has not been run on a real Mac yet.

1. Open the `.pkg`. If the Mac refuses it, see [Warnings you'll see](#warnings-youll-see).
2. Go through the installer. It puts **WittyMediaServer** and **Uninstall WittyMediaServer** in Applications, and the `wittymediaserver` command in `/usr/local/bin`.
3. **Customize** lets you leave out **Run in the background**, which otherwise starts the server when the Mac starts, as the person who installed it, even when nobody is signed in. Without it, the server runs while the app is open.

Your library, accounts and settings are in `~/Library/Application Support/WittyMediaServer`.

Running a newer `.pkg` updates it in place, and takes over from what the `install.sh` of 0.5.2-alpha or older put in `/opt`, keeping its library.

**A limit on a Mac.** A module's helper program runs in Apple's own sandbox, which hasn't been run on a real Mac yet, and Settings, Modules says if it couldn't be used. So a module from another publisher can't be chosen on a Mac for now. See [Modules](MODULE-STORE.md#using-someones-fork).

**Without installing.** Unpack the `.tar.gz` and open `WittyMediaServer.app`.

## Linux

```bash
sudo apt install ./wittymediaserver-<version>-linux-amd64.deb    # Debian, Ubuntu and relatives
sudo dnf install ./wittymediaserver-<version>-linux-amd64.rpm    # Fedora, RHEL and relatives
```

That's all: the service is running. Open `http://<this computer>:9797/` to finish setting up.

There's no package repository to add yet, so `apt upgrade` and `dnf upgrade` won't find new versions. Download each new package and install it the same way: it updates in place. It also takes over from what the `install.sh` of 0.5.2-alpha or older installed, keeping its data folder, port and account.

| To see or change | Where |
|---|---|
| Its status and log | `systemctl status wittymediaserver`, `journalctl -u wittymediaserver` |
| Program | `/usr/lib/wittymediaserver`, with the command at `/usr/bin/wittymediaserver` |
| Library, accounts and settings | `/var/lib/wittymediaserver` |
| The account it runs as | `wms`, which needs read permission on your media folders |
| The port, data folder or anything else | `sudo systemctl edit wittymediaserver` |

For example, to use port 8080, put this in what `systemctl edit` opens, and save:

```ini
[Service]
Environment=WMS_LISTEN=:8080
```

For a different data folder, set both of these, as the service can only write where it's told:

```ini
[Service]
Environment=WMS_DATA_DIR=/tank/witty
ReadWritePaths=/tank/witty
```

A desktop gets **WittyMediaServer** in its app menu. The tray app shows the service, and doesn't start a second server.

**On another distribution, with systemd.** Unpack the `.tar.gz`, and:

```bash
sudo ./install.sh                      # /opt/wittymediaserver, data in /var/lib/wittymediaserver, port 9797
sudo ./install.sh --prefix /srv/witty --data /tank/witty --port 8080 --user media
```

**Without installing.** Run `./WittyMediaServer`, or `./bin/wittymediaserver serve` with no tray icon.

Run either a service or the tray app on a port, not both by hand. The tray app notices a server already answering, and only shows it.

## FreeBSD

As root:

```sh
pkg add wittymediaserver-<version>-freebsd-amd64.pkg
sysrc wittymediaserver_enable=YES
service wittymediaserver start
```

Then open `http://<this computer>:9797/` to finish setting up. As with FreeBSD's own packages, it doesn't start until you enable it. If it says FFmpeg is missing, run `pkg install ffmpeg`. The arm64 package always needs this, as there's no FFmpeg build of ours for it.

| To see or change | Where |
|---|---|
| Program | `/usr/local/lib/wittymediaserver`, with the command at `/usr/local/bin/wittymediaserver` |
| Library, accounts and settings | `/var/db/wittymediaserver`, and the log is `wms.log` in there |
| The account it runs as | `wms`, which needs read permission on your media folders |
| The account, data folder or port | `sysrc wittymediaserver_user=media`, `sysrc wittymediaserver_data=/tank/witty`, `sysrc wittymediaserver_listen=:8080` |
| A settings file | `/usr/local/etc/wittymediaserver/wms.json` |

**To update,** run `pkg add -f` with the newer package, then `service wittymediaserver start`, as `pkg` stops it. It also takes over from what `install.sh` installed, keeping its data folder, port and account. A package made for FreeBSD 14 needs `env IGNORE_OSVERSION=yes pkg add <file>` on 15, which hasn't been tried.

**A limit on FreeBSD.** A module's helper program runs with limits here, not a sandbox: it can't start programs, open a network connection or write into a file, but it can still read what the account can read, delete its files and signal its programs. So run the server as its own account, as the package does, and a module from another publisher can't be chosen, and WittyCouch isn't available, as its host has no FreeBSD build. See [Modules](MODULE-STORE.md#using-someones-fork).

**Without a package.** Unpack the `.tar.gz` and run `./install.sh` as root, with the same options as on Linux. It starts the service straight away.

## Docker

```bash
docker run -d --name wittymediaserver --restart unless-stopped   -p 9797:9797   -v wms-config:/config   -v /path/to/media:/media:ro   ghcr.io/wittywilla/wittymediaserver:alpha
```

Then open `http://<this computer>:9797/`. From another device, setup asks for the code that `docker logs wittymediaserver` shows.

[Docker](DOCKER.md) has the rest: ready compose files (plain, locked down, Intel and AMD graphics, NVIDIA, behind a reverse proxy), the tags, every folder and setting, running as your own user, hardware transcoding, and updating. The image is for `amd64`, `arm64` and `armv7`, runs as an ordinary user, and works with a read-only root.

## A NAS

Most NAS systems run containers, and that's the way to do it. Each of these has a guide with its own screens, folders, user IDs and graphics steps:

| | |
|---|---|
| **UGREEN** (UGOS Pro) | [Docker on a UGREEN NAS](DOCKER-UGREEN.md) |
| **Synology** (Container Manager) | [Docker on a Synology NAS](DOCKER-SYNOLOGY.md) |
| **QNAP** (Container Station) | [Docker on a QNAP NAS](DOCKER-QNAP.md) |
| **Unraid** | [Docker on Unraid](DOCKER-UNRAID.md) |
| **TrueNAS SCALE** | [Docker on TrueNAS SCALE](DOCKER-TRUENAS.md) |
| **Asustor** (ADM) | [Docker on an Asustor NAS](DOCKER-ASUSTOR.md) |
| Anything else running Linux | [Docker on a Linux machine](DOCKER-LINUX.md) |
| An older or 32-bit NAS | `armv7` and `amd64` images exist, but no 32-bit x86 one: run the `linux-386.tar.gz` archive by hand |

If your NAS runs Debian underneath, as OpenMediaVault does, the `.deb` should work too. It hasn't been tried there.

## What the programs are

You only ever need to open the first one.

| Program | What it does |
|---|---|
| `WittyMediaServer.exe` (Windows), `WittyMediaServer.app` (Mac), **WittyMediaServer** in the app menu (Linux) | **The app you open.** Puts an icon in the tray or menu bar, starts the server and opens the web app |
| `Uninstall WittyMediaServer.exe`, `Uninstall WittyMediaServer.app` | Removes it, asking first. See [Uninstalling](UNINSTALL.md) |
| `bin/wittymediaserver` | The server itself, and the command line for repairs and support. Try `wittymediaserver help`, or see the [command line reference](REFERENCE.md#commands) |
| `bin/ffmpeg`, `bin/ffprobe` | FFmpeg: converts video when a device needs it, and reads what's in a file |
| `modules` | Signed modules for offline use, in a package that came with any. Otherwise modules arrive from the store, into the data folder, with their own programs such as `wittyplay-game-runner` |
| `package-info.json` | Says how this copy was installed, which decides how it updates |
| `wms` | Only in the `.zip` and `.tar.gz`, so a copy of 0.5.2-alpha can update to this one. It starts `bin/wittymediaserver`, and is safe to delete otherwise |

On Windows each says what it is in its **Properties**, and in Task Manager's **Details**. On a Mac the same programs are inside the app, under `Contents/MacOS` and `Contents/Helpers`, and `modules` under `Contents/Resources`.

## The tray app

An icon appears in the system tray or menu bar. It turns grey while the server is stopped.

| Menu item | What it does |
|---|---|
| **● Running at** `http://localhost:9797` | Shows the status. With a service, it says **Running as a background service** |
| **Open WittyMediaServer** | Opens the web app, or the setup wizard the first time |
| **Stop server**, **Start server**, **Restart server** | Not available when a background service is running it: the service looks after itself |
| **Open data folder**, **View log** | Open what they say |
| **Start at login** | Starts the tray app when you sign in, through a Windows Run entry, a macOS LaunchAgent or an XDG autostart file |
| **About WittyMediaServer** | Shows the version |
| **Quit (stops the server)** | Stops the server it started, with any FFmpeg jobs, and exits. A background service carries on |

The app restarts the server if it crashes. It stops trying after three failures within ten seconds of starting, so a bad configuration doesn't loop.

## Updating

Admins see a dot on **Settings** when a newer version is out. **Settings → About**, under **Updates**, shows the installed and the newest version with its release notes. **Update now** installs it:

1. **Downloading:** the release's checksum list and its signature come from GitHub, over HTTPS, and only from GitHub.
2. **Checking:** the checksum list must be signed with the release key built into the server, and the downloaded package must match its checksum. Anything else is refused and nothing changes.
3. **Installing:** the new files are unpacked next to the program and swapped in. The `data` folder, `wms.json` and any other folder the server keeps data in are never written.
4. **Restarting:** the tray app, systemd or the server itself starts the new version, and the page reloads by itself.

The server looks for new versions twice a day while **Check for updates automatically** is on, which it is to begin with. **Check now** asks at once. It only fetches GitHub's list of releases: nothing about you or your library is sent, and nothing is installed until an admin presses **Update now**.

**Update now** needs a copy installed from a release package, in a folder the server can write. Otherwise Settings says why:

| Installed with | How to update |
|---|---|
| Windows installer, **Just me**, or an unpacked folder on Windows, Linux or FreeBSD | **Update now** |
| Windows installer, **Everyone on this computer** | Run the new installer, as Windows protects Program Files |
| `.deb`, `.rpm`, FreeBSD `.pkg` | Install the new package the same way, as your package manager owns these files |
| `install.sh` | Unpack the new archive and run `sudo ./install.sh` again |
| macOS | Run the new `.pkg` |
| Docker | `docker compose pull`, then `docker compose up -d`. With `docker run`: `docker pull`, then remove and recreate the container. Your `/config` volume stays |

A copy of 0.5.2-alpha or older takes the update as well, from a folder it can write. The server moves to `bin/` at the same time, and a small `wms` stays where the old server was, so whatever started it still can. Scripts of yours that call `wms` should change to `wittymediaserver`. In Docker that name has already gone.

## At home and away

WittyMediaServer runs on your own computer, in your home. **Out of the box it's available on your home network only:** every device on the same Wi-Fi or cable network can open `http://<this computer's address>:9797`. The setup wizard and **Settings → About** show the address.

**Setting up from another device.** Until the first account exists, whoever makes it owns the server. On the computer running it, setup just works. From any other device, your phone included, setup asks for a **setup code**, because someone else on your network could otherwise get there first. Find it in either place:

- on the server's console when it starts (`This server has no accounts yet...`, in `docker logs` or `journalctl -u wittymediaserver`);
- in `setup-code.txt` in the server's data folder, which goes away once setup is done.

It's never kept in the log files or shown in the log viewer. If you trust everyone on your network and would rather not use it, set [`setupFromHome`](CONFIG.md#security-switches).

A server nobody can reach from a home network first, such as a rented one, also needs [`remoteAccess`](CONFIG.md#security-switches) turned on, and HTTPS.

**If other devices can't connect on Windows.** The installer's firewall rule covers networks you've marked private, not public ones. Set your network to **Private** in Windows' network settings.

**Away from home.** It works once you turn on **Settings → Remote Access** and follow its steps. Until then the server refuses connections from outside your home, even if your router lets them through. [Remote access](REMOTE.md) explains each way in: a private VPN, which is the easiest and safest; forwarding a port with HTTPS; your own reverse proxy; or a rented server. The server refuses unencrypted connections from the internet, so use HTTPS, and strong passwords for every adult profile.

## Behind a reverse proxy

WittyMediaServer serves plain HTTP, so put Caddy, nginx or Traefik in front for HTTPS. Video segments are a few MB each and long requests are normal, so turn off body-size limits and raise read timeouts. Set `WMS_PUBLIC_URL` or [`publicUrl`](CONFIG.md#what-you-can-set) to the outside address, and list the proxy in `WMS_TRUSTED_PROXIES` or in **Settings → Remote Access**: forwarded headers are believed from nobody else. See [A reverse proxy you already run](REMOTE.md#a-reverse-proxy-you-already-run).

```text
media.example.com {
    reverse_proxy 127.0.0.1:9797
}
```

## Warnings you'll see

The programs in this alpha aren't code-signed yet, so systems warn about them. Everything works once you get past the warning. Each download's checksum is in `SHA256SUMS.txt`, if you want to check it.

| System | What you see | What to do |
|---|---|---|
| **Windows** | "Windows protected your PC", from SmartScreen | Choose **More info**, then **Run anyway** |
| **Windows,** for everyone | The administrator prompt says "Unknown publisher" | Choose **Yes** if you trust the download |
| **macOS** | The Mac refuses to open the `.pkg` | Right-click it and choose **Open**, then **Open** again. If it still refuses, choose **Open Anyway** in **System Settings → Privacy & Security** |
| **Linux, FreeBSD** | Nothing, for a file you install by name | Nothing. The packages carry no signature of their own |

## For developers

**Kubernetes.** `deploy/k8s` in the source has a setup: `kubectl apply -k deploy/k8s`. Edit `deployment.yaml` for your media volume (NFS, hostPath or CSI) and `ingress.yaml` for your hostname and TLS. The deployment uses `Recreate`, as SQLite has one writer, and an in-memory `emptyDir` for transcoding. The config volume holds the database and artwork. It uses the `latest` image, which exists once there's a full release, so set `newTag` in `kustomization.yaml` to `alpha` until then.

**Making the installers,** and everything about signing: `docs/PUBLISHING.md` and `scripts/test/README.md` in the source.

---

<sub>WittyMediaServer documentation</sub>
