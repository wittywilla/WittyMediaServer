# Docker on an Asustor NAS

WittyMediaServer on ADM 4.2 or later, with **Portainer** to create it from a compose file, and Intel graphics for hardware transcoding where the model has them. For what each line means, see [Docker](DOCKER.md).

> **Not tried on an Asustor yet.** These steps follow ADM's and Portainer's own screens and the same compose file run elsewhere. If you try it, the [last section](#tell-us-how-it-went) says what to send.

- [What you need](#what-you-need)
- [1. Make the folders](#1-make-the-folders)
- [2. Find your user and the render group](#2-find-your-user-and-the-render-group)
- [3. Create the stack](#3-create-the-stack)
- [4. Check the graphics](#4-check-the-graphics)
- [Updating](#updating)
- [Tell us how it went](#tell-us-how-it-went)

## What you need

- ADM 4.2 or later, with **Docker Engine** and **Portainer CE** from App Central.
- For hardware transcoding, a model with Intel graphics, such as the Lockerstor and Flashstor Gen 2, or the AS54, AS66 and AS67 series. The ARM models (Drivestor) transcode in software.
- SSH, once, for step 2: **Services → Terminal → Enable SSH service**.

## 1. Make the folders

Docker Engine makes a shared folder called `Docker`. In **File Explorer**, make `Docker/wittymediaserver`. Your media stays where it is, such as the `Media` shared folder.

On the command line these are `/volume1/Docker/wittymediaserver` and `/volume1/Media`. If your shares are on another volume, the number changes.

## 2. Find your user and the render group

Sign in over SSH (`ssh <your account>@<the NAS's address>`) and run:

```sh
id
ls -ln /dev/dri
```

`id` prints something like `uid=1000(you) gid=100(users)`: those go in `user:` as `"1000:100"`. The fourth column of the `renderD128` line is the render group's number, for `group_add`. If it starts `crw-------`, see [Only root may open the render node](DOCKER.md#only-root-may-open-the-render-node).

Let the account own the server's folder:

```sh
sudo chown -R 1000:100 /volume1/Docker/wittymediaserver
```

> **Folders empty in the app, or "permission denied"?** The user and group numbers don't match your folders. [User and group IDs](DOCKER.md#asustor-user-and-group-ids) says how to tell, find the right ones and fix it.

## 3. Create the stack

Open **Portainer**, choose the local environment, then **Stacks → Add stack**. Name it `wittymediaserver` and paste this into the web editor. Change the marked lines, and leave out `group_add` and `devices` on a model without graphics:

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
      - /volume1/Docker/wittymediaserver:/config
      - /volume1/Media:/media:ro   # <- your media
```

**Deploy the stack**. Open `http://<the NAS's address>:9797/`. The setup code is in **Containers → wittymediaserver → Logs**.

## 4. Check the graphics

In **Settings → Live transcoding**, *Intel Quick Sync* and *VAAPI* should say **Working on this machine**. If they don't, the page says why. Over SSH:

```sh
sudo docker exec wittymediaserver wittymediaserver hardware
```

## Updating

In Portainer, **Stacks → wittymediaserver → Editor → Update the stack**, with **Re-pull image** on. Your data stays in `Docker/wittymediaserver`.

## Tell us how it went

Open an issue on [GitHub](https://github.com/wittywilla/WittyMediaServer/issues) with your model, ADM version and the output of:

```sh
ls -ln /dev/dri
sudo docker exec wittymediaserver wittymediaserver hardware
```

---

<sub>WittyMediaServer documentation</sub>
