// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Fragment, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { parse, type Inline } from '../markdownModel'

function spans(list: Inline[], externalLinks: boolean, inert?: boolean): ReactNode {
  return list.map((x, i) => {
    switch (x.t) {
      case 'text': return <Fragment key={i}>{x.v}</Fragment>
      case 'code': return <code key={i}>{x.v}</code>
      case 'bold': return <strong key={i}>{spans(x.c, externalLinks, inert)}</strong>
      case 'italic': return <em key={i}>{spans(x.c, externalLinks, inert)}</em>
      case 'link':
        // A device with no browser shows a web link as its words.
        if (inert || (x.external && !externalLinks)) return <span key={i}>{spans(x.c, externalLinks, inert)}</span>
        return x.external
          ? <a key={i} href={x.to} target="_blank" rel="noopener noreferrer">{spans(x.c, externalLinks)}</a>
          : <Link key={i} to={x.to}>{spans(x.c, externalLinks)}</Link>
    }
  })
}

// The Markdown subset, drawn as elements: no HTML is ever interpreted.
export function Markdown({ text, externalLinks, inert }: { text: string; externalLinks: boolean; inert?: boolean }) {
  return (
    <>
      {parse(text).map((b, i) => {
        if (b.t === 'p') return <p key={i}>{spans(b.c, externalLinks, inert)}</p>
        if (b.t === 'h') { const H = (['h2', 'h3', 'h4'] as const)[b.level - 1]; return <H key={i}>{spans(b.c, externalLinks, inert)}</H> }
        const L = b.t
        return <L key={i}>{b.items.map((it, k) => <li key={k}>{spans(it, externalLinks, inert)}</li>)}</L>
      })}
    </>
  )
}
