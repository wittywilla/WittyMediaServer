# Truly modular modules: module API 5

For the owner and every session working on modules: how each module comes to own its whole feature (UI, HTTP API, data and lifecycle), how the server merges modules into the interface people use today without changing how it looks, and the order the code moves in.

**Status:** plan, 2026-11-13. Nothing here is built yet. Phase 2 waits for the owner's approval, and for the Apple (s3) and Android (s4) sessions to confirm the [client apps](#client-apps) section.

Surveyed against server `v0.8.0-alpha` at `e90ae00a`, WittyModule `561dbdf`, WittyPlay 0.4.2 (`27f00d1`) and every module repository at its `v0.8.0-alpha`. Line counts leave out tests.

> **The owner, 2026-11-13: no backwards compatibility, ever.** "We are building a new standard. I want NO old code or standards in the latest version." Module API 5 is the only module API. There are no aliases of the server's old paths, no server copies of a module's feature for older module versions (no `legacyFor`), no downgrade or export-back work, and no import of the server's old data unless losing it would hurt testing (default: a clean start). The server's copy of each feature is deleted in the same step that moves it. The client apps keep working by being **updated to the new API alongside each step**, not by compatibility layers; contract tests assert the new contract. Where the text below still describes aliases, imports or legacy states, this note wins.

This plan **supersedes the "server builds with it" half of [module-isolation.md](module-isolation.md)**. That plan put shared game, disc and broadcast code in core libraries compiled into the server, which makes a server release the only way to update that code. Its other parts still stand: the encryption guards stay the server's, duplicated code is removed, and modules are usable without the server.

