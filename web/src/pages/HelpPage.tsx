import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, NavLink, useLocation, useParams, useSearchParams } from 'react-router-dom'
import { BackLink, Spinner, VersionTag } from '../components/common'
import { allGuides, GROUPS, guideBySlug, loadGuide, slugOf, type Guide } from '../help/guides'
import { GuideBody } from '../help/GuideBody'
import { parse, plain, type Block } from '../help/markdown'
import { search, type Hit } from '../help/search'
import { IconRight, IconSearch } from '../icons'
import { useApp } from '../state'
import '../help/help.css'

// Help: the guides from docs/, built into the web app, so it all works with no internet and
// with no module installed. Anyone can read it, signed in or not, since the guides are public.
// /help is the list and the search, /help/<guide> is a guide.

export default function HelpPage() {
  const { user, version } = useApp()
  const { guide } = useParams()
  const [params, setParams] = useSearchParams()
  const q = params.get('q') ?? ''
  const current = guide ? guideBySlug(guide) : undefined
  const setQ = (v: string) => setParams(v ? { q: v } : {}, { replace: true })
  const onGuide = !!guide && !q
  return (
    <div className={'help' + (onGuide ? ' on-guide' : '')}>
      <nav className="help-side" aria-label="Help">
        <div className="help-side-title"><Link to="/help">Help</Link></div>
        {GROUPS.map((g) => (
          <div key={g.title} className="help-side-group">
            <div className="help-side-head">{g.title}</div>
            {g.guides.map((it) => <NavLink key={it.file} to={`/help/${slugOf(it.file)}`} className="help-side-link">{it.title}</NavLink>)}
          </div>
        ))}
      </nav>
      <div className="help-main">
        <SearchBox q={q} onChange={setQ} />
        {q ? <Results q={q} />
          : !guide ? <Index version={version} />
          : current ? <GuideView guide={current} signedIn={!!user} />
          : <NotFound />}
      </div>
    </div>
  )
}

// The same page for someone with no account here yet, or in the middle of setting up.
export function HelpPublicPage() {
  return (
    <div className="help-public">
      <header className="help-public-head"><Link to="/" className="back-link">Back to the sign-in page</Link></header>
      <main id="main" tabIndex={-1}><HelpPage /></main>
    </div>
  )
}

function SearchBox({ q, onChange }: { q: string; onChange: (v: string) => void }) {
  const [text, setText] = useState(q)
  useEffect(() => { setText(q) }, [q])
  useEffect(() => {
    if (text === q) return
    const t = setTimeout(() => onChange(text.trim()), 200)
    return () => clearTimeout(t)
  }, [text]) // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <div className="search help-search" role="search">
      <IconSearch />
      <input aria-label="Search help" type="search" placeholder="Search help…" value={text} onChange={(e) => setText(e.target.value)} />
    </div>
  )
}

function Index({ version }: { version: string }) {
  return (
    <>
      <header className="set-head help-head">
        <div>
          <h1>Help</h1>
          <p>Everything about this server, built in. It works with no internet connection, and it matches the version you’re running: <VersionTag version={version} /></p>
        </div>
      </header>
      <ul className="help-quick">
        <li><Link to="/help/getting-started"><b>New here?</b><span>Start with Getting started.</span></Link></li>
        <li><Link to="/help/troubleshooting"><b>Something’s wrong?</b><span>Look it up by what you’re seeing.</span></Link></li>
        <li><Link to="/help/settings"><b>Looking for a setting?</b><span>Every settings page, one by one.</span></Link></li>
      </ul>
      {GROUPS.map((g) => (
        <section key={g.title} className="help-group" aria-label={g.title}>
          <h2>{g.title}</h2>
          <ul>
            {g.guides.map((it) => (
              <li key={it.file}><Link to={`/help/${slugOf(it.file)}`} className="help-row"><b>{it.title}</b><span>{it.about}</span></Link></li>
            ))}
          </ul>
        </section>
      ))}
    </>
  )
}

function Results({ q }: { q: string }) {
  const [hits, setHits] = useState<Hit[] | null>(null)
  const [failed, setFailed] = useState(false)
  useEffect(() => {
    let live = true
    setFailed(false)
    search(q).then((h) => { if (live) setHits(h) }).catch(() => { if (live) setFailed(true) })
    return () => { live = false }
  }, [q])
  if (failed) return <p role="status">The search couldn’t load the guides. You can still pick one from the list.</p>
  if (!hits) return <div role="status" aria-label="Searching"><Spinner /></div>
  return (
    <>
      <h1 className="help-results-title">Search help</h1>
      <p className="muted" role="status">{hits.length === 0 ? `Nothing found for “${q}”. Try fewer words, or a different word for the same thing.` : `${hits.length}${hits.length === 40 ? ' or more' : ''} ${hits.length === 1 ? 'place' : 'places'} for “${q}”`}</p>
      <ul className="help-hits">
        {hits.map((h) => (
          <li key={h.to}>
            <Link to={h.to} className="help-row">
              <b>{h.heading === h.guide.title ? h.guide.title : `${h.guide.title}: ${h.heading}`}</b>
              <span>{h.before}{h.match && <mark>{h.match}</mark>}{h.after}</span>
            </Link>
          </li>
        ))}
      </ul>
    </>
  )
}

