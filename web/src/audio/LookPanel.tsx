// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import ArtView from './ArtView'
import { ART_STYLES, BACKDROPS, CANVAS_PLACES, type Look, type SongLook } from './look'

// The Look panel of the full player: how the cover looks, what is behind the player, where a Canvas video plays,
// for every song or for this one alone. Each choice is a small picture of itself, drawn with this song's cover, so
// what is picked is what is seen. Changes show at once.

export type LookMode = 'art' | 'canvas' | 'viz'

export default function LookPanel({ look, scope, onScope, own, onReset, onLook, mode, onMode, hasCanvas, canCanvas, artUrl, title, admin, onStudio }: {
  look: Look; scope: 'all' | 'song'; onScope: (s: 'all' | 'song') => void; own: boolean; onReset: () => void
  onLook: (patch: SongLook) => void
  mode: LookMode; onMode: (m: LookMode) => void
  hasCanvas: boolean; canCanvas: boolean // this song has one; songs here can have one
  artUrl: string; title: string; admin: boolean; onStudio?: () => void
}) {
  const tileArt = artUrl ? undefined : <span className="lk-noart" aria-hidden="true">♪</span>
  return (
    <div className="lk">
      <div className="lk-scope" role="radiogroup" aria-label="Change the look of">
        <button type="button" role="radio" aria-checked={scope === 'all'} className={scope === 'all' ? 'on' : ''} onClick={() => onScope('all')}>Every song</button>
        <button type="button" role="radio" aria-checked={scope === 'song'} className={scope === 'song' ? 'on' : ''} onClick={() => onScope('song')}>
          This song{own && <span className="lk-dot" aria-label=", has its own look" />}
        </button>
      </div>
      {scope === 'song' && (
        <p className="lk-note">{own ? <>“{title}” has a look of its own. <button type="button" className="lk-link" onClick={onReset}>Use every song’s look</button></> : `Changes here are for “${title}” alone.`}</p>
      )}

      <section aria-labelledby="lk-art">
        <h4 id="lk-art">Cover</h4>
        <div className="lk-grid" role="radiogroup" aria-labelledby="lk-art">
          {ART_STYLES.map((s) => (
            <button type="button" key={s.id} role="radio" aria-checked={look.art === s.id} className={'lk-tile' + (look.art === s.id ? ' on' : '')} onClick={() => onLook({ art: s.id })} title={s.note}>
              <span className="lk-pic"><ArtView look={{ art: s.id, pulse: false }} src={artUrl} caption={title} size="tile" fallback={tileArt} /></span>
              <span className="lk-name">{s.name}</span>
            </button>
          ))}
        </div>
      </section>

      <section aria-labelledby="lk-behind">
        <h4 id="lk-behind">Behind the player</h4>
        <div className="lk-grid wide" role="radiogroup" aria-labelledby="lk-behind">
          {BACKDROPS.map((b) => {
            const on = mode === 'art' && look.backdrop === b.id
            return (
              <button type="button" key={b.id} role="radio" aria-checked={on} className={'lk-tile' + (on ? ' on' : '')} onClick={() => { onMode('art'); onLook({ backdrop: b.id }) }}>
                <span className={'lk-scene bd-' + b.id} style={artUrl ? { '--cover': `url("${artUrl}")` } as React.CSSProperties : undefined}><i /></span>
                <span className="lk-name">{b.name}</span>
              </button>
            )
          })}
          {canCanvas && (
            <button type="button" role="radio" aria-checked={mode === 'canvas'} className={'lk-tile' + (mode === 'canvas' ? ' on' : '')} disabled={!hasCanvas && !admin} onClick={() => (hasCanvas ? onMode('canvas') : onStudio?.())}
              title={hasCanvas ? 'This song’s Canvas video' : admin ? 'This song has no Canvas video yet: make one' : 'This song has no Canvas video'}>
              <span className="lk-scene bd-canvas"><i /></span>
              <span className="lk-name">Canvas video{!hasCanvas && <small>{admin ? 'Make one' : 'None yet'}</small>}</span>
            </button>
          )}
          <button type="button" role="radio" aria-checked={mode === 'viz'} className={'lk-tile' + (mode === 'viz' ? ' on' : '')} onClick={() => onMode('viz')}>
            <span className="lk-scene bd-viz"><i /><b /><b /><b /><b /><b /></span>
            <span className="lk-name">Visualizer</span>
          </button>
        </div>
      </section>

      {canCanvas && (
        <section aria-labelledby="lk-canvas">
          <h4 id="lk-canvas">Canvas video</h4>
          <div className="lk-seg" role="radiogroup" aria-labelledby="lk-canvas">
            {CANVAS_PLACES.map((c) => (
              <button type="button" key={c.id} role="radio" aria-checked={look.canvasPlace === c.id} className={look.canvasPlace === c.id ? 'on' : ''} onClick={() => { onLook({ canvasPlace: c.id }); if (hasCanvas) onMode('canvas') }}>{c.name}</button>
            ))}
          </div>
          <p className="lk-note">{hasCanvas ? 'In the cover’s place, it takes the cover’s shape here, in the bar and in karaoke.' : 'This song has no Canvas video.'}</p>
          {admin && onStudio && <button type="button" className="lk-studio" onClick={onStudio}>{hasCanvas ? 'Edit the Canvas video…' : 'Make a Canvas video…'}</button>}
        </section>
      )}

      <section aria-labelledby="lk-extras">
        <h4 id="lk-extras">Touches</h4>
        <label className="lk-row"><span>Reflection under the cover</span>
          <button type="button" role="switch" aria-checked={look.reflection} className={'switch' + (look.reflection ? ' on' : '')} onClick={() => onLook({ reflection: !look.reflection })}><i /></button>
        </label>
        <label className="lk-row"><span>Pulse with the beat</span>
          <button type="button" role="switch" aria-checked={look.pulse} className={'switch' + (look.pulse ? ' on' : '')} onClick={() => onLook({ pulse: !look.pulse })}><i /></button>
        </label>
      </section>
    </div>
  )
}
