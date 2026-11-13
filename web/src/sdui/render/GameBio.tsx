// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A game's bio in WittyPlay's detail panel: its logo, the console it plays on, what it's about, who made it, its rating, pictures
// from the game and its RetroAchievements (greyed: what can be earned, not what has been). Loaded for the focused card only, a moment
// after focus settles, so running along a row doesn't ask the server about every game it passes.
import { useEffect, useState, type ReactNode } from 'react'
import { api, mediaUrl, type GameDetailsInfo, type Item } from '../../api'
import { IconLeft, IconRight } from '../../icons'
import { Dialog } from '../../ui/Overlay'

type Cheevo = { id: number; title: string; description: string; points: number; badge?: string }
type Cheevos = {
  state: 'ok' | 'nokey' | 'unsupported' | 'notfound' | 'error'
  game?: { id: number; title: string; url: string; points: number } | null
  achievements?: Cheevo[]
  message?: string
}
type Bio = { item?: Item; meta?: GameDetailsInfo; cheevos?: Cheevos | null }

const SETTLE_MS = 220

function useBio(itemId: string): Bio {
  const [bio, setBio] = useState<Bio>({})
  useEffect(() => {
    setBio({})
    const ctl = new AbortController()
    const t = window.setTimeout(() => {
      const keep = (part: Partial<Bio>) => !ctl.signal.aborted && setBio((b) => ({ ...b, ...part }))
      api.get<Item>(`/items/${itemId}`, ctl.signal).then((item) => keep({ item }), () => {})
      api.get<GameDetailsInfo>(`/games/${itemId}/metadata`, ctl.signal).then((meta) => keep({ meta }), () => {})
      api.get<Cheevos>(`/items/${itemId}/achievements`, ctl.signal).then((cheevos) => keep({ cheevos }), () => keep({ cheevos: null }))
    }, SETTLE_MS)
    return () => { window.clearTimeout(t); ctl.abort() }
  }, [itemId])
  return bio
}

// The rating a person reads: the board's (ESRB E), and a score out of 100 when a source has one.
function ratingWords(item?: Item, meta?: GameDetailsInfo): string[] {
  const out: string[] = []
  const age = meta?.ages?.[0]
  if (age) out.push(`${age.system} ${age.value}`)
  else if (item?.contentRating) out.push(item.contentRating)
  const score = meta?.scores?.find((s) => s.kind === 'critic') || meta?.scores?.[0]
  if (score) out.push(`${Math.round(score.value)}/100`)
  else if (item?.rating) out.push(`${item.rating.toFixed(1)}/10`)
  return out
}

function Logo({ url, title }: { url: string; title: string }) {
  const [ok, setOk] = useState(true)
  if (!ok) return null
  return <img className="wplay-bio-logo" src={mediaUrl(url)} alt={title} onError={() => setOk(false)} />
}

export const SHOT_WORDS: Record<string, string> = { screenshot: 'In the game', title: 'Title screen', artwork: 'Artwork' }

// A picture from the game: a button that opens it large.
function Shot({ url, label, onOpen }: { url: string; label: string; onOpen: () => void }) {
  const [ok, setOk] = useState(true)
  if (!ok) return null
  return (
    <button type="button" className="wplay-bio-shot" onClick={onOpen} aria-label={`${label}: show larger`}>
      <img src={mediaUrl(url)} alt="" loading="lazy" onError={() => setOk(false)} />
    </button>
  )
}

// The pictures large, one at a time: Left and Right (or a pad's d-pad) between them, Esc or B to close.
export function ShotViewer({ title, shots, at, onMove, onClose }: { title: string; shots: { url: string; kind: string }[]; at: number; onMove: (i: number) => void; onClose: () => void }) {
  const n = shots.length
  const go = (d: number) => onMove((at + d + n) % n)
  const s = shots[at]
  return (
    <Dialog title={title} onClose={onClose} size="wide" className="wplay-shot-view">
      <div className="wplay-shot-stage" onKeyDown={(e) => {
        if (e.key === 'ArrowLeft') { e.preventDefault(); e.stopPropagation(); go(-1) }
        else if (e.key === 'ArrowRight') { e.preventDefault(); e.stopPropagation(); go(1) }
      }}>
        {n > 1 && <button type="button" className="wplay-shot-arrow" onClick={() => go(-1)} aria-label="Previous picture"><IconLeft /></button>}
        <figure>
          <img src={mediaUrl(s.url)} alt={`${title}: ${SHOT_WORDS[s.kind] || 'picture'}`} />
          <figcaption>{SHOT_WORDS[s.kind] || 'Picture'}{n > 1 ? ` · ${at + 1} of ${n}` : ''}</figcaption>
        </figure>
        {n > 1 && <button type="button" className="wplay-shot-arrow" onClick={() => go(1)} aria-label="Next picture" autoFocus><IconRight /></button>}
      </div>
    </Dialog>
  )
}

