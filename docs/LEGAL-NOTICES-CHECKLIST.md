# Legal notices checklist

Which notice each kind of screen needs. The words live in one place,
[`internal/legal/notices.json`](../internal/legal/notices.json), so a screen only names the kind:

```tsx
import LegalNote from '../components/LegalNote'   // the path depends on where the screen is

<LegalNote kind="webResults" />
```

That's all it takes: it's small, screen readers announce it as a legal notice, and it links to the part
of the Legal page it's about. On a screen someone shouldn't be taken away from, add `newTab`. Directly
under a settings page's header, add `className="set-legal"`.

Show one notice per screen, near the content it's about, not one per result or per card. Every page
in the app's frame already has the `general` notice at its foot, which hides itself when the page shows
a notice of its own.

## Which notice

| The screen shows… | Kind | On master now |
|---|---|---|
| Anything in the app's frame, with no notice of its own | `general` | `App.tsx` page foot |
| Search over this server's own library | `searchResults` | Search |
| Results fetched from a service on the web: lookups, directories, artwork choices | `webResults` | Artwork picker, podcast search, Metadata agents, setup wizard's agents step |
| A title's page with details or artwork from outside | `metadata` | Movie and episode pages, shows, albums |
| Media streaming from outside: podcasts, stations | `webMedia` | Radio, podcast libraries and pages |
| TV channels and guide data | `tv` | TV guide, Channels & tuners |
| Studio, network, channel or console logos, and any screen choosing one | `logos` | Library pages with logo cards, a show or title with one, the brand page, the guide, Channels & tuners, a channel's editor, a tuner's channel table, the edit dialog's Logos, the logo chooser, Metadata agents. `components/LogoNotice` shows it only when one is on the screen |
| A logo on something that's only up for a moment | `logosBrief` | The banner in the TV player |
| Which streaming services list a title | `listings` | Library pages and the brand page showing listings, the disclosure screen |
| Games and game details | `games` | Games library, game pages |
| Downloading cores | `cores` | Settings, Consoles & cores, and the install dialog |
| Modules and their updates | `modules` | Settings, Modules |
| Creator videos and the people named in them | `creators` | Creator libraries, channels, watch page, vertical videos' creators and About |
| Books and comics, reader content | `reader` | Books library, book pages |
| A connection to a server someone else runs | `thirdPartyServer` | Game PCs: `pages/Couch.tsx`, `pages/Rigs.tsx`, Settings, Game PCs |
| An app's choice of server | `connect` | The apps' connect screens |
| A place where someone adds their own folders | `local` | Settings, Libraries, and the setup wizard's tick-box (only there, once) |
| A place where someone shares | `sharing` | Share dialog, Settings, Sharing |
| What a person sees when opening a share link | `sharedView` | Shared page, and its viewer |
| Turning on access from outside | `remote` | Settings, Remote Access |
| Age limits and Kids profiles | `ratings` | Add profile, profile limits |
| Something likely to flash | `flashing` | Part of `games` today. WittyVisualizer has its own, stronger notice instead: see below |

If nothing fits, add a kind to `notices.json` with its `section` on the Legal page, and to the list in
`internal/legal/legal_test.go`. Keep the core words for third-party content: *aren't created by,
associated with, endorsed by or checked by the WittyMediaServer project, its contributors or affiliates*.

## Screens with more to keep

Keep to these when changing one of these areas. Don't edit a notice's words for one screen: add a kind if it really differs.

| Area | Screen | Kind | Also |
|---|---|---|---|
| Radio | Station search and directory results | `webResults` | `all.api.radio-browser.info` and its servers stay in `services` in `legal.json`. The directory is on by default, so it stays in the privacy section's "On by default" line and in `THIRD_PARTY_LICENSES.md` |
| Radio | Stations from the directory; radio pages | `webMedia` | On the Radio library |
| Game details | Game details, ratings, box art, every source it adds | `games` on the page, `webResults` on match or search results | Add each source to `services`. Credit each source on the game's page, as `GameDetails` now does for Wikidata |
| Game PCs | Moonlight or any connection to someone's gaming PC or server | `thirdPartyServer` | Add the bridge's GitHub download and its licenses (AGPL-3.0-or-later, with Moonlight-derived parts under GPL-3.0) to `services` and the licenses page. The bridge's install dialog takes `cores`-style wording: add a kind if needed. The added encryption is mentioned (RSA, ECDSA, AES) in the `cryptography` section |
| Custom cores | Installing or adding a core or firmware | `cores` | In the add dialog. "New custom modules" are third-party: `modules` where those are added |
| Game settings and controllers | Settings around games | None, if they show no third-party content | If a screen shows a game or its details, `games`. Button pictures must not copy a console maker's logos |
| Licenses | Settings, About, third-party licenses | None | Keep `<div id="licenses" />` just above the license list, since Legal links to `/settings/about#licenses` |
| WittyCD | Album lookups | `webResults` on results, `metadata` on album pages | Add each lookup host to `services`; credit it as its terms ask |
| WittyVisualizer | Visualizer | None | Its own notice appears before each person first turns it on, and has to be accepted, so `flashing` would only repeat it. Each person accepts for themselves, on the server and in the desktop player |
| Modules | The Modules page, in any new form | `modules` | Keep it directly under the page's header. New custom modules: `modules` in the install dialog too |
| Logs and the diagnoser | Logs, and anything sent or copied off the server | None, unless it sends something | If it can send anything, it goes on the privacy list and says so first. `Logs & privacy`'s header now says "Logs are never sent anywhere": keep that true |
| WittyMediaPlayer | Standalone player | `local` on first run, `webResults` on any lookup | It's its own program, so copy the wording from this file. The owner checks its look before it lands |
| Discs | Anything added to disc pages | None | Use the copy-protection wording below in any new message |

## Before merging

1. `go test ./internal/legal ./internal/server`. It fails when:
   - a screen in the table in `internal/legal/legal_test.go` lost its notice (add new screens there);
   - the code names a host that isn't on the privacy list (add it to `services` in
     `internal/legal/legal.json`, or to `notContacted` if it's only a link for people to click);
   - anything tells people to decrypt a disc;
   - the wording names another media server.
2. A new migration wasn't needed for this work: it stores everything in existing settings.

## Still to fill in

The owner replaces these marked gaps in `internal/legal/legal.json`. The Legal page shows them as marked until then.

- [[FILL IN: a contact for legal questions about the project, such as an email address set up for this]]

## Wording for refusing encrypted media

Say that it's refused. Never say how to get a copy that isn't.

> This disc is encrypted, so it can't be played: encrypted media isn't supported, and nothing here bypasses copy protection.

## Module READMEs

Each repository's README ends with a **Legal** section. For a new repository, adapt this:

```markdown
## Legal

<Name> is an independent open-source project. It isn't associated with, sponsored by or endorsed by
<the format's or platform's owners, makers and services it works with>. <What it doesn't include: for
example, no media comes with it, and you're responsible for having the rights to what you open.> Any
third-party names here only describe what it works with, and belong to their owners. It's provided
“as is”, with no warranty; see [LICENSE](LICENSE). Local law applies to how you use it. This isn't
legal advice.
```

- **WittyCD:** say that it isn't a licensed Compact Disc product and uses no disc logos, and where album
  lookups come from.
- **WittyVisualizer:** add the `flashing` notice.
- **WittyMediaPlayer:** add `local`, and credit any optional playback libraries it uses.
- **WittyCouch, WittyCouch Host, WittyRigs:** say that it isn't associated with the projects it connects to.

---

<sub>WittyMediaServer documentation</sub>
