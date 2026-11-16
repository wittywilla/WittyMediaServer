// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { IconCheck } from '../../icons'
import { Button, Segmented, Sheet } from '../../ui'
import { appColours, clearThemeColours, forgetPreviewSize, forgetTheme, PREVIEW_SIZES, pickTheme, previewSizeName, resetLook, setLook, setThemeColour, type Look } from './look'
import { baseColours, FONTS, isHex, SHAPES, SLOTS, themeById, themeTokens, THEMES, type Font, type Shape, type Slot } from './tvThemes'
import BackgroundPlayChoice from './BackgroundPlayChoice'
import { useGuidePrefs } from './guidePrefs'
import { PAGE_SIZES, pageSizeName, type PageSize } from './guideLogic'
import './tvthemes.css'
import { list, num, t, tk } from '../../i18n'

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
    <div className="ctv-themes" role="radiogroup" aria-label={t('WittyTV theme')} onKeyDown={(e) => moveAmongCards(e, (i) => pickTheme(THEMES[i].id, look))}>
      {THEMES.map((th) => {
        const on = look.theme === th.id
        const shape = th.id === 'custom' ? look.custom.shape : th.shape
        const mine = on ? look.colours : undefined
        return (
          <button key={th.id} type="button" role="radio" aria-checked={on} tabIndex={on ? 0 : -1} className={'ctv-themecard' + (on ? ' on' : '')} onClick={() => pickTheme(th.id, look)}>
            <ThemePicture shape={shape} tokens={themeTokens(th.id, mine, app)} />
            <b>{t(th.name)}</b>
            <small>{t(th.blurb)}</small>
            {th.follows && !(on && Object.keys(look.colours).length) && <small className="ctv-pic-tag">{t('Takes its colours from your app theme')}</small>}
            {on && <span className="ctv-themecard-check"><IconCheck size={14} /></span>}
          </button>
        )
      })}
    </div>
  )
}

// Which wins, in words, and where what's showing came from.
function WhoChose({ look }: { look: Look }) {
  const name = t(themeById(look.theme)!.name)
  return (
    <div className="ctv-who" role="status">
      <p>
        {look.source === 'you' && t('{theme} is your own choice.', { theme: name })}
        {look.source === 'app' && t('{theme} is what your app theme suggests. Pick another here and yours wins.', { theme: name })}
        {look.source === 'owner' && t("{theme} is what the server's owner chose for new viewers. Pick another here and yours wins.", { theme: name })}
      </p>
      <p>{t("Your own choice comes first, then what your app theme suggests, then the owner's default for new viewers.")}</p>
      {look.source === 'you' && <Button size="sm" variant="ghost" onClick={forgetTheme}>{t('Stop choosing, and use the default')}</Button>}
    </div>
  )
}

const PALETTE: [string, string][] = [['#000000', tk('Black')], ['#0f1115', tk('Near black')], ['#06164a', tk('Deep blue')], ['#17223b', tk('Slate')], ['#1f3d36', tk('Deep green')], ['#3b1f47', tk('Deep purple')], ['#7955ff', tk('Violet')], ['#2196f3', tk('Blue')], ['#00acc1', tk('Teal')], ['#2fb67c', tk('Green')], ['#ffd60a', tk('Yellow')], ['#ffb300', tk('Amber')], ['#e8590c', tk('Orange')], ['#d6336c', tk('Pink')], ['#f5f0e4', tk('Paper')], ['#ffffff', tk('White')]]

