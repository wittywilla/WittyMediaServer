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
| WittyModule | The shared tools and manifest format modules are built with, and the kit that writes a module's workflows (`kit/`) | **WittyModule** | Exists |
| WittyDisc | Reading disc images, used by the disc engines | **WittyDisc** | Exists |
| WittyDVD | The DVD engine | **WittyDVD** | Exists |
| WittyBluray | The Blu-ray engine | **WittyBluray** | Exists |
| WittyCD | The CD image engine and on-screen CD player | **WittyCD** | Exists |
| WittyCouch | A Moonlight client in the page, for any Sunshine server: its relay, protocol code and player. **Apache-2.0** | **WittyCouch** | Exists |
| WittyCouchHost | The separate program built on Moonlight's own code, which speaks to the PC. **GPL-3.0**, with source offered beside each release | **WittyCouchHost** | **Create**: rename the empty WittyMoonlightHost |
| WittyRigs | An advanced option: hosting demanding games on the server's own hardware, using a separately installed Sunshine. **Apache-2.0** | **WittyRigs** | **Create**: rename the empty WittySunshine |
| WittyIntegratedFileBrowser | The built-in file browser | **WittyIntegratedFileBrowser** | Exists |
| WittyPhotos | Photo processing | **WittyPhotos** | Exists |
| WittyTuner | TV tuners and IPTV | **WittyTuner** | Exists |
| WittyPlay | Console games streamed from the server | **WittyPlay** | Exists |
| WittyInk | Reader plugin: notes and pen | **WittyInk** | Exists |
| WittyEbook | Reader plugin: EPUB | **WittyEbook** | Exists |
| WittyPDF | Reader plugin: PDF | **WittyPDF** | Exists |
| WittyComics | Reader plugin: comics and manga | **WittyComics** | Exists |
| WittyVisualizer | The audio visualizer plugin, for the server and the desktop player | **WittyVisualizer** | Exists |
| WittyReader | The standalone reader app | **WittyReader** | Exists |
| WittyMediaPlayer | The standalone desktop player | **WittyMediaPlayer** | Exists |
| WittyMediaClient | The client apps for desktop, Android, Apple and others | `clients/wittymediaclient` | Exists |
| WittyStats | A statistics screen you arrange yourself, as a page of its own | New | **Create** |
| WittyModuleCatalog | The signed list of modules and versions that a server's store reads. Holds no module code | New | **Create** |

Each desktop product makes its own installers, with the same rules for names and for removing everything: `packaging/` in the player's repository, `desktop/` in the client's and the reader's.

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

### The server needs none of them

A build of the server needs only WittyModule. Before WittyModule's new version is published, point at a checkout of it, once in
each checkout of the server, which writes a `go.work` that git ignores:

```bash
node scripts/dev-modules.mjs WittyModule=D:/projects/WittyModule
```

Once it's published, nothing needs setting up at all. To try real modules on a server, see
[Developing a server and a module together](MODULES.md#developing-a-server-and-a-module-together).

A module's own checkout that builds on another (WittyDVD on WittyDisc, say) uses the copy outside every repository:

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

The module's tests run in its worktree with `go test ./...`. Commit there, on your branch, and report the branch.

A change to what the server calls (WittyPlay and WittyCouch) means rerunning the generator, which writes the stubs on both
sides. See [Developing a server and a module together](MODULES.md#developing-a-server-and-a-module-together).

### From WSL

A worktree's git link is a Windows path that WSL can't follow, so WSL uses clones, on its own disk. Module
repositories are read through `/mnt/d/projects`:

```bash
git clone /mnt/d/projects/WittyMediaServer ~/wms-my-chat
```

```bash
cd ~/wms-my-chat && git fetch origin my-branch && git checkout FETCH_HEAD && node scripts/dev-modules.mjs --repos /mnt/d/projects
```

To test a module's branch from WSL, clone it the same way, and build a catalogue from the clones with
`scripts/make-test-catalog.sh`.

### A branch from before the split

A change under `modules/`, `web/src/modules/`, `apps/` or `clients/wittymediaclient/` belongs in the module's
repository now. Each file sits at the top of the repository instead of under its folder, so a diff moves across with
its prefix removed:

```bash
git diff release/0.6.0-alpha...HEAD --relative=modules/wittyplay | git -C D:/projects/WittyPlay/.claude/worktrees/my-chat apply
```
