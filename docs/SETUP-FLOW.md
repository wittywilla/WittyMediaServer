# First-time setup: the flow

This page says what first-time setup asks, in what order, and why. It is the plan the setup pages
(`web/src/pages/setup/`, `SetupWizard.tsx`, `setup/plan.ts`) and the server's setup routes
(`internal/server/setup_state.go`) follow. [FIRST-TIME-SETUP.md](FIRST-TIME-SETUP.md) is the guide for people
setting up a server; this page is for whoever changes setup.

## The rules

1. **Six numbered steps**: Welcome, Your account, Modules, Your media, Look, Finish. The count never changes
   under someone.
2. **Each page has a short plain title, one sentence at most, and one obvious main button.** No "Skip for now"
   under the card, and no "You can change this later in Settings, …" on the app's own pages: Welcome says once
   that everything can be changed later. A module's own page may still say where its settings are.
3. **The usual answer is already chosen.** Pressing the main button on every page gives a working server.
4. **Two ways through**, chosen on Welcome and kept in this browser (`wms.setupAdvanced`) until setup ends:
   - **Quick setup** shows only what's needed to start watching. Every other choice is behind the step's
     **Customize** button (`parts.tsx`, `Customize` and `Option`): a dialog of folded parts, each with what it
     is in a few words. Nobody has to open it.
   - **Advanced setup** gives each of those choices a page of its own (`advanced: true` in `plan.ts`), next to
     the step it belongs with, and drops the Customize buttons that would repeat them.
5. **Modules come before the media and the look**, so both are about what the server has. **Your media** also
   offers the module a kind of library needs, right on the kind.
6. **The words**: no "port", "P2P", "NFO", "metadata agent" or "API key" on a quick-setup page. A module is a
   module everywhere (not an add-on, not an extension). The server is named once on any page.
7. **TV mode**: a part a remote does badly (colouring letters, importing a file, pasting keys) says so in the
   app's usual words (`pkg/sdui/data/tvlevels.json`, docs/TV-MODE.md). Quick setup never needs one of them.

## The steps

| # | Step | Quick setup | Customize (quick) | Advanced pages |
|---|------|-------------|-------------------|----------------|
| 1 | Welcome | Language; Get started, Advanced setup, Restore from a backup | Disc menus and sound tracks; time zone | Language and time zone |
| 2 | Your account | Sign-in name, password, server name, the legal tick; where data is kept (desktop installs that ask) | Name people see; two-step sign-in | Server name (colours, mark); People (profiles, required looks) |
| 3 | Modules | Modules, the folder browser ticked; Install | When a module has a new version | (on the page) |
| 4 | Your media | Kinds and folders; a module's own page (Live TV, Games) when installed | Posters and details beside files; one or separate libraries | Discs; Posters and details; Logos; Lyrics and artists; Karaoke voices; Streaming lists |
| 5 | Look | Layout, with its preview | Colours; TV guide (WittyTuner) | Colours; TV guide |
| 6 | Finish | Connect a device; add a profile; this screen as a TV; Start watching | Home network; posters and details; logos; lyrics; karaoke; streaming lists | Home network; Away from home |

**Defaults**: language and disc tracks follow the browser; the computer's time zone; details saved beside
files; one library per kind; the folder browser ticked; module updates "tell me"; each look's own theme; apps
at home find the server by name; logos, online lyrics, artist pages, Demucs and streaming lists off.

**Connect a device** is the same component as Settings, Remote Access (`components/ConnectDevice.tsx`): one
short code and a QR code, with the details further in. Setup doesn't explain peer-to-peer or relays.

## The server's names for the steps

`GET /api/setup/state` answers `steps: welcome, account, modules, media, look, ready`; a page is saved as the
numbered step it's part of. The names 0.8.0 to 0.8.2 used are still taken (`oldSetupSteps` in
`setup_state.go`): language → welcome; server, name, storage, people → account; libraries, modulesetup, tv,
games, discs → media; looks, theme, colours, wittytv, tvguide → look; network, remote, extras and its pages →
ready. In the browser, an old page address, or an advanced page opened in quick setup, opens the page that
holds its question now (`currentStep` in `plan.ts`).

## What 0.8.3 removed, merged or reworded (#610)

- **Merged**: Language into Welcome; the server's name into Your account; Libraries and the module pages into
  Your media; Layout, Colours and WittyTV into Look; People, Away from home and the five Extras pages into
  Finish (quick) or their own pages (advanced).
- **Removed from quick setup** (still in advanced setup or Customize): Home network and its port editor, the
  disc start choice, required looks per profile, the peer-to-peer explanation, posters and keys, NFO
  conversion, logos, lyrics, karaoke, streaming lists.
- **Removed altogether**: the dimmed "After you choose modules" and "Nothing to set up" steps, "Skip the
  extras", the second "Skip for now" under the card, the per-page "change this later" lines, the Encrypted
  connections note, and Ready's eight-line summary and its notes about library transcoding, TV channels and
  `-listen`.
- **Reworded**: every page title and lead to one sentence; "Modules" lead; disc page; people page ("What people
  can change"); module updates.

---

<sub>WittyMediaServer documentation</sub>
