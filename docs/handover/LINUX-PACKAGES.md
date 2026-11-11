# Hand-over: the AppImage, the Flatpak and the Snap

For whoever picks this up on the owner's Windows 11 machine, in WSL. Written 2026-11-11, on a Mac, during 0.8.0-alpha.

The owner, 2026-11-11: "Keep the linux appimage and flatpak and snap stuff in the repository. I'm working on a mac right now. I'll further refine and build that functionality on my Windows 11 machine with WSL."

**Where it stands.** Everything is written and none of it has been built. `appimagetool`, `flatpak-builder` and `snapcraft` are Linux programs, so on the Mac the three build scripts were only run against stand-ins that recorded what they were asked to do. The scripts ran to the end that way for amd64 and arm64, and the file names they make match what the workflows and the release checklist expect. Nothing has been pushed, so the workflows have never run either.

## What there is

| What | Where |
|---|---|
| The AppImage | `packaging/linux/appimage/build-appimage.sh`, with its tools pinned in `tools.lock` |
| The Flatpak | `packaging/linux/flatpak/`: the manifest, the AppStream metainfo, `build-flatpak.sh` |
| The Snap | `packaging/linux/snap/`: `snapcraft.yaml`, `build-snap.sh` |
| What starts the server inside an AppImage and a Flatpak | `packaging/linux/launcher.sh` |
| CI | `linux-packages` in `.github/workflows/build.yml`, `installers-linux` in `release.yml` |
| The updater's part | `internal/layout` (the three kinds), `internal/update` (`cannotApply`, `InstallerName`, `installCommand`) |
| For people installing | [Installing](../INSTALL.md) |

Each script takes the folder `scripts/build-release.sh` made: `<version> <package folder> <arch> [out folder]`.

## Building them in WSL

Use a clone on WSL's own disk, as [CONTRIBUTING](../../CONTRIBUTING.md#tests-and-checks) says. Snap needs systemd, which WSL has when `/etc/wsl.conf` holds `[boot]` and `systemd=true`.

```bash
sudo apt-get update && sudo apt-get install -y flatpak flatpak-builder
```

```bash
sudo snap install snapcraft --classic
```

```bash
scripts/build-release.sh 0.8.0-dev linux/amd64
```

```bash
packaging/linux/appimage/build-appimage.sh 0.8.0-dev dist/wittymediaserver-0.8.0-dev-linux-amd64 amd64 packages
```

```bash
packaging/linux/flatpak/build-flatpak.sh 0.8.0-dev dist/wittymediaserver-0.8.0-dev-linux-amd64 amd64 packages
```

```bash
SNAPCRAFT_BUILD_ENVIRONMENT=host packaging/linux/snap/build-snap.sh 0.8.0-dev dist/wittymediaserver-0.8.0-dev-linux-amd64 amd64 packages
```

## What to try first, most likely to fail first

1. **The Snap's build.** CI builds it on the host (`SNAPCRAFT_BUILD_ENVIRONMENT=host`), not as root, and makes the arm64 one on an amd64 machine. snapcraft sometimes wants root or LXD for that.
2. **The Flatpak's metainfo.** `flatpak-builder` runs `appstreamcli compose` on it. It may refuse the `.webp` screenshot, the 256 px icon, or a version such as `dev~abcdef12` on a build that isn't a tag. The arm64 build runs under QEMU.
3. **Running the Flatpak with no desktop.** CI runs it on a machine with no session D-Bus. Warnings are expected; a failure isn't known either way.
4. **The Snap's service.** CI expects the strictly confined service, as root, to answer on 9797 within about 30 seconds of its first start.
5. **Modules.** A module's helper is sandboxed with user namespaces, which a Flatpak and a strict Snap will probably refuse. Install a module in each and see. If it fails, [Installing](../INSTALL.md) has to say so plainly, or the sandbox needs another way in.
6. **Start at login in the Flatpak.** The app writes `~/.config/autostart`, which is private to the sandbox there, so the switch does nothing. It needs the Background portal, or to be hidden. (In an AppImage it starts the AppImage itself: `cmd/wittymediaserver-tray/platform_other.go`.)
7. **Stopping an AppImage.** `launcher.sh` runs the server in a loop so it can restart itself. A signal sent to the launcher alone, not its process group, may leave the server running.

## Decisions for the owner

- **The Flatpak's name.** It is `com.wittywilla.WittyMediaServer`. The Mac package is `com.wittywilla.wittymediaserver`, in small letters. A Flatpak's name can't change once people have it, and Flathub would want the domain proved.
- **Whether these hold a release up.** Today a failure in any of the three stops the release, as the other installers do. For the first tag it may be kinder to publish without one that fails.
- **The Snap's permissions.** Its service runs as root, so reaching home folders, drives, tuners and cameras each needs a `snap connect` by hand, and installing the file needs `--dangerous` because it isn't in the store. The name `wittymediaserver` isn't registered there.
- **The Flatpak's reach.** The network, the graphics card, and Videos, Music and Pictures to read. Anything else is a `flatpak override` the docs describe.

The client's Flatpak and Snap are in the same state, in WittyMediaClient: its `docs/PORTING.md` and `docs/OWNER-STEPS.md` say where.
