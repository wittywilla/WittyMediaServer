// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// WittyPlay's game pictures in use: the choices (the person's own in /me/ui, the server's from GET /api/games/pictures), what a game card
// draws for them (its box art fitted, or the game in its case: GameCase.tsx), and the one-game chooser, opened from a game's detail panel
// and offered to any page that wants it (GamePictureButton). The rules are gamePictures.ts.
import { useCallback, useEffect, useId, useMemo, useState, type ReactNode } from 'react'
import { api, type GameConsole, type GameDetailsInfo } from '../api'
import { logoImageUrl } from '../components/BrandLogo'
import { Placeholder } from '../components/common'
import { getConsoles } from '../pages/games/consoles'
import { useApp } from '../state'
import { Button, Dialog, Segmented } from '../ui'
import { useUserPref } from '../ui/userPrefs'
import type { GameArtApi, GameArtArgs } from '../sdui/render/glyphs'
import { useBrandLogos } from './brandLogos'
import { FitArt, GameCase } from './GameCase'
import {
  CASE_STYLES, FAMILY_VARIANTS, LABELS, MEDIA, PICTURES, PLAY_PICTURES_KEY, VARIANTS, cleanLook, emptyLook, familyVariant, lookFor, mediaDef, mediaFor,
  picturesPref, resolveLook, styleDef, stylesOf, usualStyle, withFamilyVariant, withLook, withMediaStyle,
  type CaseStyle, type LabelPick, type Look, type MediaId, type PicturesPref, type Resolved, type ServerPictures, type Variant,
} from './gamePictures'

// ---- the choices ----

export function usePicturesPref(): [PicturesPref, (p: PicturesPref) => void] {
  const [saved, save] = useUserPref<unknown>(PLAY_PICTURES_KEY, null)
  const pref = useMemo(() => picturesPref(saved), [saved])
  return [pref, (p) => { void save(p).catch(() => {}) }]
}

let serverAsked: Promise<ServerPictures> | null = null
const SERVER_CHANGED = 'wms-game-pictures-changed'
export function loadServerPictures(fresh = false): Promise<ServerPictures> {
  if (!serverAsked || fresh) {
    const p = api.get<ServerPictures>('/games/pictures')
    serverAsked = p
    p.catch(() => { if (serverAsked === p) serverAsked = null })
  }
  return serverAsked.catch(() => ({}))
}
export function serverPicturesChanged() { serverAsked = null; window.dispatchEvent(new Event(SERVER_CHANGED)) }

export function useServerPictures(): ServerPictures {
  const [s, setS] = useState<ServerPictures>({})
  useEffect(() => {
    let on = true
    const ask = () => { void loadServerPictures().then((x) => { if (on) setS(x) }) }
    ask()
    window.addEventListener(SERVER_CHANGED, ask)
    return () => { on = false; window.removeEventListener(SERVER_CHANGED, ask) }
  }, [])
  return s
}

// The server's consoles, found by id or by the short name a card carries ("SNES").
export type ConsoleIndex = { list: GameConsole[]; find: (idOrShort: string) => GameConsole | undefined }
export function useConsoleIndex(): ConsoleIndex {
  const [list, setList] = useState<GameConsole[]>([])
  useEffect(() => { getConsoles().then(setList).catch(() => {}) }, [])
  return useMemo(() => {
    const byId = new Map(list.map((c) => [c.id, c]))
    const byShort = new Map(list.map((c) => [c.short.toLowerCase(), c]))
    return { list, find: (k: string) => byId.get(k) || byShort.get(k.toLowerCase()) || byId.get(k.toLowerCase()) }
  }, [list])
}
export const consoleMedia = (c: GameConsole | undefined, id: string): MediaId => mediaFor(c?.id || id, c?.shelf?.case?.kind, c?.media)

// ---- a game's logo picture, for a label that wants it: asked for only when shown, a few at a time ----
const logos = new Map<string, Promise<string>>()
const queue: (() => void)[] = []
let running = 0
const next = () => { while (running < 4 && queue.length) { running++; queue.shift()!() } }
function gameLogo(itemId: string): Promise<string> {
  let p = logos.get(itemId)
  if (!p) {
    p = new Promise<string>((done) => {
      queue.push(() => {
        api.get<GameDetailsInfo>(`/games/${encodeURIComponent(itemId)}/metadata`)
          .then((m) => done(m?.art?.find((a) => a.kind === 'logo')?.url || ''), () => done(''))
          .finally(() => { running--; next() })
      })
      next()
    })
    logos.set(itemId, p)
  }
  return p
}
function useGameLogo(itemId: string, want: boolean): string {
  const [url, setUrl] = useState('')
  useEffect(() => {
    if (!want) { setUrl(''); return }
    let on = true
    void gameLogo(itemId).then((u) => { if (on) setUrl(u) })
    return () => { on = false }
  }, [itemId, want])
  return url
}

