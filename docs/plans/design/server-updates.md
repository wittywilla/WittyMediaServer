# Design: updating the server from the app

> **Status on 2026-11-04**
>
> **In progress:** readable notes and a download button, [#72](https://github.com/wittywilla/WittyMediaServer/issues/72). **Planned:** installing from the app, [the idea](https://github.com/wittywilla/WittyMediaServer/discussions/23).

The owner, 2026-11-02, looking at **Settings → Updates** with 0.6.1-alpha available: "There's no obvious way for me to update in the UI. How do I do it with our current implementation? ... Can we also make sure the updates are easier to find as well within the UI?"

Not built. Not tested.

---

## What happens today

From `internal/update`:

- The server checks GitHub's release list twice a day (switch: **Check for updates automatically**), and a dot appears on **Settings**.
- It can install an update **itself only when it can write to its own program folder**: a copy unpacked from the `.zip` or `.tar.gz`. It downloads the archive, checks the signed `SHA256SUMS`, swaps the files and restarts.
- A copy installed by the **MSI** (Windows, in `C:\Program Files\WittyMediaServer`), the **`.pkg`** (macOS), **`.deb`/`.rpm`** (Linux), **FreeBSD `pkg`** or **Docker** can't write there, so `canApply` is false and the page says why. All it offers is **Release page on GitHub ›**.

So today, on the owner's PC: open the release page, download `wittymediaserver-<version>-windows-amd64.msi`, and run it. It upgrades in place and keeps the libraries and settings, which live in the data folder.

### Faults visible in the owner's screenshot

1. **No button that does anything.** The only action is a link to a GitHub page, where the person has to choose the right file out of 26.
2. **Release notes shown as raw Markdown.** The table of downloads appears as `| Windows | .msi |` lines, and links as `[install guide](https://...)`. The notes are written for GitHub's page, not for this card.
3. **The wrong notes.** The card shows the download table (which file for which system), not what changed in this version.
4. **The way it's found.** A dot on **Settings**, then the right page inside Settings. Easy to miss.

---

## 1. Easier to find (small: 0.6.3-alpha)

- **A banner for admins** at the top of Home and of Settings: "WittyMediaServer 0.6.1-alpha is available: Firefox disc menus fixed, and 2 more changes. [Update] [What's new] [Later]". **Later** hides it for that version for this admin.
- **The activity button** in the top bar (added in 0.6.0) shows an "Update available" item for admins, and the download and install progress as a task, as other tasks do.
- **The user menu** gets an **Update available** line for admins.
- **About** shows the installed version, the newest, and the same button.
- **WittyMediaClient** shows the same line in its server menu for admins, from the same route.
- **The tray icon** (Windows, macOS) gets **Update to 0.6.1-alpha…** in its menu, and a notification when a new version is found, if the owner allows notifications.
- Non-admins never see any of it.

## 2. Notes people can read (small: 0.6.3-alpha)

- **What's new, not the download table.** Each release's notes start with a short summary the publish script writes from `CHANGELOG.md` (the section for that version). The card shows that summary, rendered as Markdown with the same safe renderer the help pages use (tables, links, lists; no raw HTML, links opened outside the app with `rel="noopener noreferrer"`).
- The download table stays on GitHub, under the summary, for people who download by hand.
- **Changes since your version:** when someone skips versions, the card lists each version's summary in order, newest first.

## 3. A button that works for every kind of install

The right action depends on how the server was installed. The server knows that already (`noteInstalled` on Windows; the packaging tells it elsewhere). Each gets a real button.

| Installed with | The button | How (to build) |
|---|---|---|
| **Archive** (zip, tar.gz) | **Update and restart** | As today: download, verify, swap, restart |
| **Windows MSI** | **Download and install** | The server downloads the matching `.msi` for its system and CPU (amd64 or arm64), checks it against the signed `SHA256SUMS`, then hands it to the tray app, which runs `msiexec /i <file> /qb` after Windows' own admin prompt (UAC). When the server runs as the Windows service (SYSTEM), it can start the upgrade itself with no prompt, since the service already has the rights; it says so first and asks for the admin's password in the web page |
| **macOS pkg** | **Download and install** | Downloads and verifies the `.pkg`, then opens it with macOS's installer (needs the admin's password, which macOS asks for itself). Signed and notarised packages only (see the code signing plan) |
| **Linux deb / rpm** | **How to update** | Shows the one command for this system (`sudo apt install ./file.deb` with the file already downloaded and checked, or `sudo apt upgrade` once the apt and dnf repositories are hosted). A server never runs `sudo` itself |
| **FreeBSD pkg** | **How to update** | As Linux, with `pkg` |
| **Docker** | **How to update** | The `docker compose pull && docker compose up -d` lines for this container, copied with one click |

**Also:**

- **Never a half-installed server.** Before handing over, the server makes a backup of its database (the 0.6.0 ZIP backup, without media), and the installer keeps the data folder. If the new version won't start, the tray app offers to go back to the previous version (the old installer is kept until the new version has started once).
- **Timing:** "Update now" or "Update tonight at 3:00" (when nobody's watching). An update never starts while someone's watching, recording or playing a game unless the admin says so, and anyone watching gets a notice first.
- **Signing first:** under Windows Smart App Control, an unsigned installer is blocked, so the Windows button depends on the [code signing plan](code-signing-pipeline.md). Until the MSI is signed, the button still downloads and verifies, then explains: "Windows may warn that this installer isn't recognised. Choose More info, then Run anyway."
- **Modules** already update from the store; the banner says when a server update needs module updates too, and Update All runs after the server restarts.

## 4. Automatic updates (later)

An admin switch, off by default: **Install updates by themselves**, at a quiet hour, for archive installs and for Windows when running as the service. It never jumps a major version on its own. A version taken back in the signed list (`withdrawn`) is never installed, and a server already on it is told to go back.

---

## Where it goes

| Release | Work |
|---|---|
| **0.6.3-alpha** | Easier to find (banner, activity item, user menu, About, client, tray menu); readable "what's new" notes; a direct **Download the installer** button for the right file, so nobody hunts through GitHub |
| **0.7.0-alpha** | **Download and install** for the Windows MSI (tray app and service), with backup first, timing and the "nobody's watching" rule |
| **0.8.0-alpha** (next major) | macOS pkg after notarisation; Linux and FreeBSD command help; Docker help; going back after a failed update; automatic updates at a quiet hour |

## Done when

On the owner's Windows PC, with an MSI install: a newer release shows a banner on Home within a day; one click downloads, verifies and installs it after Windows' prompt; the server comes back on the new version with libraries, settings and watch history unchanged; and the notes say what changed in plain words.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
