// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Link, Navigate, useParams } from 'react-router-dom'
import { useApp } from '../state'
import { GAP_PAGES, gapPage } from './catalogue'
import SpecPage from './SpecPage'

// /design/gaps for administrators: each of the 124 drawn pages, in the look and theme in use, with the canvas's
// own sample wording, to hold beside docs/design-2/images/gaps. /design/gaps/<key> shows one.
export default function GapsReview() {
  const { user } = useApp()
  const { key } = useParams()
  if (!user?.isAdmin) return <Navigate to="/" replace />
  const page = key ? gapPage(key) : undefined
  if (key && !page) return <Navigate to="/design/gaps" replace />
  if (page) {
    return (
      <>
        <p><Link to="/design/gaps">All drawn pages</Link></p>
        <SpecPage page={page} />
      </>
    )
  }
  return (
    <div className="spec-page">
      <header className="spec-head"><p className="spec-eyebrow">Design</p><h1>Every feature, drawn</h1><p className="spec-lead">The {GAP_PAGES.length} pages of the canvas, built from the same data. Wording is the canvas’s sample.</p></header>
      <section className="spec-sec" aria-label="Pages">
        <ul className="spec-rows">
          {GAP_PAGES.map((p) => <li key={p.key}><Link className="spec-row live" to={`/design/gaps/${p.key}`}><span className="spec-row-main"><span className="spec-row-title">{p.title}</span><small className="spec-row-sub">{p.eyebrow}</small></span><span className="spec-row-value">{p.key}</span></Link></li>)}
        </ul>
      </section>
    </div>
  )
}
