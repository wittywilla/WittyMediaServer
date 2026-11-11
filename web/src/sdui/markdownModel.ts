// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The small Markdown a Text may hold (docs/SDUI.md, "Text placeholders and Markdown"):
// bold, italic, code, # to ### headings, - and 1. lists, paragraphs, and links to a
// route in the app or an http(s) page. Anything else, raw HTML included, is the
// characters it is. Parsed to plain data; render/markdown.tsx draws it.

export type Inline = { t: 'text'; v: string } | { t: 'bold' | 'italic'; c: Inline[] } | { t: 'code'; v: string } | { t: 'link'; to: string; external: boolean; c: Inline[] }
export type Block = { t: 'p'; c: Inline[] } | { t: 'h'; level: 1 | 2 | 3; c: Inline[] } | { t: 'ul' | 'ol'; items: Inline[][] }

// linkKind: where a link may go. A route in the app, an http(s) page, or nowhere.
export function linkKind(to: string): 'route' | 'external' | null {
  if (/^\/(?!\/)/.test(to)) return 'route'
  if (/^https?:\/\/[^\s]+$/i.test(to)) return 'external'
  return null
}

export function inline(s: string): Inline[] {
  const out: Inline[] = []
  let text = ''
  const flush = () => { if (text) { out.push({ t: 'text', v: text }); text = '' } }
  let i = 0
  while (i < s.length) {
    const rest = s.slice(i)
    let m: RegExpExecArray | null
    if ((m = /^`([^`]+)`/.exec(rest))) { flush(); out.push({ t: 'code', v: m[1] }); i += m[0].length; continue }
    if ((m = /^\*\*([^*]+(?:\*(?!\*)[^*]+)*)\*\*/.exec(rest))) { flush(); out.push({ t: 'bold', c: inline(m[1]) }); i += m[0].length; continue }
    if ((m = /^\*([^*\s][^*]*)\*/.exec(rest))) { flush(); out.push({ t: 'italic', c: inline(m[1]) }); i += m[0].length; continue }
    if ((m = /^\[([^\]]*)\]\(([^)\s]*)\)/.exec(rest))) {
      const kind = linkKind(m[2])
      flush()
      // A link that goes nowhere allowed is its text.
      if (kind) out.push({ t: 'link', to: m[2], external: kind === 'external', c: inline(m[1]) })
      else out.push(...inline(m[1]))
      i += m[0].length
      continue
    }
    text += s[i++]
  }
  flush()
  return out
}

export function parse(src: string): Block[] {
  const blocks: Block[] = []
  let para: string[] = []
  let listKind: 'ul' | 'ol' | null = null
  let items: Inline[][] = []
  const endPara = () => { if (para.length) { blocks.push({ t: 'p', c: inline(para.join(' ')) }); para = [] } }
  const endList = () => { if (listKind) { blocks.push({ t: listKind, items }); listKind = null; items = [] } }
  for (const raw of src.replace(/\r\n?/g, '\n').split('\n')) {
    const line = raw.trim()
    let m: RegExpExecArray | null
    if (!line) { endPara(); endList(); continue }
    if ((m = /^(#{1,3})\s+(.*)$/.exec(line))) { endPara(); endList(); blocks.push({ t: 'h', level: m[1].length as 1 | 2 | 3, c: inline(m[2]) }); continue }
    if ((m = /^-\s+(.*)$/.exec(line))) { endPara(); if (listKind !== 'ul') endList(); listKind = 'ul'; items.push(inline(m[1])); continue }
    if ((m = /^\d+\.\s+(.*)$/.exec(line))) { endPara(); if (listKind !== 'ol') endList(); listKind = 'ol'; items.push(inline(m[1])); continue }
    endList()
    para.push(line)
  }
  endPara(); endList()
  return blocks
}
