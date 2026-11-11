// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The page editor's thinking, without a page: the palette and the forms made from a
// catalogue, the tree of sections, where an issue points and what a button can do about
// it, what changed, and undo. Run with:
//   node --experimental-strip-types --test web/src/pages/settings/pages/editorModel.test.mjs
// testdata/catalog.json is a trimmed copy of a real GET /sdui/catalog answer (fewer
// components and sources, no prose about how each is drawn), so the shapes are the server's.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import {
  applyFix, canHold, componentFields, conditionFields, conditionText, copyPage, describeChanges, duplicateAt, editPath, exampleSection, fieldFor, flatten, foldDiff,
  fresh, fromText, getIn, insertAt, isSettingsEntry, lineDiff, listIds, locateIssue, matchText, moveBy, moveIn, moveItem, moveOut, moveTo, navScreens, navTitle,
  newNavItem, newPage, newSection, pageIds, pageName, pageRoute, palette, parseFix, parseNavigationJson, parsePageJson, placementOf, previewDevice, previewInputs,
  previewLibrary, previewScale, readExport, removeAt, removeIn, renameId, renameIn, rootSections, samePage, sectionAt, sectionChoices, sectionTitle, setIn,
  setPlacement, setValue, slug, sourceFields, sourceGroups, sourceRule, tidyCondition, tidyNavigation, tidyPage, toText, undoBack, undoForward, undoPush,
  undoStart, uniqueId, updateAt, validId, variablesFor, withRootSections, words,
} from './editorModel.ts'

const real = JSON.parse(readFileSync(new URL('./testdata/catalog.json', import.meta.url), 'utf8'))

// A catalogue of made-up components: nothing in the editor may depend on the real names.
const prop = (name, type, more = {}) => ({ name, type, label: words(name), ...more })
const tiny = {
  components: [
    { type: 'Sheet', label: 'Sheet', description: 'The page.', icon: 'grid', category: 'structure', authored: false, data: 'none', container: true },
    { type: 'Wall', label: 'Wall', description: 'A wall of things.', icon: 'grid', category: 'things', authored: true, data: 'required', container: false,
      props: [prop('heading', 'text', { maxLength: 80, help: 'Above the wall.' }), prop('size', 'enum', { default: 'm', options: [{ value: 's', label: 'Small' }, { value: 'm', label: 'Medium' }] }), prop('glow', 'bool', { default: true, formFactors: ['tv'] }), prop('most', 'int', { min: 1, max: 9, required: true, default: 3 })] },
    { type: 'Brick', label: 'Brick', description: 'Made by the server.', icon: 'photo', category: 'things', authored: false, data: 'none', container: false },
    { type: 'Drawer', label: 'Drawer', description: 'Holds anything.', icon: 'list', category: 'boxes', authored: true, data: 'none', container: true },
    { type: 'Cabinet', label: 'Cabinet', description: 'Holds shelves only.', icon: 'more', category: 'boxes', authored: true, data: 'none', container: true, children: ['Board'], props: [prop('opens', 'section')] },
    { type: 'Board', label: 'Board', description: 'One shelf of a cabinet.', icon: 'more', category: 'boxes', authored: true, data: 'none', container: true },
    { type: 'Note', label: 'Note', description: 'Words.', icon: 'info', category: 'words', authored: true, data: 'optional', accepts: ['quotes'], container: false, props: [prop('words', 'markdown'), prop('hologram', 'hologram3d')] },
  ],
  sources: [
    { type: 'latest', label: 'Latest', description: 'The newest things.', icon: 'plus', category: 'shelf', params: [prop('library', 'library'), prop('limit', 'int', { min: 0, max: 500 })], yields: ['movie'] },
    { type: 'quotes', label: 'Quotes', description: 'Lines to show.', icon: 'info', category: 'words', params: [prop('about', 'ref', { required: true })], yields: ['quote'], pageKinds: ['detail'] },
    { type: 'pieces', label: 'Pieces', description: 'Parts of a title.', icon: 'show', category: 'context', params: [prop('item', 'ref', { required: true })], yields: ['episode'], pageKinds: ['detail'] },
  ],
  conditions: [
    { field: 'formFactor', name: 'formFactor', type: 'enumList', label: 'Device', options: [{ value: 'tv', label: 'Television' }, { value: 'phone', label: 'Phone' }] },
    { field: 'admin', name: 'admin', type: 'bool', label: 'Administrator' },
    { field: 'hasLibrary', name: 'hasLibrary', type: 'enum', label: 'Has a library of kind', options: [{ value: 'movies', label: 'Movies' }] },
    { field: 'timeFrom', name: 'timeFrom', type: 'string', label: 'From time of day' },
  ],
  pageKinds: [
    { kind: 'home', label: 'Home', description: 'The first page.', route: '/', params: ['tab'] },
    { kind: 'detail', label: 'A title', description: 'One title.', route: '/item/{item}', params: ['item', 'show', 'tab'], variables: ['$item', '$about'], match: ['mediaKind'] },
    { kind: 'library', label: 'A library', description: 'One library.', route: '/library/{library}', params: ['library', 'tab'], variables: ['$library'], match: ['library', 'libraryKind'] },
    { kind: 'custom', label: 'A page of your own', description: 'Any other page.', route: '/page/{id}', params: ['tab'] },
  ],
  placements: [{ value: 'primary', label: 'In the bar' }, { value: 'more', label: 'Under More' }, { value: 'hidden', label: 'Not shown' }],
  nativeScreens: [{ screen: 'settings', label: 'Settings', description: '', route: '/settings', icon: 'settings' }, { screen: 'player', label: 'Player', description: '', route: '/watch/{item}', icon: 'play' }, { screen: 'share', label: 'Share', description: '', route: '', icon: 'share' }],
  limits: { maxSections: 200, maxVariants: 8 },
}

const types = (groups) => groups.flatMap((g) => g.items.map((i) => i.type))

// ---------------------------------------------------------------- palette

test('the palette holds what the catalogue lets an administrator place, in its groups', () => {
  const p = palette(tiny)
  assert.deepEqual(p.map((g) => g.category), ['things', 'boxes', 'words'])
  assert.deepEqual(p.map((g) => g.label), ['Things', 'Boxes', 'Words'])
  // Not the page itself, not what only the server makes, not what belongs inside something else.
  assert.deepEqual(types(p), ['Wall', 'Drawer', 'Cabinet', 'Note'])
  assert.deepEqual(p[0].items[0], { type: 'Wall', label: 'Wall', description: 'A wall of things.', icon: 'grid' })
})

