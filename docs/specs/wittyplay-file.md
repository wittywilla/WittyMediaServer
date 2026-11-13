<!-- SPDX-License-Identifier: CC0-1.0 -->
# The .wittyplay file format, version 1

A `.wittyplay` file says how a games front end should picture games: the box art fitted into its tile, the box art
filling its tile, or the game drawn inside a cartridge or disc case like the ones its console used. It can say this for
every game, for each console, and for single games. It is small, plain JSON, and meant to be written by people as
easily as by programs.

WittyMediaServer reads and writes it (WittyPlay Settings › Game pictures: Export and Import), and reads one placed
beside a game file. Anyone may read or write it in any program.

- **File extension:** `.wittyplay`
- **Media type:** `application/vnd.wittyplay+json` (a JSON document; `application/json` readers can read it)
- **Encoding:** UTF-8, optionally with a byte-order mark. At most 64 KiB.
- **JSON Schema:** [wittyplay.schema.json](wittyplay.schema.json) (JSON Schema 2020-12)

## Licence

This specification and its JSON Schema are dedicated to the public domain under
[CC0 1.0 Universal](https://creativecommons.org/publicdomain/zero/1.0/). You may implement, copy, change and
redistribute them for any purpose, without asking and without credit. (WittyMediaServer's own code that reads them is
under its own licence; the format is not.)

## Purpose

- **A portable presentation profile.** Someone exports how they like their games shown and imports it on another
  server, or shares it with a friend.
- **A per-game or per-collection note.** A collection of games can carry a file saying "draw these in a cartridge,
  this one in a plain box".
- **A sidecar beside a game file.** A file named after a game file, in the same folder, says how that one game is
  pictured. For `Made Up Quest (USA).sfc` it is `Made Up Quest (USA).wittyplay` (or, also accepted,
  `Made Up Quest (USA).sfc.wittyplay`). A program only ever reads these: games folders are often read-only, and a
  program must never write one there.

A `.wittyplay` file never holds pictures, logos or other media, and never anything that identifies a person.

## Example

```json
{
  "wittyplay": 1,
  "name": "Living room",
  "generator": "WittyMediaServer 0.8.0",
  "default": { "picture": "case", "case": "auto", "label": "boxart" },
  "consoles": {
    "ps1": { "picture": "fitted" },
    "snes": { "case": "cart-super" }
  },
  "games": [
    { "match": { "file": "Made Up Quest (USA).sfc" }, "case": "generic-box", "label": "title" },
    { "match": { "title": "Another Made Up Game", "console": "ps1" }, "picture": "filled" }
  ]
}
```

## Versioning

The top-level `"wittyplay"` field is the format's version, a whole number, and is required. This document describes
version **1**.

- New optional fields, and new values of `case`, may be added without changing the version. A reader ignores what it
  does not know (see below).
- The version goes up only for a change an older reader would get wrong. A reader refuses a file whose version is
  higher than the highest it knows, and says so ("made for a newer version"), rather than guess.
- A writer writes the lowest version that holds what it writes.

## Fields

### Top level

| Field | Type | Required | Meaning |
|---|---|---|---|
| `wittyplay` | integer | yes | The format's version: `1`. Writers put it first so a person opening the file sees what it is. |
| `name` | string, ≤ 200 characters | no | A name for people: "Living room", "Arcade night". |
| `description` | string, ≤ 1000 characters | no | Anything else for people. |
| `generator` | string, ≤ 200 characters | no | The program that wrote it. For people; never acted on. |
| `default` | look | no | The look for every game the file covers. In a sidecar, the look of the game beside it. |
| `consoles` | object of looks, ≤ 200 | no | A look per console, keyed by console id. |
| `games` | array of games, ≤ 2000 | no | A look per game. |

### Look

A look says how pictures are shown at one level. Every field is optional: one left out is decided by the level above
(see *Precedence*).

| Field | Values | Meaning |
|---|---|---|
| `picture` | `fitted`, `filled`, `case` | `fitted`: the whole box art, nothing cut off, over a blurred and dimmed copy of itself so the tile has no empty bars. `filled`: the box art fills the tile and is cropped to it. `case`: the game is drawn in a cartridge, disc case or similar, with its art on the label. |
| `case` | a case style id, or `auto` | Which case to draw when `picture` is `case`. `auto` (or leaving it out) lets the console decide. |
| `label` | `boxart`, `logo`, `title` | What goes on the case's label: the box art, the game's own logo picture (the box art when it has none), or the game's title set in type. |

### Game

A game is a look (the fields above) with a `match`:

| Field | Type | Meaning |
|---|---|---|
| `match.item` | integer ≥ 1 | One server's own id for the game. Meaningful only on the server that wrote it; another program ignores entries it cannot match. |
| `match.file` | string, a file name without any folder, ≤ 200 characters | The game file's name, compared without regard to case. |
| `match.title` | string, ≤ 200 characters | The game's title, compared without regard to case or surrounding spaces. |
| `match.console` | console id | The console the game is on. |

A match gives at least one of `item`, `file` or `title`; every field it gives must agree for the entry to apply. When
several entries match one game, a later entry's fields win over an earlier one's.

### Console ids

Lower-case letters, digits, `-` and `_`, at most 32 characters (`^[a-z0-9][a-z0-9_-]{0,31}$`). The ids WittyMediaServer
uses include `nes`, `snes`, `n64`, `gc`, `wii`, `wiiu`, `switch`, `gb`, `gba`, `nds`, `3ds`, `vb`, `md`, `sms`, `gg`,
`segacd`, `32x`, `saturn`, `dc`, `ps1`, `ps2`, `ps3`, `psp`, `xbox`, `pce`, `a2600`, `a5200`, `a7800`, `lynx`, `jaguar`,
`ws`, `ngp`, `neogeo` and `arcade`. Another program may map its own names onto these.

### Case styles

Original drawings that evoke how each kind of console delivered its games. None reproduces a product, a brand name or
a logo. A program may draw them however it likes; these are what the ids mean.

| Id | Drawing | Usual consoles |
|---|---|---|
| `cart-nes` | An 8-bit front-loading grey cartridge: ridged sides and a label window | `nes` |
| `cart-atari` | A short first-generation cartridge with a wide label | `a2600`, `a5200`, `a7800` |
| `cart-super` | A 16-bit cartridge, rounded at the top, with grip ridges | `snes` |
| `cart-64` | A tall 64-bit cartridge with a ridged, sloping top | `n64`, `jaguar` |
| `cart-mega` | A black 16-bit cartridge with a slanted label top | `md`, `32x`, `sms`, `neogeo` |
| `cart-pocket` | A small handheld cartridge with a notched corner | `gb`, `gg`, `ngp`, `ws`, `vb` |
| `cart-advance` | A wide, short handheld cartridge | `gba` |
| `card-slim` | A thin game card | `nds`, `3ds`, `switch`, `switch2`, `pce`, `lynx` |
| `case-jewel` | A square CD jewel case with hinge and tray | `ps1`, `saturn`, `segacd`, `dc` |
| `case-keep` | A tall DVD keep case | `ps2`, `xbox`, `wii`, `wiiu`, `ps3` |
| `case-mini` | A small, squarer disc case | `gc` |
| `case-umd` | A small disc in a shell with a window | `psp` |
| `disk-floppy` | A 3.5-inch floppy disk with shutter and label | home computers (`amiga`, `dos`, `atarist`) |
| `tape-cassette` | An audio cassette with two reels | home computers (`c64`, `zx`, `cpc`, `msx`) |
| `arcade-marquee` | An arcade cabinet's lit marquee | `arcade` |
| `generic-box` | A plain box, for anything else | any |

A reader that meets a case style id it does not know draws the console's own (as for `auto`), and keeps the id.

## Precedence

For one game, each field of the look (`picture`, `case`, `label`) is taken from the first of these that sets it:

1. the game's entry (or entries) in `games`;
2. its console's entry in `consoles`;
3. `default`;
4. the program's own default.

A program that combines several files (a person's own choices, a server's choices, a sidecar) decides their order.
WittyMediaServer's is: the person's choice for the game, the server's choice for the game (an administrator's, then a
sidecar's), the person's choice for the console, the server's for the console, the person's default, the server's
default, and finally `fitted` with the console's case and the box art.

## Unknown fields

A reader ignores fields it does not know, at every level (top level, looks, matches and games), and does not refuse the
file for them. A program that reads a file and writes it again keeps every field it did not know, unchanged, so
nothing is lost passing through an older program. Names beginning with `x-` are never given a meaning by this
specification and are free for a program's own use.

## Validation

A reader refuses a file that:

- is larger than 64 KiB, is not UTF-8, or is not a single JSON object;
- has no `wittyplay`, or one that is not a whole number of at least 1 (a higher number than it knows is refused as
  *newer*);
- gives a known field the wrong type, or `picture` or `label` a value not listed here;
- gives `case` a value that is not a case style id (`^[a-z0-9][a-z0-9-]{0,39}$`);
- gives a console id that does not fit its pattern;
- has a game with no usable `match`, a `file` containing `/` or `\`, or an `item` below 1;
- has more than 200 consoles or 2000 games, or text longer than its limit.

The JSON Schema says the same, apart from the size limit and refusing newer versions, which a reader checks itself.

## Security and privacy

A `.wittyplay` file is data, never instructions: it can name nothing to fetch, run or write. Readers enforce the size
limit before parsing. Match file names are names, not paths, so a file can never point outside its folder.
