<h1 align="center">WittyIntegratedFileBrowser</h1>

<p align="center">
  <b>Choose folders on a server from any device.</b><br>
  A folder browser in a window, for apps whose files live on another computer: a phone, a TV or a laptop can pick a media server's folders.
</p>

<p align="center">
  <a href="LICENSE"><img alt="Licence: Apache-2.0" src="https://img.shields.io/badge/licence-Apache--2.0-blue.svg"></a>
  <img alt="Go 1.26+" src="https://img.shields.io/badge/Go-1.26%2B-00ADD8.svg">
  <img alt="React 18+" src="https://img.shields.io/badge/React-18%2B-61dafb.svg">
  <img alt="Read-only" src="https://img.shields.io/badge/files-read--only-success.svg">
</p>

> Read-only mirror of `modules/wittyintegratedfilebrowser` in [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer),
> where development happens. Please open issues and pull requests there.

A browser's own "choose folder" window only sees the device the browser runs on. WittyIntegratedFileBrowser
lists the **server's** drives and folders instead, so any device can choose them. It has two halves:

| Half | Where | Does |
|---|---|---|
| **Go** | this folder | lists drives (with free space), usual folders, a folder's folders and files, and what's in it |
| **React** | [`web/`](web) | the window: places, back/forward/up, a path bar you can click or type into, filtering, sorting, several folders at once |

## What it does

| | |
|---|---|
| 💽 **Places** | drives with their label and free space, Home, Desktop, Videos, Music and the rest, plus places of your own (folders already in use, for example) |
| 🧭 **Getting around** | back, forward, up, a clickable path that turns into a text box (network paths too, such as `\NAS\Media` or `/mnt/nas`), recently chosen folders |
| 🔎 **Seeing what's there** | files with their kind (video, audio, book, disc image, subtitles, picture), DVD and Blu-ray folders marked, sizes, dates, a count of what's inside, hidden items on request |
| ✅ **Choosing** | the folder you're in, or tick several folders and use them all |
| ⌨️ **Every way of using it** | keyboard (arrows, Enter, Space, Backspace, `/` to filter, Ctrl+L for the path), touch, mouse and screen readers; a full-screen layout on phones |
| 🔒 **Safe** | it only reads: nothing is created, renamed or deleted. Put the Go half behind your app's sign-in |

## Go

```go
import fb "github.com/wittywilla/wittyintegratedfilebrowser"

places := fb.Places(fb.Place{Name: "Movies", Path: `D:\Movies`, Group: "libraries"})
l, err := fb.List(`D:\Movies`, fb.Options{ShowHidden: false})
// l.Entries: folders first, then files; l.Counts: {"folder": 12, "video": 30, "disc": 4}
```

| Call | Does |
|---|---|
| `Places(extra...)` | where to start: drives (label, free and total space), the user's folders, then `extra` |
| `List(path, Options)` | a folder's folders and files (`ErrNotFound`, `ErrDenied`) |
| `FileKind(name)`, `DiscFolder(path)`, `IsHidden(name)` | the helpers it uses |

`Options.Kind` replaces the file classifier, `MaxEntries` limits very large folders (3,000 by default).
No cgo; its only dependency is `golang.org/x/sys`. Serve both as JSON: `List`'s result is what the window reads.

## React

```tsx
import FileBrowser from '@wittywilla/wittyintegratedfilebrowser'

<FileBrowser
  list={(path, hidden) => fetch(`/api/fs?path=${encodeURIComponent(path)}&hidden=${hidden ? 1 : 0}`).then((r) => r.json())}
  onPick={(paths) => addFolders(paths)}
  onClose={() => setOpen(false)}
/>
```

`list('')` returns `{ places }`; any other path returns a `List` result. It uses your app's theme variables
(`--bg`, `--fg`, `--accent` and the rest) and `.btn`/`.input` styles when they exist, and has its own when they don't.

## The Witty family

| Project | Role |
|---|---|
| [WittyMediaServer](https://github.com/wittywilla/WittyMediaServer) | the media server it was made for |
| [WittyDVD](https://github.com/wittywilla/WittyDVD) · [WittyBluray](https://github.com/wittywilla/WittyBluray) · [WittyDisc](https://github.com/wittywilla/WittyDisc) | disc engines |
| [WittyEbook](https://github.com/wittywilla/WittyEbook) · [WittyPDF](https://github.com/wittywilla/WittyPDF) · [WittyComics](https://github.com/wittywilla/WittyComics) · [WittyInk](https://github.com/wittywilla/WittyInk) | readers |

## Licence

**Apache-2.0**, the same licence as WittyMediaServer. © 2026 WittyWilla and contributors.
Use, change and share it; if you redistribute it or build on it, keep the [NOTICE](NOTICE)
attribution, *“WittyIntegratedFileBrowser, developed by WittyWilla”*, in your NOTICE file, documentation or About screen.

## Legal

WittyIntegratedFileBrowser is an independent open-source project. It isn't associated with,
sponsored by or endorsed by any publisher, rights holder, service or other media software project.
It lists folders on the machine it runs on, and doesn't provide or check what's in them. Any
third-party names here only describe what it works with, and belong to their owners. It's provided
“as is”, with no warranty; see [LICENSE](LICENSE). Local law applies to how you use it. This isn't
legal advice.

<!-- module-kit -->
## Releasing

A version tag builds a program for every platform, signs the checksum list, and publishes a release that servers can install.

Versions move in small steps as work lands. `go run github.com/wittywilla/wittymodule/cmd/wittymodule@v0.2.1 bump .` raises it
everywhere it is written (add `-part minor` for a bigger step). A bump isn't a release: nothing is
tagged or published until you tag it. CI fails when code changed since the last tag and the version
didn't, because tags never move.

To release:

1. Check that `version` in `module.json` is the one to publish, with the bump committed.
2. Tag that commit and push the tag: `git tag vX.Y.Z`, then `git push origin vX.Y.Z`. A tag with a
   suffix, such as `v1.2.0-beta`, is published as a prerelease.
3. Watch the *Actions* tab. The signing step uses the `WMS_SIGNING_KEY` secret of the `release` environment, and nothing is published without it.

To take a version back, run the *withdraw* workflow from the *Actions* tab. It adds the version to a signed list on the release tagged `withdrawals`, which servers read.

Licensed under Apache-2.0: see [LICENSE](LICENSE) and [NOTICE](NOTICE).
<!-- /module-kit -->
