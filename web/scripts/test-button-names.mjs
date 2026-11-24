// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Every button has a name a screen reader can say. Reads every .tsx file under src/ and fails on a
// <button> (or an element with role="button") whose only content is an icon, a picture or nothing,
// with no aria-label, aria-labelledby or title. A screen reader says just "button" for one of those.
//
// Text, a {words} expression or a component that is passed text all count as a name. A button that
// spreads its props ({...rest}) is left to whoever uses it. Nothing else is needed: the parser is the
// one Vite already brings.
//   node scripts/test-button-names.mjs            (exits 1 and lists each unnamed button)
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parseAst } from 'vite'

const root = fileURLToPath(new URL('../src', import.meta.url))
const files = []
const walk = (d) => { for (const n of readdirSync(d)) { const p = join(d, n); if (statSync(p).isDirectory()) walk(p); else if (p.endsWith('.tsx')) files.push(p) } }
walk(root)

// Components that draw only a picture: an icon (Icon*, *Icon, *Glyph), an avatar or a logo.
const PICTURE = /^(Icon|svg$|img$|i$|path$|Avatar|BrandLogo$|.*(Icon|Glyph|Logo|Svg)$)/
const NAMES = new Set(['aria-label', 'aria-labelledby', 'title'])
const attrName = (a) => (a.type === 'JSXAttribute' ? (a.name.type === 'JSXNamespacedName' ? a.name.name.name : a.name.name) : null)
const tagName = (n) => (n.type === 'JSXIdentifier' ? n.name : n.type === 'JSXMemberExpression' ? tagName(n.object) + '.' + n.property.name : '')
const attrValue = (el, name) => {
  const a = el.openingElement.attributes.find((x) => attrName(x) === name)
  if (!a) return undefined
  if (!a.value) return true
  return a.value.type === 'Literal' ? a.value.value : a.value
}

// Does this child give the button words? Unknown expressions count as words: only a plain picture fails.
function speaks(node) {
  switch (node.type) {
    case 'JSXText': return node.value.trim() !== ''
    case 'JSXExpressionContainer': {
      const e = node.expression
      if (e.type === 'JSXEmptyExpression') return false
      if (e.type === 'JSXElement' || e.type === 'JSXFragment') return speaks(e)
      if (e.type === 'LogicalExpression') return speaks({ type: 'JSXExpressionContainer', expression: e.right })
      if (e.type === 'ConditionalExpression') return speaks({ type: 'JSXExpressionContainer', expression: e.consequent }) || speaks({ type: 'JSXExpressionContainer', expression: e.alternate })
      if (e.type === 'Literal') return typeof e.value === 'string' ? e.value.trim() !== '' : typeof e.value === 'number'
      return true
    }
    case 'JSXFragment': return node.children.some(speaks)
    case 'JSXElement': {
      const tag = tagName(node.openingElement.name)
      if (attrValue(node, 'aria-hidden') === 'true' || attrValue(node, 'aria-hidden') === true) return false
      if (tag === 'img') { const alt = attrValue(node, 'alt'); return alt !== undefined && alt !== '' }
      if (PICTURE.test(tag)) return attrValue(node, 'aria-label') !== undefined || attrValue(node, 'title') !== undefined
      // A component handed words through a prop draws them (a <Tip text=…>, a <Label children=…>).
      if (/^[A-Z]/.test(tag) && node.openingElement.attributes.some((a) => a.type === 'JSXSpreadAttribute' || ['label', 'text', 'children'].includes(attrName(a)))) return true
      if (/^[A-Z]/.test(tag) && node.children.length === 0) return true // an unknown component: assume it speaks
      return node.children.some(speaks)
    }
    default: return true
  }
}

const found = []
function visit(node, file, src, inLabel = false) {
  if (!node || typeof node !== 'object') return
  if (Array.isArray(node)) { for (const n of node) visit(n, file, src, inLabel); return }
  if (node.type === 'JSXElement') {
    const el = node.openingElement
    const tag = tagName(el.name)
    const role = attrValue(node, 'role')
    // A hidden submit button (Enter in a form) is never seen or reached, so it needs no name.
    if ((tag === 'button' || role === 'button') && attrValue(node, 'hidden') === undefined) {
      const named = el.attributes.some((a) => a.type === 'JSXSpreadAttribute' || NAMES.has(attrName(a)))
      // A button inside a <label> is named by the label's words.
      if (!named && !inLabel && !node.children.some(speaks)) {
        const line = src.slice(0, node.start).split('\n').length
        found.push(`${relative(root, file).replaceAll('\\', '/')}:${line}  ${src.slice(node.start, node.end).replace(/\s+/g, ' ').slice(0, 110)}`)
      }
    }
  }
  const label = inLabel || (node.type === 'JSXElement' && tagName(node.openingElement.name) === 'label')
  for (const k in node) if (k !== 'start' && k !== 'end' && typeof node[k] === 'object') visit(node[k], file, src, label)
}

for (const f of files) {
  const src = readFileSync(f, 'utf8')
  let ast
  try { ast = parseAst(src, { lang: 'tsx' }, f) } catch (e) { console.error(`${f}: could not read: ${e.message}`); process.exitCode = 1; continue }
  visit(ast, f, src)
}
if (found.length) {
  console.error(`${found.length} button(s) with no name. Give each an aria-label (and a title for the tooltip):`)
  for (const l of found) console.error('  ' + l)
  process.exit(1)
}
console.log(`every button in ${files.length} files has a name`)