// A game's RetroAchievements on their own (a game's WittyPlay page): undefined while loading, null when the server has none to give.
export function useCheevos(itemId: string | number): Cheevos | null | undefined {
  const [c, setC] = useState<Cheevos | null | undefined>(undefined)
  useEffect(() => {
    setC(undefined)
    if (!itemId) return
    const ctl = new AbortController()
    api.get<Cheevos>(`/items/${itemId}/achievements`, ctl.signal).then((x) => !ctl.signal.aborted && setC(x), () => !ctl.signal.aborted && setC(null))
    return () => ctl.abort()
  }, [itemId])
  return c
}

export function Achievements({ cheevos }: { cheevos: Cheevos | null | undefined }) {
  if (cheevos === undefined) return null
  const list = cheevos?.state === 'ok' ? cheevos.achievements || [] : []
  const words = !cheevos
    ? 'RetroAchievements isn\'t set up on this server yet.'
    : cheevos.state === 'ok'
      ? list.length ? '' : 'This game has no achievements yet.'
      : cheevos.message || {
          nokey: 'An admin can add a RetroAchievements key in Settings to show them here.',
          unsupported: 'RetroAchievements doesn\'t cover this console.',
          notfound: 'RetroAchievements doesn\'t know this copy of the game.',
          error: 'RetroAchievements couldn\'t be reached just now.',
        }[cheevos.state]
  const points = list.reduce((n, a) => n + a.points, 0)
  return (
    <section className="wplay-cheevos" aria-label="RetroAchievements">
      <h3>
        <span>RetroAchievements</span>
        {list.length > 0 && <span className="wplay-cheevos-count">{list.length} · {points} points</span>}
      </h3>
      {words && <p className="wplay-cheevos-note">{words}</p>}
      {list.length > 0 && <p className="wplay-cheevos-credit">Achievements from <a href={cheevos?.game?.url || 'https://retroachievements.org'} target="_blank" rel="noopener noreferrer">RetroAchievements.org</a></p>}
      {list.length > 0 && (
        <ul className="wplay-cheevos-list" tabIndex={0} aria-label={`${list.length} achievements, none earned`}>
          {list.map((a) => (
            <li key={a.id} title={a.description}>
              {a.badge ? <img src={mediaUrl(a.badge)} alt="" loading="lazy" /> : <i aria-hidden="true" />}
              <span><b>{a.title}</b><span>{a.description}</span></span>
              <em>{a.points}</em>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

// children: the panel's title, line and buttons, which go under the logo and above the rest.
export function GameBio({ itemId, title, console: consoleName, children }: { itemId: string; title: string; console?: string; children: ReactNode }) {
  const { item, meta, cheevos } = useBio(itemId)
  const [open, setOpen] = useState<number | null>(null)
  const logo = meta?.art?.find((a) => a.kind === 'logo')
  const shots = (meta?.art || []).filter((a) => a.kind === 'screenshot' || a.kind === 'title' || a.kind === 'artwork').slice(0, 3)
  const makers = [...new Set([meta?.developer || item?.studio, meta?.publisher].filter(Boolean) as string[])]
  const year = meta?.released?.slice(0, 4) || (item?.year ? String(item.year) : '')
  const rating = ratingWords(item, meta)
  const about = item?.overview?.trim()
  return (
    <div className="wplay-bio" data-loaded={item ? '' : undefined}>
      {logo && <Logo key={logo.url} url={logo.url} title={title} />}
      {children}
      <p className="wplay-bio-line">
        {consoleName && <span className="wplay-bio-console">{consoleName}</span>}
        {year && <span>{year}</span>}
        {rating.map((r) => <span key={r} className="wplay-bio-rating">{r}</span>)}
      </p>
      {makers.length > 0 && <p className="wplay-bio-makers"><span className="wplay-bio-label">Made by</span> {makers.join(' · ')}</p>}
      {about && <p className="wplay-bio-about">{about}</p>}
      {item?.genres?.length ? <p className="wplay-bio-genres">{item.genres.join(' · ')}</p> : null}
      {shots.length > 0 && <div className="wplay-bio-shots" data-nav-group>{shots.map((s, i) => <Shot key={s.url} url={s.url} label={SHOT_WORDS[s.kind] || 'Picture'} onOpen={() => setOpen(i)} />)}</div>}
      {open !== null && shots[open] && <ShotViewer title={title} shots={shots} at={open} onMove={setOpen} onClose={() => setOpen(null)} />}
      <Achievements cheevos={cheevos} />
    </div>
  )
}