// ---- a card's picture ----
export function GameCardArt({ look, itemId, consoleId, image, title, brand }: { look: Resolved; itemId: string; consoleId: string; image: string; title: string; brand?: string | null }) {
  const [failed, setFailed] = useState('')
  const logo = useGameLogo(itemId, look.picture === 'case' && look.label === 'logo')
  if (look.picture === 'case') {
    return <GameCase style={look.case} consoleId={consoleId} art={image} logo={logo} title={title} label={look.label} brand={brand} />
  }
  if (!image || failed === image) return <Placeholder title={title} />
  return <FitArt src={image} className="gpic-tile" onFail={() => setFailed(image)} />
}

// What WittyPlay's frame gives the renderer (sdui/render/glyphs.ts GameArtContext): each game card's picture as chosen, and the
// detail panel's button for one game.
export function useGameArtApi(): GameArtApi {
  const [pref] = usePicturesPref()
  const server = useServerPictures()
  const consoles = useConsoleIndex()
  const brands = useBrandLogos()
  return useMemo<GameArtApi>(() => ({
    art: ({ itemId, console: short, image, title }: GameArtArgs) => {
      const c = consoles.find(short)
      const id = c?.id || short.toLowerCase()
      const look = lookFor(pref, server, itemId, id, consoleMedia(c, id))
      if (look.picture === 'filled') return null
      const brand = brands?.enabled ? brands.logos[id] : undefined
      return <GameCardArt look={look} itemId={itemId} consoleId={id} image={image} title={title} brand={brand ? brandUrl(brand) : null} />
    },
    detail: (a: GameArtArgs) => <GamePictureButton itemId={a.itemId} title={a.title} console={a.console} image={a.image} />,
  }), [pref, server, consoles, brands])
}
// A light logo reads on the dark plate a case gives it.
const brandUrl = (b: { id: number; version: number; opaque: boolean }) => logoImageUrl(b, b.opaque ? 'original' : 'white')

// ---- choosing for one game ----

// Choices as a radio group of buttons (arrows, a remote and a pad reach each; A, Enter or a click chooses), the first being "as the
// level above has it".
function Choices<T extends string>({ label, value, options, onChange, inherit, noInherit }: { label: string; value: T | ''; options: { id: T; label: string; note?: string; art?: ReactNode }[]; onChange: (v: T | '') => void; inherit: string; noInherit?: boolean }) {
  const id = useId()
  const all: { id: T | ''; label: string; note?: string; art?: ReactNode }[] = noInherit ? options : [{ id: '', label: inherit }, ...options]
  return (
    <fieldset className="gpic-choices">
      <legend id={id}>{label}</legend>
      <div role="radiogroup" aria-labelledby={id} data-nav-group className="gpic-choice-row">
        {all.map((o) => (
          <button key={o.id || 'inherit'} type="button" role="radio" aria-checked={o.id === value} className={'gpic-choice' + (o.id === value ? ' on' : '')} onClick={() => onChange(o.id)}>
            {o.art && <span className="gpic-choice-art" aria-hidden="true">{o.art}</span>}
            <span className="gpic-choice-text"><b>{o.label}</b>{o.note && <span>{o.note}</span>}</span>
          </button>
        ))}
      </div>
    </fieldset>
  )
}

