# Platform pieces: WittyModule, WittyModuleCatalog and WittyIntegratedFileBrowser: update plans

> **Status on 2026-11-04**
>
> **Planned:** Module API 4, [the idea](https://github.com/wittywilla/WittyMediaServer/discussions/30). **Open:** [WittyIntegratedFileBrowser#2](https://github.com/wittywilla/WittyIntegratedFileBrowser/issues/2), and the store honouring version ranges [#106](https://github.com/wittywilla/WittyMediaServer/issues/106).

---

## WittyModule

**Now:** 0.3.0. Library, API 3. The manifest, version ranges, the helper protocol, services, the release tool (`wittymodule pack`, `check`, `release`) and `kit/facade`. No dependencies.

| Release | Theme |
|---|---|
| **0.4.0** (with server 0.7.0-alpha) | **Module API 4:** release channels, `channelFloor`, `wittymodule release --channel`, `wittymodule promote` ([design](../design/module-release-channels.md)) |
| 0.5.0 (with 0.8.0-alpha) | `MinAPI` to 4; context-menu actions a module can add to items of its library kind; the shared insert-animation engine's types if modules other than WittyPlay use it |
| Before beta | Deprecation markers printed by `check`; API ranges in manifests; `dataTo` for one-step rollback ([../releases/beta-and-1.0.md](../releases/beta-and-1.0.md)) |

**Also:**

- **Sandbox on FreeBSD and macOS** (roadmap: not built): Capsicum or a jail on FreeBSD; what macOS still supports. Today a helper there gets a small environment and a soft memory limit only, and the UI must say so.
- **Reproducible builds** for every module's archive, checked in CI.
- The publishing blocker found in 0.6.0 (modules pinning a `wittymodule` version GitHub didn't have) shouldn't recur: a CI check in each module that its pinned `wittymodule` tag exists.

---

## WittyModuleCatalog

**Now:** the signed catalogue (`catalog.json`, copied into the server as `internal/modules/known.json`), icons, scripts.

| Release | Theme |
|---|---|
| With 0.7.0-alpha | Catalogue format +1 with `channel` per version; every first-party version marked alpha; new matching disc icons |
| Ongoing | Promotions by pull request, signed; a CI check that `known.json` equals `catalog.json` |

**Also:** a check that every icon has alt text and passes contrast in both themes; screenshots per module kept current from `D:\WittyDemoMedia`.

---

## WittyIntegratedFileBrowser

**Now:** 0.2.0. Helper, API 3, read-only. Choosing a server's folders from any device: drives and usual folders, DVD and Blu-ray folders marked, sizes and counts, stops at 3,000 entries unless told otherwise.

| Release | Theme |
|---|---|
| 0.2.x | Fixes |
| **0.3.0** | Search inside a folder; recent and pinned folders; network shares listed (read-only); clearer reasons when a folder can't be read (permission, offline drive) |
| Later | A picker mode for files as well as folders (for the poster picker's "use a file on the server", the disc set editor and core uploads), still read-only |

**Rule kept:** it only reads. Nothing is created, renamed or deleted.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
