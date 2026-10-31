import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, posterUrl, type Item, type Library } from '../api'
import { BackLink, EmptyState, Placeholder, SkeletonGrid, Spinner } from '../components/common'
import { IconGamepad, IconPlay, IconSettings } from '../icons'
import { toast, useApp } from '../state'
import './rigs.css'

// A gaming PC's games: the library page, and a game's own page. The game runs on that PC.
// This server only works out what each device can do to get there: see docs/GAME-RIGS.md.

export type RigView = {
  id: number; libraryId: number; name: string; paired: boolean; state: string; message: string; currentGame?: string
  canWake: boolean; canQuit: boolean; browserPlay: boolean
  address?: string; port?: number; remoteAddress?: string; version?: string; fingerprint?: string; hasMac?: boolean; public?: boolean; webUrl?: string
}

type Method = { kind: string; label: string; oneClick: boolean; note?: string; url?: string; args?: string[]; text?: string; steps?: string[] }
type Plan = {
  rig: RigView; game: { id: number; title: string }; away: boolean; ready: boolean; message?: string; methods: Method[]
  install?: { name: string; url: string; note?: string; license: string; source: string }; setup?: string[]
}

type Moonlight = { status(): Promise<{ installed: boolean }>; launch(args: string[]): Promise<{ ok: boolean; error?: string }> }
const moonlightApp = (): Moonlight | undefined => (window as unknown as { wittyClient?: { moonlight?: Moonlight } }).wittyClient?.moonlight

function device(): { client: string; os: string } {
  const ua = navigator.userAgent
  let os = ''
  if (/android/i.test(ua)) os = 'android'
  else if (/iphone|ipad|ipod/i.test(ua) || (/macintosh/i.test(ua) && navigator.maxTouchPoints > 1)) os = 'ios'
  else if (/cros/i.test(ua)) os = 'chromeos'
  else if (/windows/i.test(ua)) os = 'windows'
  else if (/macintosh/i.test(ua)) os = 'mac'
  else if (/linux/i.test(ua)) os = 'linux'
  return { client: moonlightApp() ? 'desktop' : 'browser', os }
}

// Follows one rig, more often just after someone tried to wake it. Only while the page shows.
function useRig(libraryId: number) {
  const [rig, setRig] = useState<RigView | null>(null)
  const [fast, setFast] = useState(0)
  const load = () => api.get<RigView[]>('/rigs').then((list) => setRig(list.find((r) => r.libraryId === libraryId) || null)).catch(() => {})
  useEffect(() => {
    load()
    const t = window.setInterval(() => { if (!document.hidden) load() }, fast > Date.now() ? 4000 : 15000)
    return () => clearInterval(t)
  }, [libraryId, fast]) // eslint-disable-line react-hooks/exhaustive-deps
  return { rig, reload: load, watchWake: () => setFast(Date.now() + 120000) }
}

function RigStatus({ rig, reload, watchWake }: { rig: RigView; reload: () => void; watchWake: () => void }) {
  const { user } = useApp()
  const [busy, setBusy] = useState(false)
  const wake = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ note?: string }>(`/rigs/${rig.id}/wake`)
      toast(r.note || 'A wake-up signal was sent.')
      watchWake()
    } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  const quit = async () => {
    setBusy(true)
    try { await api.post(`/rigs/${rig.id}/quit`); reload() } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  const ok = rig.state === 'online'
  return (
    <div className={'rig-status' + (ok ? ' ok' : '')} role="status">
      <span className="rig-dot" aria-hidden="true" />
      <div className="grow">
        <strong>{rig.name}:</strong>{' '}
        {rig.state === 'busy' && rig.currentGame ? `is running ${rig.currentGame}.` : rig.message}
        {rig.state === 'asleep' && !rig.canWake && user?.isAdmin && !rig.hasMac && <span className="muted"> Waking it from here needs it to have been paired and on once.</span>}
      </div>
      {rig.canWake && <button className="btn sm" disabled={busy} onClick={wake}>Wake it up</button>}
      {rig.canQuit && <button className="btn sm ghost" disabled={busy} onClick={quit}>End what’s running</button>}
      {user?.isAdmin && !rig.paired && <Link className="btn sm" to="/settings/rigs"><IconSettings size={15} /> Pair it</Link>}
    </div>
  )
}