function NotFound() {
  return (
    <>
      <h1>There’s no guide here</h1>
      <p>It may have moved. <Link to="/help">See all the guides</Link>, or search above.</p>
    </>
  )
}

function GuideView({ guide, signedIn }: { guide: Guide; signedIn: boolean }) {
  const [md, setMd] = useState<{ file: string; text: string } | null>(null)
  const [err, setErr] = useState('')
  const { hash } = useLocation()
  useEffect(() => {
    let live = true
    setErr('')
    loadGuide(guide).then((text) => { if (live) setMd({ file: guide.file, text }) }).catch(() => { if (live) setErr('This guide couldn’t be loaded. The server may have been updated: reload the page.') })
    return () => { live = false }
  }, [guide])
  const blocks = useMemo<Block[]>(() => (md && md.file === guide.file ? parse(md.text) : []), [md, guide.file])
  const article = useRef<HTMLElement>(null)
  useArrowScroll(article)

  // Go to the part the address names, once it's there, and tell a screen reader where it landed.
  const ready = blocks.length > 0
  useEffect(() => {
    if (!ready) return
    if (!hash) { window.scrollTo({ top: 0 }); return }
    const el = document.getElementById(decodeURIComponent(hash.slice(1)))
    if (el) { el.scrollIntoView(); el.focus({ preventScroll: true }) }
  }, [ready, hash, guide.file])

  if (err) return <p role="alert">{err}</p>
  if (!ready) return <div role="status" aria-label="Loading"><Spinner /></div>

  const parts = blocks.filter((b): b is Extract<Block, { t: 'heading' }> => b.t === 'heading' && b.level === 2)
  const at = allGuides.indexOf(guide)
  const next = allGuides[at + 1]
  return (
    <>
      <div className="help-back"><BackLink to="/help" label="All guides" /></div>
      <article className="help-prose" ref={article}>
        <GuideBody blocks={blocks.slice(0, firstSection(blocks))} signedIn={signedIn} />
        {parts.length >= 3 && (
          <details className="help-toc" open={parts.length <= 12}>
            <summary>On this page</summary>
            <ul>{parts.map((p) => <li key={p.id}><Link to={`#${p.id}`}>{plain(p.c)}</Link></li>)}</ul>
          </details>
        )}
        <GuideBody blocks={blocks.slice(firstSection(blocks))} signedIn={signedIn} />
      </article>
      {next && (
        <p className="help-next">
          <Link to={`/help/${slugOf(next.file)}`} className="help-row"><span>Next guide</span><b>{next.title} <IconRight size={16} /></b></Link>
        </p>
      )}
    </>
  )
}

const firstSection = (blocks: Block[]) => {
  const i = blocks.findIndex((b, n) => n > 0 && b.t === 'heading')
  return i < 0 ? blocks.length : i
}

// A guide can be many screens of words with nothing to focus between its links, and a remote
// has only arrows. So up and down read on: they scroll while the next link in that direction
// is off the screen, and move to it once it's in sight. tv.ts takes the arrows otherwise.
function useArrowScroll(article: React.RefObject<HTMLElement | null>) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.key !== 'ArrowDown' && e.key !== 'ArrowUp') || e.altKey || e.ctrlKey || e.metaKey) return
      const el = article.current
      const active = document.activeElement as HTMLElement | null
      const inside = !!active && el?.contains(active)
      if (!el || (active && active !== document.body && active.id !== 'main' && !inside)) return
      const down = e.key === 'ArrowDown'
      const h = window.innerHeight
      const from = inside ? active!.getBoundingClientRect() : null
      const nextInSight = Array.from(el.querySelectorAll<HTMLElement>('a[href], summary, button, [tabindex="0"]')).some((n) => {
        const r = n.getBoundingClientRect()
        if (r.height === 0) return false
        return down
          ? r.top < h * 0.9 && r.top >= (from ? from.bottom - 1 : 0)
          : r.bottom > h * 0.1 && r.bottom <= (from ? from.top + 1 : h)
      })
      if (nextInSight) return
      const box = el.getBoundingClientRect()
      if (down ? box.bottom <= h : box.top >= 0) return
      e.preventDefault()
      e.stopImmediatePropagation()
      window.scrollBy({ top: (down ? 1 : -1) * Math.round(h * 0.6) })
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [article])
}
