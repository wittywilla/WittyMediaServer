# Docker on a Synology NAS

WittyMediaServer in **Container Manager** on DSM 7.2 or later, with Intel graphics for hardware transcoding on the models that have it. For what each line means, see [Docker](DOCKER.md).

> **Not tried on a Synology yet.** These steps follow DSM's own screens and the same compose file run elsewhere. If you try it, the [last section](#tell-us-how-it-went) says what to send.

- [What you need](#what-you-need)
- [1. Make the folders](#1-make-the-folders)
- [2. Find your user and the render group](#2-find-your-user-and-the-render-group)
- [3. Create the project](#3-create-the-project)
- [4. Check the graphics](#4-check-the-graphics)
- [Updating](#updating)
- [Tell us how it went](#tell-us-how-it-went)

## What you need

- DSM 7.2 or later with **Container Manager** from the Package Center. It needs an x86 (Intel or AMD) or 64-bit ARM model.
- For hardware transcoding, a model with Intel graphics, such as the DS224+, DS423+, DS720+, DS920+ or DS1520+. Models with an AMD Ryzen processor and no graphics (DS923+, DS1522+) and the ARM models transcode in software, which is fine for one or two streams at 1080p.
- SSH, once, for step 2: **Control Panel → Terminal & SNMP → Enable SSH service**.

## 1. Make the folders

Container Manager makes a shared folder called `docker`. In **File Station**, inside it, make a folder called `wittymediaserver`. Your media stays where it is, such as the `video` shared folder.

On the command line these are `/volume1/docker/wittymediaserver` and `/volume1/video`.

Give your account **Read/Write** on `docker` and at least **Read** on your media: **Control Panel → Shared Folder → Edit → Permissions**.

## 2. Find your user and the render group

Sign in over SSH (`ssh <your account>@<the NAS's address>`) and run:

```sh
id
ls -ln /dev/dri
```

`id` prints something like `uid=1026(you) gid=100(users)`: those numbers go in `user:` as `"1026:100"`.

`ls -ln /dev/dri` is only there on models with graphics. The fourth column of the `renderD128` line is the render group's number, which goes under `group_add`. If that line starts `crw-------` (only root may use it), see [Only root may open the render node](DOCKER.md#only-root-may-open-the-render-node): on a Synology, a **Triggered Task** that runs at boot as root (**Control Panel → Task Scheduler → Create → Triggered Task → User-defined script**) is the place for that command.

Then let your account own the server's folder:

```sh
sudo chown -R 1026:100 /volume1/docker/wittymediaserver
```

## 3. Create the project

In **Container Manager → Project → Create**: name it `wittymediaserver`, set the path to `/docker/wittymediaserver`, choose **Create docker-compose.yml**, and paste this. Change the marked lines, and leave out `group_add` and `devices` on a model without graphics:

```yaml
services:
  wittymediaserver:
    image: ghcr.io/wittywilla/wittymediaserver:alpha
    container_name: wittymediaserver
    restart: unless-stopped
    user: "1026:100"         # <- your numbers from id
    group_add:
      - "937"                # <- the render group from ls -ln /dev/dri
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
      - /volume1/docker/wittymediaserver:/config
      - /volume1/video:/media/video:ro   # <- your media
```

Skip the web portal page, and finish. Open `http://<the NAS's address>:9797/`. The setup code is in **Container Manager → Container → wittymediaserver → Log**.

If DSM's firewall is on, allow TCP port 9797: **Control Panel → Security → Firewall → Edit Rules**.

## 4. Check the graphics

In **Settings → Live transcoding**, *Intel Quick Sync* and *VAAPI* should say **Working on this machine**. If they don't, the page says why. Over SSH:

```sh
sudo docker exec wittymediaserver wittymediaserver hardware
```

## Updating

**Container Manager → Project → wittymediaserver → Action → Build** pulls the newest `:alpha` and starts it again. Or **Image → ghcr.io/wittywilla/wittymediaserver → Update**. Your data stays in `docker/wittymediaserver`.

## Tell us how it went

Open an issue on [GitHub](https://github.com/wittywilla/WittyMediaServer/issues) with your model, your DSM version and the output of:

```sh
ls -ln /dev/dri
sudo docker exec wittymediaserver wittymediaserver hardware
```

---

<sub>WittyMediaServer documentation</sub>