function RigCard({ item, rig }: { item: Item; rig?: string }) {
  return (
    <Link to={`/item/${item.id}`} className="card" title={item.title}>
      <div className="poster">
        {item.poster ? <img src={posterUrl(item)} alt="" loading="lazy" /> : <Placeholder title={item.title} label="PC" />}
        <div className="play"><span><IconPlay /></span></div>
      </div>
      <div className="title">{item.title}</div>
      <div className="sub">{rig ? `Plays on ${rig}` : 'Plays on your PC'}</div>
    </Link>
  )
}

// The library page of a gaming PC. Games are ordinary items, so this list obeys every limit
// the account has, and a Kids profile sees only what it's been given.
export default function RigLibrary({ lib }: { lib: Library }) {
  const [items, setItems] = useState<Item[] | null>(null)
  const { rig, reload, watchWake } = useRig(lib.id)
  const { user } = useApp()
  useEffect(() => {
    api.get<{ items: Item[] }>(`/items?library=${lib.id}&kind=game&sort=title&limit=500`).then((r) => setItems(r.items)).catch(() => setItems([]))
  }, [lib.id, rig?.state])
  return (
    <>
      <div className="toolbar">
        <h1>{lib.name}</h1>
        {items && <span className="count">{items.length} {items.length === 1 ? 'game' : 'games'}, played on this PC</span>}
        <div className="grow" />
        {user?.isAdmin && <Link className="btn ghost sm" to="/settings/rigs"><IconSettings size={15} /> Manage</Link>}
      </div>
      {rig && <RigStatus rig={rig} reload={reload} watchWake={watchWake} />}
      {!items ? <SkeletonGrid /> : items.length === 0
        ? <EmptyState icon={<IconGamepad />} title="No games to show here" text={rig?.paired === false ? 'This PC’s games appear here once it’s paired.' : 'Games appear here from the list in Sunshine on that PC.'} />
        : <div className="grid">{items.map((it) => <RigCard key={it.id} item={it} rig={lib.name} />)}</div>}
    </>
  )
}

