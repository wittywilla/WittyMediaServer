import { useEffect, useState, type CSSProperties } from 'react'
import { Link } from 'react-router-dom'
import { api, mediaUrlQ, type GameConsole, type GamePlaying, type GameShelfShape, type Item } from '../../api'
import { Placeholder } from '../../components/common'
import { Chip } from '../../ui'
import { IconGamepad, IconPlay, IconRight, IconStar, IconTune } from '../../icons'
import { cardCover, crispIfSmall, fmtAgo, fmtPlayed, saveThumb, useGameLinks, type GameContinue } from './data'
import './shelves.css'

// Shelves: the games home (WittyMediaClient's docs/WITTYPLAY-EXPERIENCE.md, the contract every
// device builds to). Each console is a shelf, and its games stand on it three ways, by choice:
// Cases (the default: each game in its console's kind of case, cover showing), Cartridges (each
// game as its cartridge, card or disc, cover on the label) and Shelf (spines, as books stand).
// Pressing one pulls it out, with Resume or Play under the shelf. A game in progress has a ribbon.
// Above the shelves is where you left off, the one obvious next step.
//
// Nothing here is a console maker's: cases and cartridges are drawn here as plain shapes in each
// console's proportions and materials, around the game's own cover; a spine is a tinted strip with
// the title, the console's short name and its era's grain, never a box's real spine.

export const ERA_NAMES: Record<string, string> = { first: 'The first consoles', '8-bit': '8-bit', '16-bit': '16-bit', 'early-3d': 'Early 3D', disc: 'Disc era', pocket: 'Pocket' }
// The grain behind a console's spines and band: the size of its era's pixel.
const ERA_GRAIN: Record<string, number> = { first: 14, '8-bit': 12, '16-bit': 10, 'early-3d': 6, disc: 5, pocket: 7 }

export const eraClass = (c?: GameConsole) => (c?.era ? `era-${c.era}` : '')
// The short name on a spine's foot: the console's own short name when it fits, else its ID.
const footName = (c?: GameConsole) => !c ? '' : c.short.length <= 5 ? c.short : c.id.replace(/^custom-/, '').toUpperCase().slice(0, 4)

// ---------------------------------------------------------------- the colour of a cover

// A cover's main colour from an 8-pixel copy, the colourful parts counting more: hue and
// saturation, kept for the page's life. Spines are many, so each asks for the smallest picture.
type Tint = [number, number]
const tints = new Map<number, Tint | null>()
const asking = new Map<number, Promise<void>>()

function sample(it: Item): Promise<void> {
  const had = asking.get(it.id)
  if (had) return had
  const p = new Promise<void>((done) => {
    const img = new Image()
    img.decoding = 'async'
    img.onload = () => {
      try {
        const c = document.createElement('canvas')
        c.width = c.height = 8
        const g = c.getContext('2d', { willReadFrequently: true })
        if (!g) throw new Error('no canvas')
        g.drawImage(img, 0, 0, 8, 8)
        const d = g.getImageData(0, 0, 8, 8).data
        let r = 0, gr = 0, b = 0, w = 0
        for (let i = 0; i < d.length; i += 4) {
          const mx = Math.max(d[i], d[i + 1], d[i + 2]), mn = Math.min(d[i], d[i + 1], d[i + 2])
          const k = 1 + ((mx - mn) / 255) * 6
          r += d[i] * k; gr += d[i + 1] * k; b += d[i + 2] * k; w += k
        }
        tints.set(it.id, w ? hueSat(r / w, gr / w, b / w) : null)
      } catch { tints.set(it.id, null) }
      done()
    }
    img.onerror = () => { tints.set(it.id, null); done() }
    img.src = `${cardCover(it).replace(/([?&])w=\d+/, '$1w=48')}`
  })
  asking.set(it.id, p)
  return p
}

function hueSat(r: number, g: number, b: number): Tint {
  r /= 255; g /= 255; b /= 255
  const mx = Math.max(r, g, b), mn = Math.min(r, g, b), l = (mx + mn) / 2, d = mx - mn
  if (!d) return [0, 0]
  const s = d / (1 - Math.abs(2 * l - 1))
  const h = mx === r ? ((g - b) / d + 6) % 6 : mx === g ? (b - r) / d + 2 : (r - g) / d + 4
  return [Math.round(h * 60), s]
}

// A game with no picture still gets a colour of its own, from its title.
function titleHue(t: string) {
  let h = 0
  for (const ch of t) h = (Math.imul(h, 31) + ch.charCodeAt(0)) >>> 0
  return h % 360
}

