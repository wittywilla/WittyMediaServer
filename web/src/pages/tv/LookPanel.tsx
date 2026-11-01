import { useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { IconCheck } from '../../icons'
import { Button, Segmented, Sheet } from '../../ui'
import { appColours, clearThemeColours, forgetPreviewSize, forgetTheme, PREVIEW_SIZES, pickTheme, previewSizeName, resetLook, setLook, setThemeColour, type Look } from './look'
import { baseColours, FONTS, isHex, SHAPES, SLOTS, themeById, themeTokens, THEMES, type Font, type Shape, type Slot } from './tvThemes'
import './tvthemes.css'

// WittyTV's look, on the guide's Look panel and in Settings. The first thing is one big choice,
// the theme, with each drawn as a small picture of itself. Everything here shows straight away,
// and is kept with the account.

function Choice<T extends string | number>({ label, value, options, onChange, hint }: { label: string; value: T; options: { v: T; label: string }[]; onChange: (v: T) => void; hint?: string }) {
  return (
    <fieldset className="ctv-choice">
      <legend>{label}</legend>
      <div className="ctv-choice-opts">
        {options.map((o) => (
          <label key={String(o.v)} className={value === o.v ? 'on' : ''}>
            <input type="radio" name={label} checked={value === o.v} onChange={() => onChange(o.v)} /> {o.label}
          </label>
        ))}
      </div>
      {hint && <small>{hint}</small>}
    </fieldset>
  )
}

// A theme's small picture: the guide's own proportions, in its own colours, as they are now.
export function ThemePicture({ shape, tokens }: { shape: Shape; tokens: Record<string, string> }) {
  return (
    <span className="ctv-pic" data-shape={shape} style={tokens as React.CSSProperties} aria-hidden="true">
      <i className="t1" /><i className="t2" /><i className="t3" /><i className="pv" /><i className="pf" />
      <i className="r r1"><u /><u /><u /></i><i className="r r2"><u /><u /><u /></i><i className="r r3"><u /><u /><u /></i>
      <i className="nl" />
    </span>
  )
}

// One of many cards. The arrows move by where the cards really are, so it works at any width; at
// the edge of the set an arrow isn't used, so a remote carries on to what's beside it.
function moveAmongCards(e: React.KeyboardEvent<HTMLElement>, go: (i: number) => void) {
  const cards = [...e.currentTarget.querySelectorAll<HTMLElement>('[role=radio]')]
  const at = cards.indexOf(document.activeElement as HTMLElement)
  if (at < 0) return
  const box = (el: HTMLElement) => el.getBoundingClientRect()
  const me = box(cards[at])
  let to = -1
  if (e.key === 'ArrowLeft' && at > 0 && box(cards[at - 1]).top === me.top) to = at - 1
  else if (e.key === 'ArrowRight' && at < cards.length - 1 && box(cards[at + 1]).top === me.top) to = at + 1
  else if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
    const down = e.key === 'ArrowDown'
    let best = Infinity
    cards.forEach((c, i) => {
      const b = box(c)
      if (down ? b.top <= me.top + 1 : b.top >= me.top - 1) return
      const d = Math.abs(b.top - me.top) * 4 + Math.abs(b.left - me.left)
      if (d < best) { best = d; to = i }
    })
  } else if (e.key === 'Home') to = 0
  else if (e.key === 'End') to = cards.length - 1
  if (to < 0) return
  e.preventDefault()
  go(to)
  cards[to].focus()
}

function ThemeCards({ look }: { look: Look }) {
  const app = appColours()
  return (
    <div className="ctv-themes" role="radiogroup" aria-label="WittyTV theme" onKeyDown={(e) => moveAmongCards(e, (i) => pickTheme(THEMES[i].id, look))}>
      {THEMES.map((t) => {
        const on = look.theme === t.id
        const shape = t.id === 'custom' ? look.custom.shape : t.shape
        const mine = on ? look.colours : undefined
        return (
          <button key={t.id} type="button" role="radio" aria-checked={on} tabIndex={on ? 0 : -1} className={'ctv-themecard' + (on ? ' on' : '')} onClick={() => pickTheme(t.id, look)}>
            <ThemePicture shape={shape} tokens={themeTokens(t.id, mine, app)} />
            <b>{t.name}</b>
            <small>{t.blurb}</small>
            {t.follows && !(on && Object.keys(look.colours).length) && <small className="ctv-pic-tag">Takes its colours from your app theme</small>}
            {on && <span className="ctv-themecard-check"><IconCheck size={14} /></span>}
          </button>
        )
      })}
    </div>
  )
}

