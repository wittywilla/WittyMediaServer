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
  useEffect(() => {
    const was = document.title
    document.title = `${guide.title}: Help`
    return () => { document.title = was }
  }, [guide.title])
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

  const intro = blocks.slice(0, firstSection(blocks)).filter((b) => !isContents(b))
  const parts = blocks.filter((b): b is Extract<Block, { t: 'heading' }> => b.t === 'heading' && b.level === 2)
  const at = allGuides.indexOf(guide)
  const next = allGuides[at + 1]
  return (
    <>
      <div className="help-back"><BackLink to="/help" label="All guides" /></div>
      <article className="help-prose" ref={article}>
        <GuideBody blocks={intro} signedIn={signedIn} />
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

// A list that's nothing but links to parts of the same guide.
const isContents = (b: Block) => b.t === 'list' && b.items.every((it) => it.length === 1 && it[0].t === 'p' && it[0].c.every((n) => n.t === 'link' ? n.href.startsWith('#') : n.t === 'text' && /^[\s,:;]*$/.test(n.v)) && it[0].c[0]?.t === 'link')

const firstSection = (blocks: Block[]) => {
  const i = blocks.findIndex((b, n) => n > 0 && b.t === 'heading')
  return i < 0 ? blocks.length : i
}

// A guide can be many screens of words with few links, and a remote has only arrows. So in a
// guide, up and down read on: they scroll while the next stop that way is off the screen, and
// move to it once it's in sight. tv.ts picks the nearest control on the screen, which here would
// be the bars that stay in view, so it only gets the arrows at the ends of the page and for
// moving sideways. Left and right also move a table or code too wide for the screen.
const STOPS = 'input, a[href], summary, button, [tabindex="0"]'

function useArrowScroll(article: React.RefObject<HTMLElement | null>) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.altKey || e.ctrlKey || e.metaKey) return
      const main = article.current?.closest('.help-main')
      const active = document.activeElement as HTMLElement | null
      if (!main || !active) return
      if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
        if (!active.matches('.help-table, .help-code pre') || !main.contains(active)) return
        const left = active.scrollLeft
        active.scrollLeft = left + (e.key === 'ArrowRight' ? 240 : -240)
        if (active.scrollLeft !== left) { e.preventDefault(); e.stopImmediatePropagation() }
        return
      }
      if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
      if (!main.contains(active) && active !== document.body && active.id !== 'main') return
      const down = e.key === 'ArrowDown'
      const h = window.innerHeight
      const stops = Array.from(main.querySelectorAll<HTMLElement>(STOPS)).filter((n) => n.getBoundingClientRect().height > 0)
      const at = stops.indexOf(active)
      let next: HTMLElement | undefined
      if (at >= 0) next = stops[at + (down ? 1 : -1)]
      else {
        // From a heading, or the page itself: the first stop past it, or past the screen's edge.
        const from = main.contains(active) ? active.getBoundingClientRect() : null
        next = down
          ? stops.find((n) => n.getBoundingClientRect().top >= (from ? from.bottom - 1 : 0))
          : stops.filter((n) => n.getBoundingClientRect().bottom <= (from ? from.top + 1 : h)).pop()
      }
      const box = next?.getBoundingClientRect()
      const inSight = !!box && (down ? box.top < h * 0.9 : box.bottom > h * 0.1)
      const canScroll = down ? document.documentElement.scrollHeight - window.scrollY - h > 1 : window.scrollY > 0
      if (!next && !canScroll) return
      e.preventDefault()
      e.stopImmediatePropagation()
      if (next && (inSight || !canScroll)) {
        next.focus({ preventScroll: true })
        next.scrollIntoView({ block: 'nearest' })
      } else window.scrollBy({ top: (down ? 1 : -1) * Math.round(h * 0.6) })
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [article])
}