// Spines and bands keep at least 38% saturation, deep, with white words, whatever the theme:
// they're objects on a shelf, not panels.
export const deepColour = (it: Item | undefined) => {
  if (!it) return 'var(--bg-4)'
  const t = tints.get(it.id)
  if (!t) return `hsl(${titleHue(it.title)} 40% 30%)`
  return `hsl(${t[0]} ${Math.round(Math.max(t[1], 0.38) * 100)}% 30%)`
}

// useTints asks for the colours of a list of covers and draws again as they come.
export function useTints(items: Item[]) {
  const [, redraw] = useState(0)
  const key = items.map((it) => it.id).join(',')
  useEffect(() => {
    let on = true
    const want = items.filter((it) => it.poster && !tints.has(it.id))
    if (want.length) Promise.all(want.map(sample)).then(() => { if (on) redraw((n) => n + 1) })
    return () => { on = false }
  }, [key]) // eslint-disable-line react-hooks/exhaustive-deps
}

// ---------------------------------------------------------------- spines and shelves

type PullProps = { pulled: string | null; onPull: (key: string | null) => void }

// ---------------------------------------------------------------- the three views

export type ShelfView = 'cases' | 'carts' | 'spines'
const VIEW_KEY = 'wms.gm.shelfView'
export const SHELF_VIEWS: { id: ShelfView; label: string }[] = [{ id: 'cases', label: 'Cases' }, { id: 'carts', label: 'Cartridges' }, { id: 'spines', label: 'Shelf' }]

// The view is this device's choice, kept here only.
export function useShelfView(): [ShelfView, (v: ShelfView) => void] {
  const [view, setView] = useState<ShelfView>(() => {
    try { const v = localStorage.getItem(VIEW_KEY); if (v === 'carts' || v === 'spines') return v } catch { /* the default, then */ }
    return 'cases'
  })
  const set = (v: ShelfView) => { setView(v); try { localStorage.setItem(VIEW_KEY, v) } catch { /* for this visit only */ } }
  return [view, set]
}

export function ViewSwitch({ view, onView }: { view: ShelfView; onView: (v: ShelfView) => void }) {
  return (
    <div className="gm-views chips" role="group" aria-label="How games stand on the shelves">
      {SHELF_VIEWS.map((v) => <Chip key={v.id} selected={view === v.id} onClick={() => onView(v.id)}>{v.label}</Chip>)}
    </div>
  )
}

// A console's case and cartridge, from the server; for a console it doesn't describe, a box and
// a cartridge, or a keep case and a disc for disc consoles.
const caseOf = (c?: GameConsole): GameShelfShape => c?.shelf?.case || (c?.media === 'disc' ? { kind: 'keep', aspect: 0.71, height: 1 } : { kind: 'box', aspect: 0.72, height: 1 })
const cartOf = (c?: GameConsole): GameShelfShape => c?.shelf?.cart || (c?.media === 'disc' ? { kind: 'disc', aspect: 1, height: 0.66 } : { kind: 'cart', aspect: 0.88, height: 0.85 })

function Cover({ it, c }: { it: Item; c?: GameConsole }) {
  return it.poster ? <img src={cardCover(it)} alt="" decoding="async" loading="lazy" onLoad={crispIfSmall} /> : <Placeholder title={it.title} label={c?.short} />
}

// A game in its case, cover showing.
function GameCase({ it, c, ribbon, fav, shelf, pulled, onPull }: { it: Item; c?: GameConsole; ribbon: boolean; fav: boolean; shelf: string } & PullProps) {
  const key = `${shelf}:${it.id}`, on = pulled === key, sh = caseOf(c)
  return (
    <button type="button" className={`gm-case kind-${sh.kind}${on ? ' pulled' : ''}`} data-gm-item={it.id} style={{ '--aspect': sh.aspect, '--h': sh.height } as CSSProperties}
      aria-label={`${it.title}${c ? ', ' + c.short : ''}${ribbon ? ', in progress' : ''}${fav ? ', a favourite' : ''}${on ? ', chosen' : ''}`} aria-expanded={on}
      onClick={() => onPull(on ? null : key)}>
      <span className="gm-case-art"><Cover it={it} c={c} /></span>
      {ribbon && <i className="gm-ribbon" aria-hidden="true" />}
      {fav && <IconStar className="gm-item-fav" size={12} />}
    </button>
  )
}

