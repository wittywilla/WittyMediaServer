# Module API 5: a module owns its whole feature

For developers of modules and of the server: what module API 5 adds to a module, what the server does with it, and how a module's feature reaches the apps unchanged. The plan this comes from, with the order modules move in, is [plans/MODULAR-MODULES.md](plans/MODULAR-MODULES.md). The rest of the module contract is [MODULES.md](MODULES.md) and [SDUI-MODULES.md](SDUI-MODULES.md).

- [In short](#in-short)
- [integration.json](#integrationjson)
- [Routes](#routes)
- [Aliases: the server's own paths](#aliases-the-servers-own-paths)
- [The host API](#the-host-api)
- [Lifecycle](#lifecycle)
- [The web part](#the-web-part)
- [Building a web part](#building-a-web-part)
- [Versions and what a server offers](#versions-and-what-a-server-offers)
- [While a module moves: the server's own copy](#while-a-module-moves-the-servers-own-copy)
- [Testing](#testing)
- [Not yet](#not-yet)

---

## In short

| A module of API 5 has | Where | What the server does with it |
|---|---|---|
| `integration.json` | beside `module.json` and `sdui.json`, in every archive | Reads it strictly at install and at every start; refuses a version whose file doesn't pass; applies it while the version is in use |
| HTTP routes | its service, under `/http` | Mounts each at `/api/modules/<id><path>` and at its aliases, checks who may call it, passes the request on |
| A web part that exports `register(host)` | `web/` in its archive | Tells the web app what the module claims; loads the bundle when one of its screens is first needed |
| Its own data | a SQLite database in its data folder | Backs it up; lets it read the server's own tables it takes over, once |
| Lifecycle calls | its service, on the helper protocol | Calls them at a version's first start and before the module is taken off |

`wittymodule.API` and the server's `API` are 5. A server takes 3 to 5. A version of API 5 must carry `integration.json` if it's a helper, program or web bundle (`wittymodule check` and `catalog-add` refuse one without it).

## integration.json

Read strictly: one object, at most 256 KiB, and an unknown field anywhere is refused. The Go types are `wittymodule.Integration`; `wittymodule check` checks it the way a server does.

```jsonc
{
  "format": "wittymodule-integration", "formatVersion": 1, "module": "wittyplay",
  "host": { "http": ">=1 <2", "web": ">=5", "host": ">=1" },
  "feature": "games",
  "http": [
    { "path": "/games/{id}/play", "methods": ["POST"], "auth": "user", "item": "id", "plays": true,
      "alias": ["/api/games/{id}/play"], "limits": { "body": "256KB", "time": "3m" } },
    { "path": "/play/{sid}/stream", "methods": ["GET", "POST"], "auth": "user", "upgrade": "websocket", "stream": true,
      "alias": ["/api/play/{sid}/stream"] },
    { "path": "/kasm/{sid}/{ticket}/{rest...}", "methods": ["GET"], "auth": "ticket", "csp": "sandboxed-page" }
  ],
  "web": {
    "routes": [ { "path": "/wittyplay/*", "frame": "fullscreen" }, { "path": "/game/:id", "frame": "fullscreen" } ],
    "cards": [ { "mediaKind": "game", "art": true, "detail": true, "link": true } ],
    "settings": [ { "id": "gaming", "label": "Consoles & cores", "admin": true, "search": ["cores", "BIOS"] } ],
    "setup": [ { "id": "games" } ],
    "navFold": [ { "libraryKind": "games", "into": "wittyplay-online" } ]
  },
  "sdui": { "ops": [ { "id": "wittyplay.room.create", "http": "POST /rooms" } ] },
  "data": { "store": "sqlite", "import": "server-games-v1", "backup": "saves" },
  "lifecycle": { "install": true, "upgrade": true, "uninstall": "ask-keep-data" },
  "live": { "sessions": true }
}
```

| Field | What it is |
|---|---|
| `host` | Ranges of what the module needs of the server, by host feature: `http` (routes), `web` (the web host API), `sdui` (the catalogue revision), `host` (the host API). `">=1 <2"`, `">=5"`, `"5"` |
| `feature` | The parental feature everything here belongs to (`games`, `recordings`, …, one the server knows). A profile without it can't call the routes |
| `http` | [Routes](#routes) |
| `web` | [The web part](#the-web-part) |
| `sdui.ops` | Its SDUI commands, each answered by one of its own routes (`"METHOD /path"`) |
| `data` | Its own database: `store` is `sqlite`; `import` names the server's data it takes over once ([the host API](#the-host-api)); `backup` is the part of a backup it goes in (`settings`, `saves`, `library`). Needs the `data` permission and a `dataFormat` in `module.json` |
| `lifecycle` | Which [lifecycle calls](#lifecycle) it answers |
| `live` | `sessions`: it reports live sessions (games, streams) to the server |

Routes, data and lifecycle need a module with a service (`delivery` `helper`, with `service.Config.HTTP`).

## Routes

Each route is mounted at `/api/modules/<id><path>` and `/api/v1/modules/<id><path>`. A module's path can't start with one of the server's own names under `/api/modules/{id}/` (`install`, `versions`, `web`, `enable`, …: `modules.ReservedPaths`).

| Field | What it does |
|---|---|
| `path` | Go `ServeMux` style: `/games/{id}/play`, `/kasm/{sid}/{rest...}` |
| `methods` | `GET`, `HEAD`, `POST`, `PUT`, `PATCH`, `DELETE` |
| `auth` | `user`: anyone signed in. `admin`. `ticket`: nobody's sign-in; the module checks a secret in the path, the request carries no identity and no cookie, and only `GET`/`HEAD` |
| `item` | A path parameter that is a library item's id. The server checks the profile may see it (404 otherwise), and tells the module about it with its files |
| `plays` | Calling it starts or goes on playing: it counts towards screen time and is refused when that's used up (a `DELETE` never is) |
| `upgrade` | `websocket`: the route takes a WebSocket. One from another site's page is refused |
| `stream` | The answer is a stream, sent as it comes, with no size or time limit |
| `alias` | [The server's own paths](#aliases-the-servers-own-paths) it also answers |
| `limits` | `body` (default 1 MB), `answer` (default 16 MB), `time` (default 1 minute) |
| `csp` | `sandboxed-page`, for a ticket route whose answer is a page: it's shown in a sandboxed frame with an origin of nobody's |

**What the server checks, before the module hears of a request:** sign-in, administrator, the profile's feature and screen time, the item, the size of the request, and for a WebSocket that it comes from this site. **What it drops:** the `Authorization` header, its sign-in cookie, a stream token in the query (`token`, `st`), and every `X-Witty-` header a browser sent. **What it adds** (`wittymodule/service`, `service.Who(r)`):

| Header | Value |
|---|---|
| `X-Witty-Service-Token` | The secret of this start of the service |
| `X-Witty-User`, `X-Witty-Admin` | Who is asking (none on a ticket route) |
| `X-Witty-Item` | The item a route's `item` names: id, library, kind, title, facets (`console`), and its files as paths to read. base64url JSON |
| `X-Witty-Features` | The features this profile may use, space separated |

**What it does with the answer:** drops `Set-Cookie`, adds `X-Content-Type-Options: nosniff`, holds a non-stream answer to its limit, and leaves `Cache-Control` to the module.

A module checks again what matters to it (`service.People`, `service.Admins`), and decides what's whose: a save, a session, a person's settings.

**A route never takes a file path from a browser.** A file comes from `X-Witty-Item`, or from the host API's `Item`, both opened after the server's own access check.

## Aliases: the server's own paths

The Apple and Android apps call the server's own paths (`/api/games/{id}/play`, `/api/play/{sid}/ws`, …), and they must keep working, unchanged, whichever side answers. A route's `alias` lists those paths; an alias must have the same parameters as the route.

- **Only the server's own routes for that module can be aliased.** Those are the routes the server registers for a module's feature with `needing(id)` (internal/server/api_modules.go): the server's own copy. An alias of anything else is left out and logged.
- **Route by route.** While a version of API 5 that claims an alias is in use, that one route goes to the module; every other route of the server's copy still answers as it did. When the module is absent, the alias answers `424 Failed Dependency` with `module` and `moduleName`, as it always has.
- **Signed in and classed as before.** An aliased request is signed in and checked for parental controls by the server's own route, exactly as it was, then passed to the module as above.
- The `/api/v1/` form follows by itself.

So a module moves its API into itself one route group at a time, with the apps none the wiser: the answer must be the same status and shape (see [Testing](#testing)).

## The host API

A module's way back to the server: package `wittymodule/host`. The server listens on this machine only, on an address of its own (never its public one), and tells each service where, and a secret of its own, when it starts it (`service.StartParams`, `service.Host()`). `host.New()` is a client for it.

Every call that's on someone's behalf says who (`host.On{User}`, or `host.For(r)` from a route's request), and the server answers with that person's access: a module can't see through the host what the person asking couldn't. A module only ever sees libraries of the kinds its `module.json` lists in `libraries`.

| Call | Path (`/host/v1`) | What it does |
|---|---|---|
| `Hello` | `GET /hello` | The module the server thinks this is, the server's version, what it offers |
| `Item` | `GET /items/{id}` | An item, with its files, if the person may see it (403) and it's in one of the module's kinds of library (404) |
| `Items` | `GET /items?libraryKind=&library=&kind=&facet.console=&search=&offset=&limit=` | Items the person may see, without files, at most 500 |
| `May` | `GET /may?feature=` | Whether the person may use a feature now: their limits, parental rules and screen time |
| `Settings` | `GET /settings` | The module's settings (from `sdui.json`): the server's, then the person's own |
| `Secret` | `GET /secrets/{name}` | A secret an administrator gave the module (kept as `module.<id>.secret.<name>`), or "" |
| `PutImage` | `POST /images?item=&role=` | Keeps a picture (PNG, JPEG, GIF or WebP, up to 8 MB) and says where pages show it: `/api/module-images/<module>/<name>` |
| `StartSession`, `EndSession` | `POST /sessions`, `DELETE /sessions/{id}` | Something live, for metrics, screen time and "playing now" |
| `Events` | `GET /events?after=&wait=` | Waits up to `wait` (at most 55s) for events after a number: `server.started`, `scan.finished`, … |
| `Task` | `POST /tasks/{name}` | A long piece of work, shown with the server's tasks (administrators only) |
| `Legacy` | `GET /legacy/{table}?after=&limit=` | A page of rows of one of the server's own tables that `data.import` names, by rowid, each as a JSON object |

Refusals are JSON `{"error"}` with the status; an `X-Witty-Host` header tells a refusal of the host API's from a call this server doesn't have (`host.ErrNotOffered`).

**The legacy tables** each import may read are listed in the server (`legacyImports`, internal/server/host_api.go): `server-games-v1` (WittyPlay's 23 tables), `server-couch-v1` (WittyCouch's and WittyRigs'). Ids are kept: the module's data refers to the server's item ids, user ids, save slots and session ids as they are, because the apps keep them.

## Lifecycle

Calls on the helper protocol's pipe, to the service (`service.Config.Lifecycle`), with `wittymodule.LifecycleArgs`:

| Call | When | Args |
|---|---|---|
| `lifecycle.install` | The first start of the module's first version of API 5 on this server | `to` |
| `lifecycle.upgrade` | The first start of a version after another was set up | `from`, `to` |
| `lifecycle.uninstall` | Before the module is taken off (`POST /api/modules/{id}/remove`, `?keepData=0` to remove its data; kept otherwise) | `keepData` |

A version isn't used until its `install` or `upgrade` went through: an error stops the start, is reported as a failure of that version, and the next start tries again (three failures in a day take the version out of use, as for any failure). The version that was set up is kept in the module's state (`integrated`). A module that can't be told it's being taken off still goes.

## The web part

What `web` claims, the web app learns from `GET /api/modules/web` (each bundle's `integration`), and keeps for the next visit, so a module's screen isn't first drawn by the app's own copy. The bundle's `register(host)` export hands over the components, loaded when one is first needed (web/src/moduleRegistry.tsx).

| Claim | What the web app does |
|---|---|
| `routes` with `frame: "fullscreen"` | The route takes the window in place of the app's frame (as the players and WittyPlay's frame do) |
| `routes` with `frame: "app"` or `"tv"` | The route's page comes before the app's own, inside the app's frame |
| `cards` | Cards of that media kind take their art, detail panel and link from the module (`game` today) |
| `settings` | A section of Settings: one with a built-in section's id takes its place, one with `after` goes after that one, any other under "From modules" |
| `setup`, `players`, `readers`, `takeover`, `navFold` | Declared and checked now; drawn from the module as each module that needs one moves ([Not yet](#not-yet)) |

`register(host)` returns a `Registration` (types in WittyModule `kit/web/host`, package `@wittywilla/wms-host`):

```ts
import type { ModuleHost, Registration } from '@wittywilla/wms-host'
export function register(host: ModuleHost): Registration {
  return {
    routes: { '/wittyplay/*': PlayFrame, '/game/:id': GamePage },
    cards: { game: { art: (a) => <GameArt {...a} />, link: (to) => playLink(to) } },
    settings: { gaming: GamingSettings },
  }
}
```

**What a module is given** (`ModuleHost`): the app's API client (`http`) and its own routes (`moduleApi`), the app's state (`useApp`, `useTvMode`), `toast` and `ask`, the app's UI kit (`ui`) and icons, `ModuleMissing` and `LegalNote`, the SDUI renderer's contexts (`sdui`), TV focus (`input`), and `has(feature)`. A screen that crashes is reported as the module's failure and replaced with what's missing; the app goes on.

**Looking the same.** A module draws with the app's own components, and its stylesheet uses the app's design tokens as `var(--name)` (WittyModule `kit/web/tokens.json`, from `scripts/export-module-kit.mjs`), never copies of their values, so every theme and the server's own colours follow. A bundle's stylesheet is on only while its screen is.

## Building a web part

WittyModule `kit/web/build-bundle.mjs` is the one builder (no module keeps a copy of its own any more):

```bash
node ../WittyModule/kit/web/build-bundle.mjs . dist
```

It writes `index.js` and `style.css`. React, react-dom, the JSX runtime, **react-router-dom**, WittyInk and pdf.js become stand-ins for the app's own copies (`globalThis.__wmsModuleHost`): a second copy of any of them is refused at build time, because its context would be nobody's. It uses Vite 8's `codeSplitting: false` (and the old name on Vite 6).

## Versions and what a server offers

A server offers each host feature at a version (`modules.Offers()`): `http` 1, `web` 5, `sdui` its catalogue revision, `host` 1. A version whose `host` ranges this server doesn't meet is refused, with the reason. Adding to a host feature raises its version; a module asks for `">=n"` and uses `host.has(…)` (web) or `Client.Has(…)` (Go) for anything newer that it can do without.

## While a module moves: the server's own copy

Until every module has moved, the server keeps its own copy of each module's feature (its routes, its pages), used whenever that module has no version of API 5 in use: absent, turned off, or a version of API 3 or 4. That's what keeps an older module working on a newer server, and the apps working with any of them. A module's version of API 5 takes the copy's place route by route (aliases) and screen by screen (the web part). When every stock module is on API 5, a later server deletes the copies and raises its oldest module API to 5.

## Testing

- **The apps' contract:** `internal/server/contract_modules_test.go` sends the apps' own calls (from WittyMediaClient) and compares each answer's status and shape (every key, and the kind of every value) with `testdata/contract/<state>.json`: `absent` today, `legacy` and `api5` with a module from the test catalogue. `WMS_UPDATE_CONTRACT=1` rewrites them; only ever on purpose, agreed with the apps.
- **Routes:** `module_routes_test.go`: identity, sign-in dropped, forged headers dropped, admin routes, features, items, aliases, ticket routes, paths out of a route.
- **Host API:** `host_api_test.go`.
- **The host object:** `web/src/moduleHost.test.mjs` keeps `hostFor` and WittyModule `kit/web/host` the same (`WITTYMODULE=<checkout>` if it isn't beside this one).

## Not yet

These are designed in the plan and come with the module that needs them:

- Setup steps, players, readers, library takeover and menu folding from the web part (WittyPlay's move, then discs and readers).
- Live sessions counted in metrics and screen time, and doctor checks from modules.
- Module databases in backups.
- `/capabilities` features from what modules are in use (today they are the server's fixed list; changing that is agreed with the apps first).
- `wittymodule web preview`: a module's screens without a server.
- Generated parental path prefixes and module menu folding from the manifests.

---

<sub>WittyMediaServer documentation</sub>