test('what a container holds is the catalogue’s say', () => {
  assert.deepEqual(types(palette(tiny, 'Cabinet')), ['Board'])
  assert.deepEqual(types(palette(tiny, 'Drawer')), ['Wall', 'Drawer', 'Cabinet', 'Note'])
  assert.deepEqual(types(palette(tiny, 'Wall')), [], 'a wall holds nothing')
  assert.equal(canHold(tiny, '', 'Board'), false)
  assert.equal(canHold(tiny, 'Cabinet', 'Board'), true)
  assert.equal(canHold(tiny, 'Cabinet', 'Wall'), false)
  assert.equal(canHold(tiny, 'Drawer', 'Brick'), false)
  assert.equal(canHold(tiny, 'Drawer', 'Nonsense'), false)
})

test('the real catalogue: Page, Card and a Tab outside Tabs are not offered', () => {
  const top = types(palette(real))
  for (const t of ['Page', 'Card', 'Tab', 'ListItem']) assert.ok(!top.includes(t), t)
  for (const t of ['Shelf', 'Grid', 'Hero', 'Tabs', 'Stack', 'List', 'NativeView']) assert.ok(top.includes(t), t)
  assert.deepEqual(types(palette(real, 'Tabs')), ['Tab'])
  assert.deepEqual(types(palette(real, 'List')), ['ListItem'])
  assert.ok(types(palette(real, 'Tab')).includes('Shelf'))
  assert.ok(!types(palette(real, 'Tab')).includes('Tab'))
  assert.deepEqual(types(palette(real, 'Shelf')), [])
  // Every entry carries what the palette shows.
  for (const g of palette(real)) for (const i of g.items) assert.ok(i.label && i.description && i.icon, i.type)
  assert.equal(palette(real).find((g) => g.items.some((i) => i.type === 'Shelf')).label, 'Media')
})

// ---------------------------------------------------------------- the form model

test('a component’s fields carry its label, help, default, range and where it applies', () => {
  const [heading, size, glow, most] = componentFields(tiny, 'Wall')
  assert.equal(heading.kind, 'text')
  assert.equal(heading.help, 'Above the wall.')
  assert.equal(heading.limits, 'At most 80 characters')
  assert.equal(heading.placeholders, true)
  assert.equal(heading.hasDefault, false)
  assert.equal(size.kind, 'enum')
  assert.equal(size.defaultText, 'Default: Medium')
  assert.equal(size.placeholder, 'Medium')
  assert.deepEqual(size.options.map((o) => o.value), ['s', 'm'])
  assert.equal(glow.kind, 'bool')
  assert.equal(glow.defaultText, 'Default: On')
  assert.equal(glow.only, 'Only on television')
  assert.equal(most.kind, 'int')
  assert.equal(most.required, true)
  assert.equal(most.limits, '1 to 9')
  assert.equal(most.defaultValue, 3)
  assert.deepEqual(componentFields(tiny, 'Nonsense'), [])
})

test('a type this editor has never heard of is a plain text field', () => {
  const [, hologram] = componentFields(tiny, 'Note')
  assert.equal(hologram.kind, 'text')
  assert.equal(hologram.known, false)
  assert.equal(hologram.type, 'hologram3d')
  assert.equal(fromText(hologram, 'anything at all'), 'anything at all')
})

test('there is a field for every type of property', () => {
  const kinds = {
    string: 'text', markdown: 'markdown', int: 'int', number: 'number', bool: 'bool', enum: 'enum', enumList: 'enumList', icon: 'icon', route: 'route', image: 'image',
    url: 'url', section: 'section', library: 'library', ref: 'ref', scope: 'scope', object: 'json', list: 'json',
  }
  for (const [type, kind] of Object.entries(kinds)) {
    const f = fieldFor(tiny, prop('x', type))
    assert.equal(f.kind, kind, type)
    assert.equal(f.known, true, type)
  }
  // Text that may run long gets a box that grows; a short one a line.
  assert.equal(fieldFor(tiny, prop('x', 'text')).kind, 'multiline')
  assert.equal(fieldFor(tiny, prop('x', 'text', { maxLength: 120 })).kind, 'text')
  assert.equal(fieldFor(tiny, prop('x', 'text', { maxLength: 2000 })).kind, 'multiline')
})

test('every property, parameter and condition of the real catalogue gets a known field', () => {
  const seen = new Set()
  for (const c of real.components) for (const f of componentFields(real, c.type)) { assert.ok(f.known, `${c.type}.${f.name}: ${f.type}`); assert.ok(f.label, `${c.type}.${f.name}`); seen.add(f.type) }
  for (const s of real.sources) for (const f of sourceFields(real, s.type)) { assert.ok(f.known, `${s.type}.${f.name}: ${f.type}`); seen.add(f.type) }
  for (const f of conditionFields(real)) { assert.ok(f.known, f.name); seen.add(f.type) }
  for (const t of ['string', 'text', 'markdown', 'int', 'bool', 'enum', 'enumList', 'icon', 'image', 'route', 'url', 'section', 'library', 'ref']) assert.ok(seen.has(t), `the sample has no ${t}`)
  const card = componentFields(real, 'Shelf').find((f) => f.name === 'cardStyle')
  assert.equal(card.kind, 'enum')
  assert.equal(card.defaultValue, 'auto')
  assert.equal(card.defaultText, 'Default: The source’s usual style'.replace('’', "'"))
  assert.ok(card.options.some((o) => o.value === 'poster' && o.label === 'Poster'))
  const limit = sourceFields(real, 'recentlyAdded').find((f) => f.name === 'limit')
  assert.equal(limit.limits, '0 to 500')
  assert.deepEqual(conditionFields(real).map((f) => f.name).slice(0, 4), ['formFactor', 'platform', 'profileKind', 'admin'])
})

test('what is typed becomes the value it stands for', () => {
  const int = fieldFor(tiny, prop('n', 'int'))
  assert.equal(fromText(int, '12'), 12)
  assert.equal(fromText(int, ' 12 '), 12)
  assert.equal(fromText(int, ''), undefined)
  assert.equal(fromText(int, '1.5'), '1.5', 'left for the validator to refuse')
  const lib = fieldFor(tiny, prop('library', 'library'), ['$library', '$each'])
  assert.equal(fromText(lib, '3'), 3)
  assert.equal(fromText(lib, '$each'), '$each')
  assert.deepEqual(lib.variables, ['$library', '$each'])
  const text = fieldFor(tiny, prop('t', 'text', { maxLength: 50 }))
  assert.equal(fromText(text, 'Two words '), 'Two words ', 'a space typed between words stays')
  assert.equal(fromText(text, '   '), undefined)
  assert.deepEqual(fromText(fieldFor(tiny, prop('o', 'object')), '{"a":1}'), { a: 1 })
  assert.equal(toText(3), '3')
  assert.equal(toText(undefined), '')
  assert.equal(toText({ a: 1 }), '{"a":1}')
  // Only what someone chose is written down.
  assert.deepEqual(setValue({ a: 1, b: 2 }, 'a', undefined), { b: 2 })
  assert.deepEqual(setValue(undefined, 'a', 0), { a: 0 })
  assert.deepEqual(setValue({ a: 1 }, 'a', false), { a: false })
})