// Which wins, in words, and where what's showing came from.
function WhoChose({ look }: { look: Look }) {
  const name = themeById(look.theme)!.name
  return (
    <div className="ctv-who" role="status">
      <p>
        {look.source === 'you' && <><b>{name}</b> is your own choice.</>}
        {look.source === 'app' && <><b>{name}</b> is what your app theme suggests. Pick another here and yours wins.</>}
        {look.source === 'owner' && <><b>{name}</b> is what the server's owner chose for new viewers. Pick another here and yours wins.</>}
      </p>
      <p>Your own choice comes first, then what your app theme suggests, then the owner's default for new viewers.</p>
      {look.source === 'you' && <Button size="sm" variant="ghost" onClick={forgetTheme}>Stop choosing, and use the default</Button>}
    </div>
  )
}

const PALETTE = ['#000000', '#0f1115', '#06164a', '#17223b', '#1f3d36', '#3b1f47', '#7955ff', '#2196f3', '#00acc1', '#2fb67c', '#ffd60a', '#ffb300', '#e8590c', '#d6336c', '#f5f0e4', '#ffffff']

function Colours({ look }: { look: Look }) {
  const t = themeById(look.theme)!
  const [slot, setSlot] = useState<Slot>('bg')
  const base = baseColours(look.theme, appColours())
  const tokens = look.tokens
  const hasAny = Object.keys(look.colours).length > 0
  const value = (s: Slot) => look.colours[s] || base[s]
  const set = (v: string) => setThemeColour(look.theme, slot, v)
  const [typed, setTyped] = useState('')
  const typing = useRef(false)
  const shown = typing.current ? typed : value(slot)
  const onType = (v: string) => {
    typing.current = true
    setTyped(v)
    if (isHex(v)) set(v.toLowerCase())
  }
  // What was picked against what it came out as: a colour that can't be read gets moved.
  const asDrawn: Partial<Record<Slot, string>> = { bg: tokens['--ctv-bg'], cell: tokens['--ctv-cell'], live: tokens['--ctv-live'], hi: tokens['--ctv-hi'], accent: tokens['--ctv-accent'], text: tokens['--ctv-text'] }
  const moved = SLOTS.filter((s) => look.colours[s.slot] && asDrawn[s.slot] && asDrawn[s.slot] !== look.colours[s.slot])
  const followWord = t.follows ? 'From your app theme' : `${t.name}'s own`
  return (
    <section aria-labelledby="ctv-colours-h">
      <h3 id="ctv-colours-h">Colours of {t.name}</h3>
      <p>{hasAny ? <>These override the {t.follows ? 'app theme\'s colours' : 'theme\'s own colours'}, for this theme only.</> : t.follows ? <>These come from your app theme, so they change with it. Choose any of them here to override it, for this theme only.</> : <>{t.name} has colours of its own. Choose any of them here to override them, for this theme only.</>}</p>
      <Segmented<Slot> className="ctv-slot-tabs" label="Which colour" value={slot} onChange={(s) => { typing.current = false; setSlot(s) }}
        options={SLOTS.map((s) => ({ value: s.slot, label: <><span className="ctv-slot-dot" style={{ background: value(s.slot) }} />{s.name}</>, name: s.name }))} />
      <div className="ctv-swatches" role="group" aria-label={`Colours for ${SLOTS.find((s) => s.slot === slot)!.name}`} data-nav-group>
        {PALETTE.map((c) => <button key={c} type="button" className={'ctv-swatch' + (value(slot).toLowerCase() === c ? ' on' : '')} style={{ background: c }} aria-label={`Use ${c}`} aria-pressed={value(slot).toLowerCase() === c} onClick={() => { typing.current = false; set(c) }} />)}
      </div>
      <div className="ctv-colour-row">
        <label>Any colour <input type="color" value={value(slot)} onChange={(e) => { typing.current = false; set(e.target.value) }} /></label>
        <label>As text <input className="input" type="text" spellCheck={false} maxLength={7} value={shown} onChange={(e) => onType(e.target.value)} onBlur={() => { typing.current = false; setTyped('') }} /></label>
        <span className="ctv-colour-state">{look.colours[slot] ? 'Your own' : followWord}</span>
        {look.colours[slot] && <Button size="sm" variant="ghost" onClick={() => { typing.current = false; setThemeColour(look.theme, slot, undefined) }}>Use {t.follows ? 'the app theme\'s' : 'the theme\'s'} {SLOTS.find((s) => s.slot === slot)!.name.toLowerCase()}</Button>}
      </div>
      {hasAny && <div className="ctv-editor-actions"><Button size="sm" onClick={() => { typing.current = false; clearThemeColours(look.theme) }}>{t.follows ? 'Back to following the app theme' : `Back to ${t.name}'s own colours`}</Button></div>}
      <p className="ctv-colour-note">{moved.length ? `${moved.map((m) => m.name).join(', ')} moved a little, so that the words stay readable. ` : ''}Text is always worked out to be readable on what it's on, whatever you choose.</p>
    </section>
  )
}

function CustomParts({ look }: { look: Look }) {
  const app = appColours()
  const shapes = Object.keys(SHAPES) as Shape[]
  const setShape = (shape: Shape) => setLook({ custom: { ...look.custom, shape } })
  return (
    <section aria-labelledby="ctv-custom-h">
      <h3 id="ctv-custom-h">Your own layout</h3>
      <p>Custom is kept with your account, and stays as you left it if you try another theme.</p>
      <div className="ctv-shapes" role="radiogroup" aria-label="Layout" onKeyDown={(e) => moveAmongCards(e, (i) => setShape(shapes[i]))}>
        {shapes.map((s) => {
          const on = look.custom.shape === s
          return (
            <button key={s} type="button" role="radio" aria-checked={on} tabIndex={on ? 0 : -1} className={'ctv-themecard' + (on ? ' on' : '')} onClick={() => setShape(s)}>
              <ThemePicture shape={s} tokens={themeTokens('custom', look.colours, app)} />
              <b>{SHAPES[s].name}</b>
            </button>
          )
        })}
      </div>
      <label className="ctv-font ctv-colour-row">Type
        <select className="select" value={look.custom.font} onChange={(e) => setLook({ custom: { ...look.custom, font: e.target.value as Font } })}>
          {FONTS.map((f) => <option key={f.v} value={f.v}>{f.name}</option>)}
        </select>
      </label>
    </section>
  )
}

const ThemePick = ({ look }: { look: Look }) => (
  <section aria-label="Theme">
    <ThemeCards look={look} />
    <WhoChose look={look} />
  </section>
)

export function LookEditor({ look, inSettings }: { look: Look; inSettings?: boolean }) {
  const sizeChosen = look.previewW != null
  return (
    <div className="ctv-editor">
      <ThemePick look={look} />
      {look.theme === 'custom' && <CustomParts look={look} />}
      <Colours look={look} />
      <section aria-label="Preview, rows and details">
        <Choice label="Preview" value={look.preview} onChange={(v) => setLook({ preview: v })} options={[{ v: 'highlight', label: 'The highlighted channel' }, { v: 'last', label: 'The last channel I watched' }, { v: 'off', label: 'Hidden' }]}
          hint="A preview runs a stream on the server, and on a tuner it uses up one tuner. If the server struggles, use the last channel, or hide it." />
        {look.preview !== 'off' && (
          <>
            <Choice label="Preview side" value={look.previewSide} onChange={(v) => setLook({ previewSide: v })} options={[{ v: 'right', label: 'Right' }, { v: 'left', label: 'Left' }]} />
            <fieldset className="ctv-choice">
              <legend>Preview size</legend>
              <div className="ctv-choice-opts">
                <label className={!sizeChosen ? 'on' : ''}><input type="radio" name="Preview size" checked={!sizeChosen} onChange={forgetPreviewSize} /> Automatic</label>
                {PREVIEW_SIZES.map((s) => <label key={s.w} className={look.previewW === s.w ? 'on' : ''}><input type="radio" name="Preview size" checked={look.previewW === s.w} onChange={() => setLook({ previewW: s.w })} /> {s.name}</label>)}
              </div>
              <small>{sizeChosen ? `Now: ${previewSizeName(look.previewW!)}.` : 'Automatic suits the theme and the size of the window.'} You can also drag the corner of the preview, or focus that corner and use the arrow keys. It's held to what fits this screen.</small>
            </fieldset>
          </>
        )}
        <Choice label="Row size" value={look.density} onChange={(v) => setLook({ density: v })} options={[{ v: 'compact', label: 'Compact' }, { v: 'normal', label: 'Normal' }, { v: 'large', label: 'Large' }]} />
        <Choice label="Hours across the screen" value={look.hours} onChange={(v) => setLook({ hours: v })} options={[{ v: 0, label: 'What fits' }, { v: 1.5, label: '1½' }, { v: 2, label: '2' }, { v: 3, label: '3' }, { v: 4, label: '4' }]} />
        <fieldset className="ctv-choice">
          <legend>Show</legend>
          <div className="ctv-choice-checks">
            <label><input type="checkbox" checked={look.nowLine} onChange={(e) => setLook({ nowLine: e.target.checked })} /> A line at the current time, down all channels</label>
            <label><input type="checkbox" checked={look.progress} onChange={(e) => setLook({ progress: e.target.checked })} /> How far into each programme it is</label>
            <label><input type="checkbox" checked={look.subtitles} onChange={(e) => setLook({ subtitles: e.target.checked })} /> Episode names and details in the rows</label>
            <label><input type="checkbox" checked={look.logos} onChange={(e) => setLook({ logos: e.target.checked })} /> Channel logos</label>
            <label><input type="checkbox" checked={look.art} onChange={(e) => setLook({ art: e.target.checked })} /> The programme's picture at the top, in the themes that have one</label>
          </div>
        </fieldset>
        {inSettings ? <p className="ctv-colour-note"><Link to="/tv">See it on the guide</Link></p> : <p className="ctv-colour-note">This is also in <Link to="/settings/appearance#wittytv">Settings, Appearance</Link>.</p>}
      </section>
    </div>
  )
}

export default function LookPanel({ look, onClose }: { look: Look; onClose: () => void }) {
  return (
    <Sheet className="ctv-look" title="Look of WittyTV" onClose={onClose}
      actions={<><Button variant="ghost" onClick={resetLook}>Back to how it started</Button><Button variant="primary" onClick={onClose}>Done</Button></>}>
      <p className="muted">Changes show straight away, and follow you to your other devices.</p>
      <LookEditor look={look} />
    </Sheet>
  )
}
