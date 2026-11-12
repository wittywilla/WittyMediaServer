// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import LogoCredits from '../../components/LogoCredits'
import PosterCredits from '../../components/PosterCredits'
import { useEffect, useState, type ReactNode } from 'react'
import { Link, useLocation } from 'react-router-dom'
import legal from '../../../../internal/legal/legal.json'
import { api } from '../../api'
import { Brand, BrandMark, LoadError } from '../../components/common'
import { useApp } from '../../state'
import SpecPage from '../../spec/SpecPage'
import { NOTICE_HELP, ownerFormSpec, ownerNoticeSpec } from '../../spec/specs/legal'
import { Field, Textarea } from '../../ui'
import { PageHead, SaveBar, Section, useServerSettings } from './ui'
import { WebLink } from '../../components/WebLink'
import './settings.css'

// The full Legal page: the words are in internal/legal/legal.json, which native
// apps get from GET /api/legal. Anyone signed in reads it in Settings, and anyone
// else (a shared link, the setup wizard, the sign-in screen) at /legal.

type Owner = { notice: string; contact: string }

// **bold**, [text](address) and [[FILL IN: what to write]]: all the markup legal.json uses.
const INLINE = /\*\*(.+?)\*\*|\[\[FILL IN: ([^\]]+)\]\]|\[([^\]]+)\]\(([^)\s]+)\)/g

function Inline({ text, signedIn }: { text: string; signedIn: boolean }) {
  const out: ReactNode[] = []
  let at = 0
  for (const m of text.matchAll(INLINE)) {
    if (m.index > at) out.push(text.slice(at, m.index))
    const key = out.length
    if (m[1]) out.push(<b key={key}>{m[1]}</b>)
    else if (m[2]) out.push(<mark key={key} className="legal-fill">[To be filled in: {m[2]}]</mark>)
    else if (m[4].startsWith('/')) {
      // Settings pages can't open for someone who isn't signed in: they keep the words, without the link.
      if (signedIn || !m[4].startsWith('/settings/')) out.push(<Link key={key} to={m[4]}>{m[3]}</Link>)
      else out.push(m[3])
    } else out.push(<WebLink key={key} href={m[4]}>{m[3]}</WebLink>)
    at = m.index + m[0].length
  }
  if (at < text.length) out.push(text.slice(at))
  return <>{out}</>
}

function Body({ lines, signedIn }: { lines: string[]; signedIn: boolean }) {
  // Lines starting with "- " are bullets; neighbours make one list.
  const out: ReactNode[] = []
  let bullets: string[] = []
  const flush = () => {
    if (bullets.length) out.push(<ul key={out.length}>{bullets.map((b) => <li key={b}><Inline text={b} signedIn={signedIn} /></li>)}</ul>)
    bullets = []
  }
  for (const l of lines) {
    if (l.startsWith('- ')) { bullets.push(l.slice(2)); continue }
    flush()
    out.push(<p key={out.length}><Inline text={l} signedIn={signedIn} /></p>)
  }
  flush()
  return <>{out}</>
}

function Services({ signedIn }: { signedIn: boolean }) {
  return (
    <ul className="legal-services" aria-label="Outside services and what each one receives">
      {legal.services.map((s) => (
        <li key={s.name}>
          <b>{s.name}</b>
          <dl>
            <div><dt>Receives</dt><dd>{s.gets}</dd></div>
            <div><dt>When</dt><dd>{s.when}</dd></div>
            <div><dt>Control</dt><dd>{s.setting && signedIn ? <Link to={s.setting}>{s.control}</Link> : s.control}</dd></div>
          </dl>
        </li>
      ))}
    </ul>
  )
}

// useOwner: what this server's owner wrote about their own server.
function useOwner() {
  const [owner, setOwner] = useState<Owner | null>(null)
  useEffect(() => { api.get<{ owner: Owner }>('/legal/notices').then((r) => setOwner(r.owner)).catch(() => {}) }, [])
  return owner
}

function OwnerNotice({ owner }: { owner: Owner | null }) {
  const page = ownerNoticeSpec(owner)
  return page ? <SpecPage embedded page={page} /> : null
}

function Doc({ signedIn }: { signedIn: boolean }) {
  const loc = useLocation()
  const owner = useOwner()
  // After the page settles: arriving from a notice, the link names the part it's about.
  useEffect(() => {
    if (!loc.hash) return
    const f = requestAnimationFrame(() => document.getElementById(decodeURIComponent(loc.hash.slice(1)))?.scrollIntoView({ block: 'start' }))
    return () => cancelAnimationFrame(f)
  }, [loc.hash])
  return (
    <div className="legal-doc">
      <nav className="legal-toc" aria-label="On this page">
        <ul>{legal.sections.map((s) => <li key={s.id}><a href={`#${s.id}`} onClick={(e) => { e.preventDefault(); document.getElementById(s.id)?.scrollIntoView({ block: 'start' }) }}>{s.title}</a></li>)}</ul>
      </nav>
      <OwnerNotice owner={owner} />
      {legal.sections.map((s) => (
        <Section key={s.id} id={s.id} title={s.title}>
          <div className="set-prose"><Body lines={s.body} signedIn={signedIn} /></div>
          {s.id === 'privacy' && <Services signedIn={signedIn} />}
          {s.id === 'third-party' && signedIn && <><h3 className="legal-credit-head" id="poster-credits">Posters from Wikimedia Commons</h3><PosterCredits /></>}
          {s.id === 'trademarks' && signedIn && <><h3 className="legal-credit-head" id="logo-credits">Where the logos here came from</h3><LogoCredits /></>}
        </Section>
      ))}
      <p className="legal-updated muted">Last changed {legal.updated}. This page is plain-language information, not legal advice.</p>
    </div>
  )
}

// What a server's owner can add for the people using their server.
function OwnerForm() {
  const st = useServerSettings()
  const { s, set } = st
  if (!s) return st.failed ? <LoadError what="This page" onRetry={st.retry} /> : null
  return (
    <>
      <SpecPage embedded page={ownerFormSpec(s.legalOwnerContact || '')} handled={new Set(['contact'])} onField={(id, v) => { if (id === 'contact') set('legalOwnerContact', String(v)) }}
        after={{ 0: (
          <Field label="Your notice" hint={NOTICE_HELP}>
            <Textarea rows={4} maxLength={4000} value={s.legalOwnerNotice || ''} onChange={(e) => set('legalOwnerNotice', e.target.value)} />
          </Field>
        ) }} />
      <SaveBar {...st} />
    </>
  )
}

export default function LegalSettings() {
  const { user } = useApp()
  return (
    <>
      <PageHead title="Legal" desc="Terms of use, who’s responsible for what, what leaves this server, and the notices the law or a license asks for." />
      <Doc signedIn />
      {user?.isAdmin && <OwnerForm />}
    </>
  )
}

// The same page for someone with no account here.
export function LegalPublicPage() {
  const { user, brandName } = useApp()
  return (
    <div className="legal-public">
      <header className="legal-public-head">
        <div className="brand"><BrandMark /><Brand name={brandName} /></div>
        <h1>Legal</h1>
      </header>
      <Doc signedIn={!!user} />
    </div>
  )
}
