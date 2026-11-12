# SDUI conformance kit

Three renderers draw WittyMediaServer's pages: React on the web, SwiftUI on
Apple devices, Compose on Android. This kit is how each proves it draws them
the same way, without comparing pixels.

`fixtures/` holds one JSON file per case (120 today; `fixtures/index.json`
lists them). Every file has the same shape:

```
{
  "kit": 1, "schemaVersion": 1, "catalogRevision": 4,
  "id": "home-tv-hd",
  "title": "The built-in Home on an Apple TV HD",
  "checks": ["…what this case is there to pin down…"],
  "input": {
    "page":    { …the authored page… },
    "request": { …who asks, on what device, knowing which native views… },
    "data":    { …canned answers for the page's data sources… }
  },
  "expected": {
    "document": { …the tree the server sends… },
    "layout":   { …what a renderer makes of that tree… },
    "more":     [ { "section", "cursor", "season", "answer": { …a section's next page… } } ],
    "notFound": false
  }
}
```

The files are stored compact, one line each, to keep the repository small.
To read one:

```sh
jq . pkg/sdui/conformance/fixtures/home-tv-hd.json | less
python3 -m json.tool pkg/sdui/conformance/fixtures/home-tv-hd.json   # without jq
jq '.expected.layout.visible[] | select(.type == "Shelf")' pkg/sdui/conformance/fixtures/home-tv-hd.json
```

and to see what a contract change did to one, `git diff --word-diff=porcelain`
or `diff <(git show HEAD~1:path | jq .) <(jq . path)`.

There are two halves to every case.

**The server's half: `input` → `expected.document`.** The Go tests in this
package do that (`go test ./pkg/sdui/conformance`). Renderer authors don't
need to.

**The renderer's half: `expected.document` → `expected.layout`.** This is
yours.

## What a renderer's test does

For every fixture where `expected.notFound` is not true:

1. Decode `expected.document` with the decoder your app really uses.
2. Hand it to your renderer and let it build what it builds: view models, a
   SwiftUI view tree description, composables' semantics, React elements. Do
   this with the code your app really runs, not a second implementation.
3. Describe what was built in the **normalised layout** shape below.
4. Compare with `expected.layout`. They must be equal as JSON values (key
   order aside; a missing key and its zero value are the same).

For every entry of `expected.more`: decode `answer` as a section's page of
items, feed it to the same code that handles a real `more.url` response for
the section `section`, and check the section now holds its old cards
followed by `answer.items` (or only `answer.items` when `answer.replace`).

The point of step 2 is that the description comes from your views. A
normaliser that reads the JSON and never touches your renderer proves
nothing; use the reference implementation (`normalise.go`) to understand a
rule, not as something to port line by line.

## The normalised layout

```
{
  "kit": 1,
  "page": { "title", "formFactor", "breakpoint", "density",
            "contentWidth", "sectionGap", "scope" },
  "visible": [ Element, … ],      // every component drawn, depth first, in display order
  "focus": { "order": [ids…], "default": id, "groups": { groupId: [ids…] } }
}
```

`page.contentWidth` is the width sections have to themselves and
`page.sectionGap` the distance between them, in points: both follow from
`document.layout` by the rules in docs/SDUI.md, "Units, sizes and
breakpoints".

An `Element`:

