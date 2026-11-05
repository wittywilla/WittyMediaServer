# Design: matching disc icons for WittyDVD and WittyBluray

> **Status on 2026-11-04**
>
> **Planned:** [the idea](https://github.com/wittywilla/WittyMediaServer/discussions/58). The owner's questions below are still open.

For WittyDVD 0.4.0 and WittyBluray 0.4.0. The owner:

> Fix the images to "match". I want the module images to be both disk images, but one with a B in the center, and one with a D in the center. Do not accidentally make a copyrighted logo or anything.

Not drawn yet.

---

## Where the icons are

`D:\projects\WittyModuleCatalog\icons\wittydvd.png` and `wittybluray.png`, alongside every other module's icon. They appear in the module store, each module's page in **Settings → Modules**, and the module READMEs. The catalogue is signed, so a new icon goes out as a catalogue change.

## The design

**One drawing, two letters.** Both icons are the same disc, at the same size, in the same colours, with the same light. Only the letter in the centre differs: **D** for WittyDVD, **B** for WittyBluray.

| Part | Spec |
|---|---|
| Canvas | Square, transparent background, drawn at 512 × 512, with the disc filling about 88% of it so it lines up with the other module icons |
| Disc | A flat circle with a soft, generic rainbow sheen (a conic gradient, like light on any optical disc), a thin outer rim, and two or three faint concentric data rings |
| Hub | A clear inner ring and a centre hole, drawn as a ring around the letter |
| Letter | A single capital, centred in the hub's label area, in a bold geometric sans serif. Use a font with an open licence (for example Inter, SIL OFL) converted to outlines, or draw the letter from simple shapes, so no font file ships |
| Colours | From the project's design tokens, so both icons sit with the rest of the store in light and dark themes. The letter's colour is the same in both icons. Check contrast of the letter against the hub in both themes |

**Should the two have different accent colours?** The owner asked for them to "match", so this plan keeps them identical except for the letter. One safe variant to show the owner as an option: the same disc, with the sheen tinted slightly warm for D and slightly cool for B, for telling them apart at a glance in a long list.

## Staying clear of real logos

The formats' own logos are trademarks, each with a distinctive wordmark and shape. To stay clear of them:

- **One letter only.** Never "DVD", never "BD", never two letters together.
- **No oval or swoosh under or around the letter.** Keep the disc a plain circle seen face on.
- **No wordmark styling:** no italics, stretched letters, cut-outs or overlapping letters that echo a format's logo.
- **Not blue text on a disc for B.** Keep the letter in the shared neutral colour.
- **Our own drawing,** from simple shapes, with no stock art or traced images.
- **No text other than the letter.**

Note the result in the private legal review (outside the repository), as with other logos and names.

## A set, if the owner wants it

The same disc works for the other disc modules. Not asked for, so offered, not assumed:

- **WittyCD**: the same disc with **C**.
- **WittyDisc** (the library both are built on): the same disc with no letter, or a small stack of two discs.

## Deliverables

1. `icons/src/disc.svg`: the master, with the letter as a named group.
2. `icons/src/wittydvd.svg` and `wittybluray.svg`, made from it by a small script, so the two can't drift apart.
3. PNG exports at the sizes the catalogue uses (**to verify** in `icons/README.md`), from a command-line SVG renderer with a permissive licence (for example resvg, MPL-2.0 or Apache-2.0: check), run by a script in WittyModuleCatalog.
4. Alt text: "WittyDVD: a disc with the letter D" and "WittyBluray: a disc with the letter B".
5. Updated READMEs in WittyDVD and WittyBluray if they show the icon.

## Checks

- Side by side at 32, 64, 128 and 512 pixels, light and dark, in the store grid and on a module page.
- At 32 pixels the letter must still read. If it doesn't, thicken it for small sizes (a separate small master).
- A look by someone who didn't draw them: "does either of these remind you of a logo you've seen?"

---

<sub>Future plans, written 2026-11-02.</sub>