function Colours({ look }: { look: Look }) {
  const th = themeById(look.theme)!
  const thName = t(th.name)
  const slotName = (x: Slot) => t(SLOTS.find((s) => s.slot === x)!.name)
  const [wanted, setSlot] = useState<Slot>('bg')
  const offered = SLOTS.filter((s) => !SHAPES[look.shape].noSlots?.includes(s.slot))
  const slot = offered.some((s) => s.slot === wanted) ? wanted : 'bg'
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
  const followWord = th.follows ? t('From your app theme') : t("{theme}'s own", { theme: thName })
  return (
    <section aria-labelledby="ctv-colours-h">
      <h3 id="ctv-colours-h">{t('Colours of {theme}', { theme: thName })}</h3>
      <p>{hasAny ? (th.follows ? t("These override the app theme's colours, for this theme only.") : t("These override the theme's own colours, for this theme only.")) : th.follows ? t('These come from your app theme, so they change with it. Choose any of them here to override it, for this theme only.') : t('{theme} has colours of its own. Choose any of them here to override them, for this theme only.', { theme: thName })}</p>
      <Segmented<Slot> className="ctv-slot-tabs" label={t('Which colour')} value={slot} onChange={(s) => { typing.current = false; setSlot(s) }}
        options={offered.map((s) => ({ value: s.slot, label: <><span className="ctv-slot-dot" style={{ background: value(s.slot) }} />{t(s.name)}</>, name: t(s.name) }))} />
      <div className="ctv-swatches" role="group" aria-label={t('Colours for {slot}', { slot: slotName(slot) })} data-nav-group>
        {PALETTE.map(([c, name]) => <button key={c} type="button" className={'ctv-swatch' + (value(slot).toLowerCase() === c ? ' on' : '')} style={{ background: c }} aria-label={t(name)} title={t(name)} data-colour={c} aria-pressed={value(slot).toLowerCase() === c} onClick={() => { typing.current = false; set(c) }} />)}
      </div>
      <div className="ctv-colour-row">
        <label>{t('Any colour')} <input type="color" value={value(slot)} onChange={(e) => { typing.current = false; set(e.target.value) }} /></label>
        <label>{t('As text')} <input className="input" type="text" spellCheck={false} maxLength={7} value={shown} onChange={(e) => onType(e.target.value)} onBlur={() => { typing.current = false; setTyped('') }} /></label>
        <span className="ctv-colour-state">{look.colours[slot] ? t('Your own') : followWord}</span>
        {look.colours[slot] && <Button size="sm" variant="ghost" onClick={() => { typing.current = false; setThemeColour(look.theme, slot, undefined) }}>{th.follows ? t("Use the app theme's {slot}", { slot: slotName(slot).toLowerCase() }) : t("Use the theme's {slot}", { slot: slotName(slot).toLowerCase() })}</Button>}
      </div>
      {hasAny && <div className="ctv-editor-actions"><Button size="sm" onClick={() => { typing.current = false; clearThemeColours(look.theme) }}>{th.follows ? t('Back to following the app theme') : t("Back to {theme}'s own colours", { theme: thName })}</Button></div>}
      <p className="ctv-colour-note">{moved.length ? t('{colours} moved a little, so that the words stay readable.', { colours: list(moved.map((m) => t(m.name))) }) + ' ' : ''}{t("Text is always worked out to be readable on what it's on, whatever you choose.")}</p>
    </section>
  )
}

function CustomParts({ look }: { look: Look }) {
  const app = appColours()
  const shapes = Object.keys(SHAPES) as Shape[]
  const setShape = (shape: Shape) => setLook({ custom: { ...look.custom, shape } })
  return (
    <section aria-labelledby="ctv-custom-h">
      <h3 id="ctv-custom-h">{t('Your own layout')}</h3>
      <p>{t('Custom is kept with your account, and stays as you left it if you try another theme.')}</p>
      <div className="ctv-shapes" role="radiogroup" aria-label={t('Layout')} onKeyDown={(e) => moveAmongCards(e, (i) => setShape(shapes[i]))}>
        {shapes.map((s) => {
          const on = look.custom.shape === s
          return (
            <button key={s} type="button" role="radio" aria-checked={on} tabIndex={on ? 0 : -1} className={'ctv-themecard' + (on ? ' on' : '')} onClick={() => setShape(s)}>
              <ThemePicture shape={s} tokens={themeTokens('custom', look.colours, app)} />
              <b>{t(SHAPES[s].name)}</b>
            </button>
          )
        })}
      </div>
      <label className="ctv-font ctv-colour-row">{t('Type')}
        <select className="select" value={look.custom.font} onChange={(e) => setLook({ custom: { ...look.custom, font: e.target.value as Font } })}>
          {FONTS.map((f) => <option key={f.v} value={f.v}>{t(f.name)}</option>)}
        </select>
      </label>
    </section>
  )
}

// One thing to show or hide: the whole row is the switch, its state said in words at the end (never by colour alone).
function SwitchRow({ label, hint, on, onChange }: { label: string; hint?: string; on: boolean; onChange: (v: boolean) => void }) {
  return (
    <button type="button" role="switch" aria-checked={on} className={'ctv-switch-row' + (on ? ' on' : '')} onClick={() => onChange(!on)}>
      <span className="ctv-switch-text"><b>{label}</b>{hint && <small>{hint}</small>}</span>
      <span className="ctv-switch-state">{on ? t('On') : t('Off')}</span>
    </button>
  )
}

const ThemePick = ({ look }: { look: Look }) => (
  <section aria-labelledby="ctv-theme-h">
    <h3 id="ctv-theme-h">{t('WittyTV look')}</h3>
    <ThemeCards look={look} />
    <WhoChose look={look} />
  </section>
)