// ---------------------------------------------------------------- sources

test('which sources a component takes, and on which pages', () => {
  assert.equal(sourceRule(tiny, 'Wall'), 'required')
  assert.equal(sourceRule(tiny, 'Note'), 'optional')
  assert.equal(sourceRule(tiny, 'Drawer'), 'none')
  assert.deepEqual(sourceGroups(tiny, 'Drawer', 'home'), [])
  // A note takes quotes only; a wall takes anything.
  assert.deepEqual(types(sourceGroups(tiny, 'Note', 'detail')), ['quotes'])
  const onHome = sourceGroups(tiny, 'Wall', 'home')
  assert.deepEqual(onHome.map((g) => g.label), ['Shelf', 'Words', 'Context'])
  const quotes = onHome[1].items[0]
  assert.equal(quotes.fits, false)
  assert.equal(quotes.why, 'Only on a title pages')
  assert.equal(sourceGroups(tiny, 'Wall', 'detail')[1].items[0].fits, true)
  assert.deepEqual(variablesFor(tiny, 'detail', false), ['$item', '$about'])
  assert.deepEqual(variablesFor(tiny, 'home', true), ['$each'])
  assert.deepEqual(variablesFor(tiny, 'home', false), [])
})

test('the real catalogue: a season list takes episodes, on a title’s page', () => {
  assert.deepEqual(types(sourceGroups(real, 'SeasonList', 'detail')), ['episodes'])
  assert.equal(sourceGroups(real, 'SeasonList', 'home')[0].items[0].fits, false)
  assert.equal(sourceRule(real, 'Banner'), 'optional')
  assert.equal(sourceRule(real, 'Text'), 'none')
  assert.ok(variablesFor(real, 'library', false).includes('$library'))
  const show = sourceFields(real, 'episodes', variablesFor(real, 'detail', false)).find((f) => f.name === 'show')
  assert.equal(show.required, true)
  assert.ok(show.variables.includes('$show'))
})

test('a new section starts with what it must have', () => {
  const wall = newSection(tiny, 'Wall', 'home', ['wall'])
  assert.equal(wall.id, 'wall-2')
  assert.deepEqual(wall.props, { most: 3 })
  assert.deepEqual(wall.source, { type: 'latest' }, 'the first source that needs nothing chosen and works on this page')
  const cabinet = newSection(tiny, 'Cabinet', 'home', [])
  assert.equal(cabinet.children.length, 1)
  assert.equal(cabinet.children[0].component, 'Board')
  assert.equal(newSection(tiny, 'Drawer', 'home', []).children, undefined)
  // The real one: a season list on a title's page asks for the page's own show.
  const seasons = newSection(real, 'SeasonList', 'detail', [])
  assert.deepEqual(seasons.source, { type: 'episodes', show: '$show' })
  assert.equal(newSection(real, 'SeasonList', 'home', []).source, undefined, 'nothing fits there: the validator will say so')
  assert.equal(newSection(real, 'Shelf', 'home', ['row']).id, 'row-2')
  assert.equal(newSection(real, 'Tabs', 'home', []).children[0].component, 'Tab')
  const ex = exampleSection(real, 'Shelf', ['recent'])
  assert.equal(ex.component, 'Shelf')
  assert.equal(ex.id, 'recent-2')
  assert.equal(exampleSection(real, 'Tab', []), null)
})

// ---------------------------------------------------------------- ids

test('IDs: made from words, and never one that is taken', () => {
  assert.equal(slug('Family Film Night!'), 'family-film-night')
  assert.equal(slug('  Éte 2024 '), 'ete-2024')
  assert.equal(slug('123'), 'section')
  assert.equal(slug('FilterBar'), 'filter-bar')
  assert.ok(validId(slug('Ünïcode & more')))
  assert.equal(validId('Home'), false)
  assert.equal(validId('a'.repeat(65)), false)
  assert.equal(validId('family-night_2'), true)
  assert.equal(uniqueId('row', []), 'row')
  assert.equal(uniqueId('row', ['row']), 'row-2')
  assert.equal(uniqueId('row', ['row', 'row-2', 'row-3']), 'row-4')
  assert.equal(uniqueId('row-2', ['row', 'row-2']), 'row-3')
})

// ---------------------------------------------------------------- the tree

const S = (id, component = 'Wall', children) => (children ? { id, component, children } : { id, component })
const tree = () => [S('a'), S('box', 'Drawer', [S('b'), S('c'), S('inner', 'Drawer', [S('d')])]), S('e')]
const ids = (l) => flatten(l).map((r) => r.section.id)

test('the tree as rows, and a section by its place', () => {
  const rows = flatten(tree())
  assert.deepEqual(rows.map((r) => r.section.id), ['a', 'box', 'b', 'c', 'inner', 'd', 'e'])
  assert.deepEqual(rows.map((r) => r.depth), [0, 0, 1, 1, 1, 2, 0])
  assert.deepEqual(rows[5].path, [1, 2, 0])
  assert.equal(rows[2].parent, 'Drawer')
  assert.equal(rows[2].siblings, 3)
  assert.equal(sectionAt(tree(), [1, 2, 0]).id, 'd')
  assert.equal(sectionAt(tree(), [1, 9]), undefined)
  assert.equal(sectionAt(tree(), []), undefined)
})

test('insert, remove and change, without touching the tree that was', () => {
  const was = tree()
  const next = insertAt(was, [1], 1, S('new'))
  assert.deepEqual(ids(next), ['a', 'box', 'b', 'new', 'c', 'inner', 'd', 'e'])
  assert.deepEqual(ids(was), ['a', 'box', 'b', 'c', 'inner', 'd', 'e'])
  assert.deepEqual(ids(insertAt(was, [], 99, S('z'))), [...ids(was), 'z'])
  assert.deepEqual(ids(removeAt(was, [1, 2])), ['a', 'box', 'b', 'c', 'e'])
  assert.equal(sectionAt(updateAt(was, [1, 0], (s) => ({ ...s, scope: 'x' })), [1, 0]).scope, 'x')
  assert.equal(sectionAt(was, [1, 0]).scope, undefined)
})

