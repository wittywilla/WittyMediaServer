# Building blocks

Everything WittyInk exports, for building your own reader or a new kind of document viewer.
Import from `@wittywilla/wittyink`.

## Host

| Export | What it is |
|---|---|
| `HostContext`, `useHost()` | Provide and read the [Reader host](Reader-host). |
| `ReaderHost`, `Ann`, `BookMeta`, `EpubInfo`, `SearchHit`, `Definition` | The types. |

## Notes

| Export | What it does |
|---|---|
| `useAnnotations()` | The book's notes from the host, plus `add`, `create`, `update`, `remove`, `undo`, `redo`, `canUndo` and `canRedo`. Deletions show at once and come back if saving fails. `add` records an undo step; `create` doesn't. |
| `useColorLabels()` | `[labels, setLabel]`: colour meanings, kept on the device and the same in every book. |
| `NotesPanel` | The notes list with kind, colour and text filters, colour naming and Export. You provide `place(chapter)` (for example "Page 12"), `open(ann)` and `remove(ann)`. |
| `SelectionBar` | The toolbar over selected text. Leave out `onDefine` or `onRead` to hide those buttons. |
| `HighlightEditor` | Colour, note, citation and removal for one highlight. |
| `Palette` | The highlight colour picker. |
| `Define` | The definitions pop-up (uses `host.define`). |
| `cite(book, quote, place)` | The citation text. |
| `exportNotes(book, anns, labels, heading)` | Downloads the notes as Markdown. |
| `NAMED`, `QUICK`, `hex(c)`, `colorName(c)`, `fill(c, alpha)` | Highlight colours: the four named ones, a colour as hex, `#ffd60a` mapped back to `yellow`, and a see-through fill. |

## Handwriting

| Export | What it does |
|---|---|
| `InkBar` | The ink toolbar (tools, colours, thickness, undo, pen settings). Takes the settings and a setter, plus undo and redo. The `extra` slot adds your own buttons. |
| `loadInk()`, `saveInk(p)`, `INK_DEF` | Tool settings, remembered on the device. |
| `InkStroke` | Draws one stroke as SVG in its own units; you position and scale it. |
| `InkDefs` | The pencil grain filter. Set `freq` to suit your units: 9 for ems, about 0.7 for PDF points. |
| `pointPressure(event, tool, usePressure)` | The pressure to store for a point (pens only; tilt widens the pencil). |
| `outline(pts, width, varying)` | A pressure-sensitive stroke as a filled SVG path. |
| `centerline(pts)` | A smooth centre line (used for the highlighter). |
| `hits(pts, width, x, y, tolerance)` | Whether a point touches a stroke (for the eraser). |
| `INK_COLORS`, `MARKER_COLORS` | The 36 ink and 18 highlighter colours. |

### Drawing loop

The readers draw the stroke being written straight into one SVG path (no React re-render per
point), using the pen's coalesced events for full detail:

```ts
onPointerMove(e) {
  for (const c of e.getCoalescedEvents?.() ?? [e]) pts.push(c.clientX, c.clientY, pointPressure(c, tool, prefs.pressure))
  livePath.setAttribute('d', tool === 'marker' ? centerline(pts) : outline(pts, prefs.size[tool], true))
}
```

When the pen lifts, the points are converted to the document's units and saved as an `ink`
annotation.

## Icons

`IconBack`, `IconBook`, `IconSearch`, `IconSettings`: inline SVG, so no icon set is needed.