// A game as its cartridge, card or disc, cover on the label.
function GameCart({ it, c, ribbon, fav, shelf, pulled, onPull }: { it: Item; c?: GameConsole; ribbon: boolean; fav: boolean; shelf: string } & PullProps) {
  const key = `${shelf}:${it.id}`, on = pulled === key, sh = cartOf(c)
  return (
    <button type="button" className={`gm-cart kind-${sh.kind}${on ? ' pulled' : ''}`} data-gm-item={it.id} style={{ '--aspect': sh.aspect, '--h': sh.height } as CSSProperties}
      aria-label={`${it.title}${c ? ', ' + c.short : ''}${ribbon ? ', in progress' : ''}${fav ? ', a favourite' : ''}${on ? ', chosen' : ''}`} aria-expanded={on}
      onClick={() => onPull(on ? null : key)}>
      <span className="gm-cart-label"><Cover it={it} c={c} /></span>
      {ribbon && <i className="gm-ribbon" aria-hidden="true" />}
      {fav && <IconStar className="gm-item-fav" size={12} />}
    </button>
  )
}

function Spine({ it, c, ribbon, fav, shelf, pulled, onPull }: { it: Item; c?: GameConsole; ribbon: boolean; fav: boolean; shelf: string } & PullProps) {
  const key = `${shelf}:${it.id}`
  const width = 34 + Math.min(14, Math.round(it.title.length * 0.55))
  if (pulled === key) return <FaceOut it={it} c={c} ribbon={ribbon} shelf={shelf} pulled={pulled} onPull={onPull} from={width} />
  const style = { '--sw': width, '--tint': deepColour(it), '--grain': `${ERA_GRAIN[c?.era || ''] || 10}px` } as CSSProperties
  return (
    <button type="button" className={`gm-spine media-${c?.media || 'cart'}`} style={style} data-gm-item={it.id}
      aria-label={`${it.title}${c ? ', ' + c.short : ''}${ribbon ? ', in progress' : ''}${fav ? ', a favourite' : ''}`}
      onClick={() => onPull(key)}>
      {ribbon && <i className="gm-ribbon" aria-hidden="true" />}
      {fav && <IconStar className="gm-spine-fav" size={12} />}
      <span className="gm-spine-title" aria-hidden="true">{it.title}</span>
      <span className="gm-spine-foot" aria-hidden="true">{footName(c)}</span>
    </button>
  )
}

function FaceOut({ it, c, ribbon, shelf, pulled, onPull, from }: { it: Item; c?: GameConsole; ribbon: boolean; shelf: string; from?: number } & PullProps) {
  const key = `${shelf}:${it.id}`
  const isPulled = pulled === key
  return (
    <button type="button" className={`gm-faceout media-${c?.media || 'cart'}${isPulled ? ' pulled' : ''}`} data-gm-item={it.id}
      style={from ? { '--sw0': `${from}px` } as CSSProperties : undefined}
      aria-label={`${it.title}${c ? ', ' + c.short : ''}${ribbon ? ', in progress' : ''}${isPulled ? ', chosen' : ''}`} aria-expanded={isPulled}
      onClick={() => onPull(isPulled ? null : key)}>
      <span className="gm-faceout-art">
        {it.poster ? <img src={cardCover(it)} alt="" decoding="async" loading="lazy" onLoad={crispIfSmall} /> : <Placeholder title={it.title} label={c?.short} />}
      </span>
      {ribbon && <i className="gm-ribbon" aria-hidden="true" />}
    </button>
  )
}

function PulledInfo({ it, c, cont }: { it: Item; c?: GameConsole; cont?: GameContinue }) {
  const links = useGameLinks()
  const ready = c?.ready ?? true
  const facts = [c?.short, it.studio, it.genres?.[0]].filter(Boolean)
  return (
    <div className="gm-pulled" role="group" aria-label={it.title}>
      <b>{it.title}</b>
      <small>{facts.join(' · ')}</small>
      {cont && <small className="gm-pulled-left"><i aria-hidden="true" />Where you left off: {fmtAgo(cont.lastPlayed)}</small>}
      <div className="gm-pulled-actions">
        {ready
          ? <Link className="btn primary sm pill" to={links.play(it.id, cont?.save ? 'resume=1' : '')}><IconPlay size={16} /> {cont?.save ? 'Resume' : 'Play'}</Link>
          : <Link className="btn sm pill" to={links.item(it.id)}><IconTune size={16} /> Can’t play yet</Link>}
        <Link className="btn ghost sm pill" to={links.item(it.id)}>About</Link>
      </div>
    </div>
  )
}