test('moving: among neighbours, out of a container, into one, and never into itself', () => {
  const was = tree()
  let got = moveBy(was, [0], 1)
  assert.deepEqual(got.sections.map((s) => s.id), ['box', 'a', 'e'])
  assert.deepEqual(got.path, [1])
  assert.equal(moveBy(was, [0], -1), null)
  assert.equal(moveBy(was, [2], 1), null)
  got = moveBy(was, [1, 1], -1)
  assert.deepEqual(ids(got.sections), ['a', 'box', 'c', 'b', 'inner', 'd', 'e'])
  got = moveOut(was, [1, 0])
  assert.deepEqual(ids(got.sections), ['a', 'box', 'c', 'inner', 'd', 'b', 'e'])
  assert.deepEqual(got.path, [2])
  assert.equal(moveOut(was, [0]), null)
  got = moveIn(tiny, was, [2])
  assert.deepEqual(ids(got.sections), ['a', 'box', 'b', 'c', 'inner', 'd', 'e'])
  assert.deepEqual(got.path, [1, 3])
  assert.equal(sectionAt(got.sections, [1, 3]).id, 'e')
  assert.equal(moveIn(tiny, was, [1]), null, 'a wall holds nothing')
  // Dragged from the top into a container further down: the target's place shifts as it leaves.
  got = moveTo(was, [0], [1, 2], 0)
  assert.deepEqual(ids(got.sections), ['box', 'b', 'c', 'inner', 'a', 'd', 'e'])
  assert.deepEqual(got.path, [0, 2, 0])
  assert.equal(moveTo(was, [1], [1, 2], 0), null, 'not into its own inside')
  assert.equal(moveTo(was, [1], [1], 0), null)
  assert.equal(moveTo(was, [0], [], 1), null, 'where it already is')
  got = moveTo(was, [1, 2, 0], [], 0)
  assert.deepEqual(ids(got.sections), ['d', 'a', 'box', 'b', 'c', 'inner', 'e'])
  assert.deepEqual(ids(was), ['a', 'box', 'b', 'c', 'inner', 'd', 'e'])
})

test('a duplicate, a preset and an example get IDs the list doesn’t have', () => {
  const was = tree()
  const got = duplicateAt(was, [1], listIds(was))
  assert.deepEqual(ids(got.sections), ['a', 'box', 'b', 'c', 'inner', 'd', 'box-2', 'b-2', 'c-2', 'inner-2', 'd-2', 'e'])
  assert.deepEqual(got.path, [2])
  assert.equal(new Set(ids(got.sections)).size, 12)
  const f = fresh(S('a', 'Drawer', [S('a'), S('b')]), ['a'])
  assert.deepEqual(ids([f]), ['a-2', 'a-3', 'b'])
  assert.equal(fresh({ id: 'Bad ID', component: 'Wall' }, []).id, 'bad-id')
  assert.deepEqual(listIds(was), ['a', 'box', 'b', 'c', 'inner', 'd', 'e'])
  assert.equal(pageIds({ id: 'p', kind: 'home', title: 'P', sections: was, variants: [{ id: 'v', when: {}, sections: [S('a')] }] }).length, 8)
})

test('renaming a section keeps what pointed at it pointing at it', () => {
  const list = [{ id: 'cab', component: 'Cabinet', props: { opens: 'one' }, children: [S('one', 'Board'), S('two', 'Board')] }]
  const next = renameId(tiny, list, [0, 0], 'first')
  assert.equal(next[0].children[0].id, 'first')
  assert.equal(next[0].props.opens, 'first')
  assert.equal(list[0].props.opens, 'one')
  const realList = [{ id: 'filters', component: 'FilterBar', props: { target: 'all' } }, { id: 'all', component: 'Grid' }]
  assert.equal(renameId(real, realList, [1], 'everything')[0].props.target, 'everything')
  assert.deepEqual(sectionChoices(tiny, list, [0]).map((o) => o.value), ['one', 'two'])
  assert.deepEqual(sectionChoices(real, realList, [0]).map((o) => o.value), ['all'])
})

test('what a section is called in the tree', () => {
  assert.equal(sectionTitle(real, { id: 'x', component: 'Shelf', props: { title: 'Jump back in' } }), 'Jump back in')
  assert.equal(sectionTitle(real, { id: 'x', component: 'Shelf', source: { type: 'nextUp' } }), 'Next up')
  assert.equal(sectionTitle(real, { id: 'x', component: 'Shelf' }), 'Row')
  assert.equal(sectionTitle(real, { id: 'x', component: 'Gone' }), 'Gone')
})

// ---------------------------------------------------------------- issues

test('an issue’s path names the section and the field it is about', () => {
  let at = locateIssue('/sections/2/source/library')
  assert.deepEqual([at.where, at.variant, at.path, at.field, at.pointer], ['section', -1, [2], 'source/library', '/sections/2/source/library'])
  at = locateIssue('/pages/night/sections/0/props/cardStyle', 'night')
  assert.deepEqual([at.where, at.path, at.field, at.pointer], ['section', [0], 'props/cardStyle', '/sections/0/props/cardStyle'])
  at = locateIssue('/sections/7/children/0/component')
  assert.deepEqual([at.path, at.field], [[7, 0], 'component'])
  at = locateIssue('/sections/1/children/2/children/0/when/any/0/platform/0')
  assert.deepEqual([at.path, at.field], [[1, 2, 0], 'when'])
  at = locateIssue('/variants/1/sections/3/repeat/kinds/0')
  assert.deepEqual([at.where, at.variant, at.path, at.field], ['section', 1, [3], 'repeat/kinds'])
  at = locateIssue('/variants/0/when')
  assert.deepEqual([at.where, at.variant, at.field], ['variant', 0, 'when'])
  at = locateIssue('/sections/4')
  assert.deepEqual([at.where, at.path, at.field], ['section', [4], ''])
  at = locateIssue('/sections/3/props')
  assert.equal(at.field, 'props')
  for (const [path, field] of [['/title', 'title'], ['/match/libraryKind', 'match/libraryKind'], ['/personal/allow/0', 'personal/allow'], ['/sections', 'sections'], ['', '']]) {
    at = locateIssue(path)
    assert.deepEqual([at.where, at.field], ['page', field], path)
  }
  // An issue about another page, or about the menu, isn't this page's.
  assert.equal(locateIssue('/pages/other/title', 'night').where, 'other')
  assert.equal(locateIssue('/navigation/items/0/page', 'night').where, 'other')
  assert.equal(locateIssue('/theme/tokens/accent', 'night').where, 'other')
  // The menu's own.
  at = locateIssue('/items/3/placement/tv', '', true)
  assert.deepEqual([at.where, at.item, at.field, at.pointer], ['nav', 3, 'placement/tv', '/items/3/placement/tv'])
  at = locateIssue('/navigation/items/1/when/can', '', true)
  assert.deepEqual([at.item, at.field, at.pointer], [1, 'when', '/items/1/when/can'])
  assert.equal(locateIssue('/items', '', true).item, -1)
  assert.equal(locateIssue('/pages/home/title', '', true).where, 'other')
})

