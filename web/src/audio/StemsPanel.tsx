// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef } from 'react'
import { HelpLink } from '../components/HelpLink'
import { TaskLine } from '../extras/jobs'
import '../extras/jobs.css'
import { IconClose } from '../icons'
import { Progress } from '../ui'
import { STEM_LABELS, backingOf, fmtSize, levelsAreDefault, present, reasonWords, sourcesOf, type StemInfo, type StemName } from './stems'
import type { StemKaraoke } from './useStems'
import './stems.css'

// The stems of the song in karaoke: what it has and how loud each part is, and, for a
// song with none, what this server can make. It says plainly what each thing is: a real
// part, a guess, or something this browser can't play.

const SOURCE_WORDS: Record<string, string> = { sidecar: 'beside the song', upload: 'uploaded', separated: 'separated on this server', approximate: 'approximate' }

export function StemsPanel({ sk, admin, onClose }: { sk: StemKaraoke; admin: boolean; onClose: () => void }) {
  const box = useRef<HTMLElement>(null)
  useEffect(() => { box.current?.querySelector<HTMLElement>('button, input')?.focus({ preventScroll: true }) }, [])
  const a = sk.answer
  const stems: StemInfo[] = a?.stems || []
  const named = sourcesOf(stems)
  const parts = present(sk.sources)
  const usesStems = sk.choice.engine === 'stems'
  const real = named.vocals && backingOf(named) !== 'none'
  const hasBacking = backingOf(named) !== 'none'
  const approx = a?.separation.approximate
  const ext = a?.separation.external
  const total = stems.filter((s) => !s.multitrack).reduce((n, s) => n + s.size, 0)
  const info = (n: StemName) => stems.find((s) => s.name === n)
  const stemChoices = sk.choices.filter((c) => c.engine === 'stems')
  const moved = usesStems && !levelsAreDefault(sk.sources, sk.choice.id, sk.levels)
  return (
    <section className="kar-mix" ref={box} aria-label="Stems">
      <div className="kar-mix-head">
        <h3>Stems</h3>
        <button type="button" className="mp-ibtn" onClick={onClose} aria-label="Close stems"><IconClose /></button>
      </div>
      <div className="kar-mix-body">
        {!a && <p className="kar-mix-text">Looking for this song's stems…</p>}

        {sk.load.state === 'loading' && <Progress label="Getting the stems" value={Math.round(sk.load.progress * 100)} showValue />}
        {sk.load.state === 'failed' && <p className="kar-mix-warn" role="alert">{sk.load.error}</p>}

        {a && parts.length > 0 && stemChoices.length > 0 && (
          <>
            <p className="kar-mix-text">
              {usesStems
                ? sk.active ? 'Playing this song\'s own parts together.' : 'Getting the parts ready. The song plays as it is until they are.'
                : `This song has its parts. Choose ${stemChoices.map((c) => c.name).join(' or ')} to play them${total ? ` (about ${fmtSize(total)} to fetch)` : ''}.`}
            </p>
            <div className={'kar-levels' + (usesStems && sk.active ? '' : ' off')} role="group" aria-label="How loud each part is">
              {parts.map((n) => {
                const st = info(n)
                const ready = sk.active && (sk.loaded.includes(n) || !sk.canLevel)
                const level = sk.levels[n]
                return (
                  <label key={n} className="kar-level">
                    <span className="kar-level-name">{STEM_LABELS[n]}{st && <small>{st.approximate ? 'approximate' : SOURCE_WORDS[st.source] || ''}</small>}</span>
                    {sk.canLevel
                      ? <input className="range" type="range" aria-label={`${STEM_LABELS[n]} level`} min={0} max={100} step={5} disabled={!ready} value={Math.round(level * 100)} aria-valuetext={`${Math.round(level * 100)} percent`}
                          style={{ '--pct': level * 100 + '%' } as React.CSSProperties} onChange={(e) => sk.setLevel(n, Number(e.target.value) / 100)} />
                      : <input type="checkbox" aria-label={STEM_LABELS[n]} className="kar-switch" disabled={!ready} checked={level > 0} onChange={(e) => sk.setLevel(n, e.target.checked ? 1 : 0)} />}
                    <span className="kar-level-pct" aria-hidden="true">{sk.canLevel ? `${Math.round(level * 100)}%` : level > 0 ? 'On' : 'Off'}</span>
                  </label>
                )
              })}
            </div>
            {moved && <button type="button" className="btn sm ghost kar-mix-reset" onClick={sk.resetLevels}>Back to {sk.choice.name}</button>}
            {usesStems && sk.active && sk.load.partial && <p className="kar-mix-text">This song is long, so only the parts being played were fetched. The others stay as they are.</p>}
            {!sk.canLevel && <p className="kar-mix-text">These parts are tracks of one Native Instruments stem file. This browser can switch each on or off, not turn it part-way, and the key can't be moved while they play.</p>}
          </>
        )}

        {named.approximate && approx?.note && <p className="kar-mix-note"><b>Approximate.</b> {approx.note.replace(/^Approximate:\s*/i, '').replace(/^./, (c) => c.toUpperCase())}</p>}
        {named.vocals && !hasBacking && <p className="kar-mix-text">Only the voice part is here. Taking the voice out needs the music without it: an instrumental, or drums, bass and the rest.</p>}
        {sk.multitrack === 'unsupported' && <p className="kar-mix-warn">This song's parts are tracks of one Native Instruments stem file. This browser can't play the tracks of a file separately (Safari can), so the parts can't be used here.</p>}
        {sk.multitrack === 'unused' && <p className="kar-mix-text">There is a Native Instruments stem file too. The separate parts are used, as they can be mixed.</p>}

        {a && !real && (
          <div className="kar-make">
            {parts.length === 0 && sk.multitrack === 'none' && <p className="kar-mix-text">This song has no stems. <b>Voice lower</b> and <b>Voice lowest</b> work without them, by cancelling the middle of the mix as it plays.</p>}

            {!hasBacking && approx && (approx.task
              ? <TaskLine id={approx.task} label="Making an approximate instrumental" />
              : approx.available ? (
                <div className="kar-make-row">
                  <button type="button" className="btn sm" disabled={!!sk.starting} onClick={sk.makeApproximate}>Make an approximate instrumental</button>
                  {approx.error && <p className="kar-mix-warn" role="alert">{approx.error}</p>}
                  {approx.note && <p className="kar-mix-text">{approx.note} It's made once, by FFmpeg on the server, and kept.</p>}
                </div>
              ) : <p className="kar-mix-text">An approximate instrumental can't be made: {lower(reasonWords(approx.reason, 'approximate'))}</p>)}

            {ext && (ext.task
              ? <TaskLine id={ext.task} label="Separating this song into stems" />
              : ext.available ? (
                <div className="kar-make-row">
                  <button type="button" className="btn sm" disabled={!!sk.starting} onClick={sk.separate}>Separate with the installed separator</button>
                  {ext.error && <p className="kar-mix-warn" role="alert">The separator failed: {ext.error}</p>}
                  <p className="kar-mix-text">Runs the program this server's owner installed, for real stems. It can take minutes, and shows in Tasks.</p>
                </div>
              ) : (admin || ext.reason === 'admin-only') && <p className="kar-mix-text">{reasonWords(ext.reason, 'external')}</p>)}
          </div>
        )}
        <p className="kar-mix-text"><HelpLink to="lyrics-karaoke#stems-the-real-vocal-track" newTab>About stems</HelpLink></p>
      </div>
    </section>
  )
}

const lower = (s: string) => s.replace(/^./, (c) => c.toLowerCase())
