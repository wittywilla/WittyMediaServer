# Docker on a QNAP NAS

WittyMediaServer in **Container Station 3** on QTS 5 or QuTS hero, with Intel or AMD graphics for hardware transcoding where the model has them. For what each line means, see [Docker](DOCKER.md).

> **Not tried on a QNAP yet.** These steps follow Container Station's own screens and the same compose file run elsewhere. If you try it, the [last section](#tell-us-how-it-went) says what to send.

- [What you need](#what-you-need)
- [1. Make the folders](#1-make-the-folders)
- [2. Find your user and the render group](#2-find-your-user-and-the-render-group)
- [3. Create the application](#3-create-the-application)
- [4. Check the graphics](#4-check-the-graphics)
- [Updating](#updating)
- [Tell us how it went](#tell-us-how-it-went)

## What you need

- QTS 5.1 or QuTS hero h5.1 or later, with **Container Station** 3 from the App Center.
- For hardware transcoding, a model with Intel graphics (most Celeron, Pentium and Core models, such as the TS-464, TS-264 and TS-664) or an AMD Ryzen Embedded V-series. Models without graphics, and the ARM models, transcode in software.
- SSH, once, for step 2: **Control Panel → Network & File Services → Telnet / SSH → Allow SSH connection**.

## 1. Make the folders

Container Station makes a shared folder called `Container`. In **File Station**, make `Container/wittymediaserver`. Your media stays where it is, such as the `Multimedia` shared folder.

On the command line these are `/share/Container/wittymediaserver` and `/share/Multimedia`.

## 2. Find your user and the render group

Sign in over SSH and run:

```sh
id <your account>
ls -ln /dev/dri
```

Use an ordinary account here, not `admin`, which is root on a QNAP. `id` prints something like `uid=1000(you) gid=100(everyone)`: those go in `user:` as `"1000:100"`. That account needs Read/Write on `Container` and Read on your media (**Control Panel → Shared Folders → Edit Shared Folder Permission**).

The fourth column of the `renderD128` line from `ls -ln /dev/dri` is the render group's number, for `group_add`. If the line shows `crw-rw-rw-`, everyone may use it already, and `group_add` does no harm. If it shows `crw-------`, see [Only root may open the render node](DOCKER.md#only-root-may-open-the-render-node).

Then let the account own the server's folder:

```sh
chown -R 1000:100 /share/Container/wittymediaserver
```

## 3. Create the application

In **Container Station → Applications → Create**, name it `wittymediaserver`, and paste this. Change the marked lines, and leave out `group_add` and `devices` on a model without graphics:

```yaml
services:
  wittymediaserver:
    image: ghcr.io/wittywilla/wittymediaserver:alpha
    container_name: wittymediaserver
    restart: unless-stopped
    user: "1000:100"         # <- your numbers from id
    group_add:
      - "44"                 # <- the render group from ls -ln /dev/dri
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
      - /share/Container/wittymediaserver:/config
      - /share/Multimedia:/media:ro   # <- your media
```

Press **Validate**, then **Create**. Open `http://<the NAS's address>:9797/`. The setup code is in **Containers → wittymediaserver → Logs**.

QNAP's own apps use some ports; if 9797 is taken, change the left-hand number, as in `"9800:9797"`.

## 4. Check the graphics

In **Settings → Live transcoding**, *Intel Quick Sync* (Intel) or *VAAPI* (Intel and AMD) should say **Working on this machine**. If they don't, the page says why. Over SSH:

```sh
docker exec wittymediaserver wittymediaserver hardware
```

QNAP's own media apps may hold the graphics while they transcode. That's fine: both can use it at once, up to what the chip can do.

## Updating

**Applications → wittymediaserver → ⋯ → Recreate**, with **Pull the latest image** ticked. Your data stays in `Container/wittymediaserver`.

## Tell us how it went

Open an issue on [GitHub](https://github.com/wittywilla/WittyMediaServer/issues) with your model, QTS version and the output of:

```sh
ls -ln /dev/dri
docker exec wittymediaserver wittymediaserver hardware
```

---

<sub>WittyMediaServer documentation</sub>
