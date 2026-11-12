
## Design 2 (W1)

Built: design tokens (type roles, space steps, elevation, focus by input, input and nav tables, `tv` units, breakpoints, density, motion) in `web/src/styles.css` and `scripts/design-tokens.py` -> `themes/tokens.json`; TV pass (`tv` block) on the eight themes; the theme response's `tv`, `elevation`, `density`; SDUI catalogue revision 3 (`Dock`, `KeyLegend` required; Shelf `peek` and `seeAllCard`; Hero `peek`; `layout.peek`, `layout.dock`, text role `display`; TV safe area 90, TV text sizes 57/48/38; client command `control.open`); conformance fixtures (85) and schema regenerated.

Open / not verified:
- Full `go test ./...` was not run under a heavy slot (both were busy). Run: `go test ./internal/server ./internal/themes/... ./pkg/...` all pass; `go vet ./...` clean.
- The web renderer's conformance test (`web/src/sdui/model.test.mjs`) and the Apple and Android renderers must learn Dock, KeyLegend, `peek`, `seeAllCard`, `layout.dock`; until then they fail on the new fixtures (W2 to W4).
- `control.open` jump: the client pages the grid until a title with the letter arrives (at most 20 pages); there is no server-side letter index.
- Focus ring vs accent fills: the ring is checked against surfaces and neutral raised fills; an accent-filled control is separated from its ring by the focus gap, not by 3:1 colour contrast (gold on gold cannot pass).
- Existing component fallbacks (Hero, SeasonList, ...) are kept as the model's general mechanism; only the new components have none.
