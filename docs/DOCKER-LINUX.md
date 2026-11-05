# Docker on a Linux machine

WittyMediaServer with Docker on any Linux computer or server, such as Debian, Ubuntu, Fedora or OpenMediaVault, with Intel, AMD or NVIDIA graphics for hardware transcoding. For what each line means, see [Docker](DOCKER.md). Without Docker, the [packages](INSTALL.md#linux) are another way.

> **Tried so far:** these steps, with the hardened compose file, on Ubuntu 26.04 under WSL. A real Intel, AMD or NVIDIA card on a Linux machine hasn't been tried yet: NVENC has been run through WSL's own graphics device.

- [1. Install Docker](#1-install-docker)
- [2. Make the folders](#2-make-the-folders)
- [3. Find your user and the render group](#3-find-your-user-and-the-render-group)
- [4. Write the compose file](#4-write-the-compose-file)
- [5. Start it](#5-start-it)
- [Graphics](#graphics)
- [Updating](#updating)

## 1. Install Docker

Use Docker's own packages, which include Compose: [docs.docker.com/engine/install](https://docs.docker.com/engine/install/). Then let your account use it without `sudo`, and sign in again:

```sh
sudo usermod -aG docker "$USER"
```

## 2. Make the folders

```sh
mkdir -p ~/wittymediaserver/config
cd ~/wittymediaserver
```

`config` holds the server's data. Your media can stay where it is.

## 3. Find your user and the render group

```sh
id -u; id -g
ls -ln /dev/dri
```

The first two numbers go in `user:`. The fourth column of the `renderD128` line is the render group, for `group_add` (on most systems it's the `render` group: `getent group render`). No `/dev/dri` means no Intel or AMD graphics driver is loaded.

> **Folders empty in the app, or "permission denied"?** The user and group numbers don't match your folders. [User and group IDs](DOCKER.md#linux-user-and-group-ids) says how to tell, find the right ones and fix it.

## 4. Write the compose file

Save this as `compose.yml` in `~/wittymediaserver`, with your numbers and media folder:

```yaml
services:
  wittymediaserver:
    image: ghcr.io/wittywilla/wittymediaserver:alpha
    container_name: wittymediaserver
    restart: unless-stopped
    user: "1000:1000"        # <- id -u and id -g
    group_add:
      - "105"                # <- the render group from ls -ln /dev/dri
    devices:
      - /dev/dri:/dev/dri
    read_only: true
    tmpfs:
      - /tmp
    cap_drop:
      - ALL
    security_opt:
      - no-new-privileges:true
    ports:
      - "9797:9797"
    volumes:
      - ./config:/config
      - /srv/media:/media:ro  # <- your media
```

Leave out `group_add` and `devices` on a machine without Intel or AMD graphics. [More examples](https://github.com/wittywilla/WittyMediaServer/tree/master/deploy/docker) cover NVIDIA and a reverse proxy.

## 5. Start it

```sh
docker compose up -d
docker compose logs wittymediaserver | grep -i "setup code"
```

Open `http://<this machine>:9797/` and enter the setup code.

## Graphics

**Intel and AMD:** the file above already does it. **NVIDIA:** install the driver and the [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html), check it with `docker run --rm --gpus all ubuntu nvidia-smi`, and use [the NVIDIA file](https://github.com/wittywilla/WittyMediaServer/blob/master/deploy/docker/compose.nvidia.yml).

Then tick the encoder in **Settings → Live transcoding**, and check:

```sh
docker exec wittymediaserver wittymediaserver hardware
```

## Updating

```sh
cd ~/wittymediaserver
docker compose pull
docker compose up -d
```

Your data stays in `config`. [Updating](DOCKER.md#updating-and-going-back) says how to go back to an older version.

---

<sub>WittyMediaServer documentation</sub>
