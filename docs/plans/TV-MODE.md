# TV mode on the website

> **Status: built, 2026-11-05.** Written before the build, as its contract, and corrected where building it showed more. It replaces *Big screen mode*.

**TV mode** is the website laid out and driven like the Apple TV app, which Android TV matches: the same top bar, the
same screens, the same focus, read from across the room and used with a remote or a game controller. It's for a TV's
own browser, the desktop app on a TV (`--tv`) and Plasma Bigscreen, and for anyone who wants the couch view on a
computer.

The source of truth for how a TV looks is WittyMediaClient's `docs/APP-DESIGN.md`, section **On a TV**, with its top
bar as the TV session (tv-top-menu) is finishing it. Where this page and that one differ, that one wins and this page
is corrected.

- [Turning it on and off](#turning-it-on-and-off)
- [Sizes](#sizes)
- [The frame](#the-frame)
- [The screens](#the-screens)
- [Driving it](#driving-it)
- [Themes](#themes)
- [Off means off](#off-means-off)
- [How it's built](#how-its-built)
- [Checking it](#checking-it)

## Turning it on and off

It's **one option**, `TV mode`, and it replaces *Big screen mode* (bigger words and a stronger outline), which is gone.
It's **per device**, as before: kept in this browser's storage (`wms.tvmode`), not on the account. Someone who had Big
screen mode on finds TV mode on (the old `wms.tenfoot` is read once and moved over).

| Where | What |
|---|---|
| Settings → Appearance → Motion and big screens | A switch, **TV mode**, "Laid out and driven like the Apple TV and Android TV apps, for a screen across the room. Only on this device." |
| The setup wizard's Look step | The same switch, in place of "This screen is across the room" |
| The account menu (top right) | **TV mode**, which turns it on |
| The address | `?tv=1` turns it on and `?tv=0` off, and both are remembered. The desktop app's `--tv` and Plasma Bigscreen already send `?tv=1` (WittyMediaClient `desktop/tv.js`) |
| You, in TV mode | **Leave TV mode**, which turns it off and goes to Home |

Turning it on goes to Home, as a TV app opens there. Couch mode (`/big`, games only) stays as it is.

## Sizes

Every size here is in **TV units**, as APP-DESIGN gives them: one unit is a point on a 1920×1080 Apple TV. On the web
a unit is `--tvu`, 1/1920 of the window's width (or 1/1080 of its height, whichever is smaller), so the same layout
fills a 1280×720 TV browser and a 3840×2160 one alike, as a TV app does.

| Name | Size | Weight | Use |
|---|---|---|---|
| Display | 48 | Bold | Who's watching?, the hero's title |
| Title | 38 | Semibold | A screen's heading |
| Headline | 30 | Semibold | A profile's name on the password screen |
| Section | 26 | Semibold | Above a row or a group |
| Body, Label | 24 | Regular, Medium | Sentences; pills, rows and tiles |
| Secondary | 20 | Regular | Facts |
| Caption | 18 | Medium | Badges, captions |

- **Gutter** 90 on both sides; the top bar 24 from the top edge.
- **Focus:** a 4-wide ring in `text`, 4 outside the thing, **concentric** with its corners (the outline follows
  `border-radius`, so a radius-14 tile's ring has an outer radius of 22), and a 1.06 scale. Focusable things sit at
  least 16 apart, and rows leave 24 above and below so a ring is never cut off. Only one thing has focus, always.
- **Every control is a pill**, 66 high with 32 inside; a quiet one is 56 high. Keys of the on-screen keyboard are rounded
  squares (radius 14), and a focused key fills with `text` instead of taking a ring.
- Pages that aren't TV screens of their own (an item, a library, Settings…) keep their own layout, with the type a
  quarter larger, the stronger focus and the wider gutter that Big screen mode gave them.

## The frame

- **The top bar** is on each tab's first screen: Home, Libraries, Games, Live TV, Search and You.
- **A screen opened from a tab has the whole screen**: no top bar. Back returns to the tab, where focus is what it was.
  For a mouse or a finger there is a round Back button at the top left, which the arrows pass over.
- Switching tabs doesn't add to the browser's history, so Back always means "up a level", never "the tab before".
- The player, books, couch mode and WittyTV's watch view are full screen as before.

## The screens

### Who's watching

The website's own profile screen, laid out as APP-DESIGN's TV one: "Who's watching?" in Display, centred; the profile
tiles in one centred row, 48 apart, **200** avatars, names in Label centred under them, and `@username` in Secondary
under that when a profile name is set and differs. Focus affects **only the avatar**: it lifts 6 and grows 1.06, with a
`background` band 4 wide and then the `text` ring 4 wide, both concentric with its corners. The name brightens from
`textSecondary` to `text`. The row leaves room above for the lifted ring. The first profile has focus. Kids and Guest
badges sit on the avatar's lower edge, centred, half over it.

The password screen is one centred column: the avatar at 200, the name in 38 semibold, `@username`, "Enter your
password or PIN." in 24 `textSecondary`, then the field and Sign in, 640 wide, then Forgot your password?.

### The top bar

Five pills on the left, in this order: **Home, Libraries, Games, Live TV, Search**, each its icon and its name in Label.
**You**, the person's avatar at 28 (a circle) with their profile name, is **pinned to the right end of the same row**,
set apart by the space between. 16 between pills. The chosen pill has `accent` at 14% behind it and its words in
`accentText`; the others are `textSecondary`. A focused pill takes the ring and the scale. All six always show; a server
with no games or no live TV shows that tab with a plain empty state that says what's missing and that an owner adds it
on the server's website.

### Home

The website's Home, at TV size: the hero, then its rows (Jump back in, On TV now, Recommended, Next up, Recently added…)
in the order the person chose. *Your libraries* isn't on a TV's Home: Libraries is a tab of its own. The hero fills the
width, 620 high, under the bar; its words sit in a column
**at most half the screen wide**, the title in Display **wrapping to up to 3 lines and never cut off**: "Harry Potter
and the Philosopher's Stone" shows whole. Under it the facts, two lines of the synopsis, and **Play** (primary) and
**Details** as pills. Focus starts on Play. Card titles in the rows wrap to two lines rather than being cut.

### Libraries

Every library the profile can see, as a grid of tiles: each tile is the library's icon in `accentText` on `accent` at
14%, then its name in Label and its count in Secondary. A tile opens the library (a pushed screen); a games library
opens Games' view of it.

### Games

WittyPlay's couch library. With one games library it opens straight in, as the existing couch library. With several,
one shelf per library, as Home's rows are: the library's name, its games' covers (each opens that game's page) and an
**All games** pill that opens the whole library. A game's page and the library are pushed screens; Play starts the game
full screen, and leaving the game comes back to its page. These are the existing games pages (`pages/games/`), until
the WittyPlay design session's look replaces them.

### Live TV

The website's own WittyTV guide, in the account's WittyTV look, with Recordings reachable from it, as it is today, under
the top bar.

### Search

A field across the top and an **on-screen keyboard** on the left: the letters A–Z and 0–9 as rounded-square keys,
then Space, Delete and Clear as wider keys, reachable by arrows and chosen with OK. A real keyboard and a TV's own
keyboard type too. Results come as you type (after a short pause), grouped into Shows and everything else, as a grid of
posters on the right. Focus starts on the first key.

### You

Two columns, as on Apple TV: the **header on the left third**, vertically centred (the avatar at 200, the profile name
in Title, `@username · Role` in Secondary, then the pills **Switch profile**, **Leave TV mode** and **Sign out**), and
the **groups on the right two-thirds** as a focusable list, scrolling: each row 72 high, a 32 icon tile in its colour,
the name in Label, a value in Secondary, a chevron. The rows are APP-DESIGN's You table, each opening the website's own
page for it (a pushed screen), hidden where the server would refuse them. The first row has focus.

### The player

The website's player with TV-size controls. The picture has the arrows: **left and right seek 10 seconds**, up and down
change the volume, OK brings up the controls, Play/Pause and the media keys work. **Back leaves the player** (in TV mode
it stops, rather than shrinking to the mini player, as a TV app does). The disc remote's button stays on the controls
for every disc type; the remote itself is separate work.

### Everything else

Every other page of the website (an item, a show, a library, collections, Settings, Help…) opens as a pushed screen,
with Big screen mode's sizes. Nothing is left out.

## Driving it

**Focus** moves with the website's spatial navigation (`ui/spatial.ts`, `tv.ts`), with these TV rules:

- **Up from a tab's first row reaches the bar**, and lands on the chosen tab's pill, wherever the row was.
- **Down from the bar** goes back where focus was in the tab, or else to the tab's main action.
- **Focus starts on the screen's main action** and comes back where it was left (rows remember their card).

**Back** (Escape, GoBack, BrowserBack, Backspace outside a field, a remote's Back):

| Where | Back |
|---|---|
| A dialog, menu or the player is open | Closes it (as now) |
| A pushed screen | Goes back to where it was opened |
| Inside a tab | Focus goes to the bar, on the chosen pill |
| The bar, on a tab other than Home | Goes to Home, focus on Home's pill |
| The bar, on Home | Leaves: to the app around the page (`data-app-back`, or the desktop app's own menu) |

**Game controllers** (the Gamepad API, as today):

| Button | Does |
|---|---|
| D-pad, left stick | Move |
| A (Cross; B on Nintendo) | Choose |
| B (Circle; A on Nintendo) | Back |
| LB, RB | The tab to the left or right (on a tab's first screen) |
| Start (Menu, Options) | The screen's options: Customize on Home, Look in the guide, Filters on a library; a disc's menu in the player |
| X, Y, triggers, Select | As in the player today |

**Remotes:** arrows, Enter or OK, Back, and the media keys (Play, Pause, Play/Pause, Stop, Rewind, Fast forward,
Previous, Next) as the player has them. **A mouse and touch still work**: everything is a real button or link, and the
round Back button stands in for the remote's Back on pushed screens.

## Themes

Every server theme, built-in, uploaded or made, works in TV mode through the same roles: `background` is `--bg`,
`panel` `--bg-2`, `card` `--bg-3`, `text` `--fg`, `textSecondary` `--fg-2`, `accent` `--accent`, `accentText`
`--accent-text`. Nothing in TV mode has a colour of its own.

## Off means off

Everything is behind `html.tv-mode`, set only in TV mode. With it off, the phone and desktop layouts are exactly as
they were: no new route, element or style reaches them. (The old `?tv=1` meaning, bigger words only, is TV mode now.)

## How it's built

| File | What |
|---|---|
| `web/src/tvmode/mode.ts` | On or off, kept per device; `?tv=`; the old key moved over |
| `web/src/tvmode/nav.ts` | The tabs, which tab a page belongs to, LB/RB stepping, what Back does: plain rules, tested |
| `web/src/ui/gamepad.ts` | A controller's buttons and sticks to key presses, out of `tv.ts` so it can be tested |
| `web/src/tvmode/TvFrame.tsx` | The frame, the top bar, the TV keys and focus rules |
| `web/src/tvmode/*.tsx` | Libraries, Games, Search with its keyboard, You |
| `web/src/tvmode/tvmode.css` | Everything TV mode draws, in TV units, all under `html.tv-mode` |

Tests: `web/src/tvmode/nav.test.mjs` and `web/src/ui/gamepad.test.mjs`, beside the existing `ui/spatial.test.mjs`, run in
CI with the other web tests.

## Checking it

At 1920×1080, signed in as a test profile, in the default theme and one light theme, each of: Who's watching, the
password screen, Home (with a long hero title), Libraries, Games, Live TV, Search with the keyboard, You, the player,
and one pushed screen (an item). For each, compare with APP-DESIGN's On a TV and the Apple TV screenshots: the bar, the
pills, the ring, the sizes. Then the same with TV mode off at phone and desktop widths, which must not have changed.