// The questions for one level (everything, a console, a game), with what each leaves to the level above in words. For a console or a
// game the case is one of its medium's styles (or any other); for every game, a style per kind of medium, with quick choices for all
// cartridges and all discs at once.
export function LookChooser({ look, onChange, above, consoleId, media, everyMedium, sample }: { look: Look; onChange: (l: Look) => void; above: Resolved; consoleId?: string; media: MediaId; everyMedium?: boolean; sample: { image: string; title: string; itemId?: string } }) {
  const l = cleanLook(look)
  const [all, setAll] = useState(false)
  const set = (k: 'picture' | 'case' | 'label', v: string) => { const n: Look = { ...l }; if (v) n[k] = v as never; else delete n[k]; onChange(n) }
  const own = resolveLook([l], media)
  const shown: Resolved = { picture: l.picture || above.picture, case: own.caseChosen ? own.case : above.case, label: l.label || above.label, caseChosen: own.caseChosen, media }
  const name = (list: { id: string; label: string }[], v: string) => list.find((x) => x.id === v)?.label || v
  const styleName = (id: string) => styleDef(id)?.label || id
  const art = (id: string) => <GameCase style={id} consoleId={consoleId} art="" title="" label="title" />
  const preview = (r: Resolved) => <span className="gpic-preview"><GameCardArt look={r} itemId={sample.itemId || ''} consoleId={consoleId || ''} image={sample.image} title={sample.title} /></span>
  const mine = stylesOf(media)
  const styles = all ? [...mine, ...CASE_STYLES.map((c) => c.id).filter((c) => !mine.includes(c))] : mine
  return (
    <div className="gpic-look">
      <div className="gpic-look-now" aria-hidden="true">{shown.picture === 'filled' ? <span className="gpic-preview"><span className="gpic-filled">{sample.image ? <img src={sample.image} alt="" /> : <Placeholder title={sample.title} />}</span></span> : preview(shown)}</div>
      <div className="gpic-look-choices">
        <Choices label="Picture" value={l.picture || ''} inherit={'As before: ' + name(PICTURES, above.picture)} onChange={(v) => set('picture', v)}
          options={PICTURES.map((p) => ({ id: p.id, label: p.label, note: p.note }))} />
        {shown.picture === 'case' && (
          <>
            {everyMedium ? <MediumChoices look={l} onChange={onChange} consoleId={consoleId} /> : (
              <>
                <Choices label="Case" value={(l.case as CaseStyle) || ''} inherit={'As before: ' + styleName(above.case)} onChange={(v) => set('case', v)}
                  options={styles.map((id) => ({ id, label: styleName(id), art: art(id) }))} />
                <button type="button" className="gpic-more" aria-expanded={all} onClick={() => setAll(!all)}>{all ? 'Only the styles for ' + mediaDef(media).label.toLowerCase() : 'Show every style'}</button>
              </>
            )}
            <Choices label="On the label" value={(l.label as LabelPick) || ''} inherit={'As before: ' + name(LABELS, above.label)} onChange={(v) => set('label', v)}
              options={LABELS.map((x) => ({ id: x.id, label: x.label, note: x.note }))} />
          </>
        )}
      </div>
    </div>
  )
}

// For every game: all cartridges and all discs at once, and each kind of medium on its own.
export function MediumChoices({ look, onChange, consoleId, quick }: { look: Look; onChange: (l: Look) => void; consoleId?: string; quick?: boolean }) {
  const l = cleanLook(look)
  const vname = (v: Variant) => VARIANTS.find((x) => x.id === v)?.label || v
  const family = (f: 'cart' | 'disc', label: string, sampleMedia: string) => (
    <Choices label={label} value={familyVariant(l, f)} inherit="Each its usual" onChange={(v) => onChange(withFamilyVariant(l, f, v || null))}
      options={FAMILY_VARIANTS[f].map((v) => ({ id: v, label: vname(v), art: <GameCase style={mediaDef(sampleMedia).variants[v] || usualStyle(sampleMedia)} consoleId={consoleId} art="" title="" label="title" /> }))} />
  )
  return (
    <>
      {family('cart', 'Cartridges and cards', 'cart-16bit')}
      {family('disc', 'Discs', 'cd')}
      {!quick && (
        <details className="gpic-media">
          <summary>Each kind of medium</summary>
          {MEDIA.map((md) => (
            <Choices key={md.id} label={md.label} value={l.cases?.[md.id] || ''} inherit={'Usual: ' + (styleDef(usualStyle(md.id))?.label || '')} onChange={(v) => onChange(withMediaStyle(l, md.id, v || null))}
              options={stylesOf(md.id).map((id) => ({ id, label: styleDef(id)?.label || id, art: <GameCase style={id} art="" title="" label="title" /> }))} />
          ))}
        </details>
      )}
    </>
  )
}

