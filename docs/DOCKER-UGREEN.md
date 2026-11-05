# Docker on a UGREEN NAS

WittyMediaServer on a UGREEN NASync (UGOS Pro), with its Intel graphics for hardware transcoding. For what each line means, see [Docker](DOCKER.md).

> **Tried so far:** this compose file is the one a UGREEN owner runs, and it plays. Hardware transcoding with the fixes below hasn't been run on a UGREEN NAS yet. If you try it, the [last section](#tell-us-how-it-went) says what to send.

- [What you need](#what-you-need)
- [1. Make the folders](#1-make-the-folders)
- [2. Find your user and the render group](#2-find-your-user-and-the-render-group)
- [3. Create the project](#3-create-the-project)
- [4. Check the graphics](#4-check-the-graphics)
- [Updating](#updating)
- [Tell us how it went](#tell-us-how-it-went)

## What you need

- UGOS Pro, with the **Docker** app from the App Center.
- An Intel model for hardware transcoding: the DXP2800, DXP4800, DXP4800 Plus, DXP6800 Pro and DXP8800 Plus all have Intel graphics. On any model, software transcoding works.
- SSH turned on for step 2, once: **Control Panel → Terminal → SSH**. You can turn it off again after.

## 1. Make the folders

In **Files**, on the volume you want (usually *volume1*):

- `docker/wittymediaserver`: the server's own data. Put it on an SSD volume if you have one, as it holds the database and artwork.
- Your media can stay where it is, such as a shared folder called `Media`.

On the command line these are `/volume1/docker/wittymediaserver` and `/volume1/Media`.

## 2. Find your user and the render group

Sign in over SSH (`ssh <your account>@<the NAS's address>`) and run:

```sh
id
ls -ln /dev/dri
```

`id` prints something like `uid=999(you) gid=10(admin) ...`: the two numbers go in `user:` below as `"999:10"`. Use the account that owns `docker/wittymediaserver`, and that can read your media.

`ls -ln /dev/dri` prints lines like:

```
crw-rw---- 1 0 105 226, 128 ... renderD128
```

The fourth column, **105** here, is the render group. It goes under `group_add`. If that line shows `crw-------` (only the owner may use it), see [Only root may open the render node](DOCKER.md#only-root-may-open-the-render-node).

> **Folders empty in the app, or "permission denied"?** The user and group numbers don't match your folders. [User and group IDs](DOCKER.md#ugreen-user-and-group-ids) says how to tell, find the right ones and fix it.

## 3. Create the project

In **Docker → Project → Create**, name it `wittymediaserver`, choose `/volume1/docker/wittymediaserver` as its path, and paste this. Change the three marked lines:

```yaml
services:
  wittymediaserver:
    image: ghcr.io/wittywilla/wittymediaserver:alpha
    container_name: wittymediaserver
    restart: unless-stopped
    user: "999:10"          # <- your numbers from id
    group_add:
      - "105"               # <- the render group from ls -ln /dev/dri
    read_only: true
    tmpfs:
      - /tmp
    cap_drop:
      - ALL
    security_opt:
      - no-new-privileges:true
    ports:
      - "9797:9797"
    environment:
      - WMS_LOG=info
    volumes:
      - /volume1/docker/wittymediaserver:/config
      - /volume1/Media:/media:ro   # <- your media folder
    devices:
      - /dev/dri:/dev/dri
```

Start it, and open `http://<the NAS's address>:9797/`. The setup code is in the container's log: **Docker → Container → wittymediaserver → Log**.

In the app, add your folders from `/media` as libraries.

## 4. Check the graphics

In **Settings → Live transcoding**, under **Hardware encoders**, *Intel Quick Sync* and *VAAPI* should say **Working on this machine (/dev/dri/renderD128)**. Tick Intel Quick Sync, and press **Save changes**.

If they don't, the page says why, and so does this, over SSH:

```sh
sudo docker exec wittymediaserver wittymediaserver hardware
```

| It says | Do this |
|---|---|
| *isn't in group 105, which owns /dev/dri/renderD128* | The `group_add` number is wrong or missing. Use the one from `ls -ln /dev/dri`, then recreate the project's container |
| *No graphics device was passed to the container* | The `devices:` lines are missing |
| *can see /dev/dri/renderD128 but isn't allowed to use it* | `/dev/dri` was added as a volume instead of a device |
| *The VA-API driver for this graphics chip isn't installed* | You're on an old image: pull `:alpha` again |

## Updating

**Docker → Project → wittymediaserver → Action → Pull** (or **Rebuild**), then start it again. Your settings stay in `/volume1/docker/wittymediaserver`. Over SSH, the same is:

```sh
cd /volume1/docker/wittymediaserver && sudo docker compose pull && sudo docker compose up -d
```

## Tell us how it went

Open an issue on [GitHub](https://github.com/wittywilla/WittyMediaServer/issues) with your model, and the output of:

```sh
ls -ln /dev/dri
sudo docker exec wittymediaserver wittymediaserver hardware
```

---

<sub>WittyMediaServer documentation</sub>
