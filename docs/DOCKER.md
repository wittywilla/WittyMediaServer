# Docker

WittyMediaServer in a container, on a NAS, a Linux server or Docker Desktop: the image, its tags, the folders and settings it uses, hardware transcoding, hardening, a reverse proxy, and updating.

**On a NAS?** Start with its own guide, which has the folders, user IDs and graphics steps for that make:

| | |
|---|---|
| [UGREEN](DOCKER-UGREEN.md) | UGOS Pro's Docker app |
| [Synology](DOCKER-SYNOLOGY.md) | Container Manager on DSM 7.2 |
| [QNAP](DOCKER-QNAP.md) | Container Station 3 |
| [Unraid](DOCKER-UNRAID.md) | A template, or Compose Manager |
| [TrueNAS SCALE](DOCKER-TRUENAS.md) | Install via YAML, 24.10 and later |
| [Asustor](DOCKER-ASUSTOR.md) | Portainer on ADM |
| [Any Linux machine](DOCKER-LINUX.md) | Docker Engine and Compose |

- [Quick start](#quick-start)
- [The image](#the-image)
- [Folders](#folders)
- [Settings](#settings)
- [User and group IDs](#user-and-group-ids)
- [Locked down](#locked-down)
- [Hardware transcoding](#hardware-transcoding)
- [Behind a reverse proxy](#behind-a-reverse-proxy)
- [Updating and going back](#updating-and-going-back)
- [Is it running?](#is-it-running)
- [When something's wrong](#when-somethings-wrong)
- [Building it yourself](#building-it-yourself)
- [What has been tried](#what-has-been-tried)

## Quick start

```sh
docker run -d --name wittymediaserver --restart unless-stopped \
  -p 9797:9797 \
  -v wms-config:/config \
  -v /path/to/media:/media:ro \
  ghcr.io/wittywilla/wittymediaserver:alpha
docker logs wittymediaserver 2>&1 | grep -i "setup code"
```

Open `http://<this machine>:9797/`. From another device, setup asks for the code the second line shows. Then add your folders from `/media` as libraries.

With Compose, the files in [`deploy/docker`](https://github.com/wittywilla/WittyMediaServer/tree/master/deploy/docker) are ready to copy:

| File | For |
|---|---|
| [`compose.yml`](https://github.com/wittywilla/WittyMediaServer/blob/master/deploy/docker/compose.yml) | The plainest: a port, a volume for the server's data, your media |
| [`compose.hardened.yml`](https://github.com/wittywilla/WittyMediaServer/blob/master/deploy/docker/compose.hardened.yml) | Your own user, a read-only root, no extra powers. **Use this one if you can** |
| [`compose.intel-amd.yml`](https://github.com/wittywilla/WittyMediaServer/blob/master/deploy/docker/compose.intel-amd.yml) | Hardened, with Intel or AMD graphics |
| [`compose.nvidia.yml`](https://github.com/wittywilla/WittyMediaServer/blob/master/deploy/docker/compose.nvidia.yml) | Hardened, with an NVIDIA card |
| [`compose.reverse-proxy.yml`](https://github.com/wittywilla/WittyMediaServer/blob/master/deploy/docker/compose.reverse-proxy.yml) | Hardened, behind Caddy with HTTPS |

Save one as `compose.yml`, change the marked lines, and run `docker compose up -d`.

## The image

`ghcr.io/wittywilla/wittymediaserver`, for `linux/amd64`, `linux/arm64` and `linux/arm/v7`. Docker picks the right one by itself.

| Tag | What it is | Changes |
|---|---|---|
| `alpha` | The newest alpha release | With every alpha |
| `0.7.0-alpha` (any version) | That release, exactly | Never |
| `latest` | The newest full release. **There isn't one yet**, so this tag doesn't exist | With every full release |
| `0.7` | The newest full release in 0.7, once there is one | With each 0.7.x |
| `sha-1a2b3c4` | One build, by its commit | Never |

To stay on a version until you choose to move, use its number, such as `:0.7.0-alpha`. `alpha` moves on its own when you pull.

**What's in it:** the server and its web app, FFmpeg from Debian, `tini` to pass signals on, and graphics drivers:

| | amd64 | arm64 | armv7 |
|---|---|---|---|
| Intel graphics: VA-API and Quick Sync (Intel's media driver for Broadwell and newer, i965 for older, oneVPL) | Yes | | |
| AMD graphics: VA-API (Mesa) | Yes | | |
| NVIDIA: NVENC | From the host, with the NVIDIA Container Toolkit | Same | |
| Software: x264, x265, SVT-AV1 | Yes | Yes | Yes |

It's based on Debian 13 (trixie) slim, because the game cores are built for glibc. Modules aren't in the image: the server adds them from the store into `/config` at run time.

**Licences.** The server is AGPL-3.0-or-later. FFmpeg in the image is Debian's build, under the GPL, as are x264 and x265; Intel's media driver is MIT, with its encoding kernels from Debian's non-free section (redistributable, without source); Mesa is MIT. Every one is listed in **Settings → About** and in [THIRD_PARTY_LICENSES.md](https://github.com/wittywilla/WittyMediaServer/blob/master/THIRD_PARTY_LICENSES.md). Each Debian package's copyright file is in the image, under `/usr/share/doc/<package>/copyright`; manuals and translations are left out to save space. [sources.debian.org](https://sources.debian.org/) has each package's source.

## Folders

| In the container | What it holds | Mount it |
|---|---|---|
| `/config` | Everything the server keeps: database, settings, accounts, artwork, logs, modules and game cores, and video being transcoded (`/config/transcode`). Back it up | Read-write. Not `noexec`: modules run from it. On a NAS, an SSD if you have one |
| `/media`, or folders under it | Your media | Read-only (`:ro`) is fine, unless you want the server to save posters and NFO files next to your videos (**Settings → Metadata → Save information and artwork next to your media**) |
| `/tmp` | Small temporary files | A `tmpfs`, needed with a read-only root |
| `/config/transcode` | Video while it's being converted, a few GB at most | Optional: mount a RAM disk or SSD here (or set `WMS_TRANSCODE_DIR`) on a NAS with slow disks |

**A folder of your own for `/config`** has to belong to the user the server runs as, or it can't start. Make it first, with that owner:

```sh
mkdir -p /volume1/docker/wittymediaserver
sudo chown -R 1000:1000 /volume1/docker/wittymediaserver   # the numbers in user:, or 1000:1000
```

A volume Docker looks after (`wms-config:/config`) needs nothing: it's made with the right owner.

## Settings

Most settings are in the app. These are the ones a container sets with `environment:`. The image sets the first five itself; the [config file](CONFIG.md) has the rest.

| Variable | The image sets | What it does |
|---|---|---|
| `WMS_DATA_DIR` | `/config` | The server's own folder |
| `WMS_TRANSCODE_DIR` | `/config/transcode` | Video while it's being converted |
| `WMS_LISTEN` | `:9797` | The address and port inside the container. Change the left side of `ports:` instead, as **Settings → Network** says too |
| `WMS_DOCKER` | `1` | Says it's a container: the server doesn't replace its own files, and its messages talk about compose files |
| `XDG_CACHE_HOME` | `/config/cache/xdg` | Where graphics drivers keep their caches. Somewhere writable, for any user and a read-only root |
| `NVIDIA_DRIVER_CAPABILITIES` | `compute,video,utility` | Asks the NVIDIA Container Toolkit for the video encoder as well as the card |
| `WMS_LOG` | | `error`, `warn`, `info` (the default) or `debug` |
| `WMS_MAX_TRANSCODES` | | How many videos may be converted at once. Default `4`; `1` or `2` on a small NAS |
| `WMS_PUBLIC_URL` | | The address people use from outside, such as `https://media.example.com`. Setting it turns remote access on |
| `WMS_TRUSTED_PROXIES` | | Your reverse proxy's address, as the server sees it. [Behind a reverse proxy](#behind-a-reverse-proxy) |
| `WMS_SETUP_FROM_HOME` | | `1` lets anyone on your home network make the first account without the setup code |
| `WMS_CACHE_DIR`, `WMS_METADATA_DIR`, `WMS_LOG_DIR` | | Move the cache, the artwork or the logs out of `/config` |
| `TZ` | | Your time zone, such as `Europe/London`, for the times in logs and the TV guide. Settings, Time zone, or the first-run Name step, overrides it |

## User and group IDs

Inside the container, the server is a user and a group, by number. It can only see a media folder that number may read, and only keep its data in a folder that number may write. The image runs as `1000:1000`. Any other numbers work, so the usual fix is to make them match the account that owns your folders on the host.

**Signs the numbers are wrong:**

- A library is empty, or a folder you know has files looks empty in the app's folder browser.
- The folder browser, a library scan or the log says *permission denied*.
- The container stops at once, and its log says it *can't write /config*.

**Find the right numbers** on the host:

```sh
id <your account>        # uid=1026(you) gid=100(users) groups=100(users),101(administrators)
ls -ln /path/to/media    # the third and fourth columns: the owner's and group's numbers
```

Your NAS's own **Users** page shows the same account, and the folder's **Permissions** page who may read it. The sections below say where those are on each make.

**Fix it**, in order of preference:

1. **Run as that account:** `user: "1026:100"` in the compose file (`--user 1026:100` with `docker run`), with the numbers from `id`. Then recreate the container.
2. **Add a group the media belongs to:** if the folders belong to a group your account isn't the main member of, add it by number: `group_add: ["101"]`. The same goes for the graphics card's group, see [Hardware transcoding](#hardware-transcoding).
3. **Change the folder's permissions on the NAS:** give that account (or **Everyone**) read access to your media, and read and write to the server's own folder. On the command line: `sudo chown -R 1026:100 /volume1/docker/wittymediaserver` for the server's folder, and `chmod -R a+rX /volume1/Media` for media anyone may read.

**PUID and PGID.** Some images take `PUID` and `PGID` settings and switch to those numbers after starting as root. This image never starts as root, which is why it works with every line of [Locked down](#locked-down), so it doesn't use them: `user: "PUID:PGID"` does the same job. If you set `PUID` or `PGID` and they don't match the user it runs as, the server says so in its log when it starts.

Nothing in the image needs a name for that user, and the server never asks for more powers. Use an ordinary account, not `admin` or `root`.

### UGREEN: user and group IDs

- **Find them:** over SSH (**Control Panel → Terminal → SSH**), `id` for your account and `ls -ln /volume1` for your shared folders. The account you sign in to UGOS with is the one to use. Accounts made in UGOS can have numbers below 1000, such as `999` with group `10`, so check rather than assume `1000`.
- **Fix them:** set `user:` to your numbers in the project, and recreate it. To give an account access to a folder, use **Files**, right-click the shared folder, **Properties → Permissions**, or **Control Panel → User → Edit → Permissions**.
- See [Docker on a UGREEN NAS](DOCKER-UGREEN.md#2-find-your-user-and-the-render-group).

### Synology: user and group IDs

- **Find them:** over SSH (**Control Panel → Terminal & SNMP**), `id <account>`. The first account made on DSM is usually `1026`, the next `1027`, and the `users` group is `100`. `ls -ln /volume1` shows who owns each shared folder.
- **Fix them:** set `user:` to the account's numbers. Give it access in **Control Panel → Shared Folder → Edit → Permissions**: **Read/Write** on `docker`, **Read only** on your media. DSM also keeps its own access lists on top of the numbers, so a folder can look right with `ls -ln` and still refuse the account until it's ticked there.
- See [Docker on a Synology NAS](DOCKER-SYNOLOGY.md#2-find-your-user-and-the-render-group).

### QNAP: user and group IDs

- **Find them:** over SSH, `id <account>`. Ordinary accounts start at `1000`, and the `everyone` group is `100`. `admin` is `0`, root: don't use it. `ls -ln /share` shows the shared folders.
- **Fix them:** set `user:` to the account's numbers. Give it access in **Control Panel → Privilege → Shared Folders → Edit Shared Folder Permission**: Read/Write on `Container`, Read only on your media.
- See [Docker on a QNAP NAS](DOCKER-QNAP.md#2-find-your-user-and-the-render-group).

### Unraid: user and group IDs

- **Find them:** Unraid's shares belong to `nobody:users`, which is `99:100`, and that's the right pair almost always. In the terminal, `ls -ln /mnt/user` shows it.
- **Fix them:** the template already runs as `99:100` (in **Extra Parameters**). If a share was copied in with other owners, run **Tools → New Permissions** on it, or `chown -R 99:100 /mnt/user/appdata/wittymediaserver`.
- See [Docker on Unraid](DOCKER-UNRAID.md#the-folders-and-the-user).

### TrueNAS SCALE: user and group IDs

- **Find them:** apps run as the `apps` user, `568:568`. **Credentials → Users** shows each account's number; in **System → Shell**, `ls -ln /mnt/tank` shows who owns each dataset.
- **Fix them:** keep `user: "568:568"`, and give `apps` access: **Datasets → <dataset> → Permissions → Edit**, add the `apps` user, **Read** for media and **Modify** for the server's own dataset. A dataset with the **Apps** preset has that already. To run as your own account instead, use its numbers from **Credentials → Users**.
- See [Docker on TrueNAS SCALE](DOCKER-TRUENAS.md#1-make-the-datasets).

### Asustor: user and group IDs

- **Find them:** over SSH (**Services → Terminal**), `id <account>`. `admin` is usually `999`; accounts you make start at `1000`, and the `users` group is `100`. `ls -ln /volume1` shows the shared folders.
- **Fix them:** set `user:` to the account's numbers. Give it access in **Access Control → Shared Folders → Access Right**: Read & Write on `Docker`, Read Only on your media.
- See [Docker on an Asustor NAS](DOCKER-ASUSTOR.md#2-find-your-user-and-the-render-group).

### Linux: user and group IDs

- **Find them:** `id -u` and `id -g` for your own account, `getent group render` for the graphics group, `ls -ln /srv/media` for a folder's owner.
- **Fix them:** set `user:` to those numbers. For a folder only a group may read, add it with `group_add`, or `sudo chmod -R g+rX` it for its group. `sudo chown -R "$(id -u):$(id -g)" ./config` for the server's folder.
- See [Docker on a Linux machine](DOCKER-LINUX.md#3-find-your-user-and-the-render-group).

## Locked down

[`compose.hardened.yml`](https://github.com/wittywilla/WittyMediaServer/blob/master/deploy/docker/compose.hardened.yml) runs with everything the server doesn't need taken away, and everything it does still works:

| Line | What it does |
|---|---|
| `user: "1000:1000"` | Your own user, not root |
| `read_only: true` | Nothing can change the program's files. The server writes only to `/config` and `/tmp` |
| `tmpfs: - /tmp` | A small writable `/tmp` in memory, which a read-only root needs |
| `cap_drop: - ALL` | No special powers at all. The server listens on 9797, so it needs none |
| `security_opt: - no-new-privileges:true` | Nothing it starts can gain powers |

Tried with all of it: first-time setup, adding a library, playing and transcoding (NVENC included), installing a module from the store and running it, and the hardware report. Games, discs and backups write only to `/config` too, and haven't been tried this way yet. **What doesn't happen:** the server updating itself, which a container never does; pull a new image instead.

Publishing the port to one address only, such as `"127.0.0.1:9797:9797"`, keeps it off the network when only a reverse proxy on the same machine should reach it. Docker's published ports go around firewalls such as `ufw`.

## Hardware transcoding

The server tests every encoder when it starts, and **Settings → Live transcoding → Hardware encoders** says which work, and why the others don't, in words such as *isn't in group 105, which owns /dev/dri/renderD128*. The same from the command line:

```sh
docker exec wittymediaserver wittymediaserver hardware
```

### Intel and AMD

1. Pass the device in: `devices: - /dev/dri:/dev/dri`. As a volume it can be seen but not used.
2. Let the container's user open it. On the host, `ls -ln /dev/dri` shows the group number in the fourth column:

   ```
   crw-rw---- 1 0 105 226, 128 Nov  4 08:45 renderD128
   ```

   Add that number, as a number, with `group_add: - "105"`. The container doesn't know the host's group names.
3. Tick *Intel Quick Sync* or *VAAPI*, and press **Save changes**.

[`compose.intel-amd.yml`](https://github.com/wittywilla/WittyMediaServer/blob/master/deploy/docker/compose.intel-amd.yml) has all of it.

#### Only root may open the render node

Some NAS systems leave `renderD128` as `crw-------` (owner only, the owner being root). A group can't help until the host allows it. On the host, as root:

```sh
chmod 0660 /dev/dri/renderD128     # its group may use it; then group_add that group
```

or, if its group is root's (`0`) and you'd rather not add that group:

```sh
chmod 0666 /dev/dri/renderD128     # anyone on the NAS may use it
```

Either is undone at the next restart of the NAS, so run it from a task that runs at boot as root. The NAS guides say where that is.

### NVIDIA

1. On the host: NVIDIA's driver, and the [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html). `docker run --rm --gpus all ubuntu nvidia-smi` should list your card.
2. Give the container the card: `--gpus all` with `docker run`, or the `deploy:` lines in [`compose.nvidia.yml`](https://github.com/wittywilla/WittyMediaServer/blob/master/deploy/docker/compose.nvidia.yml). The image asks for the video encoder itself.
3. Tick *NVIDIA NVENC*.

GeForce cards allow a limited number of encodes at once (the driver decides how many). When they're in use, the next one falls back to software.

### Docker Desktop and WSL on Windows

NVIDIA cards work through Windows' own graphics device, `/dev/dxg`, with NVIDIA's libraries from Windows:

```sh
docker run -d --name wittymediaserver -p 9797:9797 \
  --device /dev/dxg -v /usr/lib/wsl:/usr/lib/wsl:ro -e LD_LIBRARY_PATH=/usr/lib/wsl/lib \
  -v wms-config:/config -v /mnt/d/Media:/media:ro \
  ghcr.io/wittywilla/wittymediaserver:alpha
```

That's for Docker running inside WSL. Docker Desktop passes the card with `--gpus all`. For a server on Windows, though, the [Windows installer](INSTALL.md#windows) is simpler and uses every graphics card directly.

## Behind a reverse proxy

[`compose.reverse-proxy.yml`](https://github.com/wittywilla/WittyMediaServer/blob/master/deploy/docker/compose.reverse-proxy.yml) runs Caddy beside the server, which gets an HTTPS certificate by itself. The server's port isn't published at all: only Caddy can reach it, on a network of their own, and only Caddy's fixed address is trusted to say who a visitor is (`WMS_TRUSTED_PROXIES`).

Read [Remote access](REMOTE.md) first: this puts your sign-in page on the internet. With another proxy, the same two settings matter, `WMS_PUBLIC_URL` and `WMS_TRUSTED_PROXIES`, and [A reverse proxy you already run](REMOTE.md#a-reverse-proxy-you-already-run) has nginx's lines.

> **Trust only the proxy's own address,** never Docker's whole network: anything trusted can claim to be at home.

## Updating and going back

```sh
docker compose pull
docker compose up -d
```

or, with `docker run`: `docker pull ghcr.io/wittywilla/wittymediaserver:alpha`, then `docker rm -f wittymediaserver` and the same `docker run` as before. Everything you keep is in `/config`, so the new container carries on from it. The server brings its database up to date by itself the first time a new version starts.

- **From 0.6.x:** nothing to change. The image still runs as `1000:1000` unless you set `user:`, and the folders are the same.
- **Going back:** use the older version's tag, such as `:0.6.2-alpha`. An older server may not read a database a newer one has changed, so **make a backup before updating** ([Backup](BACKUP.md)), and restore it if you go back.
- **Automatic updates** (Watchtower and the like) work, and are best with `:alpha` only if you're happy to get each alpha the day it's out.

## Is it running?

The image has a health check: every 30 seconds it asks the server itself, with `wittymediaserver health`. `docker ps` shows `(healthy)`, and NAS screens show a green state. It checks the port in `WMS_LISTEN`, so it keeps working if you change that.

## When something's wrong

| You see | Why, and what to do |
|---|---|
| The container stops at once, and the log says `/config` *can't be written* | The folder belongs to another user. `chown` it to the numbers in `user:` (see [User and group IDs](#user-and-group-ids)) |
| `exec format error` | The wrong architecture was pulled, such as on an old 32-bit ARM board. Remove the image and pull again |
| The page doesn't load from another device | The NAS's firewall, or another program on port 9797. Change the left side of `ports:` |
| Hardware encoders all say they can't be used | [Hardware transcoding](#hardware-transcoding); `docker exec wittymediaserver wittymediaserver hardware` says which step |
| Apps don't list the server under **On your network** | Finding servers uses multicast, which doesn't cross Docker's own network. Type the server's address in the app, or run the container with `network_mode: host` (then `ports:` isn't needed) |
| Media folders are empty in the app | The folder isn't mounted, or the user in `user:` can't read it: see [User and group IDs](#user-and-group-ids). `docker exec wittymediaserver ls -ln /media` shows what the server sees |
| *read-only file system* in the log | Something wants to write outside `/config` and `/tmp`. With `read_only: true`, `/tmp` must be a `tmpfs`. Anything else is a bug: please report it |

`docker exec wittymediaserver wittymediaserver doctor` checks the data, modules and install, and `wittymediaserver logs zip` makes a log bundle to share.

## Building it yourself

From a copy of the source:

```sh
docker build -t wittymediaserver .
docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 -t you/wittymediaserver --push .
```

The build compiles the web app and the server on the build machine and only installs packages for the target, so other architectures don't build under emulation.

## What has been tried

| | |
|---|---|
| The image | Built for amd64 in WSL (Ubuntu 26.04), and run with every line of the hardened file as users `1000`, `999:10` and `4242:4242`: healthy, read-only, setup, a library, a module, playing. Upgraded in place from the published `0.6.2-alpha` image, keeping its account and library. `scripts/test/docker-smoke.sh` checks this in CI for every change |
| Hardware | The render-group, missing-device and device-as-a-volume messages, on WSL's render node. NVENC encoding for real with an RTX 3070 through `/dev/dxg`. **Not yet:** real Intel or AMD graphics in a container, an NVIDIA card through the NVIDIA Container Toolkit on Linux |
| Compose files | Every file in `deploy/docker` is checked by `docker compose config`. The reverse-proxy file has been run, with Caddy on plain HTTP: the server answered only through Caddy |
| NAS systems | **None yet.** The UGREEN guide follows a working compose file from a UGREEN owner. Each guide says what to send if you try it |
| arm64 and armv7 | The image builds for both. Neither has been run on a real board |

---

<sub>WittyMediaServer documentation</sub>