// The validator's own words, copied from a real POST /sdui/validate.
const bad = () => ({
  id: 'night', kind: 'custm', title: 'Night', icon: 'hom',
  sections: [
    { id: 'a', component: 'Shelf', props: { cardStyle: 'postr', titel: 'x', maxItems: '3', showTitles: 'yes' }, source: { type: 'recentlyAded', library: 1 } },
    { id: 'g', component: 'Grid', source: { type: 'recentlyAdded', limt: 3, map: { subtitel: 'studio', title: 'studi' } }, when: { formFactor: ['tvv'], can: 'game', any: [{ platform: ['webb'] }] }, repeat: { over: 'library', kinds: ['movie'] } },
    { id: 't', component: 'Text', props: { text: 'hi' }, source: { type: 'nextUp' } },
    { id: 'fb', component: 'FilterBar', props: { fields: ['genre', 'genree'] } },
  ],
})

test('“Did you mean” is a button: it sets the value, or renames what it is under', () => {
  const cases = [
    ['/kind', '"custm" is not a kind of page', 'Did you mean "custom"?', (p) => p.kind === 'custom'],
    ['/icon', '"hom" is not one of the built-in icons', 'Did you mean "home"?', (p) => p.icon === 'home'],
    ['/sections/0/props/cardStyle', '"postr" is not a choice for cardStyle; the choices are auto, poster', 'Did you mean "poster"?', (p) => p.sections[0].props.cardStyle === 'poster'],
    ['/sections/0/props/titel', 'Row has no property "titel"', 'Did you mean "title"?', (p) => p.sections[0].props.title === 'x' && !('titel' in p.sections[0].props)],
    ['/sections/0/source/type', '"recentlyAded" is not a data source', 'Did you mean "recentlyAdded"?', (p) => p.sections[0].source.type === 'recentlyAdded'],
    ['/sections/1/source/limt', 'the recentlyAdded source has no property "limt"', 'Did you mean "limit"?', (p) => p.sections[1].source.limit === 3 && !('limt' in p.sections[1].source)],
    ['/sections/1/source/map/subtitel', 'map can set title, subtitle or synopsis, not "subtitel"', 'Did you mean "subtitle"?', (p) => p.sections[1].source.map.subtitle === 'studio'],
    ['/sections/1/source/map/title', '"studi" is not an item field; the fields are title, subtitle', 'Did you mean "studio"?', (p) => p.sections[1].source.map.title === 'studio'],
    ['/sections/1/when/any/0/platform/0', '"webb" is not a platform; the choices are web, tvos', 'Did you mean "web"?', (p) => p.sections[1].when.any[0].platform[0] === 'web'],
    ['/sections/1/when/formFactor/0', '"tvv" is not a formFactor; the choices are tv, phone', 'Did you mean "tv"?', (p) => p.sections[1].when.formFactor[0] === 'tv'],
    ['/sections/1/when/can', '"game" is not a choice for can; the choices are games', 'Did you mean "games"?', (p) => p.sections[1].when.can === 'games'],
    ['/sections/1/repeat/over', '"library" is not something a section can be repeated over', 'Did you mean "libraries"?', (p) => p.sections[1].repeat.over === 'libraries'],
    ['/sections/1/repeat/kinds/0', '"movie" is not a kind of library', 'Did you mean "movies"?', (p) => p.sections[1].repeat.kinds[0] === 'movies'],
    ['/sections/3/props/fields/1', '"genree" is not a choice for fields; the choices are genre', 'Did you mean "genre"?', (p) => p.sections[3].props.fields[1] === 'genre'],
  ]
  for (const [path, message, fix, ok] of cases) {
    const page = bad()
    const at = locateIssue(path)
    const f = parseFix({ message, fix }, page, at.pointer)
    assert.ok(f, `${path}: no fix parsed`)
    assert.ok(f.label, path)
    const next = applyFix(page, at.pointer, f)
    assert.ok(ok(next), `${path}: ${JSON.stringify(f)}`)
    assert.deepEqual(page, bad(), 'the page that was is left alone')
  }
  // A renamed property keeps its place among the others.
  const renamed = applyFix(bad(), '/sections/0/props/titel', { kind: 'rename', to: 'title', label: '' })
  assert.deepEqual(Object.keys(renamed.sections[0].props), ['cardStyle', 'title', 'maxItems', 'showTitles'])
})

test('the other fixes a button can do: remove, make a number, switch on, move to the end', () => {
  const page = bad()
  let f = parseFix({ message: 'a Text doesn\'t take a data source', fix: 'Remove the source.' }, page, '/sections/2/source')
  assert.equal(f.kind, 'remove')
  assert.equal(applyFix(page, '/sections/2/source', f).sections[2].source, undefined)
  f = parseFix({ message: 'kinds only applies when repeating over libraries', fix: 'Remove kinds.' }, page, '/sections/1/repeat/kinds')
  assert.deepEqual(applyFix(page, '/sections/1/repeat/kinds', f).sections[1].repeat, { over: 'library' })
  f = parseFix({ message: 'Row has no property "zzz"', fix: 'Remove it. The properties are title, subtitle.' }, { sections: [{ props: { zzz: 1, title: 'a' } }] }, '/sections/0/props/zzz')
  assert.equal(f.kind, 'remove')
  f = parseFix({ message: '"genre" is listed twice', fix: 'Remove the repeat.' }, { fields: ['genre', 'year', 'genre'] }, '/fields/2')
  assert.deepEqual(applyFix({ fields: ['genre', 'year', 'genre'] }, '/fields/2', f), { fields: ['genre', 'year'] })
  f = parseFix({ message: 'maxItems is a whole number', fix: 'Write a whole number without quotes.' }, page, '/sections/0/props/maxItems')
  assert.equal(applyFix(page, '/sections/0/props/maxItems', f).sections[0].props.maxItems, 3)
  f = parseFix({ message: 'showTitles is true or false', fix: 'Write true or false without quotes.' }, page, '/sections/0/props/showTitles')
  assert.equal(applyFix(page, '/sections/0/props/showTitles', f).sections[0].props.showTitles, true)
  f = parseFix({ message: 'title is text', fix: 'Write it in quotes.' }, { title: 7 }, '/title')
  assert.equal(applyFix({ title: 7 }, '/title', f).title, '7')
  f = parseFix({ message: 'ttlSeconds is between 0 and 86400', fix: 'Use 0 for the default.' }, { ttlSeconds: -1 }, '/ttlSeconds')
  assert.equal(applyFix({ ttlSeconds: -1 }, '/ttlSeconds', f).ttlSeconds, 0)
  f = parseFix({ message: 'a Grid loads more as it is scrolled, so the sections after it are only reached at its end', fix: 'Move the grid to the end, or use a Row for the sections above it.' }, page, '/sections/1')
  assert.equal(f.kind, 'moveToEnd')
  assert.deepEqual(applyFix(page, '/sections/1', f).sections.map((s) => s.id), ['a', 't', 'fb', 'g'])
  f = parseFix({ message: 'the app\'s own view fills the rest of the page, so nothing can follow it', fix: 'Move it to the end.' }, page, '/sections/0')
  assert.equal(f.kind, 'moveToEnd')
  f = parseFix({ message: 'Settings is always in the navigation; it can\'t have a condition', fix: 'Remove the condition.' }, { items: [{ id: 's', when: { admin: true } }] }, '/items/0/when')
  assert.deepEqual(applyFix({ items: [{ id: 's', when: { admin: true } }] }, '/items/0/when', f), { items: [{ id: 's' }] })
})

