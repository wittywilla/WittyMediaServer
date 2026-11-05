# Design: Alpha, Beta and Stable release channels for modules

> **Status on 2026-11-04**
>
> **Planned:** [the idea](https://github.com/wittywilla/WittyMediaServer/discussions/30). Modules are still on API 3 with no channel.

For the next module API (API 4), with 0.7.0-alpha. The owner: "Support Alpha, Beta, and Stable branch modules. Change over all internal modules right now to an alpha branch."

Not built. Not tested.

---

## What exists today

From `docs/MODULES.md`:

- Module API 3. `MinAPI` and `API` are both 3. `minServer` 0.6.0-alpha is the start of the line.
- Versions follow semantic versioning. Ranges `minServer` to `maxServer`, with `x` for a series.
- **Pre-release modules are offered only to a server that's itself a pre-release**, or that already runs that module's pre-release. A stable server never gets one by surprise; an admin may choose one from the list of versions after a warning.
- A signed catalogue (WittyModuleCatalog, `catalog.json`, mirrored in the server as `internal/modules/known.json`) lists versions. Versions can be taken back. Update All skips pinned versions, other publishers and taken-back versions.
- Third-party stores and forks exist, each with an admin choice, a key and a warning.

**The project stays in alpha for a while** (the owner, 2026-11-02): beta comes only once every feature they want is at least somewhat implemented. So for a long time nearly every module version will sit on the alpha channel, and beta and stable will be nearly empty. Channels are still worth building now: the machinery and the store UI are in place, and testers can already choose a version a developer marks beta when it's ready.

So there's already a rough two-channel rule: pre-release versus release, tied to semver suffixes. Channels make it explicit, give it a third step, and let a module be promoted without a rebuild.

---

## The model

**A channel is a property of a version in a catalogue, not of the module or the archive.**

| Channel | Meaning | Who should follow it |
|---|---|---|
| **alpha** | Built and tested by its developers. May change, may break, may lose module data on a downgrade | People testing, and every alpha server by default |
| **beta** | Has been on alpha without a known serious fault, and has met the real-world checks its README lists | People who want new things early but working |
| **stable** | Has been on beta through a stable server release | Everyone else, and every stable server by default |

**Why in the catalogue and not in `module.json`:** the manifest is inside the signed archive. If the channel lived there, promoting 0.4.0 from alpha to beta would mean rebuilding and re-signing, and the beta archive would not be byte for byte the one people tested. In the catalogue, promotion is one signed catalogue change, and the archive stays exactly as tested.

**The manifest may set a floor:** `"channelFloor": "alpha"` means "never list me above alpha". A developer uses it for a build that should never be promoted by accident. `wittymodule check` and the catalogue tools enforce it.

### Channels and semver pre-release tags

The two must agree, so nobody has to remember two rules:

| Version | May be on |
|---|---|
| `0.4.0-alpha.3` | alpha only |
| `0.4.0-beta.1` | alpha, beta |
| `0.4.0-rc.1` | alpha, beta |
| `0.4.0` | alpha, beta, stable |

So a plain version can sit on alpha (today's first-party modules are all plain versions, and they move to alpha with this change) and later be promoted without a new number.

### What a server follows

- Each module has a **channel to follow**: alpha, beta or stable. Stored per module on the server.
- **The default comes from the server's own version:** an alpha server follows alpha, a beta server beta, a release server stable.
- "Follow beta" means "offer the newest version on beta **or more stable**". Following alpha sees everything.
- A module never moves to a **less** stable channel without an admin choosing it, with a warning, and the password (consistent with the store's existing rule for forks and stores).
- **Moving to a more stable channel never downgrades on its own.** If the installed version is newer than anything on the new channel, it stays, and the store says "You'll get the next stable version when it's newer than this one", with a button to go back now, which warns about module data (`dataFormat`).

### The existing pre-release rule

Kept, restated in channel terms: a stable server follows stable, so it never sees alpha or beta versions unless an admin switches a module's channel. Pinning, taking back and Update All work per channel exactly as they do now.

---

## Moving every first-party module to alpha

The owner asked for this "right now". In practice it happens with the release that ships channel support (0.7.0-alpha), because a 0.6.x server has no idea what a channel is.

1. **Catalogue format version +1.** Each version entry gains `"channel"`. A missing channel means alpha, so the old entries are alpha without anyone editing them.
2. **0.6.x servers** read the catalogue as before. They ignore `channel`. That's safe: they're alpha servers, which would follow alpha anyway. A module that needs API 4 says `minServer: 0.7.0-alpha`, so 0.6.x never installs it.
3. **Every first-party module's release workflow** publishes to alpha by default: `wittymodule release --channel alpha` (the default when omitted), recorded in the catalogue entry it opens.
4. **The module READMEs** gain a line: "Channel: alpha. Beta and stable come later."
5. **The store** shows an "Alpha" badge on every version.

## Promotion

A promotion is a pull request on WittyModuleCatalog that changes one version's `channel`, signed by the catalogue key like any catalogue change.

**What a version needs before promotion**, written in each module's README so the bar is visible:

| To | Needs |
|---|---|
| beta | All its tests pass on every platform it ships for, with real modules (`WMS_TEST_CATALOG`); the README's real-world checklist done (for example: a real disc, a real tuner, a real pad); no open fault marked serious; at least two weeks on alpha; the owner's yes |
| stable | On beta through one stable server release; no serious fault found on beta; data migration tested from the previous stable; the owner's yes |

No telemetry is involved: the project doesn't collect it, and this doesn't change that. Faults come from the owner's testing and from reports people choose to send.

## Third-party stores and forks

- A third-party store may use channels. The store shows them as **"Stable, according to <publisher>"**, since only that publisher vouches for it.
- A fork follows its own catalogue's channels. The existing per-module fork warning is unchanged.
- WittyTuner still can't be forked.

---

## The UI

**Settings → Modules:**

- Each module card: the installed version and its channel badge.
- A module's page: **Updates from: Stable / Beta / Alpha** (a radio group; less stable choices carry a short warning). The list of versions shows each one's channel.
- Store filters gain **Channel**.
- **Settings → Modules → Updates:** a server-wide default channel ("Follow the server's channel", the default, or a fixed one), applied to modules that haven't been set by hand.
- The changelog panel on each module (added in 0.6.0) shows which channel each entry reached and when.

Accessible as the rest of the store: a radio group with names, arrows move, the warning is read with the choice.

## API

| Route | Change |
|---|---|
| `GET /api/modules`, `GET /api/modulestore/...` | Each version has `channel`; each installed module has `follow` |
| `PUT /api/modules/{id}/follow` | Admin. Body `{ "channel": "beta" }`. Less stable asks for the password; audited |
| `PUT /api/modules/follow-default` | Admin. The server-wide default |

## WittyModule changes (API 4)

- `wittymodule.API = 4`, `MinAPI` stays 3 through 0.7.x (unofficial compatibility, as the owner allowed for the alpha line). Raise `MinAPI` to 4 at 0.8.0.
- `channelFloor` in the manifest schema.
- `wittymodule release --channel` and `wittymodule promote <module> <version> <channel>` (writes the catalogue change for a PR, never pushes).
- Version rules: channel compatibility check in one place, used by the server, the catalogue tools and CI.

## Tests

- Table tests for every channel and version combination above, the way `docs/MODULES.md` tables the version ranges.
- A 0.6.0-alpha server reading the new catalogue (old-server compatibility).
- Switching channels both ways with a module whose data format changed.
- Update All across mixed channels, pinned versions and a taken-back alpha.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