export function LookEditor({ look, inSettings }: { look: Look; inSettings?: boolean }) {
  const sizeChosen = look.previewW != null
  // How many channels at a time is this device's own, like the set it shows.
  const [guidePrefs, setGuidePrefs] = useGuidePrefs()
  return (
    <div className="ctv-editor">
      <ThemePick look={look} />
      {look.theme === 'custom' && <CustomParts look={look} />}
      <Colours look={look} />
      <section aria-label={t('Preview, rows and details')}>
        <Choice label={t('Preview')} value={look.preview} onChange={(v) => setLook({ preview: v })} options={[{ v: 'highlight', label: t('The highlighted channel') }, { v: 'last', label: t('The last channel I watched') }, { v: 'off', label: t('Hidden') }]}
          hint={t('A preview runs a stream on the server, and on a tuner it uses up one tuner. If the server struggles, use the last channel, or hide it.')} />
        {look.preview !== 'off' && (
          <>
            <Choice label={t('Preview side')} value={look.previewSide} onChange={(v) => setLook({ previewSide: v })} options={[{ v: 'right', label: t('Right') }, { v: 'left', label: t('Left') }]} />
            <Choice label={t('Preview controls')} value={look.previewBar} onChange={(v) => setLook({ previewBar: v })} options={[{ v: 'bar', label: t('In a bar under it') }, { v: 'over', label: t('Over the picture, when it has focus') }]}
              hint={t('Over the picture leaves just the picture and its corner to resize it by. The controls show when the pointer is on it, or when it has focus from the keyboard or a remote.')} />
            <fieldset className="ctv-choice">
              <legend>{t('Preview size')}</legend>
              <div className="ctv-choice-opts">
                <label className={!sizeChosen ? 'on' : ''}><input type="radio" name="Preview size" checked={!sizeChosen} onChange={forgetPreviewSize} /> {t('Automatic')}</label>
                {PREVIEW_SIZES.map((s) => <label key={s.w} className={look.previewW === s.w ? 'on' : ''}><input type="radio" name="Preview size" checked={look.previewW === s.w} onChange={() => setLook({ previewW: s.w })} /> {t(s.name)}</label>)}
              </div>
              <small>{sizeChosen ? t('Now: {size}.', { size: previewSizeName(look.previewW!) }) : t('Automatic suits the theme and the size of the window.')} {t("You can also drag the corner of the preview, or focus that corner and use the arrow keys. It's held to what fits this screen.")}</small>
            </fieldset>
          </>
        )}
        <Choice label={t('Row size')} value={look.density} onChange={(v) => setLook({ density: v })} options={[{ v: 'compact', label: t('Compact') }, { v: 'normal', label: t('Normal') }, { v: 'large', label: t('Large') }]} />
        <Choice<PageSize> label={t('Channels at a time')} value={guidePrefs.pageSize} onChange={(v) => setGuidePrefs({ pageSize: v })} options={PAGE_SIZES.map((n) => ({ v: n, label: n ? num(n) : t(pageSizeName(n)) }))}
          hint={t('A number shows that many channels, with Previous and Next, Page Up and Page Down. All scrolls through every channel. Kept on this device.')} />
        <Choice label={t('Hours across the screen')} value={look.hours} onChange={(v) => setLook({ hours: v })} options={[{ v: 0, label: t('What fits') }, { v: 1.5, label: num(1.5) }, { v: 2, label: num(2) }, { v: 3, label: num(3) }, { v: 4, label: num(4) }]} />
        <section className="ctv-show" aria-labelledby="ctv-show-h">
          <h3 id="ctv-show-h">{t('Show')}</h3>
          <div className="ctv-switch-rows" role="group" aria-label={t('Show')} data-nav-group>
            <SwitchRow label={t('A line at the current time')} hint={t('Down all channels')} on={look.nowLine} onChange={(v) => setLook({ nowLine: v })} />
            <SwitchRow label={t('How far into each programme')} hint={t('On the cells that are on now')} on={look.progress} onChange={(v) => setLook({ progress: v })} />
            <SwitchRow label={t('Episode names and details')} hint={t('In the rows')} on={look.subtitles} onChange={(v) => setLook({ subtitles: v })} />
            <SwitchRow label={t('Channel logos')} hint={t('Contain-fit, never cropped')} on={look.logos} onChange={(v) => setLook({ logos: v })} />
            <SwitchRow label={t("The programme's picture at the top")} hint={t('In the themes that have one')} on={look.art} onChange={(v) => setLook({ art: v })} />
          </div>
        </section>
        <BackgroundPlayChoice />
        {inSettings ? <p className="ctv-colour-note"><Link to="/tv">{t('See it on the guide')}</Link></p> : <p className="ctv-colour-note">{(() => { const [a, b = ''] = t('This is also in {link}.').split('{link}'); return <>{a}<Link to="/settings/appearance#wittytv">{t('Settings, Appearance')}</Link>{b}</> })()}</p>}
      </section>
    </div>
  )
}

export default function LookPanel({ look, onClose }: { look: Look; onClose: () => void }) {
  return (
    <Sheet className="ctv-look" title={t('How the guide looks')} onClose={onClose}
      actions={<><Button variant="ghost" onClick={resetLook}>{t('Back to how it started')}</Button><Button variant="primary" onClick={onClose}>{t('Done')}</Button></>}>
      <p className="ctv-eyebrow">{t('WittyTV · Look')}</p>
      <p className="muted">{t('This changes only WittyTV. Everything else keeps the look and theme you chose. Changes show straight away, and follow you to your other devices.')}</p>
      <LookEditor look={look} />
    </Sheet>
  )
}