test('advice stays advice: no button pretends to know what was meant', () => {
  const page = bad()
  for (const [path, message, fix] of [
    ['/title', 'a page needs a title', 'Give the page a title; it is what the navigation and the window show.'],
    ['/sections/1/id', 'the ID "a" is already used at /sections/0', 'Give each section its own ID.'],
    ['/sections/1/source', 'the section is repeated but its source never uses $each', 'Use "$each" for the library, view or collection.'],
    ['/sections/0/props', 'the button doesn\'t open anything', 'Set to (a place in the app), native (a screen of the app) or url (a web page).'],
    ['/sections/0/props/maxItems', 'maxItems is between 1 and 100', ''],
    ['/sections/0/props/cardStyle', '"postr" is not a choice', 'Shorten it.'],
    ['/sections/9/props/x', '"q" is not a choice', 'Did you mean "poster"?'],
    ['', 'the page is 300000 bytes; the most is 262144', 'Split it into more than one page.'],
  ]) assert.equal(parseFix({ message, fix }, page, path), null, path)
  // "three" is no number, "perhaps" neither on nor off.
  assert.equal(parseFix({ message: 'x is a whole number', fix: 'Write a whole number without quotes.' }, { x: 'three' }, '/x'), null)
  assert.equal(parseFix({ message: 'x is true or false', fix: 'Write true or false without quotes.' }, { x: 'perhaps' }, '/x'), null)
})

test('JSON by pointer', () => {
  const doc = { a: [{ b: 1 }, { b: 2 }], c: { d: 'x' } }
  assert.equal(getIn(doc, '/a/1/b'), 2)
  assert.equal(getIn(doc, '/a/7/b'), undefined)
  assert.equal(getIn(doc, ''), doc)
  assert.deepEqual(setIn(doc, '/a/0/b', 9).a, [{ b: 9 }, { b: 2 }])
  assert.deepEqual(setIn(doc, '/c/e', 1).c, { d: 'x', e: 1 })
  assert.deepEqual(removeIn(doc, '/a/0'), { a: [{ b: 2 }], c: { d: 'x' } })
  assert.deepEqual(removeIn(doc, '/c/d'), { a: doc.a, c: {} })
  assert.deepEqual(renameIn(doc, '/c/d', 'z').c, { z: 'x' })
  assert.equal(doc.a[0].b, 1)
})

// ---------------------------------------------------------------- undo

test('undo and redo, and typing in one field as one step', () => {
  let u = undoStart('a')
  u = undoPush(u, 'b')
  u = undoPush(u, 'c')
  assert.deepEqual([u.past, u.present, u.future], [['a', 'b'], 'c', []])
  u = undoBack(u)
  assert.deepEqual([u.past, u.present, u.future], [['a'], 'b', ['c']])
  u = undoBack(undoBack(u))
  assert.equal(u.present, 'a', 'nothing before the beginning')
  u = undoForward(undoForward(u))
  assert.deepEqual([u.past, u.present, u.future], [['a', 'b'], 'c', []])
  assert.equal(undoForward(u), u)
  // A new change after going back forgets what was undone.
  u = undoPush(undoBack(u), 'd')
  assert.deepEqual([u.past, u.present, u.future], [['a', 'b'], 'd', []])
  assert.equal(undoPush(u, 'd'), u, 'the same state is no change')
  // Letters typed into one field in quick succession are one step; a pause, or another field, starts a new one.
  let t = undoStart('')
  t = undoPush(t, 'h', 'title', 1000)
  t = undoPush(t, 'he', 'title', 1200)
  t = undoPush(t, 'hey', 'title', 1400)
  assert.deepEqual([t.past, t.present], [[''], 'hey'])
  t = undoPush(t, 'hey!', 'title', 9000)
  t = undoPush(t, 'hey!?', 'icon', 9100)
  assert.deepEqual(t.past, ['', 'hey', 'hey!'])
  assert.equal(undoBack(t).present, 'hey!')
  // After an undo, typing in the same field again is its own step.
  t = undoPush(undoBack(t), 'new', 'title', 9200)
  assert.deepEqual([t.past, t.present], [['', 'hey', 'hey!'], 'new'])
  // No more than the limit is kept.
  let many = undoStart(0)
  for (let i = 1; i <= 150; i++) many = undoPush(many, i)
  assert.equal(many.past.length, 100)
  assert.equal(many.past[0], 50)
})

// ---------------------------------------------------------------- what changed

test('what differs between two revisions, in sentences', () => {
  const lines = describeChanges([
    { path: '/navigation', change: 'changed' },
    { path: '/pages/home', change: 'changed', added: ['top'], removed: ['a', 'b'], changed: ['hero'], reordered: true, other: ['title', 'variants'] },
    { path: '/pages/night', change: 'added' },
    { path: '/pages/old', change: 'removed' },
    { path: '/pages/music', change: 'added' },
    { path: '/theme', change: 'changed' },
  ], { titleOf: (id) => ({ home: 'Home', night: 'Film night' })[id] || id, isBuiltin: (id) => id === 'music' || id === 'home' })
  assert.equal(lines[0].title, 'The menu changed')
  assert.equal(lines[1].title, 'Home (home): changed')
  assert.deepEqual(lines[1].details, ['1 section added: top', '2 sections removed: a, b', '1 section changed: hero', 'The sections are in another order', 'Also different: title, variants'])
  assert.equal(lines[2].title, 'Film night (night): a new page')
  assert.equal(lines[3].title, 'old: removed')
  assert.equal(lines[4].title, 'music: changed from the built-in page')
  assert.equal(lines[5].title, 'Theme colours changed')
  assert.equal(describeChanges([{ path: '/pages/x', change: 'removed' }])[0].title, 'x: removed, or back to the built-in page')
  assert.deepEqual(describeChanges([]), [])
})

