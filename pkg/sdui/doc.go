// Package sdui implements the WittyMediaServer Server-Driven UI (SDUI) core:
// a JSON schema for describing screens, a validator, a renderer that resolves
// a screen for one particular client, a differ that produces incremental
// patches, and a set of templates for the stock screens (home, library,
// detail, player, settings).
//
// # What SDUI is
//
// In a server-driven UI the server, not the client, decides what a screen
// contains and how it is laid out. The server sends a tree of components
// (a Document); the client walks the tree and draws each component with its
// native widgets. Changing the home screen — adding a shelf, gating a row on a
// module, re-ordering sections — becomes a server-side data change that every
// client (web, TV, mobile) picks up without shipping new code.
//
// The library is pure Go, depends only on the standard library and imports
// nothing under internal/. HTTP wiring, storage and authentication live
// elsewhere; this package only knows about documents.
//
// # Lifecycle: Document → Render → Patch
//
// A Document is authored once, either by hand (JSON), with the fluent builder
// (see NewDocument and N) or from a template (see HomeTemplate). Authored
// documents may contain Visibility conditions and data Bindings, so one
// document serves every user and device.
//
// Render takes an authored Document plus a RenderContext (who is asking, on
// what platform, with which modules and features, with what data) and
// produces a concrete Document: conditions are evaluated and hidden subtrees
// removed, bindings are resolved against the context data, module theme
// overrides are applied, and Meta.GeneratedAt / Meta.ETag are stamped. The
// rendered document has no Visibility or Bindings left — the client receives
// a fully resolved tree.
//
// Diff compares two rendered documents and produces a list of Patch
// operations keyed by node ID. A client that already holds the previous
// version can apply the patches (see Apply) instead of re-downloading the
// whole screen. ETag and Canonical provide stable identifiers for
// conditional requests.
//
// # Versioning policy
//
// SchemaVersion is the wire version of Document. It is bumped only on
// incompatible changes (renaming or removing fields, changing semantics).
// Adding a new component type, a new Style field, a new Condition predicate
// or a new Action type is a compatible change and does not bump the version;
// clients must treat unknown component types as opaque containers (render
// their children, ignore the type) and ignore unknown props.
//
// Unmarshal rejects documents whose version is not SchemaVersion. Servers
// should persist authored documents and re-validate them with Validate on
// load so that a schema bump is noticed at startup rather than at request
// time.
//
// # Stability guarantees
//
//   - Validate, Render, Diff, Apply, Canonical and ETag are deterministic:
//     identical inputs produce identical outputs (maps are sorted when
//     serialised).
//   - Render never mutates its input; it works on a deep copy.
//   - Apply(old, Diff(old, new)) is structurally equal to new for any two
//     valid documents.
//   - Every template returned by this package passes Validate.
package sdui