export function GamePictureDialog({ itemId, title, console: short, image, onClose }: { itemId: string; title: string; console: string; image: string; onClose: () => void }) {
  const { user } = useApp()
  const [pref, setPref] = usePicturesPref()
  const server = useServerPictures()
  const consoles = useConsoleIndex()
  const c = consoles.find(short)
  const id = c?.id || short.toLowerCase()
  const [scope, setScope] = useState<'me' | 'server'>('me')
  const [err, setErr] = useState('')
  const own = scope === 'me' ? pref.games?.[itemId] || {} : server.games?.[itemId] || {}
  // What this game would be with nothing chosen at this level.
  const above = scope === 'me'
    ? lookFor({ ...pref, games: {} }, server, itemId, id, consoleMedia(c, id))
    : lookFor({ ...pref, games: {} }, { ...server, games: {} }, itemId, id, consoleMedia(c, id))
  const save = useCallback(async (l: Look) => {
    setErr('')
    if (scope === 'me') { setPref(withLook(pref, { game: itemId }, emptyLook(l) ? null : l)); return }
    try { await api.put(`/games/${encodeURIComponent(itemId)}/picture`, cleanLook(l)); serverPicturesChanged() } catch (e: any) { setErr(e.message) }
  }, [scope, pref, itemId, setPref])
  return (
    <Dialog title={`How ${title} is pictured`} size="wide" className="gpic-dialog" onClose={onClose}
      description="Box art fitted or filled, or the game in a case or cartridge. Anything left as before follows its console, then every game."
      actions={<><Button variant="ghost" onClick={() => void save({})} disabled={emptyLook(own)}>Back to as before</Button><Button variant="primary" onClick={onClose}>Done</Button></>}>
      {user?.isAdmin && (
        <Segmented<'me' | 'server'> label="Whose choice" value={scope} onChange={setScope}
          options={[{ value: 'me', label: 'Just for me' }, { value: 'server', label: 'For everyone here' }]} />
      )}
      {scope === 'server' && <p className="muted">Everyone on this server sees this unless they choose their own for this game.</p>}
      <LookChooser look={own} onChange={(l) => void save(l)} above={above} consoleId={id} media={consoleMedia(c, id)} sample={{ image, title, itemId }} />
      {err && <p className="error" role="alert">{err}</p>}
    </Dialog>
  )
}

// The detail panel's button: what this game's picture looks like now, and a way to change it.
export function GamePictureButton(props: { itemId: string; title: string; console: string; image: string }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button type="button" className="gpic-detail-btn" onClick={() => setOpen(true)} aria-haspopup="dialog">Picture style</button>
      {open && <GamePictureDialog {...props} onClose={() => setOpen(false)} />}
    </>
  )
}

// ---- a short choice with a live preview, for Setup and a first launch: how games are pictured, and, in their cases, whether cartridges
// show front on, at an angle or in their box, and discs in their case or alone ----
const quickCover = (w: number, h: number, a: string, b: string) => 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(
  `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="${a}"/><stop offset="1" stop-color="${b}"/></linearGradient></defs>` +
  `<rect width="${w}" height="${h}" fill="url(#g)"/><circle cx="${w * 0.68}" cy="${h * 0.4}" r="${Math.min(w, h) * 0.22}" fill="#fff" fill-opacity=".85"/><rect x="${w * 0.08}" y="${h * 0.08}" width="${w * 0.5}" height="${h * 0.09}" rx="4" fill="#fff" fill-opacity=".9"/></svg>`)
const QUICK = [
  { console: 'snes', media: 'cart-16bit', title: 'A cartridge game', image: quickCover(420, 300, '#0f8f7c', '#2a5ad8') },
  { console: 'ps1', media: 'cd', title: 'A CD game', image: quickCover(320, 320, '#e0752b', '#d8323f') },
  { console: 'ps2', media: 'dvd', title: 'A DVD game', image: quickCover(300, 420, '#4b3bd8', '#c93d8a') },
]
export function PictureQuickChooser({ value, onChange }: { value: Look; onChange: (l: Look) => void }) {
  const l = cleanLook(value)
  return (
    <div className="gpic-quick">
      <ul className="gpic-quick-previews" aria-label="Preview">
        {QUICK.map((q) => {
          const r = resolveLook([l], q.media)
          return <li key={q.console}><span className="gpic-preview" aria-hidden="true">{r.picture === 'filled' ? <span className="gpic-filled"><img src={q.image} alt="" /></span> : <GameCardArt look={r} itemId="" consoleId={q.console} image={q.image} title={q.title} />}</span></li>
        })}
      </ul>
      <Choices label="How games are pictured" value={l.picture || 'fitted'} inherit="" noInherit onChange={(v) => onChange({ ...l, picture: (v || 'fitted') as Look['picture'] })}
        options={PICTURES.map((p) => ({ id: p.id, label: p.label, note: p.note }))} />
      {l.picture === 'case' && <MediumChoices look={l} onChange={onChange} quick />}
    </div>
  )
}
