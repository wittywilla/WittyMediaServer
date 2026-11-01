# Uninstalling WittyMediaServer

Remove WittyMediaServer from any system, keeping your library or deleting everything, and clean up by hand if something went wrong.

It never touches your media. Your films, shows, music, photos, books and games stay where they are.

- [The short answer](#the-short-answer)
- [What it asks](#what-it-asks)
- [What goes and what stays](#what-goes-and-what-stays)
- [Windows](#windows)
- [macOS](#macos)
- [Linux](#linux)
- [FreeBSD](#freebsd)
- [Docker](#docker)
- [Options for scripts](#options-for-scripts)
- [If something went wrong](#if-something-went-wrong)
- [Removing the apps](#removing-the-apps)

## The short answer

| System | The usual way | On the command line |
|---|---|---|
| **Windows** | **Settings → Apps → Installed apps**, then **WittyMediaServer**, then **Uninstall** | `wittymediaserver --uninstall` |
| **macOS** | Open **Uninstall WittyMediaServer** in Applications | `sudo wittymediaserver --uninstall` |
| **Debian, Ubuntu** | `sudo wittymediaserver --uninstall` | `sudo apt remove wittymediaserver` keeps your data, `sudo apt purge wittymediaserver` deletes it |
| **Fedora, RHEL** | `sudo wittymediaserver --uninstall` | `sudo dnf remove wittymediaserver` keeps your data |
| **FreeBSD** | `wittymediaserver --uninstall`, as root | `pkg delete wittymediaserver` keeps your data |
| **Anything else** | From the program's `bin` folder: `sudo ./wittymediaserver --uninstall` | |

This works even if the installer has gone missing. If you only want to start again with an empty server and keep the program, use **Settings → Reset** instead. If you want to keep a copy to bring back later, make a [backup](BACKUP.md) first.

**How far this has been run.** The Linux packages have been removed on Debian, Ubuntu and Fedora, and the FreeBSD package in virtual machines, each with a listing from before and after that had to match. On Windows it has run under Wine, not on a real copy of Windows. On a Mac it has only been tested with stand-in folders: this has not been run on a real Mac yet.

## What it asks

It asks in plain words, one question at a time. If you say no or close the question, that part stays.

1. **Remove WittyMediaServer from this computer?** This is the program, its service, shortcuts and anything that starts it.
2. **Also delete your library, accounts and settings?** It shows how much space they take. Say no to keep them, so they're still there if you install it again.
3. **Also delete your recorded TV?** Only asked if there are recordings in the data folder.
4. **Also delete your backups?** Only asked if there are backups in the data folder. Say no to keep them, so you can restore from them after installing it again.

When it's finished, it lists what it removed and what it couldn't. On Windows and macOS that list is also saved in your temp folder, as `WittyMediaServer-uninstall-<date>.txt`.

## What goes and what stays

**Always removed:**

- The program files, and the command.
- The service, and anything that starts it at boot or sign-in.
- Shortcuts and menu entries.
- Firewall rules and registry entries, on Windows.

**Removed if you say yes to the second question.** It removes only these names, from the top of each data folder:

| In the data folder | What it is |
|---|---|
| `wms.db`, `wms.db-wal`, `wms.db-shm` | Library, accounts and settings |
| `cache` | Artwork and other caches |
| `transcode` | Temporary video |
| `logs`, `wms.log` | Logs |
| `crash` | Crash reports |
| `diagnostics` | Saved diagnostic reports |
| `certs` | HTTPS certificates |
| `branding` | Your server's logo |
| `channel-logos` | TV channel logos |
| `bumpers` | TV bumpers |
| `lyrics-corrections` | Lyrics you corrected |
| `modules` | Downloaded modules |
| `themes` | Themes |
| `gaming` | Game saves, cores and settings |
| `gamerigs`, `wittycouch` | Game PC pairings |
| `metadata` | Text files for fixing titles by hand |
| `setup-code.txt` | The code for setting up from another device |
| `update-withdrawn.json` | Which releases were withdrawn |
| `reset-request.json`, `reset-result.json` | A waiting reset, and the last result |
| `restore-staged`, `restore-request.json`, `restore-result.json` | A backup on its way back, and the last result |
| `repair-request.json`, `repair-result.json` | A waiting repair, and the last result |
| `before-reset-` and anything after it | Copies kept by an earlier reset |
| `repair-stage-` and anything after it | A repair that didn't finish |

Your `wms.json` settings file goes too, and the data folder itself once it's empty. On a Mac, so do the app's caches and preferences in your Library folder. On Linux and FreeBSD, the service's `wms` account goes as well.

**Removed only if you say yes to their own question:**

| In the data folder | What it is |
|---|---|
| `recordings` | Recorded TV |
| `backups` | Backups you made, and the safety copies from restores and repairs |

An empty one goes with the rest. A silent removal and `apt purge` leave these if they hold anything.

**Never touched, whatever you answer:**

- **Your media,** and any folder that holds a library, whatever it's called and wherever it is.
- **Anything it didn't make,** such as a file you put in the data folder. It stays, and the list says so.
- **Artwork and details saved beside your media files,** which are yours to keep with those files.
- **Originals kept by library transcoding,** in hidden `.wittymedia-originals` folders beside your media. See [Library transcoding](LIBRARY-TRANSCODING.md).
- **A folder you chose yourself,** such as a cache on another drive, in [`wms.json`](CONFIG.md#what-you-can-set), a log folder or a recordings folder in Settings. These are listed for you to delete.

**Other people's settings.** Run as an administrator or with `sudo`, it also clears what other accounts on the computer have: their data folders, settings files and shortcuts. Other accounts' recordings and backups get the same questions.

## Windows

**To remove it:**

1. Open **Settings → Apps → Installed apps**.
2. Find **WittyMediaServer**, and choose **Uninstall**.
3. Answer the questions. If it was installed for everyone, Windows asks for an administrator.
4. Read the list at the end. It's done in the background, from a copy in the temp folder, as Windows can't delete a program that's running.

`Uninstall WittyMediaServer.exe` in the program's folder does the same, and so does `wittymediaserver --uninstall`, which needs a terminal opened with **Run as administrator** if the copy is for everyone. `msiexec /x` also works, and keeps your data.

| It removes | Where |
|---|---|
| The program | `C:\Program Files\WittyMediaServer`, `%LOCALAPPDATA%\Programs\WittyMediaServer`, or the folder an unzipped copy is in. Only the files it knows |
| The service and the old boot task | Both named `WittyMediaServer` |
| The installer's record | Windows Installer's own entry for it |
| The firewall rule | Named `WittyMediaServer` |
| Starting at sign-in | The `WittyMediaServer` value in `Software\Microsoft\Windows\CurrentVersion\Run`, in `HKCU` and `HKLM` |
| Installed apps entry | `Software\Microsoft\Windows\CurrentVersion\Uninstall\WittyMediaServer`, in `HKCU` and `HKLM` |
| The installer's choices | `Software\WittyWilla\WittyMediaServer`, in `HKCU` and `HKLM`, and `Software\WittyWilla` once it's empty |
| Shortcuts | The Start menu and desktop, for you and for everyone |
| The command on the path | The installer's entry in `PATH` |
| If you say yes: your data | `%LOCALAPPDATA%\WittyMediaServer`, `C:\ProgramData\WittyMediaServer`, a `data` folder beside a portable copy, and whichever folder `WMS_DATA_DIR` or the service's settings name |
| If you say yes: settings | `%APPDATA%\WittyMediaServer\wms.json`, or a `wms.json` in the program's folder |

## macOS

This has not been run on a real Mac yet.

**To remove it:**

1. Open **Uninstall WittyMediaServer** in Applications.
2. Answer the questions. The Mac asks for an administrator's password.
3. Read the list at the end.

Dragging the app to the Trash only removes the app, and leaves the service and everything else below.

| It removes | Where |
|---|---|
| The apps | `/Applications/WittyMediaServer.app`, `/Applications/Uninstall WittyMediaServer.app` |
| The command | `/usr/local/bin/wittymediaserver` |
| The service | `/Library/LaunchDaemons/com.wittywilla.wittymediaserver.plist` |
| Starting at sign-in | `~/Library/LaunchAgents/com.wittywilla.wittymediaserver.tray.plist` |
| The installer's record | `com.wittywilla.wittymediaserver.app` and `.service`, in `pkgutil` |
| An older copy | `/opt/wittymediaserver` |
| If you say yes: your data | `~/Library/Application Support/WittyMediaServer`, or the folder the service names |
| If you say yes: caches and preferences | In `~/Library`: `Caches/com.wittywilla.wittymediaserver`, `Preferences/com.wittywilla.wittymediaserver.plist`, `Saved Application State/com.wittywilla.wittymediaserver.savedState`, `Logs/WittyMediaServer`, `HTTPStorages/com.wittywilla.wittymediaserver`, `WebKit/com.wittywilla.wittymediaserver` |

## Linux

**With the `.deb` or `.rpm`,** the command that asks is the one to use:

```bash
sudo wittymediaserver --uninstall
```

The package manager works too, and doesn't ask:

| Command | The program | Library, accounts, settings, `wms` account | Recordings, backups |
|---|---|---|---|
| `sudo apt remove wittymediaserver` | Removed | Stay | Stay |
| `sudo apt purge wittymediaserver` | Removed | Removed | Stay, and it says so |
| `sudo dnf remove wittymediaserver` | Removed | Stay | Stay |

A package removes `/usr/lib/wittymediaserver`, `/usr/bin/wittymediaserver`, the service at `/usr/lib/systemd/system/wittymediaserver.service`, its menu entry and icon, and `/usr/share/doc/wittymediaserver`. It also stops the service and any tray app that's open.

**With `install.sh`:**

```bash
sudo /opt/wittymediaserver/bin/wittymediaserver --uninstall
```

Use your own folder if you chose one with `--prefix`. This removes the program folder, `/etc/systemd/system/wittymediaserver.service`, and the menu entry in `/usr/local/share/applications` or `/usr/share/applications`.

**An unpacked copy** with no service: stop it, run `./bin/wittymediaserver --uninstall` from its folder, or just delete the folder and the data below.

If you say yes to your data, it removes:

- `/var/lib/wittymediaserver`, or the folder the service names
- `/etc/wittymediaserver/wms.json`
- for you, and for everyone when run with `sudo`: `~/.local/share/wittymediaserver`, `~/.config/wittymediaserver/wms.json`
- for you, if you set them: the same folders under `XDG_DATA_HOME` and `XDG_CONFIG_HOME`
- the `wms` account

It always removes `~/.config/autostart/wittymediaserver.desktop` and `~/.local/share/applications/wittymediaserver.desktop`, and each `.conf` file in `/etc/systemd/system/wittymediaserver.service.d`, including what you set with `sudo systemctl edit wittymediaserver`. They're all listed before anything happens.

**What stays even so:**

- Someone else's folders under their own `XDG_DATA_HOME` or `XDG_CONFIG_HOME`, because only they have those settings. Run the command as them, or name the data folder with `WMS_DATA_DIR`.

## FreeBSD

As root:

```sh
wittymediaserver --uninstall
```

| Command | The program, its service | Library, accounts, settings, `wms` account, `rc.conf` settings |
|---|---|---|
| `wittymediaserver --uninstall` | Removed | It asks about your data. The `rc.conf` settings always go |
| `pkg delete wittymediaserver` | Removed | Stay |

The package removes `/usr/local/lib/wittymediaserver`, `/usr/local/bin/wittymediaserver`, `/usr/local/etc/rc.d/wittymediaserver`, and `/usr/local/share/doc/wittymediaserver`. `rc.conf` is left alone by `pkg delete`: to tidy it, run `sysrc -x wittymediaserver_enable`.

If you say yes to your data, it removes `/var/db/wittymediaserver`, or the folder `wittymediaserver_data` names, `/usr/local/etc/wittymediaserver/wms.json`, and the `wms` account.

With `install.sh`, use `/opt/wittymediaserver/bin/wittymediaserver --uninstall`, or your own `--prefix`.

## Docker

Your media is mounted from your disks, so none of this touches it.

```bash
docker compose down -v                          # the container and its volume: a full clean-up
docker compose down                             # only the container: the volume keeps your library
```

Started with `docker run`:

```bash
docker rm -f wittymediaserver
docker volume rm wms-config                     # your library, accounts and settings
docker image rm ghcr.io/wittywilla/wittymediaserver:alpha
```

## Options for scripts

These go after `wittymediaserver --uninstall`. `Uninstall WittyMediaServer.exe` and the Mac's app take the first six as well, and on Windows you can write them `/RemoveData`, `/S` and so on.

| Option | What it does |
|---|---|
| `--keep-data` | Keeps your library, accounts and settings, without asking |
| `--remove-data` | Deletes them, without asking. Recordings and backups stay |
| `--remove-recordings` | Also deletes recorded TV in the data folder |
| `--remove-backups` | Also deletes backups in the data folder |
| `--yes`, or `/S` | Asks nothing. Without other options it keeps your data |
| `--report <file>` | Writes the list of what it did to this file |
| `--dry-run` | Says what it would remove and leave, and changes nothing |
| `--json` | Gives the plan or the result as JSON. Only with the `wittymediaserver` command |

It returns 0 when everything asked for was removed, 1 when something couldn't be, or you said no, and 2 for a wrong option.

**On Windows,** the work finishes in the background, so a script should wait for the `--report` file:

```powershell
& "$env:LOCALAPPDATA\Programs\WittyMediaServer\Uninstall WittyMediaServer.exe" /S /RemoveData --report C:\Temp\removed.txt
```

To see first what it would do:

```bash
wittymediaserver --uninstall --dry-run
```

## If something went wrong

First run the uninstaller again: it only looks for what's left, and it's safe to repeat. If you've lost the program, unpack the same version's `.zip` or `.tar.gz` anywhere and run `bin/wittymediaserver --uninstall` from it.

To do it by hand, work down the list for your system. Skip what isn't there.

**Before deleting a data folder by hand,** look inside it. Keep `recordings` and `backups` if you want them, and check that no library or file of yours is in there. The uninstaller checks this for you, and these commands don't.

### Windows, by hand

In a terminal opened with **Run as administrator**. First ask Windows Installer to remove its part, using the code this shows, or from the `HKCU` version of the same key if it's there instead:

```powershell
reg query "HKLM\Software\Microsoft\Windows\CurrentVersion\Uninstall\WittyMediaServer" /v ProductCode
msiexec /x "{the code it showed}"
```

Then:

```powershell
sc.exe stop WittyMediaServer
sc.exe delete WittyMediaServer
schtasks /Delete /TN WittyMediaServer /F
netsh advfirewall firewall delete rule name=WittyMediaServer
reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v WittyMediaServer /f
reg delete "HKLM\Software\Microsoft\Windows\CurrentVersion\Run" /v WittyMediaServer /f
reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\WittyMediaServer" /f
reg delete "HKLM\Software\Microsoft\Windows\CurrentVersion\Uninstall\WittyMediaServer" /f
reg delete "HKCU\Software\WittyWilla\WittyMediaServer" /f
reg delete "HKLM\Software\WittyWilla\WittyMediaServer" /f
```

Then delete these in File Explorer:

- The program: `C:\Program Files\WittyMediaServer`, `%LOCALAPPDATA%\Programs\WittyMediaServer`
- Shortcuts, named `WittyMediaServer`: `%APPDATA%\Microsoft\Windows\Start Menu\Programs`, `%ProgramData%\Microsoft\Windows\Start Menu\Programs`, and your desktop and the public desktop
- Your data, if you want it gone: `%LOCALAPPDATA%\WittyMediaServer`, `%APPDATA%\WittyMediaServer`, `C:\ProgramData\WittyMediaServer`

If `wittymediaserver` is still found in new terminals, remove its `bin` folder from `PATH`, under **Edit the system environment variables** in the Start menu.

### macOS, by hand

```bash
sudo launchctl bootout system /Library/LaunchDaemons/com.wittywilla.wittymediaserver.plist
launchctl unload ~/Library/LaunchAgents/com.wittywilla.wittymediaserver.tray.plist
sudo rm -f /Library/LaunchDaemons/com.wittywilla.wittymediaserver.plist
rm -f ~/Library/LaunchAgents/com.wittywilla.wittymediaserver.tray.plist
sudo rm -rf "/Applications/WittyMediaServer.app" "/Applications/Uninstall WittyMediaServer.app"
sudo rm -f /usr/local/bin/wittymediaserver
sudo pkgutil --forget com.wittywilla.wittymediaserver.app
sudo pkgutil --forget com.wittywilla.wittymediaserver.service
```

Your data, if you want it gone, after looking inside:

```bash
rm -rf "$HOME/Library/Application Support/WittyMediaServer"
```

Then the caches and preferences in the [table above](#macos).

### Linux, by hand

Ask the package manager first, with whichever fits:

```bash
sudo dpkg --purge wittymediaserver
sudo rpm -e wittymediaserver
```

Then what's left:

```bash
sudo systemctl disable --now wittymediaserver
sudo rm -f /etc/systemd/system/wittymediaserver.service /usr/lib/systemd/system/wittymediaserver.service
sudo rm -rf /etc/systemd/system/wittymediaserver.service.d
sudo systemctl daemon-reload
sudo rm -rf /usr/lib/wittymediaserver /opt/wittymediaserver /usr/share/doc/wittymediaserver
sudo rm -f /usr/bin/wittymediaserver /usr/local/bin/wittymediaserver
sudo rm -f /usr/share/applications/wittymediaserver.desktop /usr/local/share/applications/wittymediaserver.desktop
sudo rm -f /usr/share/icons/hicolor/256x256/apps/wittymediaserver.png
rm -f ~/.config/autostart/wittymediaserver.desktop ~/.local/share/applications/wittymediaserver.desktop
```

Your data and the account, if you want them gone, after looking inside:

```bash
sudo rm -rf /var/lib/wittymediaserver /etc/wittymediaserver
rm -rf ~/.local/share/wittymediaserver ~/.config/wittymediaserver
sudo userdel wms
sudo groupdel wms
```

### FreeBSD, by hand

As root:

```sh
service wittymediaserver onestop
pkg delete -y wittymediaserver
rm -f /usr/local/etc/rc.d/wittymediaserver /var/run/wittymediaserver.pid
rm -rf /usr/local/lib/wittymediaserver /opt/wittymediaserver /usr/local/share/doc/wittymediaserver
rm -f /usr/local/bin/wittymediaserver /usr/local/share/applications/wittymediaserver.desktop
sysrc -x wittymediaserver_enable
sysrc -x wittymediaserver_user
sysrc -x wittymediaserver_listen
sysrc -x wittymediaserver_data
```

Your data and the account, if you want them gone, after looking inside:

```sh
rm -rf /var/db/wittymediaserver /usr/local/etc/wittymediaserver
pw userdel wms
```

## Removing the apps

Each app removes itself, and none of them touches your media or your server. On Windows these removals have run under Wine, and the Linux ones in containers. On a Mac, nothing has been run yet.

### WittyMediaClient

| System | How | What it leaves |
|---|---|---|
| **Windows** | **Settings → Apps → Installed apps**, then **WittyMediaClient**, then **Uninstall**. It asks whether to delete your settings, saved sign-ins, logs and caches too | Other accounts' data |
| **macOS** | Run **Uninstall WittyMediaClient.command**, on the `.dmg`. It lists what it found, and asks. It's also inside the app, under `Contents/Resources`. Dragging the app to the Trash leaves everything else | Nothing it found |
| **Debian, Ubuntu** | `sudo apt remove wittymediaclient` | Your own settings: see below |
| **Fedora and relatives** | `sudo dnf remove wittymediaclient` | Your own settings: see below |
| **Arch and relatives** | `sudo pacman -R wittymediaclient` | Your own settings: see below |
| **KDE Plasma Big Screen** | The package manager for its distribution, as above. Big Screen has no uninstall screen of its own, so this needs a keyboard, or KDE Connect from a phone | Your own settings: see below |
| **FreeBSD** | `wittymediaclient --uninstall` lists what it found, and asks about the program and your settings separately. `sudo pkg delete wittymediaclient` removes the program alone | FreeBSD's Electron and a font, as other programs may use them: `sudo pkg autoremove` removes them if nothing does |
| **AppImage** | Delete the file | Your own settings: see below |
| **Android, Android TV** | Long-press the app and choose **Uninstall**, or use **Settings → Apps**. Android deletes its data with it | Nothing |
| **iPhone, iPad, Apple TV** | Hold the app and choose **Delete**. The system deletes its data with it. These apps haven't been compiled yet | Nothing |

- **For scripts on Windows:** `"Uninstall WittyMediaClient.exe" /S` keeps your settings, and `/S /RemoveData` doesn't.
- **On a Mac:** add `--dry-run` to the uninstaller to see the list and stop.
- **On Linux, FreeBSD and with the AppImage,** a package can't reach anyone's home folder. To delete your own settings: `rm -rf ~/.config/WittyMediaClient ~/.config/WittyMedia`.

### WittyMediaPlayer

It comes for Windows only. Its builds for other systems don't exist yet.

1. Open **Settings → Apps → Installed apps**, and choose **WittyMediaPlayer**, then **Uninstall**. A portable copy has `Uninstall WittyMediaPlayer.exe` in its folder.
2. It closes the player if it's open, and asks whether to delete your settings and the disc modules it downloaded.
3. It lists what it removed and what it couldn't, and keeps that list in your temp folder.

| It removes | Where |
|---|---|
| The program | `%LOCALAPPDATA%\Programs\WittyMediaPlayer`, `C:\Program Files\WittyMediaPlayer`, or the folder you unzipped. Only the files on its list |
| Shortcuts | The Start menu and the desktop |
| Registry entries | Installed apps, Default apps, Open with, its file types and its own key |
| If you say yes: settings | `%APPDATA%\WittyMediaPlayer` |
| If you say yes: downloaded modules | `%LOCALAPPDATA%\WittyMediaPlayer\modules` |

The CDs you copied and the disc images you made are never touched. For scripts: `"Uninstall WittyMediaPlayer.exe" /S` keeps your settings, `/S /RemoveData` doesn't, and `--dry-run` only says what it would do. `msiexec /x` also works, and keeps your settings.

### WittyReader

**Your library is in the app's data folder, not with the program.** WittyReader keeps its own copy of each book there, with your highlights, notes, handwriting and reading positions. Deleting that folder loses them for good, as there's no cloud copy. Export the notes you care about first. Your original book files stay where they were.

| System | How | Its data folder |
|---|---|---|
| **Windows** | **Settings → Apps → Installed apps**, then **WittyReader**, then **Uninstall**. It asks whether to delete your library, notes and settings. No is the default | `%APPDATA%\WittyReader` |
| **macOS** | To remove everything, run **Uninstall WittyReader.command**, on the `.dmg`. It lists what it found, library and notes included, and asks. To keep your library, drag the app to the Trash | `~/Library/Application Support/WittyReader` |
| **Debian, Ubuntu** | `sudo apt remove wittyreader` | `~/.config/WittyReader` |
| **Fedora and relatives** | `sudo dnf remove wittyreader` | `~/.config/WittyReader` |
| **AppImage** | Delete the file | `~/.config/WittyReader` |

- **For scripts on Windows:** `"Uninstall WittyReader.exe" /S` keeps your library, and `/S /RemoveData` doesn't.
- **On Linux,** a package can't reach anyone's home folder. To delete your library and notes as well, which can't be undone: `rm -rf ~/.config/WittyReader`.
- **In a browser,** your library is in that browser's storage for the site. Clearing the site's data removes it.

---

<sub>WittyMediaServer documentation</sub>
