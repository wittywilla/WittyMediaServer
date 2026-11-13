// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api, type GameSource, type Library } from '../../api'
import { GameKeyFields } from '../GameMeta'
import { IconCheck } from '../../icons'
import { Button } from '../../ui'
import { LaterNote, StepActions, StepHead, type StepProps } from './parts'
import { GAMES_LINE } from './plan'
import { PictureQuickChooser } from '../../shell/GamePictureArt'
import { cleanLook, type Look } from '../../shell/gamePictures'

// Belongs to WittyPlay, so it's only here when WittyPlay is.
export function Games({ info, next, back, added, goTo }: StepProps & { added: Library[]; goTo: (id: 'media') => void }) {
  const [sources, setSources] = useState<GameSource[]>([])
  const [keys, setKeys] = useState<Record<string, string>>({})
  const [open, setOpen] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  useEffect(() => { api.get<{ sources: GameSource[] }>('/gamemeta/sources').then((r) => setSources(r.sources.filter((s) => s.fields.length > 0))).catch(() => {}) }, [])
  const libs = added.filter((l) => l.type === 'games')
  // How games are pictured in WittyPlay, for everyone on the server (each person can choose their own later).
  const [look, setLook] = useState<Look>({ picture: 'fitted' })
  const save = async () => {
    setBusy(true)
    const body = Object.fromEntries(Object.entries(keys).filter(([, v]) => v.trim()).map(([k, v]) => [k, v.trim()]))
    try { if (Object.keys(body).length) await api.put('/settings', body) } catch { /* keys can be added later */ }
    try {
      const now = await api.get<{ server?: Record<string, unknown> }>('/games/pictures')
      await api.put('/games/pictures', { ...(now.server || {}), wittyplay: 1, default: cleanLook(look) })
    } catch { /* it can be chosen later, in WittyPlay's Settings */ }
    setBusy(false)
    next()
  }
  return (
    <section>
      <StepHead title="Your games" lead={GAMES_LINE} />
      {libs.length > 0
        ? <p className="oobe-added"><IconCheck size={16} /> {libs.map((l) => l.name).join(', ')} {libs.length === 1 ? 'is' : 'are'} being looked through now.</p>
        : <p className="muted oobe-hint">You haven’t added a games folder yet. <Button size="sm" onClick={() => goTo('media')}>Add one</Button></p>}
      <p className="muted oobe-hint su-gap">It works out each game’s console and name by itself, and checks each file against a list of known games. Nothing is bundled or downloaded, so the games are your own. Files with copy protection are listed with a plain reason, and aren’t played.</p>
      <h2 className="oobe-sub">How your games look</h2>
      <p className="muted oobe-hint">Box art comes in every shape. Show it whole, filling its tile, or each game in a case or cartridge like the ones its console used. Anyone can change this for themselves later, for every game, a console or one game, in WittyPlay’s Settings.</p>
      <PictureQuickChooser value={look} onChange={setLook} />
      {sources.length > 0 && (
        <>
          <h2 className="oobe-sub">More details, if you want them</h2>
          <p className="muted oobe-hint">Games already get names, pictures and descriptions from free sources. These optional sources add critic scores, age ratings and more box art. Each gives you a free key: a code that lets your server ask it for things. Only names and file fingerprints are looked up, never your files.</p>
          <div className="oobe-providers">
            {sources.map((s) => {
              const shown = open === s.id
              return (
                <div key={s.id} className="oobe-provider">
                  <button type="button" className="op-head" onClick={() => setOpen(shown ? null : s.id)} aria-expanded={shown}>
                    <b>{s.name}</b><span className="pill">{s.cost}</span>
                    <span className="op-add">{shown ? 'Hide' : 'Add a key'}</span>
                  </button>
                  <div className="muted op-note">{s.why}</div>
                  {shown && <GameKeyFields source={s} values={keys} onChange={(k, v) => setKeys((c) => ({ ...c, [k]: v }))} />}
                </div>
              )
            })}
          </div>
        </>
      )}
      <LaterNote info={info} />
      <StepActions back={back}><Button variant="primary" size="big" busy={busy} onClick={save}>Continue</Button></StepActions>
    </section>
  )
}