| Field | What your renderer reports |
|---|---|
| `id`, `type`, `depth` | The node's ID, the component you drew it as, and how deep it is (the page is 0). An unknown type is reported as `Stack`: what you must draw it as. |
| `role`, `label` | What assistive technology is told: the accessibility role (below) and label you set. `role: "none"` with no label means hidden from it. |
| `state` | `deferred`, `empty`, `error`, or absent. |
| `text` | The text you display, by part: `title`, `subtitle`, `synopsis`, `text`, `button`, `facts` (joined with " · "), `tagline`, `overview`, `remaining`, `number`, `now`, `next`, `badges` (joined with a space), `rank`, `count`, `unwatched`, `duration` (m:ss), `empty`, `placeholder`, `query`, `autoAdvance` ("8 s"), `trailing`. Only parts that are drawn. |
| `lines` | The line limit you set for a part before it is cut with an ellipsis. |
| `textSize` | The size in points of the element's main text, from `layout.text`. |
| `cards` | How many cards, rows, tracks, tabs or slides it holds. |
| `placeholders` | For a `deferred` section: how many grey placeholders you draw. |
| `cardStyle`, `cardWidth`, `cardHeight` | The style and the size in points of its cards' pictures. For a `Hero`, the hero's own size. |
| `columns` | A grid's column count. |
| `hasMore` | You will ask for more items as it is scrolled. |
| `seeAll` | The route your "See all" opens. |
| `image` | The picture you ask for: its kind and width in pixels, `"poster 320"`; `"icon movie"` when you draw an icon instead; two joined with ", " for a header's backdrop and poster. |
| `shows` | For a card: the parts drawn, in reading order. |
| `progress` | A progress bar's fill, in whole percent. |
| `buttons` | Button labels in order, the primary one with a leading `*`. For a `FilterBar`: `"Sort: Title"`, then `"Genre: Any"`. For a `SeasonList`: the seasons, the selected one starred. |
| `action` | What activating it does: `"navigate /item/3"`, `"play 3"`, `"play 3 from the start"`, `"native settings"`, `"command watched.set 3"`, `"command selectTab artists"`, `"external https://…"`. |
| `scope` | Its theme scope, when it has one of its own. |

Roles: `main`, `list`, `grid`, `button`, `group`, `region`, `tablist`, `tab`,
`toolbar`, `heading 1`, `heading 2`, `heading 3`, `text`, `status`, `alert`,
`img`, `searchbox`, `separator`, `none`. Map your platform's own traits onto
these names in your test (a SwiftUI `.isButton` trait and a Compose
`Role.Button` are both `button`; a link on the web that navigates is
`button` here too).

`focus.order` lists the IDs of everything that can take focus, in the order
Tab (or a screen reader's "next") visits them; `focus.default` is where
focus starts; `focus.groups` lists each focus section's members in order.

## Running it

**Swift (WittyKit).** Add the fixtures folder to the test target as a
resource (or read it by path from the server checkout), then:

```swift
for url in fixtureURLs where url.lastPathComponent != "index.json" {
    let fx = try JSONDecoder().decode(Fixture.self, from: Data(contentsOf: url))
    guard let doc = fx.expected.document else { continue }
    let screen = SDUIScreenModel(document: doc)            // what the app builds
    let got = try JSONValue(encoding: screen.normalised()) // your description of it
    XCTAssertEqual(got, fx.expected.layout, fx.id)
}
```

**Kotlin (JVM unit tests).** Read `fixtures/*.json` from a path given by a
Gradle property, decode with the app's own kotlinx.serialization models,
build the screen state the composables draw from, describe it, and compare
`JsonElement`s.

**TypeScript (node:test).** `JSON.parse` each file, run the renderer's pure
layout step (the function your React components take their props from), and
`assert.deepStrictEqual` after dropping `undefined`.

In all three, compare as JSON values, and print the first differing
element's `id` and field: fixtures are large.

## The cases

`fixtures/index.json` lists every case with its title and form factor, and
each file's `checks` says what the case is there to pin down. They cover the
built-in pages on a television (HD and 4K), a phone, a tablet, a desktop
window and a narrow browser; an empty server; variants; a profile's own
order, hidden, pinned and locked sections, density and card sizes; a profile
that may not rearrange; a source that fails; conditions on
the time of day, feature switches and flags; reduced motion; theme scopes;
libraries with filters, tabs and native views; a film's, a show's, an
album's and a collection's page, and a title that isn't
there; search; Live TV; a hand-made page of layout components; every card
style; long text; a field mapped into a subtitle; awkward item IDs; banners;
a books library's shelf with a row per series, statistics and likes. None of
the last needs anything new of a renderer: they are cards like any others,
and a row repeated over a library's series is a Shelf whose ID ends in the
repeat's `id` (`input.data.each[over][].id`) rather than its key.

## When a fixture changes

`go test ./pkg/sdui/conformance -update` rewrites the fixtures from the
cases (`cases_test.go`). A changed `expected.document` or `expected.layout`
is a change to the contract: it is called out in the server's changelog, and
`kit` goes up when the normalised shape itself changes. Renderers pin the
server commit their fixtures came from.

Everything in the fixtures is made up: no title, picture or name is from a
real library.
