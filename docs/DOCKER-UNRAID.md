# Docker on Unraid

WittyMediaServer on Unraid 6.12 or 7, from the template in this project or as a compose file, with Intel, AMD or NVIDIA graphics for hardware transcoding. For what each setting means, see [Docker](DOCKER.md).

> **Not tried on Unraid yet.** The template and the steps follow Unraid's own screens and the same image run elsewhere. If you try it, the [last section](#tell-us-how-it-went) says what to send.

- [What you need](#what-you-need)
- [The folders and the user](#the-folders-and-the-user)
- [With the template](#with-the-template)
- [Graphics](#graphics)
- [As a compose file](#as-a-compose-file)
- [Updating](#updating)
- [Tell us how it went](#tell-us-how-it-went)

## What you need

- Unraid 6.12 or 7, with Docker enabled (**Settings → Docker**).
- For Intel graphics, nothing else: Unraid loads the driver itself. For NVIDIA, the **Nvidia Driver** plugin from **Apps**.

## The folders and the user

| What | Where, usually |
|---|---|
| The server's data | `/mnt/user/appdata/wittymediaserver`, on the cache pool. Set the `appdata` share to stay on the cache (**Shares → appdata**) |
| Your media | A share such as `/mnt/user/Media` |
| The user | `99:100`, Unraid's `nobody:users`, which owns everything in your shares |

## With the template

1. Copy [the template](https://github.com/wittywilla/WittyMediaServer/blob/master/deploy/unraid/wittymediaserver.xml) to the flash drive, as `/boot/config/plugins/dockerMan/templates-user/my-wittymediaserver.xml`. From the Unraid terminal (**>_** at the top right):

   ```sh
   wget -O /boot/config/plugins/dockerMan/templates-user/my-wittymediaserver.xml \
     https://raw.githubusercontent.com/wittywilla/WittyMediaServer/master/deploy/unraid/wittymediaserver.xml
   ```

2. **Docker → Add Container → Template → wittymediaserver**.
3. Check the **Media** path, and **Apply**.
4. Open `http://<the server's address>:9797/`. The setup code is in the container's log: click its icon on the **Docker** page, then **Logs**.

The template runs it as `99:100`, with a read-only root and no extra powers, and passes `/dev/dri` in.

## Graphics

**Intel and AMD.** The template has `--device=/dev/dri` already. Find the render group from the terminal:

```sh
ls -ln /dev/dri
```

If the `renderD128` line shows `crw-rw-rw-`, anyone may use it, and there's nothing more to do. Otherwise, edit the container, switch on **Advanced View** at the top right, and add `--group-add <number>` to the end of **Extra Parameters**, with the fourth column's number of that line.

**NVIDIA.** With the **Nvidia Driver** plugin installed, copy the card's **GPU UUID** from **Settings → Nvidia Driver**. In the container, add `--runtime=nvidia` to the end of **Extra Parameters** (keep what's there), and add a variable `NVIDIA_VISIBLE_DEVICES` with that UUID. Remove the `/dev/dri` device if the machine has no Intel or AMD graphics.

Then tick the encoder in **Settings → Live transcoding**. Check from the terminal:

```sh
docker exec wittymediaserver wittymediaserver hardware
```

## As a compose file

With the **Docker Compose Manager** plugin, add a stack and use [the Intel and AMD file](https://github.com/wittywilla/WittyMediaServer/blob/master/deploy/docker/compose.intel-amd.yml) or [the NVIDIA file](https://github.com/wittywilla/WittyMediaServer/blob/master/deploy/docker/compose.nvidia.yml), with `user: "99:100"` and these volumes:

```yaml
    volumes:
      - /mnt/user/appdata/wittymediaserver:/config
      - /mnt/user/Media:/media:ro
```

## Updating

The **Docker** page shows **update ready** when there's a newer `:alpha`: click it, or **Check for Updates**. Your data stays in `appdata/wittymediaserver`.

## Tell us how it went

Open an issue on [GitHub](https://github.com/wittywilla/WittyMediaServer/issues) with your Unraid version, your graphics, and the output of:

```sh
ls -ln /dev/dri
docker exec wittymediaserver wittymediaserver hardware
```

---

<sub>WittyMediaServer documentation</sub>