test('two texts line by line', () => {
  const a = ['{', '  "id": "home",', '  "title": "Home",', '  "sections": []', '}'].join('\n')
  const b = ['{', '  "id": "home",', '  "title": "Start",', '  "icon": "home",', '  "sections": []', '}'].join('\n')
  const d = lineDiff(a, b)
  assert.deepEqual(d.map((l) => l.op).join(''), '  -++  ')
  assert.equal(d.filter((l) => l.op !== '+').map((l) => l.text).join('\n'), a)
  assert.equal(d.filter((l) => l.op !== '-').map((l) => l.text).join('\n'), b)
  assert.ok(lineDiff(a, a).every((l) => l.op === ' '))
  assert.deepEqual(lineDiff('', 'x').map((l) => l.op + l.text), ['-', '+x'])
  // Past the budget it still says everything that went and came.
  const big = lineDiff('a\nb\nc', 'x\ny\nz', 1)
  assert.equal(big.map((l) => l.op).join(''), '---+++')
  const long = lineDiff(Array.from({ length: 40 }, (_, i) => 'line ' + i).join('\n'), Array.from({ length: 40 }, (_, i) => (i === 20 ? 'changed' : 'line ' + i)).join('\n'))
  const folded = foldDiff(long, 2)
  assert.deepEqual(folded.map((l) => l.op).join(''), '…  -+  …')
  assert.equal(folded[0].text, '18 lines the same')
  assert.deepEqual(foldDiff(lineDiff(a, a)), [])
})

// ---------------------------------------------------------------- conditions and pages

test('a condition in words, and tidied', () => {
  assert.equal(conditionText(real, undefined), 'Always')
  assert.equal(conditionText(real, {}), 'Always')
  assert.equal(conditionText(real, { profileKind: ['kids'] }), 'Kind of profile: Kids')
  assert.equal(conditionText(real, { formFactor: ['tv', 'phone'], can: 'liveTv' }), 'Device: Television or Phone, and Profile may use: Live TV')
  assert.equal(conditionText(real, { admin: true }), 'Administrator')
  assert.equal(conditionText(real, { admin: false }), 'Not: administrator')
  assert.equal(conditionText(real, { any: [{ formFactor: ['tv'] }, { admin: true }] }), '(Device: Television, or Administrator)')
  assert.equal(conditionText(real, { not: { profileKind: ['kids'] } }), 'Not (Kind of profile: Kids)')
  assert.equal(conditionText(real, { not: {} }), 'Never')
  assert.equal(conditionText(real, { someday: 'x' }), 'Someday: x')
  assert.equal(tidyCondition({ formFactor: [], can: '', feature: 'live-tv', any: [] }).feature, 'live-tv')
  assert.deepEqual(tidyCondition({ formFactor: [], can: '' }), undefined)
  assert.deepEqual(tidyCondition({ not: {} }), { not: {} })
  assert.deepEqual(tidyCondition({ any: [{ can: '' }, { admin: false }] }), { any: [{}, { admin: false }] })
})

test('a page is saved without the empty things an editor leaves lying about', () => {
  const page = {
    id: 'night', kind: 'custom', title: 'Night', icon: '', match: {}, scope: '',
    sections: [{ id: 'a', component: 'Shelf', props: {}, source: { type: 'nextUp' }, when: { can: '' }, repeat: { over: '' }, scope: '', locked: false, children: [] }],
    variants: [{ id: 'kids', when: { profileKind: ['kids'] }, sections: [] }],
  }
  assert.deepEqual(tidyPage(page), {
    id: 'night', kind: 'custom', title: 'Night',
    sections: [{ id: 'a', component: 'Shelf', source: { type: 'nextUp' } }],
    variants: [{ id: 'kids', when: { profileKind: ['kids'] }, sections: [] }],
  })
  assert.ok(samePage(page, tidyPage(page)))
  assert.ok(!samePage(page, { ...page, title: 'Other' }))
  // Allowing nothing is a choice, and is kept; so is a lock.
  assert.deepEqual(tidyPage({ ...page, personal: { allow: [] } }).personal, { allow: [] })
  assert.equal(tidyPage({ ...page, sections: [{ id: 'a', component: 'Shelf', locked: true }] }).sections[0].locked, true)
  assert.deepEqual(newPage('n', 'N', 'library', { libraryKind: 'movies' }), { id: 'n', kind: 'library', title: 'N', sections: [], match: { libraryKind: 'movies' } })
  assert.equal(newPage('n', 'N', 'custom', {}).match, undefined)
  const copy = copyPage(page, 'night-2', 'Night again')
  assert.deepEqual([copy.id, copy.title, copy.sections[0].id], ['night-2', 'Night again', 'a'])
  copy.sections[0].id = 'changed'
  assert.equal(page.sections[0].id, 'a')
  assert.deepEqual(rootSections(page, 0), [])
  assert.equal(rootSections(page, -1).length, 1)
  assert.equal(withRootSections(page, 0, [S('z')]).variants[0].sections[0].id, 'z')
  assert.equal(withRootSections(page, -1, []).sections.length, 0)
})

test('what a page is for, in words, and where it is edited', () => {
  const libs = [{ id: 3, name: 'Films', type: 'movies' }, { id: 4, name: 'Series', type: 'shows' }]
  assert.equal(matchText(real, { kind: 'home' }), 'Home. The first page of the app.')
  assert.equal(matchText(real, { kind: 'library', match: { libraryKind: 'movies' } }), 'A library: only movies libraries')
  assert.equal(matchText(real, { kind: 'library', match: { library: 3 } }, libs), 'A library: only the library Films')
  assert.equal(matchText(real, { kind: 'detail', match: { mediaKind: 'album' } }), 'A title: only album titles')
  // A title that fills itself in is no name for a list.
  assert.equal(pageName(real, { id: 'home', title: 'Home', kind: 'home' }), 'Home')
  assert.equal(pageName(real, { id: 'library-movies', title: '{library}', kind: 'library', match: { libraryKind: 'movies' } }), 'A library: Movies')
  assert.equal(pageName(real, { id: 'detail-album', title: '{title}', kind: 'detail', match: { mediaKind: 'album' } }), 'A title: Album')
  assert.equal(pageName(real, { id: 'detail', title: '{title}', kind: 'detail' }), 'A title')
  assert.equal(pageName(real, { id: 'x', title: '', kind: 'custom' }), 'A page of your own')
  assert.equal(editPath('home'), '/settings/pages/home')
  assert.equal(editPath('navigation'), '/settings/pages/page/navigation')
  assert.equal(editPath(''), '/settings/pages')
  assert.equal(previewLibrary({ match: { libraryKind: 'shows' } }, libs), 4)
  assert.equal(previewLibrary({ match: { library: 9 } }, libs), 9)
  assert.equal(previewLibrary({}, libs), 3)
  assert.equal(previewLibrary({}, []), 0)
})

