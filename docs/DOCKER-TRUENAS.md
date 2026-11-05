# Docker on TrueNAS SCALE

WittyMediaServer as a custom app on TrueNAS SCALE 24.10 (Electric Eel) or later, which runs apps with Docker, with Intel, AMD or NVIDIA graphics for hardware transcoding. For what each line means, see [Docker](DOCKER.md).

> **Not tried on TrueNAS yet.** These steps follow TrueNAS's own screens and the same compose file run elsewhere. If you try it, the [last section](#tell-us-how-it-went) says what to send.

- [What you need](#what-you-need)
- [1. Make the datasets](#1-make-the-datasets)
- [2. Find the render group](#2-find-the-render-group)
- [3. Install the app](#3-install-the-app)
- [4. Check the graphics](#4-check-the-graphics)
- [Updating](#updating)
- [Tell us how it went](#tell-us-how-it-went)

## What you need

- TrueNAS SCALE 24.10 or later (Community Edition 25.04 and later are the same). Older releases ran apps another way, and these steps don't apply to them.
- An apps pool: **Apps → Configuration → Choose Pool**.
- For NVIDIA: **Apps → Configuration → Settings → Install NVIDIA Drivers**.

## 1. Make the datasets

In **Datasets**, on your pool (called `tank` here):

- `tank/apps/wittymediaserver`, with the **Apps** preset. Its owner is the `apps` user, `568:568`, which the app runs as.
- Your media can stay where it is, such as `tank/media`. Give the `apps` user read access: **Datasets → media → Permissions → Edit**, add `apps` with Read.

> **Folders empty in the app, or "permission denied"?** The user and group numbers don't match your folders. [User and group IDs](DOCKER.md#truenas-scale-user-and-group-ids) says how to tell, find the right ones and fix it.

## 2. Find the render group

For Intel or AMD graphics, open **System → Shell** and run:

```sh
ls -ln /dev/dri
```

The fourth column of the `renderD128` line is the render group's number, for `group_add`.

## 3. Install the app

**Apps → Discover Apps → ⋮ → Install via YAML**. Name it `wittymediaserver` and paste this. Change the marked lines:

```yaml
services:
  wittymediaserver:
    image: ghcr.io/wittywilla/wittymediaserver:alpha
    container_name: wittymediaserver
    restart: unless-stopped
    user: "568:568"
    group_add:
      - "107"                # <- the render group from ls -ln /dev/dri
    devices:
      - /dev/dri:/dev/dri    # Intel or AMD; leave out for NVIDIA only
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
      - /mnt/tank/apps/wittymediaserver:/config   # <- your pool
      - /mnt/tank/media:/media:ro                 # <- your media
```

For NVIDIA, add this under the service, at the same depth as `volumes:`:

```yaml
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]
```

Save. Open `http://<the NAS's address>:9797/`. The setup code is in **Apps → wittymediaserver → Workloads → Logs**.

## 4. Check the graphics

In **Settings → Live transcoding**, the encoder for your card should say **Working on this machine**. If not, the page says why. From **System → Shell**:

```sh
sudo docker exec wittymediaserver wittymediaserver hardware
```

## Updating

**Apps → wittymediaserver → Update** when TrueNAS shows one, or **Edit → Save** to pull `:alpha` again. Your data stays in the dataset.

## Tell us how it went

Open an issue on [GitHub](https://github.com/wittywilla/WittyMediaServer/issues) with your TrueNAS version, your graphics, and the output of:

```sh
ls -ln /dev/dri
sudo docker exec wittymediaserver wittymediaserver hardware
```

---

<sub>WittyMediaServer documentation</sub>