export type ShelfProps = {
  id: string
  title: string
  items: Item[]
  consoles: Map<string, GameConsole>
  // The console this shelf is, for its name's type and its era.
  console?: GameConsole
  // Its first game faces out, as a shop's best one does.
  faceFirst?: boolean
  // Long shelves get a divider at each new letter.
  letters?: boolean
  count?: number
  to?: string
  favs: Set<number>
  progress: Map<number, GameContinue>
  view: ShelfView
} & PullProps

export function Shelf({ id, title, items, consoles, console: c, faceFirst, letters, count, to, favs, progress, view, pulled, onPull }: ShelfProps) {
  useTints(items)
  if (items.length === 0) return null
  const chosen = pulled?.startsWith(id + ':') ? items.find((it) => `${id}:${it.id}` === pulled) : undefined
  const out: React.ReactNode[] = []
  items.forEach((it, i) => {
    if (letters && view === 'spines') {
      const l = initial(it.title), before = i ? initial(items[i - 1].title) : ''
      if (l !== before) out.push(<span key={'l' + l} className="gm-divider" aria-hidden="true">{l}</span>)
    }
    const ic = consoles.get(it.console || '')
    const ribbon = !!progress.get(it.id)?.save
    const fav = favs.has(it.id)
    out.push(view === 'cases' ? <GameCase key={it.id} it={it} c={ic} ribbon={ribbon} fav={fav} shelf={id} pulled={pulled} onPull={onPull} />
      : view === 'carts' ? <GameCart key={it.id} it={it} c={ic} ribbon={ribbon} fav={fav} shelf={id} pulled={pulled} onPull={onPull} />
      : i === 0 && faceFirst && pulled !== `${id}:${it.id}` ? <FaceOut key={it.id} it={it} c={ic} ribbon={ribbon} shelf={id} pulled={pulled} onPull={onPull} />
      : <Spine key={it.id} it={it} c={ic} ribbon={ribbon} fav={fav} shelf={id} pulled={pulled} onPull={onPull} />)
  })
  return (
    <section className="gm-shelf gm-section" data-gm-section aria-label={title}>
      <div className="gm-shelf-head" data-nav-head>
        <h2 className={`gm-shelf-name ${eraClass(c)}`}>{title}</h2>
        {c?.era && <span className="gm-shelf-era">{ERA_NAMES[c.era]}</span>}
        {count != null && <span className="gm-shelf-count">{count.toLocaleString()}</span>}
        {/* A console this server can't play yet: reason gpu (from the server games work) means it never will. */}
        {c && !c.ready && <span className="gm-shelf-note">{(c as GameConsole & { reason?: string }).reason === 'gpu' ? 'Not on this server' : 'Needs setting up'}</span>}
        {to && <Link className="gm-shelf-all" to={to} aria-label={`All of ${title}`}>All <IconRight size={14} /></Link>}
      </div>
      <div className={`gm-ledge view-${view}`} data-nav-row>{out}</div>
      {chosen && <PulledInfo it={chosen} c={consoles.get(chosen.console || '')} cont={progress.get(chosen.id)} />}
    </section>
  )
}

const initial = (t: string) => { const l = t.trim().charAt(0).toUpperCase(); return /[A-Z]/.test(l) ? l : '#' }

// ---------------------------------------------------------------- where you left off

export function ResumeCard({ e, c }: { e: GameContinue; c?: GameConsole }) {
  const links = useGameLinks()
  const it = e.item
  const played = it.progress?.positionSec || 0
  const pic = e.save?.hasThumb ? saveThumb(it.id, e.save.slot, e.save.updatedAt) : it.poster ? cardCover(it) : ''
  const ready = c?.ready ?? true
  return (
    <Link className="gm-resume-card" to={ready ? links.play(it.id, e.save ? 'resume=1' : '') : links.item(it.id)} data-gm-item={it.id} data-gm-section
      aria-label={`${e.save ? 'Resume' : 'Play'} ${it.title}${c ? ', ' + c.short : ''}, last played ${fmtAgo(e.lastPlayed)}`}>
      <span className="gm-resume-pic">{pic ? <img src={pic} alt="" decoding="async" onLoad={crispIfSmall} /> : <Placeholder title={it.title} label={c?.short} />}</span>
      {e.save && <i className="gm-ribbon" aria-hidden="true" />}
      <span className="gm-resume-text">
        <small>Where you left off</small>
        <b>{it.title}</b>
        <span>{[c?.short, played > 60 ? fmtPlayed(played) : '', fmtAgo(e.lastPlayed)].filter(Boolean).join(' · ')}</span>
      </span>
      <span className="btn primary sm pill gm-resume-go" aria-hidden="true"><IconPlay size={16} /> {e.save ? 'Resume' : 'Play'}</span>
    </Link>
  )
}