- [The owner's goal](#the-owners-goal)
- [What's wrong today](#whats-wrong-today)
- [What the server keeps](#what-the-server-keeps)
- [Module API 5](#module-api-5)
- [The install script: integration.json](#the-install-script-integrationjson)
- [The host API (Go side)](#the-host-api-go-side)
- [The web host SDK](#the-web-host-sdk)
- [Looking and feeling the same](#looking-and-feeling-the-same)
- [Updating a module without a server release](#updating-a-module-without-a-server-release)
- [When a module is missing](#when-a-module-is-missing)
- [Client apps](#client-apps)
- [First-run setup (OOBE)](#first-run-setup-oobe)
- [Security](#security)
- [Overrides: bypassing a protection, with warnings](#overrides-bypassing-a-protection-with-warnings)
- [WittyPlay: one interface, with its own store for cores and emulators](#wittyplay-one-interface-with-its-own-store-for-cores-and-emulators)
- [Module by module](#module-by-module)
- [Order of work](#order-of-work)
- [Testing](#testing)
- [Risks](#risks)
- [Decisions for the owner](#decisions-for-the-owner)

---

## The owner's goal

> Every module must be truly modular: API, UI, everything. WittyMediaServer merges them into one interface that looks and feels exactly as it does now, so a smaller update needs only the module store, not a full server update. For WittyPlay: all UI and APIs, as independent as possible, with an install script that lets the server integrate it and work as it does now. Don't break the WittyMediaClient apps. The module API should have been built for this from the start.

The owner's red flag was **how small the module downloads are**. That size is the symptom: most of each feature is compiled into the server.

## What's wrong today

### Where each feature's code lives

"Server" counts only code that exists for that one feature.

| Feature | Module (Go / web) | Server (Go / web) | Server DB |
|---|---:|---:|---|
| WittyPlay (games) | 50k / 7.8k | **~38k / ~12.5k** (+2.5k `.d.ts` copies) | 27 tables, 4 columns |
| WittyCouch + WittyRigs | 4k / 0.7k | 2.9k / 0.9k | 3 tables |
| WittyTuner | 6.3k / 0 | ~8.2k / ~1.3k (tuner-specific) | 5 tables |
| WittyDVD + WittyBluray | 30k / 0 | ~11k / ~1.7k (the DvdPlayer) | item columns |
| WittyCD | 7.9k / 0 | 2.5k / 1.4k | 1 column |
| WittyPhotos | 1k / 0 | 1.9k / 1.1k | 3 tables |
| Readers (Ink, Ebook, PDF, Comics) | 0 / 3.1k | 5.2k (mostly kept: sanitiser, unpacking) / 0.3k | 1 table |
| WittyStats, WittyVisualizer, WittyIntegratedFileBrowser | — | almost nothing | — |

The full inventories are in the shared notes folder: `s14-audit-games.md` and `s14-audit-others.md`.

### Why that happened

The contract between server and module has no place for these things:

1. **Server routes.** A module can't declare HTTP routes. The one proxy that exists (`internal/server/api_module_proxy.go`) is a hand-written allow-list for `/api/modules/wittyplay/docker/*`. Every other module API is a handler compiled into the server.
2. **Typed facades.** The server calls WittyPlay and WittyCouch through stubs generated into `internal/play/**` and `internal/couch/**` (`kit/facade`). About 30 server files import them, so a facade change needs a server release.
3. **Data.** A module has nowhere to keep structured data except a 64 KB key-value store. The games tables, tuner tables, photo albums and similar all live in the server's DB, with migrations compiled into the server.
4. **Web UI beyond a single component.** The host object (`web/src/moduleHost.ts`) shares only React, react-dom and pdf.js. It has no router, API client, UI kit, icons, theme, SDUI renderer, TV/gamepad focus or app state. The app also has no registry where a module could add:
   - routes or full-screen frames;
   - card art or card links;
   - settings sections, setup steps;
   - players for an item kind, readers for a format.

   So `App.tsx`, `SettingsPage.tsx`, `ItemPage.tsx`, `openItem.tsx`, `LibraryPage.tsx`, `sdui/render/components.tsx`, `NativeViewHost.tsx` and `moduleBundles.tsx` each name modules by hand.
5. **SDUI sources and ops.**
   - Ten of WittyPlay's eleven SDUI sources are answered by the server (`wittyplay_online.go`).
   - Every disc source is answered by the server (`disc_sources.go`).
   - Module ops have their HTTP paths hard-coded in `pkg/sdui/catalog_data.go:1154-1183`.
6. **Lifecycle.** Installing only downloads, verifies and swaps files. There is no install, upgrade or uninstall hook, and no data migration.
7. **Policy hard-coded per module.** These name modules in the server's code:
   - parental path prefixes: `parental.go:81-90`;
   - features hidden by profile limits: `sdui_limits.go`;
   - menu folding: `sdui_nav_dedupe.go`, `shell/navModel.ts`;
   - log topics: `api_logmodules.go`;
   - backup table classes: `backup.go:164-190`;
   - doctor checks: `diagnose/doctor_features.go`;
   - live-session metrics: `streams_live.go`;
   - one-off migrations: `modules/gamefailures.go`.

### Session 5's tooling limits

Each of these is fixed below.

- No shared design tokens: modules copy the server's palette.
- No TypeScript types for `__wmsModuleHost` or for the props modules are given.
- The libraries a bundle shares with the app are fixed in each module's copy of `build-bundle.mjs`.
- That copied builder breaks on Vite 8 (`inlineDynamicImports` is deprecated).
- No way to preview a module's screens without a running server.
- WittyPlay needs `GOWORK=off` to build, because `WittyMediaPlayer/engine` imports the server's `pkg/sdui` at an unpublished `wittymediaserver v0.8.0-alpha.0`. The same code is already synced into `wittymodule/sdui`.

## What the server keeps

The server is the **host**. It keeps what every module shares, and what it must check for itself:

- **People:** accounts, sign-in, tokens, profiles, parental rules and screen time, sharing links.
- **Libraries:** libraries and the generic item rows (id, kind, title, sort title, files, pictures, progress), scanning the file system, search, collections, Home, history, backup.
- **Media plumbing:** FFmpeg, HLS, the transcoder, stream tokens, device and remote control, SyncPlay.
- **The encryption guards and answer checks.** It never takes a module's word that something isn't encrypted:
  - `internal/tuner/mpegts`, `livetv/hlsgate.go`, `modules/dvdremote/scramble.go`, `bdsession/encrypted.go`;
  - checking helper answers: `dvdremote/check.go`, `filebrowser.go`;
  - the ebook sanitiser.
- **The module system:** the store, the catalogue, signatures, sandboxes, overrides, the audit log, the policy on which module may hold which permission (`mayHave`).
- **The interface:**
  - the SDUI catalogue and renderer, and the app shell (sidebar, top bar, TV frame);
  - Settings' frame, the setup wizard's frame;
  - the design tokens and the shared UI kit.

**Everything specific to one module moves into that module.** That includes its pages, frames, players, settings screens, HTTP routes, SDUI sources and ops, data and migrations, metadata sources and file identification.

## Module API 5

A new module API, built as the foundation rather than patched onto API 4.

- `API = 5`. During the move the server takes 3 to 5 (`MinAPI` stays 3), so nothing breaks before a module is rebuilt.
- After every stock module ships on API 5, a later server raises `MinAPI` to 5 and deletes the legacy paths. That is step [M9](#order-of-work).

What API 5 adds:

| Part | File in a release | What it does |
|---|---|---|
| The integration manifest | `integration.json` (signed, beside `module.json` and `sdui.json`) | Declares everything the module adds to the host: HTTP routes, web routes and frames, hooks, settings sections, setup steps, data, lifecycle, the host features it needs. This is "the install script". |
| The module's service | the existing helper/service binary | Answers its own HTTP routes, SDUI sources, SDUI ops and lifecycle calls. Every module with an API becomes a **service** (WittyPlay and WittyCouch already are). |
| The host API | `wittymodule/host` (Go client) | How the module asks the server for items, files, access checks, settings, secrets, pictures, tasks, events, notifications and transcodes. Versioned. |
| The web bundle | `web/index.js` + `style.css` | Exports `register(host)`, which adds its routes, frames, pages, views, card hooks, settings screens and players to the app. |
| Its data | its own SQLite database in `WITTYMODULE_DATA`, with migrations it owns | `dataFormat` and `dataFrom`, which already exist, finally get used. |

### Why declarative, not a script

A script in the server would run a module's code with the server's power. `integration.json` is data: signed, checked strictly at install like `sdui.json`, shown in the store, and applied by the server. Code that has to run at install, such as data migrations, runs **in the module's own sandbox**, as lifecycle calls to its service.

## The install script: integration.json

Checked strictly, like `sdui.json`: an unknown field is refused, and there's a size limit. `wittymodule check` validates it, and the catalogue carries a summary for the store.

```jsonc
{
  "format": "wittymodule-integration", "formatVersion": 1, "module": "wittyplay",
  "host": { "http": ">=1 <2", "web": ">=5 <6", "sdui": ">=8" },     // host features it needs
  "feature": "games",                                                // parental/limits feature it belongs to
  "http": [
    // Mounted at /api/modules/wittyplay/...: the module's API is its own, and apps call it there.
    { "path": "/games/{id}/play", "methods": ["POST"], "auth": "user",
      "item": "id", "limits": { "body": "256KB", "time": "45s" } },
    { "path": "/play/{sid}/ws", "methods": ["GET"], "auth": "user", "upgrade": "websocket", "stream": true },
    { "path": "/gaming/cores/{id}", "methods": ["DELETE"], "auth": "admin" },
    { "path": "/kasm/{sid}/{ticket}/{rest...}", "methods": ["GET"], "auth": "ticket", "csp": "sandboxed-page" }
  ],
  "web": {
    "routes": [
      { "path": "/wittyplay/*", "frame": "fullscreen" },             // takes over the window, like PlayFrame today
      { "path": "/game/:id", "frame": "fullscreen" },
      { "path": "/controllers", "frame": "app" }                     // inside the app's own frame
    ],
    "takeover": [ { "libraryKind": "games", "route": "/wittyplay/library/{library}" } ],
    "cards":    [ { "mediaKind": "game", "art": true, "detail": true, "link": true } ],
    "players":  [ { "mediaKind": "game" } ],
    "settings": [ { "id": "gaming", "label": "Games & emulators", "admin": true, "after": "playback",
                    "search": ["cores", "BIOS", "emulators", "region order"] } ],
    "setup":    [ { "id": "games", "after": "libraries" } ],
    "navFold":  [ { "libraryKind": "games", "into": "wittyplay-online" } ]
  },
  "sdui": { "ops": [ { "id": "wittyplay.room.create", "http": "POST /rooms" } ] },
  "data": { "store": "sqlite", "format": 3, "from": 1, "backup": "saves" },
  "lifecycle": { "install": true, "upgrade": true, "uninstall": "ask-keep-data", "enable": true, "disable": true },
  "live": { "sessions": true, "doctor": true, "logTopics": "from module.json" }
}
```

### What each part does

**`http`: module routes**

- **Mounting.** The server mounts each route at `/api/modules/<id>/…` (and `/api/v1/modules/<id>/…`). That is the module's API; apps call it there.
- **What the server checks before the request reaches the module:**
  - `auth`: `user`, `admin`, `ticket`, or `public` (refused for anything but static files).
  - The module's `feature`: parental rules and screen time.
  - The `item` parameter: may this profile see this item?
  - Body size, time, and the WebSocket `upgrade`.
- **What the module receives:**
  - `X-Witty-User`, `X-Witty-Admin`, `X-Witty-Profile`;
  - `X-Witty-Item`, already resolved: the item's kind, library and access are checked; its files are given as read handles;
  - `X-Witty-Feature-Limits`.
- This is today's `ProxyToService`, made generic.

**`web.routes` and frames**

- `fullscreen` replaces the app shell, as `PlayFrame`, the reader, the couch player and the TV player do today.
- `app` draws inside the app frame.
- `tv` gives the TV frame a screen.
- The app's router asks the registry before its own routes. A route the module claims but isn't loaded shows `ModuleMissing`, as `lazyModule` does now.

**`web.cards`, `players`, `readers`, `takeover`, `navFold`: generic hooks that replace hand-written special cases**

| Hook | Replaces |
|---|---|
| Card art, card detail (bio) and card link for a media kind | `GameArtContext`, `CardRouteContext`, `GameBio` in `sdui/render/components.tsx` |
| Player for a media kind | the disc, CD and game branches in `PlayerPage`, `ItemPage`, `openItem`, `SduiPage` |
| Reader for a format | `READERS` in `moduleBundles.tsx` |
| A library kind's page drawn by the module | `PlayRedirect` / `useGamesInPlay`, `LibraryPage` branches |
| Menu folding | `gamesBecomeWittyPlay`, `navModel.foldGames` |

**`web.settings` and `web.setup`**

- Sections of Settings and steps of the setup wizard, drawn by the module's bundle inside the host's frames.
- Includes entries for Settings' search index.
- The SDUI settings descriptions (`sdui.json`) remain for what native apps draw.

**`sdui.ops`**

- A module's SDUI ops and the routes they call.
- The catalogue entries in `pkg/sdui/catalog_data.go:1154-1183` are no longer hard-coded.
- Sources are always answered by the module's service. No more server-side resolvers.

**`data`: the module's own database**

- `backup` says which backup class the data belongs to.

**`lifecycle`: calls to the module's service, in its sandbox, at each point**

| Call | When | What it's for |
|---|---|---|
| `install` | first time | create the database |
| `upgrade{from,to}` | a new version | migrations; refused if `dataFrom` can't read the data, as today |
| `enable` / `disable` | turned on or off | |
| `uninstall{keepData}` | taken off | the admin is asked whether to keep the module's data |

- A lifecycle call that fails rolls the install back. The machinery for that already exists in `storerun.go`.

**`live`**

- The module reports live sessions (games, emulators, couch streams) through the host API.
- That feeds metrics, screen time, "playing now", parental stops and sign-out stops, which are hard-coded today in `streams_live.go`, `emulator_sessions.go` and `live_ends.go`.
- Its doctor checks are listed the same way.

**When the server applies it**

- At install, upgrade and enable: in one transaction with the module's version swap.
- At every server start: from the version in use.
- When the module is disabled or removed: its routes answer `424 Failed Dependency` with `module` and `moduleName` (today's contract), and its web routes show `ModuleMissing`.

## The host API (Go side)

What a module asks the server for, as `wittymodule/host`:

- HTTP over the service's loopback connection, with the per-start token.
- Versioned `/host/v1/…`, extended additively.
- Each call carries the identity of the request it serves, so the server enforces that person's access, never the module's own.

| Area | Calls |
|---|---|
| Items | query by library kind and filters, get, set the module's own fields and facets, set pictures, mark played; item **files as read handles**, access-checked, never a path a browser chose |
| Libraries and scan | library kinds a module owns. **Scan hooks:** for a library of that kind, the server walks the files and hands batches to the module's `identify` and `describe` calls (WittyPlay: `gameid`, `gamearchive`, `gamemeta`); the module returns generic item fields plus its own data |
| People | who is this, their profile, `may(feature)`, `canSee(item)`, friends and presence where the server keeps them |
| Settings and secrets | the module's settings (from `sdui.json`); secrets the admin entered (metadata API keys), passed only to the module that owns them |
| Pictures and cache | put an image in the server's image cache and get its URL; a cache folder of its own |
| Tasks and events | report task progress; subscribe to `scan.finished`, `item.added`/`removed`, `user.deleted`, `profile.limits.changed`, `server.stopping` |
| Notifications and live sessions | notify a person; register, update and end a live session |
| Transcoder | ask the server to run FFmpeg/HLS on a stream or file it hands over (disc presentations, CD audio, tuner streams), so the guards and the transcoder stay the server's |
| Backup | its database file is backed up with the class it declared |

### Retiring the facades

The facades (`internal/play/**`, `internal/couch/**`) go away. Instead:

- The server's remaining calls into a module are generic: lifecycle, scan hooks, SDUI sources and ops, route proxy.
- The console table reaches the server **at run time**, as data the module serves, never compiled in. The server uses it for library console choice and licence notices.
- `kit/facade` stays only for helpers whose wire types the server must check itself (DVD, Blu-ray, tuner), generated types only, as module-isolation.md #18 planned.

## The web host SDK

**`@wittywilla/wms-host`, web host API 5.** A package in WittyModule (`kit/web/host`), with its types **generated from the server's** `web/src/moduleHost.ts` and checked by a test on both sides. That answers session 5's missing types. The object on `globalThis.__wmsModuleHost` grows to:

| Part | What |
|---|---|
| Shared libraries | `react`, `react-dom`, `react/jsx-runtime`, **`react-router-dom`** (the app's single copy, so module routes and links work), `@wittywilla/wittyink`, pdf.js |
| `register(module, { routes, frames, pages, views, cards, players, readers, settings, setup })` | The registry. The bundle's `register` export is called once, when the app starts, for every enabled API-5 bundle. Components stay lazy: `register` hands over loaders, not code |
| `api` | The app's own client: `get`/`post`/`put`/`del`/`mediaUrl`/`token`, the same sign-in and error handling as the app |
| App state hooks | `useUser`, `useProfile`, `usePrefs` (`/me/ui`), `useServer` (name, colours, features), `useTvMode`, `useNavigate`, `toast`, `confirm` |
| UI kit | The app's own components: Button, IconButton, Icon (the icon set), Sheet, Overlay, Menu, Toggle, Field, Select, Slider, Spinner, Skeleton, EmptyState, Toast, ModuleMissing, settings rows (`settings/ui`), LegalNote, BrandLogo, ArtworkPicker |
| SDUI | `SduiSection`, `SduiPage`, `Card`, and the renderer's contexts (`GlyphContext`, `RowLayoutContext`, `TabsContext`, `setPageRegions`) |
| Input | TV spatial focus (`ui/spatial`), gamepad navigation (`ui/gamepad`), `inGame(on)`; replaces `tv.ts`'s check for WittyPlay's class names |
| Capabilities | `host.version`, `host.has('frames@1')`, so a bundle can use a newer hook when present and fall back when not |

**The ABI rule stays:** adding raises the web host API, while removing or changing breaks released bundles. The kit's types are the record, and a test in the server fails if the object drifts from them.

### Tooling (WittyModule `kit/web`)

- **One builder, `wittymodule web build`.** It replaces every copy of `build-bundle.mjs`, builds the stand-ins from the kit's list of shared libraries, uses `codeSplitting: false` on Vite ≥ 8, and writes `integration.json`'s web part from the bundle's `register` export, so the two can't drift.
- **`wittymodule web preview`:**
  - a Vite dev server with a mock host: fixtures for `api`, a fake user and TV mode, the real tokens and UI kit from the kit;
  - or `--server http://127.0.0.1:19805`, which proxies to a real test server with the module's bundle hot-reloading.
- **`wittymodule check`** validates `integration.json`, the registered names, and the host ranges against a server version.

## Looking and feeling the same

- **One set of tokens, from the server.**
  - The CSS custom properties in `web/src/styles.css`, `design2.css` and `theme.ts` become a documented contract: `kit/web/tokens.json`, generated from the server, a `--wms-*` name for each role. The existing names keep working as aliases.
  - Modules use the tokens and never copy colours.
  - A module's `sdui.json` theme scope still sets its accent.
  - The custom-coloured server name (`brandName`, `brandStyle`) is drawn only by the host's own components, so it survives every module.
- **The code moves, not a rewrite.**
  - `PlayChrome`, `GamePictureArt`, the Play pages, `GameBio` and the rest move **file for file** into the module.
  - Their imports from the app (`../../ui`, `../../api`, `../../state`, icons) become imports from `@wittywilla/wms-host`, which hands over the same components.
  - The same code drawing with the same components and the same CSS is how the look stays identical.
- **The CSS moves with the code.**
  - A bundle's stylesheet is enabled only while its routes are on screen, as reader stylesheets are today.
  - Classes keep their names, so the existing styles still match.
- **Proved with screenshots.** See [Testing](#testing).

## Updating a module without a server release

- **Ranges, not pins.** A module says which host features it needs (`host.http`, `host.web`, `host.sdui`), and the server says what it offers in `/api/modules` and the store. The store offers only versions that fit, and says why for those that don't. That extends today's `minServer`/`maxServer` and widening (`internal/modules/widen.go`).
- **What only a server release changes:** the host's own features. That means a new hook, a new host API call, or a token change. Everything module-specific ships in the module.
- **The server's floor for its modules.** During the move a server release may need a module version that has taken over code the server dropped. The server ships `moduleFloors` (for example `wittyplay >= 0.6.0`), and on start it updates a module below its floor from the store, as auto-update already can. Offline, it keeps the module's old version **and the legacy code path**, as described next. That's why legacy code is removed only in M9.
- **Data never goes backwards silently.** `dataFormat`/`dataFrom` refuse a downgrade that can't read the data, as today, and the module's own migrations run in `upgrade`.

## When a module is missing

| State | Routes | Web | SDUI |
|---|---|---|---|
| **Absent or turned off** | Its routes (`/api/modules/<id>/…`) answer `424` with `module` and `moduleName` | Its screens show `ModuleMissing` with "Add it"; its menu entries and inserts disappear | Stock pages stand; module pages are gone |
| **Installed (API 5)** | The module answers | The module draws its screens | The module answers its sources and ops |

A module on another API isn't supported: the store and the server refuse it.

1. **Capabilities follow what is installed and enabled.** `/capabilities` drops a module's feature flags when it is absent or turned off, by test, so an app hides what it can't use.
2. **A missing module's page still offers a way on:** a plain page with "Add it" (admins) and "Back", usable with a TV remote, inside an app's web view.
3. **Ids are the server's.** A module's data refers to the server's item, library and user ids as they are.

## Client apps

**Hard constraint:** every app in WittyMediaClient keeps working, unchanged:

- Apple: iOS, iPadOS, tvOS, macOS (`apple/`), session s3.
- Android: phone, tablet, TV (`android/`), session s4.
- Desktop (`desktop/`).

They keep working by being **updated to each step's new API** (s3, s4), never through compatibility layers on the server. Each step lists what moves where; the app sessions switch to it in the same step, and the contract tests assert the new contract.

What the apps use today (from WittyMediaClient):

- the Client API (`docs/CLIENT-API.md`, `docs/openapi.json`);
- SDUI documents, the catalogue and its conformance fixtures (`pkg/sdui/conformance`);
- module settings (`/api/v1/sdui/modules/…`);
- the games endpoints (`apple/WittyKit/Sources/WittyKit/GamesAPI.swift`, `GameStreamV2.swift`, `TouchControls.swift`, and their Android counterparts): `/api/games/*`, `/api/play/{sid}/*` (WebRTC, WebSocket, stream v2), `/api/consoles/*`, `/api/game-profiles*`, `/api/couch/*`;
- disc, CD, tuner and guide endpoints for their native players.

| Contract | Steps that touch it | How compatibility is guaranteed |
|---|---|---|
| Every HTTP path an app or the web app calls (`/api/games/*`, `/api/play/*`, `/api/tuners/*` (web only), `/api/dvd/*`, `/api/cd/*`, `/api/photos/*`, `/api/couch/*`, `/api/rigs/*`, …) | M2, M4–M7 | **Aliases**: the same path, method, status codes and JSON shape, served by the module. The legacy list fixes which paths may be aliased. The `/api/v1/…` rewrite applies to aliases as to any route |
| Response shapes | M2, M4–M7 | **Golden contract tests** (below), recorded from today's server and replayed against: module absent, module API 4 (legacy), module API 5 |
| Streams: WebRTC offers, `/ws`, stream v2, HLS, stream tokens | M2 | The proxy already carries WebSockets and flushes immediately (`FlushInterval: -1`). Tokens stay the server's. Latency is measured before and after (S5's numbers) |
| Auth, tokens, parental rules, screen time | all | The server keeps enforcing them before a module sees a request. Parental path prefixes become generated from the integration manifests and are tested against today's list |
| SDUI documents, catalogue revision, conformance fixtures | M1–M3, M7 | No catalogue change is required by the move. Module ops and sources keep their IDs. Fixtures are run in both states. A module's `NativeView` always has a TV-safe fallback (existing rule) |
| `api_capabilities.go` flags (games, library types, item kinds) | M3 | Same flags. Computed from the installed modules' manifests, and equal to today's in the tests |
| Native screens (`game`, `guide`) and native game UI in the apps | none | Untouched. App Store rules forbid apps running downloaded code, so native UI stays in the apps. What changes on the server side is served through the same paths |
| Module settings (`/api/v1/sdui/modules/{id}/settings`) | M1, M5–M7 | Unchanged. New settings move into `sdui.json` descriptions, so native apps gain them rather than lose them |
| **Server web pages opened in a WebView** (Android `net/ServerPages.kt`, `ui/WebPlayer.kt`): `/play/{item}?mode=menu\|title&ttn=&episode=&start=`, `/cd/{id}?play=1`, `/read/…`, `/tv/watch/…`, `/game/…`, `/couch-rig/{game}`, `/couch/…/play/…`, `/big`, `/photos`, `/couch`, with `?tv=1` | M1, M5–M7 | A module's web routes keep these exact addresses and query parameters. They work signed in through the `wms_token` cookie (Path=/, HttpOnly, SameSite=Lax) as well as `Authorization: Bearer`, and through the peer-to-peer local proxy. A WebView screenshot pass is part of each step |
| Games list through the generic items API: `/items?library=&kind=game&console=&sort=&desc=&search=&offset=&limit=`, `/items/{id}/poster?v=&w=` | M3 | Stays the server's. The `game` kind and the `console` filter remain on `/items` (console becomes a facet the module fills, still filterable by the same parameter) |
| Stream v2: the absolute endpoint URL returned by `GET /play/{sid}/stream`, a non-trickle SDP answer `{sdp, stream:{…}}`; the `/play/{sid}/ws?video=` upgrade with the Authorization header | M2 | The URL the server returns is always the server's public address (alias), never the module's loopback port. It must work through the peer-to-peer proxy (`127.0.0.1` port, `X-WMS-Pair-Secret` header or cookie). Golden tests cover the answer's shape |
| Live TV paths the apps really call: `/tv/{key}/tune?streamToken=1&viewer=`, `/tv/{key}/leave`, `/tv/{key}/catchup?…`, `/tv/{key}/programme?start=`, `/tv/channels`, `/tv/favourites/{key}`, `/tv/reminders[/{id}]`, `/tv-sets…`, `/recording-rules…`, `/recordings…`, `/guide?…`, `/guide/search?…`, `DELETE /hls/{streamId}` | M6 | All of these stay the server's (guide, DVR, live HLS). M6 moves only tuner admin and setup, which the apps don't call. A `streamToken` in the query keeps working with no auth header (ExoPlayer plays it directly) |
| Headers on every alias: `ETag`/`If-None-Match` → 304, `Retry-After` on 429/503, `Location`, `X-Request-Id`, `X-WMS-Screen-Time-Left`, parental refusals with today's status and body, `X-Witty-Connect`/`X-Witty-Connect-Error`; JSON error bodies (`problemFrom()`) | all | The proxy passes them through unchanged, the server adds the ones it owns (screen time, request id, parental refusals) before a module sees the request, and errors from the proxy itself use the server's JSON error shape. Each is a case in the golden tests |
| `/capabilities` feature names (an unknown name hides a whole screen) | M0b, M3, M9 | For the same set of installed and enabled modules, the flags are equal to today's server, whether a module runs on legacy or API 5. A module that is absent or disabled drops its flags. Both are tested |
| Disc routes the apps call are under `/items/`: `/items/{id}/disc`, `/disc/analysis` (GET, POST `{secondOpinion}`), `/disc/choice` (PUT `{ttn}`, DELETE), `/disc/labels`, `/disc/thumb/{ttn}/{scene}`; also games `/game-background`, `/game-bezels/{bezel}`, `/games/{id}/saves/{slot}/thumb?v=`; `/rigs/games/{id}/start`, `/rigs/{rigId}/wake`, and `/couch-rig/{gameId}` called as an API as well as opened as a page | M2, M5 | Aliased at exactly those paths. Disc analysis and choice keep their `/items/{id}/disc/…` addresses whichever side answers them |
| **Player and link conventions in the web pages** (Apple): a path starting `/play/` or `/cd/` is a player, and leaving it closes the player; links matching `^/game/\d+$` (with `?resume=1` and the like) are intercepted and played natively | M1, M5 | Module UIs keep those prefixes, and link Play, Resume, Start over and saves as plain `<a href="/game/{id}…">` on the server's origin |
| **App embedding in the web view**: `html[data-app="wittymediaclient"]`; localStorage `wms.theme`; an adopted stylesheet hiding `.tabbar`, `.section-row`, `.section-row-brand`, `.top-back`, `.app .topbar` under 700px; the `wittyconnect` cookie over peer to peer | M0c, M1, M5–M7 | Module UIs keep those class names and honour `data-app`. The page policy keeps allowing `adoptedStyleSheets`. Module routes accept both cookies the same way the host does |
| **Long and dual-use stream calls**: `POST /games/{id}/prepare` is polled about once a second until ready; `POST /games/{id}/play` waits up to the app's start time; `DELETE /play/{sid}` answers only once everything is saved; `/play/{sid}/stream` is both a POST (WebRTC offer) and a WebSocket upgrade (stream v2); WebSockets are signed in by the Authorization header on the upgrade, never a query token; stream v2 sends a text hello, then binary frames and pings | M2 | Per-route time limits in `integration.json` set to at least today's. One alias is routed by method and `Upgrade`. The proxy forwards the Authorization header on upgrades, needs no query token, and passes text and binary frames and pings through unbuffered. Latency is measured with S5's numbers |

**What doesn't change for s3/s4:** nothing they call is renamed or removed in this plan. Optional later work for them: draw module settings sections they don't draw yet. **Before Phase 2 starts, s3 and s4 are asked to confirm this list** against their code.

- **Android (s4) reviewed it on 2026-11-13** against WittyMediaClient `android/` at `cb231fa`. The six rows above from "Server web pages" down are its additions.
- **Apple (s3) reviewed it on 2026-11-13** against `apple/` at `521f08c`. The four rows above from "Disc routes" down are its additions. Apple TV is all native and has no web view.
- **Apple tests at M2, M3 and M9**, run against this session's test server:
  - WittyKit (`swift test`): GamesTests, GameStreamV2Tests, TouchControlsTests, GameShelvesTests, SDUIModulesTests, SDUIConformanceTests, DiscRankingTests, ChannelSetsTests, GuideRulesTests, TvLookTests.
  - UI tests: SmokeTests, SDUITests, MediaExtrasUITests, and on tvOS the Dock and focus cases.
  - `apple/scripts/p2p-check.sh`, including a game WebSocket over peer to peer.
  - At M5, also the disc and CD player pages in the iPad web view.
  - s3 offers to run these, given the server address and a slot for heavy work.
- **Android tests at each step:**
  - The command, from `android/`:

    ```bash
    gradle --no-daemon --max-workers=2 -PsduiFixturesRequired testDebugUnitTest lintSideload assembleSideload
    ```

  - The unit suites run at each step:

    | Step | Suites |
    |---|---|
    | M2 | GameTransportTest, GameStreamTest, GamePadTest, GameTouchTest, MoonlightTest, ServerPagesTest, PeerRulesTest |
    | M3 | the M2 suites, plus SduiConformanceTest, SduiRev8Test, SduiRulesTest, LayoutAndModulesTest, DiscoveryTest |
    | M9 | everything, plus GuideRulesTest, GuideSetsTest, TvLookTest, DiscWordsTest |

  - These unit tests can't reach a live server. So each step also gets a manual pass on the phone and TV emulators against this session's test server:
    - games: start, touch and pad, stream v2, save, stop;
    - a disc menu page in the WebView;
    - a CD play;
    - live TV tune and catch-up with `streamToken`;
    - module settings;
    - everything again over peer to peer.

    s4 offers to run this pass.

## First-run setup (OOBE)

Today's setup wizard (`web/src/pages/setup/plan.ts`, `internal/server/setup_state.go`) already has module steps hard-wired. **Live TV** needs WittyTuner, **Games** needs WittyPlay, and **Discs** needs WittyDVD or WittyBluray. Games also asks about game titles and game pictures. Each step says where it is "later" in Settings. API 5 makes these steps the modules' own.

**A module declares its steps** in `integration.json`:

```jsonc
"setup": [ { "id": "games", "title": "Games", "after": "details",
             "when": { "libraryKinds": ["games"], "admin": true },
             "page": "wittyplay-setup",                        // an SDUI page in its sdui.json: native apps draw it too
             "saves": ["module:wittyplay:titles", "module:wittyplay:pictures"],
             "later": { "where": "Settings, Games & emulators", "to": "/settings/gaming" } } ]
```

| Field | What it means |
|---|---|
| `id` | Unique within the module. Shown as `<module>.<id>` |
| `title` | The step's name in the wizard's progress bar |
| `after` / `before` | Its place relative to a built-in step: `welcome`, `account`, `name`, `look`, `theme`, `modules`, `libraries`, `details`, `people`, `remote`, `ready`. It can never go before `modules` or after `ready` |
| `when` | `libraryKinds`: shown only if such a library exists or is being added. `admin`: always true today, because setup is the administrator's |
| `page` | The step's form, as an SDUI page with the module's own settings rows and `Form` component, so every app can draw it. A web bundle may also register a richer `setup` component for the web app |
| `saves` | Only the module's own settings (its `sdui.json` settings and its own routes); the server checks a step can't write anything else |
| `later` | Where the step lives in Settings afterwards: the "Do this later" link |

**Rules:**

- **Order.**
  - Built-in steps keep their places.
  - Module steps that name the same anchor go in order of module id. A tie can't reorder built-ins.
  - Two modules can't take the same step id, because ids are namespaced.
- **Skipping.** Every module step can be skipped. Skipping records nothing and shows the step's `later` link on the Ready page.
- **Nothing added means today's wizard.** With no module adding steps, the steps, order and screens are exactly today's, checked by the screenshot comparison.
- **Installed during setup.**
  - The wizard's own Modules step installs modules. Their steps appear in the remaining plan straight away, because the plan is recomputed after Modules.
- **Installed or upgraded after setup.**
  - A version with steps the server hasn't shown gets a **"Finish setting up `<module>`"** notice for administrators, on Home and in Settings → Modules. It opens those steps alone, in the same frame as the wizard.
  - An upgrade that adds a step does the same, for the new step only.
  - Steps already done or skipped are remembered by `<module>.<id>` and a version number the module raises when a step changes meaningfully.
- **Removed during setup.** Its steps leave the plan. Anything they saved stays with the module's data, under the uninstall rule ("keep" by default).
- **Migrated steps.**
  - `games`, game titles and game pictures become WittyPlay's.
  - `tv` becomes WittyTuner's.
  - `discs` becomes WittyDisc's, shared by DVD and Blu-ray.
  - The old-version warnings on the Modules step stay the server's.
- **Apps.**
  - The setup wizard is web-only today. Apps show "This server isn't set up yet" with the address.
  - SDUI step pages let the apps draw the "Finish setting up" steps later, behind a feature flag (`module-setup`) they opt into. Nothing changes for them until then.

`integration.json` gains `web.setup[].title`, `before`, `when`, `page`, `saves` and `later` (WittyModule's `SetupStep`).

## Security

**Threat model.** Three kinds of attacker, each assumed:

1. **A module that is malicious or taken over:** an official version, a fork, or a third-party store's module.
2. **A tampered download, or a catalogue in the middle.**
3. **A person on the server's network or signed in with less access,** trying to reach through a module what they can't reach directly.

The apps and browsers talking to the server must never become a way in.

**What is guaranteed, and where:**

| Guarantee | How |
|---|---|
| Only signed code runs | Every archive's SHA-256 is in a signed list. The catalogue is signed. Keys are fixed per module, and a different key is the `untrusted-key` override. Withdrawn versions are refused and taken out of use. A downgrade asks for the admin's password. All of this exists today and is unchanged |
| Permissions are declared, shown and agreed | `module.json` `permissions` (network, programs, devices, data) and `integration.json` (routes, aliases, frames, card hooks, data import, network hosts) are shown at install. **An upgrade that asks for more asks again.** The store shows the difference ("now also: network access to api.example.com") and installs only after the admin agrees. A grant never widens silently. The server's own list (`mayHave`) still decides which stock modules may hold which permission |
| A module runs boxed in | Separate process, with each system's sandbox as today: Landlock/seccomp/namespaces, sandbox-exec, job objects, rlimits/jails. Writes only to its own data folder. No network or programs unless granted |
| The host API is the only door back | It listens on loopback only. **One secret per module per start.** Every call is answered with the asking person's access. Only libraries of the module's own kinds are visible. Another module's data, settings, secrets and pictures are never reachable. Secrets are kept by the server and given only to the module they belong to. Legacy tables can be read only for the module that `data.import` names |
| Routes can't widen access | Sign-in, admin, parental feature, screen time and item checks are the server's, before the module sees anything. Browser sign-in and `X-Witty-*` headers are dropped. Limits on size and time. Aliases only of the server's own routes for that module. Reserved names are refused. File paths come only from the server |
| Web bundles stay in the page's rules | The same Content-Security-Policy as the app: no inline scripts, no other origins. Bundles get only the host SDK, and the builder refuses a second copy of React or the router. A bundle's CSS is on only while its screen is. A crash is reported and its screen replaced |
| Network hosts are declared | `integration.json` gains `network.hosts`: every host a module contacts by itself, with a reason. They're shown at install and must be on the server's privacy list (`internal/legal/legal.json`, Settings → Legal → services), which is generated from the installed modules' lists. Where a system's sandbox can filter by host (Linux, via the network namespace and a proxy), only those hosts are reachable. Elsewhere the list is a declared promise, and an unlisted host seen in the module's log is flagged |
| Data stays the module's, and leaves with it | Its database is in its own folder, in backups under its declared class, and kept or removed at uninstall as the admin chooses ("keep" by default). The server's own tables are never written by a module |
| Misbehaviour is contained | Three failures a day take a version out of use (exists today). A module that floods the host API (rate limits per module), keeps routes over time, or sends malformed answers is throttled, then quarantined: turned off, with the reason in its log and a notice to admins |
| Everything is recorded | The module audit log (exists) gains installs and upgrades with the permissions agreed, overrides, lifecycle calls, refused host API calls and quarantines |
| Apps are protected | Apps talk to the server's routes and to modules' routes under `/api/modules/<id>`, which the server checks first; the contract tests assert both. A module can't set cookies, can't serve pages on the app's origin except as a web bundle under the CSP, and ticket pages are always sandboxed |

**Never possible, overridden or not:** a module reading the server's database or another module's folder; a module receiving a person's sign-in; an alias of a route that isn't the module's own; turning off the encryption guards (scramble, AACS, MPEG-TS, HLS gate); a module served at the app's origin without the CSP; installing a withdrawn-for-security version; a module answering for parental or screen-time decisions.

## Overrides: bypassing a protection, with warnings

Today's override system (`docs/MODULE-STORE.md`, kinds `incomplete`, `out-of-range`, `local-file`, `custom`, `untrusted-key`, `weak-sandbox`, `unsigned`) is the model. API 5 adds kinds. Every override:

- is **admin-only**, needs the admin's password (and 2FA), and at severity 3 or more needs the module's id typed;
- is **never on by default**;
- shows **a warning dialog** saying what it allows, the risk in plain words, and how to undo it, with a link to the matching section of a new guide, **`docs/MODULE-SECURITY.md`**;
- is **recorded** in the audit log, shown as an **Overridden** badge in Settings → Modules and the store, included in the diagnostic report and the support bundle;
- is **reversible** from the module's page ("Remove override"), which goes back to the last state that needed none;
- pins the module so it never auto-updates while overridden (exists today).

| Override | What it allows | Severity |
|---|---|---|
| **Developer mode** (server-wide switch, new) | Shows "Install from a file" for unsigned modules (today's `unsigned`/`custom`), plus `wittymodule web preview` against this server. Each install still asks for its own override. A banner shows on every admin page while it's on, and it turns itself off after 24 hours unless renewed | 3 |
| **Trust this publisher** (new, per third-party catalogue) | Accept a store's key for that store's modules. They still get no permissions until each is ticked | 3 |
| **Grant a permission later** (new) | Tick a permission the manifest asks for that isn't granted, on an installed module (today only at install). Re-asked if a new version changes it | 2 |
| **Allow an extra network host** (new) | Let a module reach a host its `network.hosts` doesn't list. Added to the privacy list as "added by the administrator" | 2 |
| **Run outside the sandbox** (exists as `weak-sandbox` for macOS; new elsewhere) | Start a module whose sandbox won't start. Only for modules signed by a trusted key, never for unsigned or custom ones | 4 |
| `incomplete`, `out-of-range`, `local-file`, `untrusted-key`, `unsigned` | As today | 1–4 |

**Never offered:** turning off the encryption guards; letting a module read another module's data or the server's database; giving a module people's sign-ins; installing a version withdrawn for security; aliasing routes that aren't the module's; running an unsigned module outside the sandbox; forks of WittyTuner (as today); anything at all while `WMS_NO_MODULE_FORKS=1`.

`docs/MODULE-SECURITY.md` (written with the first override that's new): the threat model above in plain words; what each permission and override means, with its risk and how to undo it; how to read the audit log; what quarantine means; and how to report a module.

## WittyPlay: one interface, with its own store for cores and emulators

The owner, 2026-11-13: WittyPlay gets its own store for cores. A Settings button launches WittyPlay, and everything happens through that one interface. The existing core and third-party emulator structure is kept as it is, and works on every server platform, especially Docker and macOS.

**One interface.**

- **WittyPlay → Settings → Cores & emulators** holds everything about what this server plays:
  - browsing, installing, updating and removing **libretro cores** and **standalone emulators** (PCSX2, Dolphin, RPCS3, Ryujinx and the rest);
  - per-console default core and per-game core;
  - BIOS and firmware;
  - custom cores;
  - each core's and emulator's licence and notices;
  - what each needs, and why one isn't available on this system.
- It's built from today's screens: Consoles & cores, What this server plays, Standalone emulators and custom cores, already in the module since M1. They're arranged as one store in WittyPlay's frame.
- **The server's Settings keeps launchers only.** Its rows for these (`gaming`, `games-support`, `emulators`) become one row, "Games & emulators", which opens `/wittyplay/settings/store`. They are no longer a second copy of the UI.
- With WittyPlay older than 0.6 the server keeps its own rows, as today.
- The apps are unchanged: they have no core screens.

**Kept exactly, owned by the module:**

- The core builds list (`corebuilds.txt`) and per-platform availability with its refusals.
- Core downloads and checksums, licence acceptance and notices, the custom-core approval, versions and rollback, and the core audit log.
- GPU checks and the cores that need a graphics card.
- Standalone emulators: the catalogue, schemas generated from each emulator's config, settings writers, manual and own installs, the Docker/KasmVNC backend with its ticket route and sandboxed page, and the native per-OS runtimes:
  - Linux: bubblewrap;
  - macOS: sandbox-exec plus ScreenCaptureKit and input injection;
  - Windows: job object, low integrity, desktop duplication, WASAPI and SendInput;
  - FreeBSD: Xvfb with resource limits, admin opt-in.
- The game runner's own sandbox.

All of this already lives mostly in WittyPlay. What moves is the server's half: `api_games_custom.go`, `core_choices.go`, `games_platforms.go`, `games_gpu.go`, `games_bios.go`, `games_standalone.go`, the docker proxy routes and `emulator_sessions.go`. It moves in M2, as routes of the module with aliases for every path an app or page calls today.

**The same trust model, in the security section's terms:**

- Core and emulator downloads keep their checksums and signatures, and the core-downloads workflow that publishes them.
- Their hosts go in `network.hosts`: the libretro buildbot, the WittyPlayCores releases and each emulator's release host. That puts them on the privacy list.
- Docker access stays an administrator's choice with its warning.
- Installing an emulator outside a container remains the admin-only "your own program" path, with its warning.

**On every platform.** Each step's acceptance includes a run on each platform:

- macOS on Apple silicon, and on Intel where a machine is available;
- the Docker image (`linux/amd64` and `linux/arm64`);
- Linux on bare metal;
- a Windows build.

On each: install WittyPlay, install a core from the store, play a test core (WittyPlay's `testcore`), and start one standalone emulator where that platform supports it (Docker or native). FreeBSD is checked for what it promises: the store shows which cores have FreeBSD builds; standalone emulators are refused unless the admin opts in to Xvfb.

## Module by module

What moves (→ module) and what stays (host). "Hook" names what in M0 makes the move possible.

| Module | Moves into the module | Stays in the server | Hooks |
|---|---|---|---|
| **WittyPlay** | **Web:** PlayChrome, playPads, playPaths, PlayReviewNotice, MainGames' providers, GamePictureArt, GameCase, gamePictures, brandLogos (games), the Play pages, `pages/games/*`, Games.tsx, GameMeta, GameReview, the games settings pages, `setup/Games`, `tvmode/Games`, GameBio, `sdui/playModel`, the games CSS, `DesignPlay` fixtures. **Go:** all games handlers in `internal/server`; `gamemeta` (with RetroAchievements and rahash), `gameid`, `gamearchive`, `gameregion`, `wittyplayfile`, `library/games*` and `game_*`, BIOS sweep, GPU and core choice, custom cores, seats, online/presence, sources and ops, `corebuilds.txt`. **Data:** the 27 game tables and `<data>/gaming/` (saves stay on disk where they are, owned by the module's data class) | Generic items with `console` as a facet; library scan walking; parental feature `games`; stream tokens; FFmpeg; metrics and screen time through `live` | Routes and aliases, frames, card/player/takeover/navFold, settings, setup, scan hooks, live sessions, data import |
| **WittyCouch / WittyRigs** | `api_couch.go`, `api_rigs.go`, `api_wittyrigs.go`, `pages/Couch.tsx`, `Rigs.tsx`, `settings/{Couch,GameRigs,WittyRigs}`; tables `wittycouch_hosts`, `wittyrigs_*`; the Sunshine setup (module-isolation #23) | Rig libraries as generic libraries; `requires` WittyPlay for the shared frame | As WittyPlay |
| **WittyTuner** | Tuner admin and setup (`api_tuners` tuner part, `api_tvadmin`, `livetv/{setup,health,help}`), EPG source fetching, catch-up lookup, `TunerSetup`/`TunerTools`/`TunerLogo`/`EncryptedHelp`, the tuner half of `TvSettings`, `setup/Tv`; tables `tuners`, `tuner_channels`, `epg_sources` | Guide, recordings/DVR, favourites, sets (shared with virtual channels), live HLS, **mpegts and hlsgate**, `xmltv` until WittyBroadcast exists | Routes, settings, setup, data import, transcoder |
| **WittyDVD / WittyBluray** | `DvdPlayer.tsx` and `discTarget` (one shared disc player bundle, in WittyDisc's web part), disc SDUI sources (`disc_sources.go`), analysis/choice routes, "DVDs and Blu-rays" settings, `DiscDialogs` via an item-menu hook; later the presentation half of `dvdsession`/`bdsession` (module-isolation #20-22) | Session transport (HLS, overlays served), **scramble and AACS guards, answer checks**, disc columns on items, the library scan | Players, item-menu slot, settings, transcoder, sources |
| **WittyCD** | `CdPlayer`, `CdGraphics`, `cd.css`, the CD part of `cdEngine` via an audio-engine slot, identify and matches routes | Audio stream through FFmpeg, cover fetching, `items.cd_disc` | Players, audio-engine slot |
| **WittyPhotos** | Gets a web part: `PhotosLibrary`, `PhotoMap`, `PhotoViewer`, `mapMath` as its views; albums routes; photo-map settings | Photo scan and EXIF storage, image serving, uploads, sharing | Views, routes, settings, data (albums) |
| **Readers** | Nothing big. The format→reader map and WittyInk precondition become the `readers` hook; `Books.tsx` goes once WittyInk's pages are the only ones; the stock `books`/`library-books` pages defer to WittyInk's | Unpacking books and comics, conversion, **the sanitiser**, annotations | Readers |
| **WittyVisualizer, WittyStats, WittyIntegratedFileBrowser** | Adopt `register`; visualizer presets come from the module, not `music.json` | Metrics and who sees them; `/api/fs` and its fallback | Audio-stage slot |

## Order of work

Every step:

- deletes the server's copy of what it moves, in the same step (no legacy code is kept);
- is committed locally in each repository it touches;
- is **never pushed, tagged or released without the owner's say-so**.

Within a step the order is: WittyModule (kit), then the server, then the module, then the catalogue.

| # | Step | Repositories | Result |
|---|---|---|---|
| **M0a** | Module API 5 spec: `integration.json` schema and Go types, `check`, catalogue summary; `wittymodule/host` client; lifecycle methods | WittyModule | Kit tagged locally as 0.5.0; docs `docs/MODULES-API5.md` |
| **M0b** | Server: integration loader, the generic route proxy (auth classes, item and feature checks, limits, WebSocket, aliases with the legacy list), host API v1, lifecycle runner with rollback, module databases and the import runner, `legacyFor(id)`, `moduleFloors`, `live` sessions, parental/limits/backup/log topics read from manifests (equal to today's lists, by test) | WittyMediaServer | Nothing moves yet; every test still green |
| **M0c** | Web host API 5: the registry and its use in `App.tsx`, `SettingsPage`, the setup wizard, `ItemPage`, `openItem`, `PlayerPage`, `LibraryPage`, SDUI cards; the shared router, UI kit, state hooks, input; generated types; tokens contract | WittyMediaServer, WittyModule `kit/web` | Today's special cases become the server's *own* registrations of its legacy UI, so the look is unchanged by construction |
| **M0d** | Tooling: `wittymodule web build`/`preview`; the golden contract harness and screenshot baseline; `WittyMediaPlayer/engine` builds with `wittymodule/sdui` instead of the server's `pkg/sdui` (ends `GOWORK=off`) | WittyModule, WittyMediaServer, WittyMediaPlayer | S5's limits all addressed |
| **M1** | **WittyPlay UI** into the module: the files listed above, re-pointed at `@wittywilla/wms-host`, registered by `integration.json`. The server keeps its copy behind `legacyFor('wittyplay')` | WittyPlay 0.5.0, server | Pixel-identical screenshots; the WittyPlay bundle carries the Play UI. **Done 2026-11-13** (same text on 14 pages, 0.4.2 against 0.5.0) |
| **M1b** | **The cores & emulators store in WittyPlay's frame**; the server's three Settings rows become one launcher row (with WittyPlay ≥ 0.6) | WittyPlay 0.6.0, server | One interface; the store lists, installs, updates and removes cores and emulators on macOS (Apple silicon), Docker and Linux |
| **M2** | **WittyPlay APIs** into the module's service, route group by route group (cores, custom cores, BIOS, GPU and platforms first, with the store; then play/session, saves, settings, profiles, standalone emulators and the docker/kasm routes, online+sources+ops, pictures, achievements), each behind aliases, each with its golden tests | WittyPlay 0.6.x, server | Apps unchanged; the facade shrinks per group; cores install and a test core plays, and a standalone emulator starts, on macOS (Apple silicon, Intel where possible), the Docker image (amd64, arm64), Linux and Windows; FreeBSD as promised |
| **M3** | **WittyPlay data and the games library kind:** scan hooks with `gameid`/`gamearchive`/`gamemeta` in the module; tables imported into its database; secrets for metadata keys; console table served at run time; facades removed | WittyPlay 0.7.0, server | Server has no games code outside `legacyFor`; the module download holds the whole feature; the platform runs of M2 pass again with the module's own data |
| **M4** | WittyCouch and WittyRigs, the same way (rigs need WittyPlay's frame: `requires` with a range) | WittyCouch, WittyRigs, server | |
| **M5** | Disc player bundle (WittyDisc web part) and WittyDVD/WittyBluray sources, settings, routes; WittyCD player | WittyDisc, WittyDVD, WittyBluray, WittyCD, server | Guards untouched |
| **M6** | WittyTuner admin, setup, EPG sources | WittyTuner, server | Guide and DVR untouched |
| **M7** | WittyPhotos web part; readers hook; visualizer, stats and file browser adopt `register`; stock pages defer to modules | the modules, server | |
| **M8** | Catalogue: every API-5 version listed with its integration summary; store shows "what this adds" | WittyModuleCatalog | |

**WittyPlay first, as asked.** M0 is the cost of doing it properly and is shared by every later step. M0a to M1 is the first thing the owner can see working.

### Other sessions' areas

Each server step touches files other sessions own. The coordinating session is told before each step starts.

| Session | Area | Steps |
|---|---|---|
| s1 | SDUI core | `pkg/sdui`: ops and sources, M0b/M0c |
| s2 | web shell | `App.tsx`, settings, M0c |
| s5 | WittyPlay engine | M1-M3 |
| s6 | live TV | M6 |
| s7 | discs | M5 |
| s8 | readers and modules | M7 |

## Testing

- **Unit and package tests in every repository**, as today.
  - Go always runs with `GOMAXPROCS=4 GOFLAGS=-p=2`.
  - One heavy build at a time.
- **Golden contract tests: the apps' guarantee.** Today's server answers a fixed script of requests from the apps' real use:
  - games home, consoles, play prepare and session, saves, profiles;
  - tuner list, disc analysis, CD view, photo months, couch hosts.

  The script runs against seeded test data. Answers are normalised (ids, times) and saved. Every step replays them against three states:
  - module absent: expect `424`s;
  - module installed (API 5): the new contract.

  Lives in `internal/server/contract_modules_test.go`, beside `contract_test.go`, with the module binaries from a local test catalogue (`scripts/make-test-catalog.sh`).
- **SDUI conformance:** `pkg/sdui/conformance` in the same three states; module documents compared before and after.
- **Pixel parity, web.**
  - Before M1, a baseline of key screens is captured in the built-in browser on this session's own test server (`127.0.0.1:19805`, catalogue `19806`, data in this session's scratchpad, generated test media only, nothing from `/Volumes/STUFF`).
  - Screens: Home, the games library, the game page, PlayChrome in each layout, Play settings, pictures, controllers, the pause menu and launch screen, TV mode, phone width.
  - Each step re-captures them and compares; any visible difference is a failure, not a judgement call.
  - Also checked by eye in the browser pane.
- **Uninstall:** with and without keeping data.
- **Native apps:** s3/s4 run their test suites (`GamesTests`, `SDUIModulesTests`, Android equivalents) against the test server at M2, M3 and M9.

- **Every platform, for WittyPlay (M1b–M3):**
  - macOS on Apple silicon (this Mac), and on Intel where available;
  - the Docker image (`linux/amd64`, `linux/arm64` under emulation where needed);
  - Linux on bare metal;
  - a Windows build.

  On each: the store lists and installs a core, the test core plays, and a standalone emulator starts where supported (Docker, or the native runtime). The sandbox level is reported as before. FreeBSD: cores listed as the builds list says; emulators only with the admin's opt-in.

## Risks

| Risk | Mitigation |
|---|---|
| The web host API becomes a permanent ABI | Expose components and hooks the app already treats as stable; version with `host.has`; generated types and a drift test |
| Two copies of react-router break navigation | The router is shared through the host, never bundled; `check` refuses a bundle that bundles it |
| Proxy latency on game streams | WebSocket and WebRTC signalling only pass through; media already flows module → browser. Measured against S5's numbers before M2 lands |
| Data import loses or splits data | Idempotent import, legacy tables kept until M9, downgrade exports back or is refused; backups before each import |
| Scan hooks slow big game libraries | Batched calls, the module's own sandboxed archive worker (already exists), progress through tasks; measured on the test library |
| Forks gain power through `integration.json` | Aliases limited to a signed list; hooks for other publishers only by explicit grant |
| Overlap with other sessions' edits | No branch switching in the shared folder; edits coordinated through the coordinating session; one step at a time |
| Scale | M0 is about the size of one large feature; M1-M3 move ~50k lines. Each step is shippable on its own, and M9 is optional until everything has moved |

## Decisions for the owner

1. **Approve module API 5 as described.** That covers `integration.json` as the install script, the host API, web host API 5, module-owned databases, and legacy paths kept until M9.
2. **Game identification and metadata (`gameid`, `gamearchive`, `gamemeta`) move into WittyPlay.** This drops the WittyGameCore-in-the-server idea from module-isolation.md, so they update through the store. The server keeps only generic items.
3. **Module data in its own SQLite database**, imported once from today's tables. The alternative, tables in the server's database under the module's name, keeps backups simpler but puts migrations back into the server.
4. **Uninstall asks whether to keep a module's data.** Recommended default: keep.
6. **WittyPlay's own store for cores and emulators** (owner, 2026-11-13): one interface in WittyPlay; the server's Settings keeps a launcher row; the core and emulator structure kept exactly and tested on every platform, especially Docker and macOS. Decided.
5. **Which server version carries M0** (suggested: 0.9.0-alpha), and whether M1-M3 ship as WittyPlay 0.5-0.7 on the alpha channel.

---

<sub>WittyMediaServer development plans</sub>
