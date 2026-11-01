# Repositories

Every repository the project has or needs. Each is a project of its own, in its own folder at
`D:\projects\<Repository>` on the development machine, with `https://github.com/wittywilla/<Repository>.git`
as its remote. This repository holds none of their source.

Every repository's README ends with a **Legal** section: no affiliation, no warranty, no media included,
and what its names mean. For a new one, copy the wording from
[LEGAL-NOTICES-CHECKLIST.md](LEGAL-NOTICES-CHECKLIST.md#module-readmes).

## Local, not on GitHub yet

Each of these has a local repository with its history kept, and nothing has been pushed. **Create** means
the GitHub repository has to be made first: under `wittywilla`, as **Public**, with no README, `.gitignore`
or licence. The rest already exist on GitHub and only need pushing.

| Repository | What it's for | From | On GitHub |
|---|---|---|---|
| WittyModule | The shared tools and manifest format modules are built with, and the kit that writes a module's workflows (`kit/`) | `modules/wittymodule` | Exists |
| WittyDisc | Reading disc images, used by the disc engines | `modules/wittydisc` | Exists |
| WittyDVD | The DVD engine | `modules/wittydvd` | Exists |
| WittyBluray | The Blu-ray engine | `modules/wittybluray` | Exists |
| WittyCD | The CD image engine and on-screen CD player | `modules/wittycd` | Exists |
| WittyCouch | A Moonlight client in the page, for any Sunshine server: its relay, protocol code and player. **Apache-2.0** | `modules/wittycouch` | Exists |
| WittyCouchHost | The separate program built on Moonlight's own code, which speaks to the PC. **GPL-3.0**, with source offered beside each release | `modules/wittycouchhost` | **Create**: rename the empty WittyMoonlightHost |
| WittyRigs | An advanced option: hosting demanding games on the server's own hardware, using a separately installed Sunshine. **Apache-2.0** | `modules/wittyrigs` | **Create**: rename the empty WittySunshine |
| WittyIntegratedFileBrowser | The built-in file browser | `modules/wittyintegratedfilebrowser` | Exists |
| WittyPhotos | Photo processing | `modules/wittyphotos` | Exists |
| WittyTuner | TV tuners and IPTV | `modules/wittytuner` | Exists |
| WittyPlay | Console games streamed from the server | `modules/wittyplay` | Exists |
| WittyInk | Reader plugin: notes and pen | `web/src/modules/wittyink` | Exists |
| WittyEbook | Reader plugin: EPUB | `web/src/modules/wittyebook` | Exists |
| WittyPDF | Reader plugin: PDF | `web/src/modules/wittypdf` | Exists |
| WittyComics | Reader plugin: comics and manga | `web/src/modules/wittycomics` | Exists |
| WittyVisualizer | The audio visualizer plugin, for the server and the desktop player | `web/src/modules/wittyvisualizer` | Exists |
| WittyReader | The standalone reader app | `apps/wittyreader` | Exists |
| WittyMediaPlayer | The standalone desktop player | `apps/wittymediaplayer` | Exists |
| WittyMediaClient | The client apps for desktop, Android, Apple and others | `clients/wittymediaclient` | Exists |

`clients/desktop` and `clients/mobile` were older, thinner shells around the web app. WittyMediaClient
replaces both, so they were retired rather than moved. Their history stays in this repository's history.

## Elsewhere

| Repository | What it's for |
|---|---|
| WittyMediaServer | The server |
| WittyMediaRemoteMedia | Connecting libraries on a NAS through a native or Docker app. Empty: work not started |

## To remove

| Repository | What to do |
|---|---|
| WittyMoonlight | Delete it: WittyCouch replaces it. It holds only a README |

## Working on them

### Set up once, in each checkout

Run this once in every checkout of the server, including each worktree:

```bash
node scripts/dev-modules.mjs
```

It writes a `go.work` and links `web/node_modules/@wittywilla/*` to the repositories beside it, all ignored by
git. `npm run build` and `npm run dev` in `web/` remake the links, since `npm ci` removes them.

A module's own checkout does the same with the copy outside every repository:

```bash
node D:/projects/dev-modules.mjs
```

`D:\projects\go.work` covers the main checkout of every module repository, which Go finds by walking up. It can't
cover worktrees: Go refuses the same module listed twice, and a checkout outside its `use` list won't build. So each
worktree has its own `go.work`. **Never run `go work use`**: remake it with the command above.

### Changing a module

Use a `git worktree` under its repository, on a branch named for your chat. Never work on `main`:

```bash
git -C D:/projects/WittyPlay worktree add .claude/worktrees/my-chat -b my-chat main
```

```bash
cd D:/projects/WittyPlay/.claude/worktrees/my-chat && node D:/projects/dev-modules.mjs
```

Then point your server worktree at it, so the two build together with nothing published:

```bash
node scripts/dev-modules.mjs WittyPlay=D:/projects/WittyPlay/.claude/worktrees/my-chat
```

`WittyPlay=` with nothing after it goes back to the main checkout. The module's tests run in its worktree with
`go test ./...`. Commit there, on your branch, and report the branch.

### From WSL

A worktree's git link is a Windows path that WSL can't follow, so WSL uses clones, on its own disk. Module
repositories are read through `/mnt/d/projects`:

```bash
git clone /mnt/d/projects/WittyMediaServer ~/wms-my-chat
```

```bash
cd ~/wms-my-chat && git fetch origin my-branch && git checkout FETCH_HEAD && node scripts/dev-modules.mjs --repos /mnt/d/projects
```

To test a module's branch from WSL, clone that too and point at the clone:

```bash
git clone --branch my-chat /mnt/d/projects/WittyPlay ~/wms-my-chat-play
```

```bash
node scripts/dev-modules.mjs WittyPlay=$HOME/wms-my-chat-play
```

### A branch from before the split

A change under `modules/`, `web/src/modules/`, `apps/` or `clients/wittymediaclient/` belongs in the module's
repository now. Each file sits at the top of the repository instead of under its folder, so a diff moves across with
its prefix removed:

```bash
git diff release/0.6.0-alpha...HEAD --relative=modules/wittyplay | git -C D:/projects/WittyPlay/.claude/worktrees/my-chat apply
```