// A game on a gaming PC.
export function RigGameDetails({ it }: { it: Item }) {
  const { libraries } = useApp()
  const { rig, reload, watchWake } = useRig(it.libraryId)
  const [plan, setPlan] = useState<Plan | null>(null)
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState('')
  const [appMissing, setAppMissing] = useState(false)
  const moonlight = moonlightApp()
  const planRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => { if (moonlight) moonlight.status().then((s) => setAppMissing(!s.installed)).catch(() => {}) }, [moonlight])
  const play = async () => {
    setBusy(true)
    setNote('')
    // A window has to be opened during the press, or browsers block it. It's filled in below,
    // and closed again if browser play turns out not to be on offer.
    const tab = rig?.browserPlay && !moonlight ? window.open('', '_blank') : null
    try {
      const p = await api.post<Plan>(`/rigs/games/${it.id}/play`, device())
      setPlan(p)
      const first = p.ready ? p.methods[0] : undefined
      if (first?.kind === 'bridge' && tab) {
        tab.opener = null
        tab.location.href = first.url!
        return
      }
      tab?.close()
      if (first?.kind === 'command' && first.oneClick && moonlight && !appMissing) await launch(first)
      else window.setTimeout(() => planRef.current?.focus(), 0)
    } catch (e: any) {
      tab?.close()
      toast(e.message)
    } finally { setBusy(false) }
  }
  const launch = async (m: Method) => {
    if (!moonlight || !m.args) return
    const r = await moonlight.launch(m.args)
    if (r.ok) setNote('Opening Moonlight…')
    else if (r.error) setNote(r.error)
  }
  const start = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ steps: string[] }>(`/rigs/games/${it.id}/start`)
      setPlan((p) => p && { ...p, methods: [{ kind: 'started', label: 'It’s started on the PC', oneClick: false, steps: r.steps }, ...p.methods.filter((m) => m.kind !== 'started')] })
      reload()
    } catch (e: any) { toast(e.message) } finally { setBusy(false) }
  }
  const copy = async (text: string) => {
    try { await navigator.clipboard.writeText(text); toast('Copied') } catch { toast('Couldn’t copy. Select the text instead.') }
  }
  const ready = !rig || rig.state === 'online' || rig.state === 'busy'
  return (
    <div className="game-page">
      <BackLink to={`/library/${it.libraryId}`} label={libraries.find((l) => l.id === it.libraryId)?.name || 'Library'} />
      <div className="game-head">
        <div className="poster game-poster">
          {it.poster ? <img src={posterUrl(it)} alt="" /> : <Placeholder title={it.title} label="PC" />}
        </div>
        <div className="game-info">
          <h1>{it.title}</h1>
          <p className="muted">{[it.tagline, it.year].filter(Boolean).join(' · ')}</p>
          {it.overview && <p>{it.overview}</p>}
          {rig && <RigStatus rig={rig} reload={reload} watchWake={watchWake} />}
          <div className="game-actions">
            <button className="btn primary" disabled={busy || !ready} onClick={play}>{busy ? <Spinner /> : <IconPlay />} Play</button>
          </div>
          {note && <p className="game-note" role="status">{note}</p>}
        </div>
      </div>
      {plan && (
        <div className="rig-plan" ref={planRef} tabIndex={-1} aria-label="Ways to play">
          {plan.message && <p className="game-note" role="status">{plan.message}</p>}
          {plan.methods.map((m, i) => (
            <section key={i} className="rig-method">
              {m.kind === 'bridge' && <a className="btn" href={m.url} target="_blank" rel="noreferrer noopener"><IconPlay /> {m.label}</a>}
              {m.kind === 'command' && moonlight && !appMissing && m.args && <button className="btn" onClick={() => launch(m)}><IconPlay /> {m.label}</button>}
              {m.kind === 'command' && !moonlight && m.text && <>
                <h2>{m.label}</h2>
                <code className="rig-command">{m.text}</code>
                <button className="btn sm" onClick={() => copy(m.text!)}>Copy</button>
              </>}
              {m.kind === 'started' && rig?.state === 'busy' && <>
                <h2>{m.label}</h2>
                <ol>{m.steps?.map((s) => <li key={s}>{s}</li>)}</ol>
              </>}
              {m.kind === 'steps' && <>
                <h2>{m.label}</h2>
                <ol>{m.steps?.map((s) => <li key={s}>{s}</li>)}</ol>
                {device().client === 'browser' && plan.ready && rig?.state !== 'busy' && <button className="btn sm ghost" disabled={busy} onClick={start}>Start it on the PC first</button>}
              </>}
              {m.note && <p className="muted">{m.note}</p>}
            </section>
          ))}
          {plan.install && (appMissing || !moonlight) && (
            <section className="rig-method">
              <h2>{appMissing ? 'Moonlight isn’t installed on this computer' : 'Don’t have Moonlight yet?'}</h2>
              <p><a href={plan.install.url} target="_blank" rel="noreferrer noopener">Get {plan.install.name}</a>{plan.install.note ? `. ${plan.install.note}` : ''}</p>
              <p className="muted">Moonlight is a free, separate app from its own project (license: {plan.install.license}, <a href={plan.install.source} target="_blank" rel="noreferrer noopener">source</a>).</p>
            </section>
          )}
          {plan.setup && (
            <details className="rig-method">
              <summary>First time on this device?</summary>
              <ol>{plan.setup.map((s) => <li key={s}>{s}</li>)}</ol>
            </details>
          )}
        </div>
      )}
    </div>
  )
}
