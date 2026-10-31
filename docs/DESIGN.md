# The design system

How the WittyMediaServer interface is put together: the rules behind it, the tokens that carry
them, the shared parts, and how to add a page that fits. If you only read one section, read
[Adding a page](#adding-a-page).

All of it lives in `web/src/styles.css` (tokens first, then shared parts). Page-specific CSS is in
`shell.css`, `audio.css`, `player.css`, `oobe.css` and `pages/settings/settings.css`.

- [Principles](#principles)
- [Tokens](#tokens)
- [Shared parts](#shared-parts)
- [States](#states)
- [Motion](#motion)
- [Keeping it fast](#keeping-it-fast)
- [Accessibility](#accessibility)
- [Screens and input](#screens-and-input)
- [Adding a page](#adding-a-page)
- [Native apps and ten feet](#native-apps-and-ten-feet)
- [What the audit found](#what-the-audit-found)

## Principles

1. **The artwork is the loudest thing.** Everything around a poster stays quiet: labels sit on dark
   plates, surfaces are close in tone, and only one thing per view is in the accent colour.
2. **The accent means "do this next".** Primary buttons, progress and focus use it. Something merely
   *selected* (a chip, a segment, a top-bar link) is inverted instead, in every theme.
3. **One of everything.** One type scale, one spacing grid, one button, one dialog. A new look for an
   old job needs a very good reason.
4. **Low-end devices first.** If an effect costs a frame on a cheap TV, it doesn't ship.
5. **Works from a theme of four colours.** Themes set a few colours; hover, selected, tinted and
   outline colours are worked out from them, so a theme can't forget a state.

## Tokens

Use a token, not a number. If no token fits, that's usually the design saying "don't".

### Type

Sizes are in `rem`, so they follow the reader's browser setting and grow for controllers.

| Token | Size | For |
|---|---|---|
| `--text-2xs` | 11 | badges, uppercase labels |
| `--text-xs` | 12 | captions, card subtitles, hints |
| `--text-sm` | 13 | secondary text, small buttons, chips |
| `--text-md` | 14 | controls, card titles, dense text |
| `--text-base` | 15 | body |
| `--text-lg` | 17 | panel headings, sub-headings |
| `--text-xl` | 20 | section headings, dialog titles |
| `--text-2xl` to `--text-5xl` | 24, 30, 38, 48 | large artwork text |
| `--page-title` | 24 to 30 | the `h1` of a normal page |
| `--display` | 30 to 48 | the title over a backdrop |

Also `--leading-tight`, `--leading-snug`, `--leading-body`; `--tracking-display`,
`--tracking-heading`, `--tracking-label` (uppercase labels); `--weight-heading`; `--font-mono`.

The font is the system's own (`--font`): no download, sharp on every screen, and it has every
alphabet a media title might use. Inter is used if it is installed.

### Space

A 4px grid: `--sp-1` 4, `--sp-2` 8, `--sp-3` 12, `--sp-4` 16, `--sp-5` 20, `--sp-6` 24, `--sp-8` 32,
`--sp-10` 40, `--sp-12` 48, `--sp-16` 64.

| Token | For |
|---|---|
| `--gutter` | the page's side margin (16 to 40, by width) |
| `--section-gap` | between sections of a page |
| `--topbar-h` | the top bar's height, for anything that sticks under it |

### Shape

Radii follow the theme's `--radius` (10 by default), so `--radius: 0` makes the whole interface
square: `--r-xs` 4 (badges), `--r-sm` 6, `--r-md` 10 (buttons, fields, posters), `--r-lg` 14 (panels),
`--r-xl` 20 (dialogs), `--r-full` (pills).

### Colour

Set by themes (see [THEMES.md](THEMES.md)): `--bg`, `--bg-2`, `--bg-3`, `--bg-4`, `--fg`, `--fg-2`,
`--fg-3`, `--accent`, `--accent-2`, `--accent-fg`, `--accent-text`, `--danger`, `--ok`, `--warn`,
`--border`, `--control-line`.

| Surface | Use |
|---|---|
| `--bg` | the page |
| `--bg-2` | panels, dialogs, sidebar: one step up |
| `--bg-3` | things inside those: fields, buttons, list rows |
| `--bg-4` | hover on `--bg-3` |

| Text | Use |
|---|---|
| `--fg` | the content |
| `--fg-2` | supporting sentences |
| `--fg-3` | captions and labels (still 4.5:1) |
| `--accent-text` | links and the accent as words; never `--accent`, which may be too dark |

Worked out for you: `--hover` and `--pressed` (work on any surface), `--accent-soft` (a selected
row), `--accent-line`, `--ok-soft`, `--warn-soft`, `--danger-soft`, `--border-strong`, `--focus`,
`--on-image` and `--scrim` (text on artwork and the plate behind it).

The player is always dark, whatever the theme: `.player` sets its own colours.

### Depth

Three steps: `--e-1` resting artwork, `--e-2` a lifted card, `--e-3` anything over the page (menus,
dialogs). Panels on the page have a hairline, not a shadow.

### Size

`--control-h`, `--control-h-sm`, `--control-h-lg` are the heights of every control (40, 32, 46).
They grow by themselves on touch screens (44, 36, 50). `--card-w` and `--card-wide-w` are the width
of a poster and of a wide episode card; they change with the screen.

## Shared parts

| Part | Class or component | Notes |
|---|---|---|
| Button | `.btn` + `.primary`, `.ghost`, `.danger`; `.sm`, `.big`; `.square`, `.icon` | One `.primary` per view |
| Icon button on video | `.ibtn` | 44px target |
| Field | `.input`, `.select`, `textarea.input` | Outline is 3:1 on every surface |
| Form | `.form` (`.full` for no width limit), `.row2`, `.field-row`, `.hint` | |
| Slider | `.range`, set `--pct` in `style` | |
| Switch | `.set-switch` (settings), `.switch` (player) | |
| Choice of a few | `.seg` with `button.active` | |
| Filter | `.chips`, `.chip`, `.chip.active` | `.chip.tag` can be removed |
| Tabs | `.tabs` | |
| Rating and small labels | `.pill` | |
| Line of facts | `.meta` | Dots between items are automatic and never start a line |
| Poster | `ItemCard`, `ShowCard`; `.card` + `.wide`, `.square` | |
| Label on artwork | `.badge`; `SeenBadge` | |
| Art for something with no picture | `Placeholder` with `title`, `label` | A colour from the title. The title is printed under the card, so never on the art; `label` is one short word, such as a console |
| Look through a library again | `ScanButton` | For administrators; shows nothing to anyone else |
| Grid and row of cards | `.grid`; `CardRow` | The row has arrows for mouse users |
| Panel | `.panel` with `h2` and `.panel-note`, `.panel-actions`; in Settings `Section`, `Row` | |
| Dialog | `.modal-backdrop` > `.modal` with `h2`, `.actions` | A bottom sheet on phones; `a11y.tsx` handles the keyboard |
| Empty page | `EmptyState` with `icon`, `title`, `text`, `action` | |
| Loading | `SkeletonGrid` for grids; `Spinner` for the rest | Both wait 0.2s, so fast loads show nothing |
| Message | `.error`, `.warn-box`; `toast()` | |
| Back | `BackLink` | |
| Icons | `web/src/icons.tsx` | 24 grid, filled, `currentColor`. Never a text character as an icon |

Class names are part of the theme contract (see the selector reference in
[THEMES.md](THEMES.md)): rename one only if you also update that list.

## States

Every interactive thing needs all of these, and they come free with the shared parts:

- **Hover** darkens or lightens the surface with `--bg-4` or `--hover`. Only in
  `@media (hover: hover)` if it moves or reveals anything, so nothing sticks after a tap.
- **Pressed**: `transform: scale(.975)` on buttons.
- **Focus**: the ring below; never remove it without replacing it.
- **Disabled**: `opacity: .45` and no hover.
- **Selected**: inverted (`--fg` on `--bg`) for small controls, `--accent-soft` for rows and tiles.

A page has three more: **loading** (shapes, not a blank), **empty** (says why, and what to do) and
**error** (says what happened, with a way forward).

## Motion

| Token | Time | For |
|---|---|---|
| `--dur-1` | 90ms | pressing |
| `--dur-2` | 150ms | hover and focus |
| `--dur-3` | 240ms | something appears |
| `--dur-4` | 420ms | a backdrop changes |

`--ease-out` for things arriving, `--ease-standard` for things that change in place. Things appear;
they don't bounce or fly. Animate `transform` and `opacity` only (colours on hover are fine).

`prefers-reduced-motion` turns transitions and animations off globally. The featured banner also
stops turning by itself; it can be paused anyway, and stops under the pointer or keyboard.

## Keeping it fast

- **No blur on anything that scrolls or covers video.** `backdrop-filter` and `filter: blur` repaint
  every frame. Use a solid or nearly solid colour.
- **No per-card effects.** A filter on one card is nothing; on 2,000 it is a slideshow. Generated
  artwork is a frame on a gradient, not a blurred copy of itself.
- **Long lists use `content-visibility`**: `.grid` and `.album-grid` already do. Copy it for anything
  that can run to hundreds of rows.
- **Lazy images**: `loading="lazy"` on everything in a list.
- **No React state for scrolling or dragging.** `CardRow` writes to the DOM directly.
- **No new UI libraries.** Every icon is a path in `icons.tsx`.
- Watch the build's CSS and JS sizes in `npm run build`.

The bar over a backdrop (it turns solid as you scroll) is the one scroll-linked effect. It animates
opacity on the compositor, and browsers without it get a plain solid bar.

## Accessibility

The bar is WCAG 2.2 AA.

- **Contrast**: `python scripts/theme-contrast.py` checks every built-in theme: text 4.5:1 on all
  surfaces (`--fg`, `--fg-2`, `--fg-3`, `--accent-text`, `--danger`, `--ok`, `--warn`), `--accent-fg` on
  `--accent`, and `--control-line` 3:1. Run it after changing a theme, use `--fix` to correct one, and
  bump the theme's version in `theme.json`.
- **Focus**: `:focus-visible` gets a ring in `--focus`, 2px wide, 3px at ten feet. Cards ring the
  poster, not the text. `scroll-padding` keeps a focused thing clear of the bars.
- **Targets** are at least 24px (WCAG 2.5.8); a small mark gets an invisible bigger box around it
  (see `.hero .dots button` and `.tip`).
- **Colour is never alone**: links in sentences are underlined, selected things change shape or fill.
- **Keyboard and screen readers**: `web/src/a11y.tsx` gives every `.modal` focus handling, Escape and
  a name from its first heading, and announces page changes through the live region. Name icon
  buttons with `aria-label`.
- `prefers-contrast: more` and `forced-colors` are handled in `styles.css`.

## Screens and input

| Width | What changes |
|---|---|
| up to 520 | three posters across; dialogs are bottom sheets (up to 600) |
| up to 700 | bottom tab bar, compact audio bar |
| up to 900 | the sidebar is a drawer |
| 1100, 1700 and up | bigger posters |
| 2200 and up | everything zoomed 1.25 (4K TVs) |

Touch makes every control taller automatically. [Ten feet](#native-apps-and-ten-feet) has its own rules.

Check a new page at 390, 820, 1440 and 1920 wide, with the keyboard alone, and in one dark and one
light theme.

## Adding a page

1. **Start from the page's job.** What is the one main action? That gets the `.primary` button.
2. **Use the frame.** Routes in `App.tsx` already get the sidebar, top bar and gutter.
3. **Title it** with `.toolbar` and an `h1` (settings pages: `PageHead`). Keep exactly one `h1`: the
   announcer reads it aloud on every page change.
4. **Build from [shared parts](#shared-parts).** A grid of things:

   ```tsx
   export default function GamesPage() {
     const [games, setGames] = useState<Game[] | null>(null)
     useEffect(() => { api.get<Game[]>('/games').then(setGames).catch(() => setGames([])) }, [])
     return (
       <>
         <div className="toolbar"><h1>Games</h1><span className="count">{games?.length ?? ''}</span></div>
         {!games ? <SkeletonGrid />
           : games.length === 0 ? <EmptyState icon={<IconGame />} title="No games yet" text="Add a folder of games in Settings." />
           : <div className="grid">{games.map((g) => <GameCard key={g.id} game={g} />)}</div>}
       </>
     )
   }
   ```

   A card is `.card` > `.poster` + `.title` + `.sub`. A backdrop page is `.detail-hero` + `.detail`
   (the top bar runs over it by itself). A settings page is `PageHead` + `Section` + `Row`.
5. **Add CSS only for what is really new**, in a file of its own imported by the page, using tokens
   only: no raw `px` font sizes, colours or radii. Name classes with a short prefix (`gm-row`).
6. **Do the three page states** (loading, empty, error) and the five control states.
7. **Name things**: `aria-label` on icon buttons, a `.modal` with an `h2` for any dialog.
8. **Keep logic out of components.** Fetching and shaping data goes in `api.ts` or the server, so
   the native apps get the same behaviour; components only lay it out.
9. **Check** it at four widths, with the keyboard, in a dark and a light theme. `npm run build` must
   stay green.

## Native apps and ten feet

The Apple TV and Android TV apps use the same design. The CSS is the single source, and
`python scripts/design-tokens.py` writes it out as plain data for them:

| File | Holds |
|---|---|
| `themes/tokens.json` | type scale, spacing, shape, depth, motion, control and card sizes, focus, the ten-foot rules |
| `themes/<id>/theme.json` | `colors`: every colour of that theme, already worked out, as `#rrggbb` or `#rrggbbaa`; `radius` |

Sizes are CSS pixels at normal scale: use them as dp or pt. Times are milliseconds, tracking is in em,
and radii are multiples of the theme's `radius`. Run the script after changing `styles.css` or a
theme; `--check` (in CI) fails when the files are out of date. Don't edit them by hand.

**The ten-foot rules**, which apply to every screen, on any client:

- **Everything can be reached and used with a D-pad.** Up, down, left, right and OK: no action needs
  a pointer. Build with real `a`, `button` and `input` elements, so `tv.ts` can move between them; a
  click handler on a `div` can't be reached.
- **Nothing is hover-only.** Hover may decorate, but the thing it reveals must also appear on focus
  (`:focus-visible` beside `:hover`), or not be needed. Row arrows are the mouse's extra way along a
  row; a remote moves card by card.
- **Focus is always obvious**: a 3px ring and a slightly larger poster, in a colour that reads on
  every surface. Exactly one thing has it. Moving it must never scroll it out of sight or clip it.
- **Large targets**: at least 48 at ten feet, and spaced so that focus moves predictably.
- **Readable from the sofa**: the base text grows by a quarter, and no text below `--text-2xs` at
  that scale. Keep lines short; long passages are for the small screen.
- **Clear of the edge**: the side margin grows, because TVs can crop the picture's edge.
- **Back always works**: every view has a way out with the Back button, and dialogs close with it.

In the web app these apply through `html.using-controller` (a controller in use, set in `tv.ts`) and
`@media (pointer: none)` (a device with no pointer, like a TV browser). To try it, connect a controller, or
add the class in the browser's developer tools.

## What the audit found

The state of the interface before this system, kept here as a record of what to avoid going back to:

- **Type**: 30-odd font sizes, including 9, 9.5, 10, 10.5, 11, 11.5, 12, 12.5, 13, 13.5, 14, 14.5, in `px`.
- **Spacing**: no grid; hero text 48px from the edge above content 28px from it.
- **Colour**: the accent on everything, selected or not; hard-coded greens, reds and oranges beside
  the theme's own; hairlines darker than the page, so invisible on dark themes and OLED.
- **Shape and depth**: 20 radii, a dozen one-off shadows, `--radius` declared but unused.
- **Icons**: text characters as icons (play, tick, arrows, plus, grip), which look different on every
  platform; the same icon for music and podcasts, books and audiobooks; a clipped search icon.
- **Motion**: no shared timing; `transition: all`; hover effects that stuck after a tap.
- **States**: a spinner or nothing while loading, plain text for errors, empty states with no way forward.
- **Performance**: `backdrop-filter` on the top bar, tab bar, audio bar, reader bars and panels over
  video; a blur filter on every generated poster.
- **Accessibility**: an auto-turning banner with no pause (2.2.2); 8px dots and 22px buttons as
  targets (2.5.8); field outlines and switches under 3:1 (1.4.11); white letters on yellow avatars;
  sticky bars able to cover the focused thing (2.4.11).
- **Content**: rows repeating one album per song, books showing "0 min", titles twice on generated posters.
- **Duplicates**: rules for `.chip`, `.crumbs` and `.modal.wide` in two files that overrode each other,
  which gave genre filters the wrong padding.
