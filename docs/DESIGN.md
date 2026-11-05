# Design standards

The WittyMediaServer interface, in one place: what it's trying to be, the values it's built from,
every shared part and its states, the patterns for whole pages, how we write, and what makes it
work on a phone, a keyboard, a screen reader and a TV across the room.

These are rules, not suggestions, and a script holds us to the checkable ones: see
[Keeping to it](#keeping-to-it). See the parts working at `/design` when signed in as an
administrator.

**New here?** Read [Principles](#principles), then [Adding a page](#adding-a-page).

- [Principles](#principles)
- [Foundations](#foundations): [colour](#colour), [type](#type), [space](#space), [shape](#shape), [depth and layers](#depth-and-layers), [motion](#motion), [icons](#icons)
- [Components](#components)
- [Patterns](#patterns): [pages](#pages), [navigation](#navigation), [dialogs and sheets](#dialogs-and-sheets), [forms](#forms), [loading, empty and error](#loading-empty-and-error)
- [Writing](#writing)
- [Accessibility](#accessibility)
- [Remote and keyboard](#remote-and-keyboard)
- [Native apps and ten feet](#native-apps-and-ten-feet)
- [Keeping it fast](#keeping-it-fast)
- [Keeping to it](#keeping-to-it)
- [Adding a page](#adding-a-page)
- [Adding a step to first-time setup](#adding-a-step-to-first-time-setup)

## Principles

1. **The artwork is the loudest thing.** Everything around a poster stays quiet: surfaces are close
   in tone, labels sit on dark plates, and only one thing in a view is in the accent colour.
2. **The accent means "do this next".** Primary buttons, progress and focus use it. Something merely
   *selected* (a chip, a segment, a top-bar link) is inverted instead, in every theme.
3. **One of everything.** One type scale, one spacing grid, one button, one dialog. A new look for an
   old job needs a very good reason.
4. **Every input is a first-class input.** Mouse, touch, keyboard, remote, game controller and screen
   reader all get the whole app. Nothing depends on hover. Nothing is reachable by pointer alone.
5. **Modest hardware first.** If an effect costs a frame on a cheap TV, it doesn't ship, or it turns
   itself off there.
6. **People choose; the owner sets the default.** A person's own choice beats the owner's default,
   which beats ours. One clear step takes you back.

## Foundations

All values are CSS custom properties in `web/src/styles.css`. Use a token, not a number. If no token
fits, that's usually the design saying "don't". `scripts/design-tokens.py` writes them out as data for
the native apps.

### Colour

Themes set a few colours; everything else is worked out from them, so a theme can't forget a state.

| Surface | Use |
|---|---|
| `--bg` | the page |
| `--bg-2` | panels, dialogs, side menu: one step up |
| `--bg-3` | things inside those: fields, buttons, rows |
| `--bg-4` | hover on `--bg-3` |

| Text | Use |
|---|---|
| `--fg` | the content |
| `--fg-2` | supporting sentences |
| `--fg-3` | captions and labels (still 4.5:1) |
| `--accent-text` | links and the accent as words. Never `--accent`, which may be too dark to read |

| Other | Use |
|---|---|
| `--accent`, `--accent-2`, `--accent-fg` | the main action, its hover, and the words on it |
| `--danger`, `--ok`, `--warn` | status, and always with words or an icon as well |
| `--border`, `--border-strong`, `--control-line` | hairlines, emphasised lines, outlines of fields (3:1) |
| `--hover`, `--pressed` | states that work on any surface |
| `--accent-soft`, `--accent-line`, `--ok-soft`, `--warn-soft`, `--danger-soft` | tinted backgrounds |
| `--focus` | the focus ring: 4.5:1 on every surface |
| `--on-image`, `--scrim`, `--picture-bg` | words on artwork, the plate behind them, and what's behind a picture or video (these never change with the theme) |

The player is always dark, whatever the theme: `.player` sets its own colours.

### Type

Sizes are in `rem`, so they follow the reader's browser setting and grow for controllers. The font is
the system's own (`--font`): no download, sharp on every screen, and it has every alphabet a title
might use.

| Token | Size | For |
|---|---|---|
| `--text-2xs` | 11 | badges, uppercase labels |
| `--text-xs` | 12 | captions, card subtitles, hints |
| `--text-sm` | 13 | secondary text, small buttons, chips |
| `--text-md` | 14 | controls, card titles, dense text |
| `--text-base` | 15 | body |
| `--text-lg` | 17 | panel headings |
| `--text-xl` | 20 | section headings, dialog titles |
| `--text-2xl` to `--text-5xl` | 24, 30, 38, 48 | large artwork text |
| `--page-title` | 24 to 30 | the `h1` of a normal page |
| `--display` | 30 to 48 | a title over a backdrop |

Also `--leading-tight`, `--leading-snug`, `--leading-body`; `--tracking-display`, `--tracking-heading`,
`--tracking-label`; `--weight-heading`; `--font-mono`.

### Space

A 4px grid: `--sp-1` 4, `--sp-2` 8, `--sp-3` 12, `--sp-4` 16, `--sp-5` 20, `--sp-6` 24, `--sp-8` 32,
`--sp-10` 40, `--sp-12` 48, `--sp-16` 64.

| Token | For |
|---|---|
| `--gutter` | the page's side margin (16 to 40, by width) |
| `--section-gap` | between sections of a page |
| `--topbar-h`, `--tabbar-h`, `--rail`, `--sidebar` | the frame's sizes, for anything placed against it |
| `--dock-space`, `--tabbar-space` | room to keep clear at the bottom. They're `0` unless the audio bar, mini player or phone bar is there |

Anything floating at the foot of the page: `bottom: calc(var(--sp-5) + var(--dock-space) + var(--tabbar-space))`.

### Shape

Radii follow the theme's `--radius` (10 by default), so `--radius: 0` makes the whole interface
square: `--r-xs` 4 (badges), `--r-sm` 6, `--r-md` 10 (buttons, fields, posters), `--r-lg` 14 (panels),
`--r-xl` 20 (dialogs), `--r-full` (pills).

### Depth and layers

Three shadows: `--e-1` resting artwork, `--e-2` a lifted card, `--e-3` anything over the page. Panels
on the page have a hairline, not a shadow.

One stacking order. Use these names, never a number above 9:

| Layer | Value | What |
|---|---|---|
| `--z-sticky` | 10 | sticky headers and save bars inside a page |
| `--z-tabbar` | 25 | the phone's bottom bar |
| `--z-dock` | 45 | audio bar, mini player |
| `--z-topbar` | 55 | the top bar, above the dock, so what opens from it is never behind |
| `--z-scrim`, `--z-drawer` | 56, 57 | the side drawer and the dim behind it |
| `--z-modal` | 60 | dialogs and sheets |
| `--z-fullscreen` | 70 | the full-screen music player |
| `--z-popover` | 80 | menus and popovers, which can open from any of these |
| `--z-toast` | 100 | toasts and tooltips |

Overlays render into `<body>` through a portal, so nothing around them can clip or cover them.

### Motion

| Token | Time | For |
|---|---|---|
| `--dur-1` | 90ms | pressing |
| `--dur-2` | 150ms | hover and focus |
| `--dur-3` | 240ms | something appears |
| `--dur-4` | 420ms | a backdrop changes |

`--ease-out` for things arriving, `--ease-standard` for things that change in place. Things appear;
they don't bounce or fly. Animate `transform` and `opacity` only (colours on hover are fine), and never
`transition: all`.

`prefers-reduced-motion` turns transitions and animations off globally.

**Moving posters** (`ui/tilt.ts`): movie and show posters lean toward the pointer, up to `--tilt-max`
degrees at `--tilt-scale`, with a soft light that follows. A remote gets it as a settle from the
direction it moved. Square covers don't move. Each person can turn it off, and it stays off when the
device asks for reduced motion or less data, or has two cores or 2 GB or less.

### Icons

`web/src/icons.tsx`: a 24 grid, filled, in `currentColor`, sized by the context (16 in dense controls,
20 by default, 24 on the phone bar). Never a text character as an icon, which looks different on every
platform. An icon on its own needs a name: `IconButton`'s `label` is required for this reason. Icons
are decoration beside words, so they carry `aria-hidden`.

## Components

Import from `web/src/ui`. Each is on `/design`. Older pages may use the class directly: classes are part
of the theme contract (see [THEMES.md](THEMES.md)) and keep working.

Every interactive part has these states, which come free:

| State | Looks like |
|---|---|
| Hover | the surface shifts with `--bg-4` or `--hover`. Only in `@media (hover: hover)` if it moves or reveals anything |
| Pressed | `scale(.975)` on buttons |
| Focus | the ring in `--focus`, on `:focus-visible` |
| Disabled | `opacity: .45`, no hover, not reachable if the control itself is disabled |
| Selected | inverted (`--fg` on `--bg`) for small controls, `--accent-soft` for rows and tiles |
| Busy | `aria-busy`, with a spinner in place of the icon |

| Part | Use it for | Notes |
|---|---|---|
| `Button` | an action. `variant`: `primary` (one per view), `ghost`, `danger`. `size`: `sm`, `big`. `icon`, `busy` | Says what happens: "Remove profile", not "OK" |
| `LinkButton` | navigation that looks like a button | A real link: it opens in a new tab |
| `IconButton` | an action with just an icon. `label` required | The label is the screen reader's name and the tooltip |
| `Tooltip` | a short hint | Never holds anything you need: touch doesn't show it. Escape dismisses it |
| `Field` | a label, hint and error around one control | Wires up `id`, `aria-describedby` and `aria-invalid` for you |
| `Input`, `Textarea`, `Select` | text and lists | `Select` is the browser's own list, which every device shows well |
| `Switch` | on or off, taking effect at once | `label` or `labelledBy`. Use a checkbox in a form that has a Save button |
| `Segmented` | one of two to four short choices | A radio group: Tab is one stop, arrows move |
| `Tabs` | sections of a page | `to` makes each a link; otherwise `panelId` ties them to a panel |
| `Chips`, `Chip` | filters that can be on or off | `onRemove` adds a named remove button |
| `Dialog` | something that needs an answer or a focused job | See [dialogs](#dialogs-and-sheets) |
| `Sheet` | things beside the page | From the side on wide screens, from the bottom on phones |
| `ask()` | "are you sure?" | Replaces `confirm()`, which a remote can't always answer. `await ask({ title, body, confirm: 'Remove', danger: true })` |
| `Menu` | a short list of actions from a button | Arrows, Home, End, typing a letter, Enter, Escape or Back. Opens upward if it must |
| `toast()` | a short note that something happened | `tone`: `ok`, `warn`, `danger`. `action` adds one button. Pauses under the pointer or focus |
| `Card` | a surface holding related things | |
| `PosterCard` | a title with artwork | `shape`: `wide`, `square`. The `ItemCard` and `ShowCard` the libraries use render the same classes |
| `Shelf`, `CardRow` | a titled row of cards | The arrows are the mouse's extra way along; a remote moves card by card |
| `Skeleton`, `SkeletonGrid`, `Spinner` | loading | They wait a moment, so a fast load shows nothing |
| `EmptyState` | nothing here | Says why, and what to do |
| `LoadError` | it couldn't load | With a retry |
| `Notice` | something to know, in the page | `tone` as for toasts |
| `Progress`, `ProgressRing` | how far along | `label` required. No `value` means "working on it" |
| `DataTable` | sortable rows | Real table semantics, `aria-sort`, stacks into labelled rows on phones |

Also: `.pill` (rating and small labels), `.meta` (a line of facts, dots between them automatically),
`.badge` on artwork, `Placeholder` (art for something with no picture), `BackLink`, `ScanButton`, and
in Settings `PageHead`, `Section`, `Row` and `SaveBar`.

## Patterns

### Pages

A page has one `h1`, which the announcer reads aloud on every page change: `.toolbar` with an `h1`, or
`PageHead` in Settings. One main action gets the `primary` button. A page over a backdrop is
`.detail-hero` + `.detail`; the top bar runs over it by itself.

### Navigation

- **Wide screens**: a side menu, a top bar, or both, chosen per device under *Settings, Appearance*.
- **Top bar**: what doesn't fit goes under **More** by measuring, and the page you're on moves its name
  onto that button, so you can always see where you are. Someone can choose to scroll sideways instead.
- **Side menu**: folds to a rail of icons, or away, and remembers that on this device.
- **Arranging**: drag by the handle, use the arrow keys on it, or the Move buttons (what a remote has).
  Hide anything; group into Watch, Listen, Read and Play dropdowns. A new library joins at the end.
- **Whose arrangement**: a person's own, else the owner's (`Use for everyone`), else ours.
- **Phones**: a row of sections scrolling sideways at the top; a floating bar at the bottom with Home,
  TV, Search, the drawer and You.
- **Activity**: `components/ActivitySlot.tsx`, left of the account menu, holds the activity button.

### Dialogs and sheets

A title that says what it's for. The safest answer comes first in the row of buttons. `alert` means it
needs an answer: it's read out at once, and a click outside doesn't dismiss it. A form fills the
dialog and ties its submit button to it with `form="id"`. Don't stack dialogs. Keep it to a dialog's
worth: a long job gets a page.

Focus moves in, Tab stays in, Escape or Back closes, and focus returns to what opened it. All of that
is `a11y.tsx`, for anything with `role=dialog`.

### Forms

A visible label on every field, above it. Hints say what makes a good answer before there's a wrong
one. Errors say what's wrong and how to fix it, beside the field. Settings that take effect at once use
`Switch` with no Save button; a group that's saved together uses `SaveBar`, which stays in view while
there's something to save.

### Loading, empty and error

Every page has all three.

- **Loading**: shapes where the content will be, after a short wait. Never a blank.
- **Empty**: says why it's empty, and offers the next step.
- **Error**: says what happened in plain words, with a way forward. Never a code alone.

## Writing

- **Plain and short.** "Remove profile", not "Delete user account entity". Say it as you would to a
  friend who isn't technical.
- **Say what will happen.** Buttons name their action. A question names the consequence: "Your files
  stay where they are."
- **Sentence case** for everything: buttons, headings, menus.
- **"You" and "your".** Not "the user".
- **No blame.** "That name is taken", not "Invalid input".
- **Punctuation**: curly quotes and apostrophes. An ellipsis in a label means more is coming
  ("Rename…"). No exclamation marks.
- **Names people choose are data**, shown exactly as written and always escaped.

## Accessibility

The bar is WCAG 2.2 AA, on every screen.

- **Contrast**: 4.5:1 for text on every surface, 3:1 for outlines. `scripts/theme-contrast.py` checks
  every built-in theme and fails CI; the theme maker can't produce one that falls short.
- **Keyboard**: everything works, in a logical order, with no traps. Escape closes what opened.
- **Focus**: always visible. `:focus-visible` gets a ring in `--focus`; cards ring the poster.
  `scroll-padding` keeps the focused thing clear of the bars.
- **Names**: real roles and labels. Icon buttons have names, fields have labels, images have `alt`
  (`alt=""` for decoration).
- **Live regions**: `announce()` for things that change without focus moving, and page changes are
  announced for you. Toasts are read out.
- **Targets**: 24px at least everywhere (2.5.8), 44px on touch, 48 at ten feet. A small mark gets an
  invisible bigger box around it.
- **Colour is never alone**: selected things change shape or fill, status has words.
- **Motion**: `prefers-reduced-motion` is respected everywhere. Nothing moves by itself without a way
  to stop it. `prefers-contrast: more` and `forced-colors` are handled.
- **Phones**: a 16px gutter, no sideways page scroll.

## Remote and keyboard

Arrow keys alone reach everything, with no extra code in most pages. The rule for which control is
next is in `web/src/ui/spatial.ts`, pure and tested, and the same rule should be ported to any client
without a platform focus engine. `web/src/tv.ts` applies it, and `web/src/a11y.tsx` handles Back and
focus restoration.

**The contract for a page:**

- Use real `a`, `button` and `input` elements. A click handler on a `div` can't be reached.
- Nothing is hover-only: what hover reveals also appears on `:focus-visible`, or isn't needed.
- `data-nav-group` on a set of controls that belong together makes coming back land where you left.
  Rows, the top bar, the side menu and the phone bars already are groups.
- `data-nav-row` on one line of things whose items differ in height or lift when focused, such as a games shelf:
  Up and Down always leave it, and Right past its end goes to the link in the `data-nav-head` beside it, as a
  `.hub-head` is for a row. It's a group too.
- `data-focus-key="..."` on a control with no `href` lets Back return to it.
- `data-tv-first` on a page's main action (Play on an item) is where focus lands when someone arrives with a
  remote. Without one, it's a primary button on screen, and otherwise the first control in the page, passing over text fields and the links in the headings of rows.
- If you handle arrows yourself, call `preventDefault`, and leave them alone at the ends, so a remote
  can move on. `Segmented`, `Tabs` and `Menu` show how.
- Text fields give up the arrows at the ends of the text. Selects open on OK. Sliders keep only the
  arrows along their length.

**How the next control is chosen** (`score` in `spatial.ts`, port this): only controls whose centre is
past the current one in that direction count. The distance along the direction counts in full; the
distance across it is penalised heavily, and more for left and right, so a far card on this row beats a
near one on the next. Lowest score wins. On entering a group, the last focused control in it wins if
it's also that way. From the link in a row's heading (`.hub-head` or `data-nav-head`), Down and Left go into that row first, as it may be
short or scrolled away and nowhere near straight ahead.

**Back**: `GoBack`, `BrowserBack`, keyCode 461 and 10009, and Backspace outside a text field, close the
top dialog or menu, or else go back a page. Going back returns focus to the control you left, per
history entry. A pointer has no such place to return to, so this is only for keyboard and remote use.
An app showing this page in a TV's web view reads what has its own Back from `<html data-own-back>`, a
selector the page sets, so the list has one home: `OWN_BACK` in `tv.ts`. An app can also set `<html data-app-back="/address">`: when
Back has nothing left to go back through, the page goes to that address, for the app to catch. It's how a controller's B leaves a web view.

`data-nav-skip` on a control: the arrows don't stop on it from outside. A programme grid has this on every cell but the
highlighted one, so it's one stop however many cells are drawn. Moving up or down the page never jumps over a `role=grid`.

From outside a scrolling list, only what can be seen in that list can be landed on. When the focused
control goes away, such as a button that removes its own row, the next arrow lands on what's closest
to where it was. A bar inside the player has to be added to `PLAYER_CONTROLS` in `tv.ts`, or a remote
can't reach it.

**Scope**: inside a dialog, menu or the full-screen player, arrows stay in it.

## Native apps and ten feet

The Apple TV and Android TV apps use this design. `python scripts/design-tokens.py` writes it out, and
`--check` (in CI) fails when the files are out of date. Don't edit them by hand.

| File | Holds |
|---|---|
| `themes/tokens.json` | type, space, shape, depth, layers, motion, control and card sizes, focus, poster tilt, the ten-foot rules |
| `themes/<id>/theme.json` | `colors`: every colour of that theme as `#rrggbb` or `#rrggbbaa`; `radius` |

Sizes are CSS pixels at normal scale: use them as dp or pt. Times are milliseconds, tracking is in em,
and radii are multiples of the theme's `radius`.

| Here | Apple TV (SwiftUI) | Android TV (Compose for TV) |
|---|---|---|
| Spacing, type and radius tokens | a constants file generated from `tokens.json` | the same |
| `theme.json` `colors` | `Color` assets | `ColorScheme` |
| `Button` | `Button` with a custom `ButtonStyle` | `Button` |
| `PosterCard` and moving posters | `.buttonStyle(.card)`: the platform's own parallax is the same idea, so use it | `Card` with `CardDefaults.scale` |
| `Shelf`, `CardRow` | `ScrollView(.horizontal)` with `.focusSection()` | `TvLazyRow` |
| `data-nav-group` | `.focusSection()` | `focusRestorer()` |
| `spatial.ts` | the platform's focus engine: don't reimplement it | the same |
| `Dialog`, `ask()` | `.alert`, `.sheet` | `AlertDialog` |
| `Menu` | `Menu` | `DropdownMenu` |
| Side rail | `TabView(.sidebarAdaptable)` | `NavigationDrawer` |
| Back | the Menu button, `.onExitCommand` | `BackHandler` |

**The ten-foot rules**, on any client:

- **Everything can be reached and used with a D-pad.** No action needs a pointer.
- **Focus is always obvious**: a 3px ring in `--fg` and a slightly larger poster. Exactly one thing has
  it, and moving it never scrolls it out of sight or clips it.
- **Large targets**: at least 48, spaced so focus moves predictably.
- **Readable from the sofa**: base text grows by a quarter, and no text below `--text-2xs` at that
  scale. Keep lines short.
- **Clear of the edge**: the side margin grows, because TVs can crop the picture.
- **Back always works**: every view has a way out with the Back button.

In the web app these apply through `html.using-controller` (a controller in use), `@media (pointer:
none)` (a device with no pointer) and `html.ten-foot` (TV mode, and couch mode).

**TV mode** (`html.tv-mode`: Appearance, the account menu, or `?tv=1`) goes further: the app as the Apple TV
and Android TV apps are, from WittyMediaClient's `docs/APP-DESIGN.md`, On a TV. Its own frame and screens are in
`web/src/tvmode/`, every size in TV units (`--tvu`, a point on a 1920×1080 TV), and with it off nothing of it
reaches the page. [plans/TV-MODE.md](plans/TV-MODE.md) is its contract. Two attributes are for it and work anywhere:
`data-nav-entry="selector"` on a group says where focus lands when it's entered from outside (the top bar: the
chosen tab), and `data-keep-focus` on a bar keeps the focus on it when one of its links changes the page.
`data-tv-options` marks a page's options, which a controller's Start opens in TV mode.

## Keeping it fast

- **No blur on anything that scrolls or covers video.** `backdrop-filter` and `filter: blur` repaint
  every frame. Use a solid or nearly solid colour.
- **No per-card effects.** One listener and one element can serve a library of thousands, as moving
  posters do. A filter on each of 2,000 cards is a slideshow.
- **Long lists use `content-visibility`**: `.grid` and `.album-grid` already do.
- **Lazy images**: `loading="lazy"` on everything in a list.
- **Lazy pages**: routes load when they're opened.
- **Lists that can run to thousands are windowed**: `pages/games/VirtualGames.tsx` draws only the rows near the screen and asks for only the pages of results those rows need. The page itself still scrolls.
- **No React state for scrolling or dragging.**
- **No polling where an event will do.**
- **No new UI libraries.** Every icon is a path in `icons.tsx`.
- Watch the CSS and JS sizes in `npm run build`.

## Keeping to it

CI runs these, and fails on a problem. Run them from the repository root.

| Command | Checks |
|---|---|
| `python scripts/design-check.py` | raw colours, one-off font sizes and radii, z-index numbers, `transition: all`, blur, unnamed icon buttons, unlabelled fields, images without `alt`, click handlers on a `div`, `confirm()` |
| `python scripts/theme-contrast.py` | every built-in theme against [Accessibility](#accessibility) |
| `python scripts/design-tokens.py --check` | the data files match the CSS |
| `node --experimental-strip-types --test web/src/ui/*.test.mjs web/src/shell/*.test.mjs web/src/pages/setup/*.test.mjs` | the navigation rule, the menu's arithmetic, the theme maker's colours, and which steps first-time setup shows |

Older code that hasn't been moved over yet is listed file by file in
`scripts/design-check-baseline.json`. Counts may go down, never up, and a file that isn't listed must be
clean. After clearing some, `python scripts/design-check.py --update` lowers the baseline, and
`--list <file>` shows a file's problems. If something really must stay, say why on the line with
`design-ok`:

```css
mask-image: linear-gradient(#000 80%, transparent); /* design-ok: a mask only reads how solid it is */
```

A drawing of a real object, which keeps its own colours in every theme, says so once around the
whole drawing. Its colours and corners are then its own, and the other rules still apply. The DVD
remote, the disc going into its tray and the CD player are drawn this way:

```css
/* design-ok-begin: a drawn remote control */
.remote { background: linear-gradient(180deg, #2a2d35, #15171c); }
/* design-ok-end */
```

Anything else in such a file still uses the tokens: the video player sets its own dark ones on
`.player`, so its panels and buttons use `var(--bg-2)`, `var(--border)` and `var(--on-image)` rather
than colours of their own.

## Adding a page

1. **Start from the page's job.** What is the one main action? That gets the `primary` button.
2. **Use the frame.** Routes in `App.tsx` already get the side menu, top bar and gutter.
3. **Title it** with `.toolbar` and an `h1` (settings pages: `PageHead`; a library's page: `LibraryHeader`, which every library page has). Exactly one `h1`.
4. **Build from [shared parts](#components).** A grid of things:

   ```tsx
   export default function GamesPage() {
     const [games, setGames] = useState<Game[] | null>(null)
     useEffect(() => { api.get<Game[]>('/games').then(setGames).catch(() => setGames([])) }, [])
     return (
       <>
         <div className="toolbar"><h1>Games</h1><span className="count">{games?.length ?? ''}</span></div>
         {!games ? <SkeletonGrid />
           : games.length === 0 ? <EmptyState icon={<IconGame />} title="No games yet" text="Add a folder of games in Settings." />
           : <div className="grid">{games.map((g) => <PosterCard key={g.id} to={`/game/${g.id}`} title={g.title} art={g.poster} />)}</div>}
       </>
     )
   }
   ```

5. **Add CSS only for what's really new**, in a file of its own imported by the page, using tokens
   only. Name classes with a short prefix (`gm-row`).
6. **Do the three page states** (loading, empty, error).
7. **Name things**: icon buttons, fields, images.
8. **Keep logic out of components.** Fetching and shaping data goes in `api.ts` or the server, so the
   native apps get the same behaviour.
9. **Check it** at 390, 820, 1440 and 1920 wide, with the keyboard alone, and in a dark and a light theme.
   `npm run test:keys` in `web` walks every page with Tab and with the arrows on a test server, and
   names any control with no name, focus that doesn't show or can't get somewhere, and pages with no
   heading. `WMS_PAGES=/your/page` walks just yours. Its first lines say what it needs.

## Adding a step to first-time setup

A step belongs to whatever needs it. `web/src/pages/setup/plan.ts` lists them, and each is a file beside it.

1. **Add a line to `STEPS`:** its `id`, its `label`, what it `needs` (`'server'`, or the modules any one of which is enough), `later` (where in Settings the same choice is: a test checks that page exists and is called that), and `when` if it depends on anything else. A module's step is skipped when the module isn't there, so nothing else has to check.
2. **Write the step** from the parts in `setup/parts.tsx`: `StepHead`, `LaterNote`, `StepActions`, `ChoiceCards`, `TickRow`. These already work with a remote, a keyboard, touch and a screen reader. A checkbox for what's saved with the step, a switch for what takes effect at once.
3. **Read what's saved when it opens, and save when the choice is made.** Someone can come back to any step. Nothing goes online until they've agreed, and leaving something off is always the easy button.
4. **Add it to `SetupWizard.tsx`**, and to `describeOutcome` if the last page should say what was chosen.
5. **Run** `npm run test:setup` in `web`, which walks the whole of it with arrows and OK alone. Its first lines say what it needs.

In `ChoiceCards`, the arrows move and OK chooses. They don't choose as they go, as a remote has to pass through the cards to reach what's below them.

---

<sub>WittyMediaServer documentation</sub>
