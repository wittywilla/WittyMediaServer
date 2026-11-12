// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useMemo, type CSSProperties, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { Dock, type DockAction } from '../ui/Dock'
import { sectionId, withIds } from './model'
import type { SpecCard, SpecChip, SpecPageData, SpecRow, SpecSection } from './types'
import './spec.css'

// One page of Design 2 drawn from data (docs/design-2, "Every feature, drawn"): eyebrow, title, lead, then rows,
// chips, notes and cards, a Dock for the page's own buttons, and a foot line. Colours, shape and type come from
// the look and theme in use (tokens only); every item that goes somewhere or is handled is a link or a button in
// the page's Tab order, reached by the arrows, a controller and a remote through the app's spatial navigation,
// with the Dock as the page's own zone. Items with neither are plain text and are not focusable.
//
// `handled` lists the ids onAction answers; an item with `to` needs no handler.
export interface SpecPageProps {
  page: SpecPageData
  onAction?: (id: string) => void
  handled?: ReadonlySet<string>
  // Extra content under a section's items (a page's own control that has no place in the spec), by section index.
  after?: Record<number, ReactNode>
  className?: string
  // Part of a page that has its own heading: no eyebrow, title or lead of its own, and sections are h3.
  bare?: boolean
}

export default function SpecPage({ page: raw, onAction, handled, after, className, bare }: SpecPageProps) {
  const page = useMemo(() => withIds(raw), [raw])
  const live = (it: { id?: string; to?: string; disabled?: boolean }) => !it.disabled && (!!it.to || (!!it.id && !!handled?.has(it.id) && !!onAction))
  const dock: DockAction[] = page.actions.map((a) => ({
    id: a.id || a.title, label: a.title, primary: a.kind === 'primary', to: a.to, disabled: a.disabled || (!a.to && !(a.id && handled?.has(a.id))),
    onClick: a.to ? undefined : () => a.id && onAction?.(a.id),
  }))
  return (
    <article className={'spec-page' + (bare ? ' bare' : '') + (className ? ' ' + className : '')} data-spec={page.key}>
      {!bare && (
        <header className="spec-head">
          <p className="spec-eyebrow">{page.eyebrow}</p>
          <h1>{page.title}</h1>
          {page.lead && <p className="spec-lead">{page.lead}</p>}
        </header>
      )}
      {page.sections.map((s, i) => (
        <Section key={i} s={s} bare={!!bare} hid={sectionId(page.key, i)} live={live} onAction={onAction}>{after?.[i]}</Section>
      ))}
      {page.foot && <p className="spec-foot">{page.foot}</p>}
      {dock.length > 0 && <Dock label={page.title} actions={dock} legend={false} className="spec-dock" />}
    </article>
  )
}

type Live = (it: { id?: string; to?: string; disabled?: boolean }) => boolean

function Section({ s, hid, live, onAction, bare, children }: { s: SpecSection; bare: boolean; hid: string; live: Live; onAction?: (id: string) => void; children?: ReactNode }) {
  const H = bare ? 'h3' : 'h2'
  const h = s.h ? <H id={hid} className="spec-h">{s.h}</H> : null
  const label = s.h ? { 'aria-labelledby': hid } : {}
  if (s.kind === 'note') {
    return <section className="spec-sec" {...label}>{h}<p className={'spec-note' + (s.tone ? ' tone' : '')}>{s.text}</p>{children}</section>
  }
  return (
    <section className="spec-sec" {...label}>
      {h}
      {s.kind === 'rows' && <ul className="spec-rows">{s.items.map((r) => <li key={r.id}><Row r={r} live={live(r)} onAction={onAction} /></li>)}</ul>}
      {s.kind === 'chips' && <div className="spec-chips" role="group" {...label}>{s.items.map((c) => <Chip key={c.id} c={c} live={live(c)} onAction={onAction} />)}</div>}
      {s.kind === 'cards' && <ul className="spec-cards">{s.items.map((c) => <li key={c.id}><Card c={c} live={live(c)} onAction={onAction} /></li>)}</ul>}
      {children}
    </section>
  )
}

function Row({ r, live, onAction }: { r: SpecRow; live: boolean; onAction?: (id: string) => void }) {
  const body = (
    <>
      <span className="spec-row-main"><span className="spec-row-title">{r.title}</span>{r.sub && <small className="spec-row-sub">{r.sub}</small>}</span>
      {r.value && <span className="spec-row-value">{r.value}</span>}
    </>
  )
  const cls = 'spec-row' + (r.on ? ' on' : '') + (live ? ' live' : '')
  const cur = r.on ? { 'aria-current': 'true' as const } : {}
  if (live && r.to) return <Link to={r.to} className={cls} {...cur}>{body}</Link>
  if (live) return <button type="button" className={cls} onClick={() => r.id && onAction?.(r.id)} {...cur}>{body}</button>
  return <div className={cls} {...cur}>{body}</div>
}

function Chip({ c, live, onAction }: { c: SpecChip; live: boolean; onAction?: (id: string) => void }) {
  const cls = 'spec-chip' + (c.on ? ' on' : '')
  if (live && c.to) return <Link to={c.to} className={cls} aria-current={c.on ? 'true' : undefined}>{c.title}</Link>
  if (live) return <button type="button" className={cls} aria-pressed={!!c.on} onClick={() => c.id && onAction?.(c.id)}>{c.title}</button>
  return <span className={cls} aria-current={c.on ? 'true' : undefined}>{c.title}</span>
}

function Card({ c, live, onAction }: { c: SpecCard; live: boolean; onAction?: (id: string) => void }) {
  const tint: CSSProperties | undefined = c.bg ? ({ '--spec-card-a': c.bg, '--spec-card-b': c.bg2 || c.bg } as CSSProperties) : undefined
  const body = (
    <>
      <span className="spec-card-art" aria-hidden="true" style={c.art ? { backgroundImage: `url("${c.art}")` } : undefined} />
      <span className="spec-card-title">{c.title}</span>
      {c.sub && <small className="spec-card-sub">{c.sub}</small>}
    </>
  )
  const cls = 'spec-card' + (c.on ? ' on' : '') + (c.art ? ' has-art' : '')
  const cur = c.on ? { 'aria-current': 'true' as const } : {}
  if (live && c.to) return <Link to={c.to} className={cls} style={tint} {...cur}>{body}</Link>
  if (live) return <button type="button" className={cls} style={tint} onClick={() => c.id && onAction?.(c.id)} {...cur}>{body}</button>
  return <div className={cls} style={tint} {...cur}>{body}</div>
}