test('JSON typed by hand is read whole or not at all', () => {
  assert.match(parsePageJson('{ "id": "home", ', 'home').error, /isn’t JSON yet/)
  assert.match(parsePageJson('[]', 'home').error, /between \{ and \}/)
  assert.match(parsePageJson('{"id":"other","sections":[]}', 'home').error, /calls itself “other”/)
  assert.match(parsePageJson('{"sections":{}}', 'home').error, /“sections” is a list/)
  const got = parsePageJson('{"kind":"home","title":"Home","sections":[{"id":"a","component":"Shelf"}]}', 'home')
  assert.equal(got.page.id, 'home')
  assert.equal(got.page.sections[0].id, 'a')
  assert.match(parseNavigationJson('{"items":{}}').error, /items/)
  assert.match(parseNavigationJson('{"items":[3]}').error, /Every entry/)
  assert.equal(parseNavigationJson('{"items":[{"id":"home","type":"page","page":"home"}]}').navigation.items.length, 1)
})

// ---------------------------------------------------------------- the menu

test('menu entries: where each sits, what it is called, and Settings', () => {
  const settings = { id: 'settings', type: 'native', screen: 'settings', placement: { phone: 'more' } }
  assert.ok(isSettingsEntry(settings))
  assert.ok(!isSettingsEntry({ id: 'g', type: 'native', screen: 'guide' }))
  assert.equal(placementOf(tiny, settings, 'phone'), 'more')
  assert.equal(placementOf(tiny, settings, 'tv'), 'primary', 'a form factor left out is in the bar')
  assert.deepEqual(setPlacement(tiny, settings, 'tv', 'hidden').placement, { phone: 'more', tv: 'hidden' })
  assert.equal(setPlacement(tiny, settings, 'phone', 'primary').placement, undefined, 'the usual place isn’t written down')
  // Only screens with a place of their own that needs nothing filled in.
  assert.deepEqual(navScreens(tiny).map((s) => s.screen), ['settings'])
  assert.deepEqual(navScreens(real).map((s) => s.screen), ['settings', 'guide', 'profiles', 'downloads', 'remote', 'nowPlaying', 'pair', 'collections'])
  const pages = [{ id: 'home', title: 'Home', kind: 'home', source: 'builtin', sections: 3 }]
  assert.equal(navTitle(real, { id: 'h', type: 'page', page: 'home' }, pages), 'Home')
  assert.equal(navTitle(real, { id: 'h', type: 'page', page: 'home', label: 'Start' }, pages), 'Start')
  assert.equal(navTitle(real, { id: 'l', type: 'library', library: 3 }, pages, [{ id: 3, name: 'Films', type: 'movies' }]), 'Films')
  assert.equal(navTitle(real, { id: 's', type: 'native', screen: 'guide' }), 'TV guide')
  assert.equal(navTitle(real, { id: 'r', type: 'route', route: '/x' }), '/x')
  assert.deepEqual(newNavItem('page', ['entry']), { id: 'entry-2', type: 'page' })
  assert.deepEqual(newNavItem('route', []), { id: 'entry', type: 'route', route: '/', label: 'New entry' })
  assert.deepEqual(tidyNavigation({ items: [{ id: 'a', type: 'page', page: 'home', route: '/left-over', label: '', when: { can: '' }, placement: {} }] }), { items: [{ id: 'a', type: 'page', page: 'home' }] })
  assert.deepEqual(moveItem(['a', 'b', 'c'], 0, 2), ['b', 'c', 'a'])
  assert.deepEqual(moveItem(['a', 'b', 'c'], 2, 0), ['c', 'a', 'b'])
  const same = ['a']
  assert.equal(moveItem(same, 0, 5), same)
})

// ---------------------------------------------------------------- preview and files

test('the preview pretends to be each kind of device', () => {
  assert.deepEqual(previewDevice('phone', 'en-GB'), { platform: 'web', formFactor: 'phone', capabilities: ['touch', 'keyboard'], locale: 'en-GB', views: [], viewport: '390x844' })
  assert.equal(previewDevice('tablet', 'en').viewport, '820x1180')
  assert.equal(previewDevice('desktop', 'en').viewport, '1440x900')
  assert.deepEqual(previewDevice('desktop', 'en').capabilities, ['pointer', 'keyboard'])
  const tv = previewDevice('tv', 'en')
  assert.equal(tv.deviceClass, 'hd')
  assert.equal(tv.viewport, undefined)
  assert.equal(previewScale(390, 'desktop'), 390 / 1440)
  assert.equal(previewScale(2000, 'phone'), 1, 'never drawn larger than life')
  assert.equal(previewScale(0, 'tv'), 1)
})

test('what a page needs chosen before it can be drawn, and its place in the app', () => {
  assert.deepEqual(previewInputs(real, 'home'), [])
  assert.deepEqual(previewInputs(real, 'library'), [{ name: 'library', required: true, kind: 'library' }])
  assert.deepEqual(previewInputs(real, 'detail'), [{ name: 'item', required: true, kind: 'text' }])
  assert.deepEqual(previewInputs(real, 'custom'), [])
  assert.equal(pageRoute(real, { id: 'home', kind: 'home' }), '/')
  assert.equal(pageRoute(real, { id: 'night', kind: 'custom' }), '/page/night')
  assert.equal(pageRoute(real, { id: 'library-movies', kind: 'library' }, { library: '3' }), '/library/3')
  assert.equal(pageRoute(real, { id: 'library-movies', kind: 'library' }), '', 'not until a library is chosen')
  assert.equal(pageRoute(real, { id: 'x', kind: 'nonsense' }), '')
})

test('a file to import is looked at before it goes anywhere', () => {
  assert.match(readExport('not json').error, /isn’t JSON/)
  assert.match(readExport('{"format":"something-else"}').error, /isn’t a file exported/)
  assert.match(readExport('{"format":"wittymedia-ui","kind":"page"}').error, /neither a page nor/)
  const page = readExport('{"format":"wittymedia-ui","formatVersion":2,"kind":"page","page":{"id":"night","title":"Film night"}}')
  assert.deepEqual([page.kind, page.name], ['page', 'Film night'])
  assert.equal(readExport('{"format":"wittymedia-ui","kind":"config","config":{"pages":{}}}').kind, 'config')
})