// The other games in progress, smaller, each with its ribbon.
export function OnTheGo({ list, consoles }: { list: GameContinue[]; consoles: Map<string, GameConsole> }) {
  const links = useGameLinks()
  if (list.length === 0) return null
  return (
    <section className="gm-onthego gm-section" data-gm-section data-nav-row aria-label="Continue playing">
      {list.map((e) => {
        const it = e.item, c = consoles.get(it.console || '')
        const pic = e.save?.hasThumb ? saveThumb(it.id, e.save.slot, e.save.updatedAt) : it.poster ? cardCover(it) : ''
        const played = it.progress?.positionSec || 0
        return (
          <Link key={it.id} className="gm-otg" to={c?.ready === false ? links.item(it.id) : links.play(it.id, e.save ? 'resume=1' : '')} data-gm-item={it.id}
            aria-label={`${e.save ? 'Resume' : 'Play'} ${it.title}, last played ${fmtAgo(e.lastPlayed)}`}>
            <span className="gm-otg-pic">{pic ? <img src={pic} alt="" decoding="async" loading="lazy" onLoad={crispIfSmall} /> : <Placeholder title={it.title} label={c?.short} />}</span>
            {e.save && <i className="gm-ribbon" aria-hidden="true" />}
            <b>{it.title}</b>
            <small>{[played > 60 ? fmtPlayed(played) : '', fmtAgo(e.lastPlayed)].filter(Boolean).join(' · ')}</small>
          </Link>
        )
      })}
    </section>
  )
}

// ---------------------------------------------------------------- a console's collection

export function CollectionBand({ c, count, first }: { c: GameConsole; count?: number; first?: Item }) {
  useTints(first ? [first] : [])
  const style = { '--tint': deepColour(first), '--grain': `${ERA_GRAIN[c.era || ''] || 10}px` } as CSSProperties
  return (
    <header className="gm-band" style={style}>
      <span className="gm-band-kicker">{c.era ? `${ERA_NAMES[c.era]} · ` : ''}Collection</span>
      <h1 className={`gm-band-name ${eraClass(c)}`}>{c.short}</h1>
      <span className="gm-band-facts">{[c.name !== c.short ? c.name : '', count != null ? `${count.toLocaleString()} ${count === 1 ? 'game' : 'games'}` : ''].filter(Boolean).join(' · ')}</span>
    </header>
  )
}

// What's yours for one console, where people look for it.
export function YoursOn({ c }: { c: GameConsole }) {
  return (
    <section className="gm-yours gm-section" aria-label={`Yours on ${c.short}`}>
      <h2>Yours on {c.short}</h2>
      <div className="gm-yours-list">
        <Link to={`/settings/games?console=${encodeURIComponent(c.id)}`}><IconTune size={18} /><span>Look and settings for {c.short} games</span><IconRight size={16} /></Link>
        <Link to={`/controllers?console=${encodeURIComponent(c.id)}`}><IconGamepad size={18} /><span>Controls for {c.short} games</span><IconRight size={16} /></Link>
      </div>
    </section>
  )
}

// ---------------------------------------------------------------- the dock

// A game of this account's running on another device: one press moves it here, from where it is.
export function GamesDock() {
  const links = useGameLinks()
  const [playing, setPlaying] = useState<GamePlaying | null>(null)
  useEffect(() => {
    let on = true
    const look = () => api.get<GamePlaying | null>('/games/playing').then((p) => { if (on) setPlaying(p && p.itemId ? p : null) }).catch(() => {})
    void look()
    const t = window.setInterval(look, 30000)
    return () => { on = false; window.clearInterval(t) }
  }, [])
  if (!playing) return null
  const pic = mediaUrlQ(`/api/items/${playing.itemId}/poster`, 'w=96')
  return (
    <div className="gm-dock" role="region" aria-label={`${playing.title} is playing${playing.device ? ' on ' + playing.device : ''}`}>
      <img src={pic} alt="" />
      <span className="gm-dock-text"><b>{playing.title}</b><small><i aria-hidden="true" />{playing.device || 'Another device'}</small></span>
      <Link className="btn sm pill" to={links.play(playing.itemId, 'resume=1')}>Continue here</Link>
    </div>
  )
}
