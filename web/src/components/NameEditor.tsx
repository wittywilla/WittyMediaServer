// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useRef, useState, type CSSProperties, type KeyboardEvent } from 'react'
import { announce } from '../a11y'
import { nameWords, PRESETS, FIXED_SWATCHES, GRADIENT_SWATCHES, ACCENT, cssVarOf, fixed, gradient, isHex, markShown, nameLength, nameWarnings, partsFromCells, presetLabel, sameColor, spanCount, MAX_SPANS, type BrandColor, type Mark, type MarkKind } from '../brandStyle.ts'
import { DEFAULT_NAME } from '../brandName'
import { Button, Input, Segmented } from '../ui'
import { t, tn, tx } from '../i18n'

// brandStyle.ts can't import the translations (node tests load it), so its “Accent on “…”” label is translated here.
const sayPreset = (label: string) => { const m = /^Accent on “(.*)”$/.exec(label); return m ? t('Accent on “{word}”', { word: m[1] }) : tx(label) }
import { Brand, markStyle } from './common'
import './nameEditor.css'

// The server's name editor (docs/design-2 BrandEditor): the name, a colour for any letters or words (the theme's, a fixed one,
// a gradient, or each letter its own), a mark for the rail and small spaces, a check that the words can be read, and the
// name drawn as it will be in every place it appears. A draft is edited here; the page around it saves it (saveBrand.ts).

export type BrandDraft = { name: string; cells: BrandColor[]; mark: Mark; logo: File | 'remove' | null }
export const MAX_LOGO = 2 << 20

const sameCells = (a: BrandColor[], b: BrandColor[]) => a.length === b.length && a.every((c, i) => sameColor(c, b[i]))

// Rename: the colours stay where they are by position, and the automatic colouring follows a name it was on.
export function renameDraft(d: BrandDraft, name: string): BrandDraft {
  const was = d.name.trim() || DEFAULT_NAME, now = name.trim() || DEFAULT_NAME
  const auto = PRESETS[0]
  const cells = sameCells(d.cells, auto.cells(was)) ? auto.cells(now) : Array.from({ length: nameLength(now) }, (_, i) => d.cells[i] || d.cells[d.cells.length - 1] || auto.cells(now)[i])
  return { ...d, name, cells }
}

// The letters as runs without a space, so a row wraps between words and never inside one (a space ends its run).
const wordsOf = (letters: string[]): number[][] => { const out: number[][] = []; let cur: number[] = []; letters.forEach((l, i) => { cur.push(i); if (/\s/u.test(l)) { out.push(cur); cur = [] } }); if (cur.length) out.push(cur); return out }
const range = (a: number, b: number) => Array.from({ length: Math.abs(b - a) + 1 }, (_, i) => Math.min(a, b) + i)
const hexOf = (c: BrandColor | undefined) => (c?.kind === 'static' ? c.hex || '' : '')

export function NameEditor({ draft, onChange, logoUrl, onLogoError, onCheck }: { draft: BrandDraft; onChange: (d: BrandDraft) => void; logoUrl: string; onLogoError?: (msg: string) => void; onCheck?: (readable: boolean) => void }) {
  const shown = draft.name.trim() || DEFAULT_NAME
  const letters = useMemo(() => Array.from(shown), [shown])
  const cells = draft.cells.length === letters.length ? draft.cells : Array.from({ length: letters.length }, (_, i) => draft.cells[i] || PRESETS[0].cells(shown)[i])
  const [anchor, setAnchor] = useState(5 < letters.length ? 5 : 0)
  const [focus, setFocus] = useState(5 < letters.length ? Math.min(9, letters.length - 1) : 0)
  const sel = range(Math.min(anchor, letters.length - 1), Math.min(focus, letters.length - 1))
  const [typed, setTyped] = useState('')
  const [stops, setStops] = useState(false)
  const set = (patch: Partial<BrandDraft>) => onChange({ ...draft, cells, ...patch })
  const paint = (c: BrandColor, say?: string) => {
    const next = cells.slice()
    for (const i of sel) next[i] = c
    set({ cells: next })
    announce(say || t('Colour changed'))
  }

  // The selection's colour, to show which swatch it has.
  const current = sel.every((i) => sameColor(cells[i], cells[sel[0]])) ? cells[sel[0]] : undefined
  const words = nameWords(shown)

  // Letters: a row, one Tab stop. Arrows move, with Shift they select a range, Space selects the one the focus is on.
  const chips = useRef<HTMLDivElement | null>(null)
  const move = (to: number, extend: boolean) => {
    const at = Math.max(0, Math.min(letters.length - 1, to))
    setFocus(at)
    if (!extend) setAnchor(at)
    chips.current?.querySelectorAll<HTMLElement>('button')[at]?.focus()
  }
  const onKey = (e: KeyboardEvent) => {
    if (e.key === 'ArrowRight') { e.preventDefault(); move(focus + 1, e.shiftKey) }
    else if (e.key === 'ArrowLeft') { e.preventDefault(); move(focus - 1, e.shiftKey) }
    else if (e.key === 'Home') { e.preventDefault(); move(0, e.shiftKey) }
    else if (e.key === 'End') { e.preventDefault(); move(letters.length - 1, e.shiftKey) }
    else if (e.key === 'a' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); setAnchor(0); setFocus(letters.length - 1) }
    else if (e.key === ' ') { e.preventDefault(); setFocus(focus); if (!e.shiftKey) setAnchor(focus) }
  }
  const selectWord = ([a, b]: [number, number]) => { setAnchor(a); setFocus(b - 1); announce(t('{letters} selected', { letters: letters.slice(a, b).join('') })) }

  // The preview and the check read the page's own colours.
  const frame = useRef<HTMLDivElement | null>(null)
  const [tick, setTick] = useState(0)
  useEffect(() => {
    const ob = new MutationObserver(() => setTick((t) => t + 1))
    ob.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme', 'style'] })
    return () => ob.disconnect()
  }, [])
  const parts = useMemo(() => partsFromCells(shown, cells), [shown, cells])
  const warnings = useMemo(() => {
    const host = frame.current || document.body
    const read = (v: string) => { const el = document.createElement('i'); el.style.color = v; host.appendChild(el); const c = getComputedStyle(el).color; el.remove(); return c }
    const resolve = (t: string) => read(t === 'brandAccent' ? 'var(--brand-accent)' : `var(${cssVarOf(t)})`)
    const css = (v: string) => read(`var(${v})`)
    return nameWarnings(parts, [{ name: t('the side menu'), colour: css('--bg-2') }, { name: t('the top bar'), colour: css('--bg') }], resolve)
  }, [parts, tick]) // eslint-disable-line react-hooks/exhaustive-deps
  const tooMany = spanCount(cells) > MAX_SPANS
  const readable = warnings.length === 0 && !tooMany
  useEffect(() => { onCheck?.(readable) }, [readable]) // eslint-disable-line react-hooks/exhaustive-deps

  const markText = markShown(shown, draft.mark)
  const mst = draft.mark.kind === 'image' ? undefined : markStyle(parts)
  const Badge = ({ big }: { big?: boolean }) => (
    <span className={'ne-badge' + (big ? ' big' : '')} aria-hidden="true" style={mst}>
      {draft.mark.kind === 'image' ? <img src={logoUrl || '/icon.svg'} alt="" onError={() => onLogoError?.(t('The logo could not be shown'))} /> : markText}
    </span>
  )
  const pickLogo = (f: File | undefined) => {
    if (!f) return
    if (!/^image\/(png|jpeg)$/.test(f.type)) { onLogoError?.(t('Use a PNG or JPEG image.')); return }
    if (f.size > MAX_LOGO) { onLogoError?.(t('The logo must be 2 MB or smaller.')); return }
    onLogoError?.('')
    set({ logo: f, mark: { kind: 'image' } })
  }
  const file = useRef<HTMLInputElement | null>(null)
  const colourInput = useRef<HTMLInputElement | null>(null)
  // What the selection is, for what's announced: the letter itself, or how many.
  const selWhat = () => (sel.length === 1 ? letters[sel[0]] : tn('{count} letter', '{count} letters', sel.length))
  const swatch = (c: BrandColor, label: string, bg: string) => (
    <button key={label} type="button" className={'ne-sw' + (current && sameColor(current, c) ? ' on' : '')} aria-pressed={!!current && sameColor(current, c)} aria-label={label} title={label}
      style={{ background: bg, ['--sw' as string]: bg } as CSSProperties} onClick={() => paint(c, t('{colour} for {what}', { colour: label, what: selWhat() }))} />
  )
  const [gradStops, setGradStops] = useState<string[]>(GRADIENT_SWATCHES[0].stops || [])
  const [angle, setAngle] = useState(90)
  const gradCss = (st: string[], a: number) => `linear-gradient(${a}deg, ${st.join(', ')})`

  return (
    <div className="ne">
      <div className="ne-form">
        <label className="ne-lab" htmlFor="ne-name">{t('Name')}</label>
        <Input id="ne-name" className="ne-name" value={draft.name} maxLength={40} placeholder={DEFAULT_NAME} onChange={(e) => onChange(renameDraft(draft, e.target.value))} />
        <span className="ne-hint">{t('Leave empty to use “{name}”.', { name: DEFAULT_NAME })}</span>

        <div className="ne-lab" id="ne-letters">{t('Choose letters or words to colour')}</div>
        <div ref={chips} className="ne-chips" role="group" aria-labelledby="ne-letters" aria-describedby="ne-letters-help" onKeyDown={onKey}>
          {wordsOf(letters).map((g, gi) => (
            <span key={gi} className="ne-group">{g.map((i) => {
              const l = letters[i], on = sel.includes(i)
              return <button key={i} type="button" className={'ne-chip' + (on ? ' on' : '')} aria-pressed={on} tabIndex={i === focus ? 0 : -1} aria-label={/\s/u.test(l) ? t('space') : l}
                style={partChipStyle(cells[i])}
                onFocus={() => setFocus(i)} onClick={(e) => { if (e.shiftKey) setFocus(i); else { setAnchor(i); setFocus(i) } }}>{/\s/u.test(l) ? '\u00a0' : l}</button>
            })}</span>
          ))}
        </div>
        <div className="ne-hint" id="ne-letters-help">{t('Arrows move, Shift and the arrows select a range, Space selects the letter.')} {words.length > 1 && <>{t('Words:')} {words.map(([a, b], k) => <span key={a}>{k > 0 && ' · '}<button type="button" className="ne-word" onClick={() => selectWord([a, b])}>{letters.slice(a, b).join('')}</button></span>)}.</>}</div>

        <div className="ne-lab">{t('Colour for the selection')}</div>
        <div className="ne-sub">{t('Theme colour (changes with the theme)')}</div>
        <div className="ne-row">{(() => { const on = !!current && sameColor(current, ACCENT); return (
          <button type="button" className={'ne-theme' + (on ? ' on' : '')} aria-pressed={on} onClick={() => paint(ACCENT, t('Theme colour for {what}', { what: selWhat() }))}>
            <span className="ne-theme-dot" aria-hidden="true" />{t('Follow the theme')}</button>) })()}</div>
        <div className="ne-sub">{t('Fixed colour (stays the same in every theme)')}</div>
        <div className="ne-row" role="group" aria-label={t('Fixed colours')}>
          {FIXED_SWATCHES.map((h) => swatch(fixed(h), h.toUpperCase(), h))}
          <Button size="sm" onClick={() => colourInput.current?.click()}>{t('Any colour…')}</Button>
          <input ref={colourInput} type="color" className="ne-hidden" tabIndex={-1} aria-label={t('Pick any colour')} value={isHex(hexOf(current)) ? hexOf(current) : '#b4491f'} onChange={(e) => paint(fixed(e.target.value))} />
          <Input className="ne-hex" inputSize="sm" aria-label={t('Colour as #rrggbb')} placeholder="#B4491F" maxLength={7} value={typed || hexOf(current).toUpperCase()}
            onChange={(e) => { const v = e.target.value.startsWith('#') ? e.target.value : '#' + e.target.value; setTyped(e.target.value); if (isHex(v)) { paint(fixed(v)); setTyped('') } }} onBlur={() => setTyped('')} />
        </div>
        <div className="ne-sub">{t('Gradient')}</div>
        <div className="ne-row" role="group" aria-label={t('Gradients')}>
          {GRADIENT_SWATCHES.map((g, i) => <button key={i} type="button" className={'ne-sw grad' + (current && sameColor(current, g) ? ' on' : '')} aria-pressed={!!current && sameColor(current, g)} aria-label={t('Gradient {stops}', { stops: (g.stops || []).join(' ' + t('to') + ' ') })}
            style={{ background: gradCss(g.stops || [], 90) }} onClick={() => paint(g, t('Gradient'))} />)}
          <Button size="sm" aria-expanded={stops} onClick={() => setStops((s) => !s)}>{t('Edit stops…')}</Button>
        </div>
        {stops && (
          <div className="ne-stops" role="group" aria-label={t('Gradient stops')}>
            {gradStops.map((s, i) => (
              <span key={i} className="ne-stop"><input type="color" aria-label={t('Stop {n}', { n: i + 1 })} value={s} onChange={(e) => setGradStops((g) => g.map((x, j) => (j === i ? e.target.value : x)))} />
                {gradStops.length > 2 && <Button size="sm" variant="ghost" aria-label={t('Remove stop {n}', { n: i + 1 })} onClick={() => setGradStops((g) => g.filter((_, j) => j !== i))}>{t('Remove')}</Button>}</span>
            ))}
            {gradStops.length < 4 && <Button size="sm" variant="ghost" onClick={() => setGradStops((g) => [...g, g[g.length - 1]])}>{t('Add a stop')}</Button>}
            <label className="ne-angle">{t('Direction')} <Input inputSize="sm" type="number" min={0} max={360} step={15} value={angle} onChange={(e) => setAngle(Math.max(0, Math.min(360, Number(e.target.value) || 0)))} /> {t('degrees')}</label>
            <span className="ne-grad-show" aria-hidden="true" style={{ background: gradCss(gradStops, angle) }} />
            <Button size="sm" variant="primary" onClick={() => paint(gradient(gradStops, angle), t('Gradient'))}>{t('Use this gradient')}</Button>
          </div>
        )}

        {tooMany && <div className="ne-note bad" role="alert"><b>{t('Too many colour changes.')}</b> {t('A name can have {max} runs of colour; this one has {count}. Use fewer colours, or a gradient.', { max: MAX_SPANS, count: spanCount(cells) })}</div>}
        {warnings.length > 0 && !tooMany && (
          <div className="ne-note" role="status"><b>{t('Check:')}</b> {warnings.slice(0, 2).map((w, i) => <span key={i}>{i > 0 && ' '}{t('“{text}” is {ratio} to 1 on {surface}.', { text: w.text, ratio: w.ratio, surface: w.surface })}</span>)} {t('Words should be at least 4.5 to 1. Use a lighter colour or follow the theme.')}</div>
        )}

        <div className="ne-lab">{t('Presets')}</div>
        <div className="ne-row wrap" role="group" aria-label={t('Presets')}>
          {PRESETS.map((p) => {
            const c = p.cells(shown)
            return <Button key={p.id} variant={sameCells(cells, c) ? 'primary' : 'default'} aria-pressed={sameCells(cells, c)} onClick={() => { set({ cells: c }); announce(tx(p.label)) }}>{p.id === 'auto' ? sayPreset(presetLabel(shown)) : tx(p.label)}</Button>
          })}
        </div>

        <div className="ne-lab">{t('Mark for the rail and small spaces')}</div>
        <div className="ne-row wrap">
          <Badge big />
          <Segmented<MarkKind> label={t('Mark')} value={draft.mark.kind} onChange={(k) => { if (k === 'image') set({ mark: { kind: 'image' } }); else set({ mark: { kind: k, ...(k === 'text' ? { text: draft.mark.text || '' } : {}) } }) }} options={[
            { value: 'letter', label: t('Letter · {mark}', { mark: markShown(shown, { kind: 'letter' }) }) },
            { value: 'initials', label: t('Initials · {mark}', { mark: markShown(shown, { kind: 'initials' }) }) },
            { value: 'text', label: t('Your own') },
            { value: 'image', label: t('Logo') },
          ]} />
        </div>
        <div className="ne-row wrap">
          {draft.mark.kind === 'text' && <Input className="ne-own" aria-label={t('Your own one or two characters')} maxLength={2} placeholder={t('1 to 2 characters')} value={draft.mark.text || ''} onChange={(e) => set({ mark: { kind: 'text', text: e.target.value } })} />}
          <Button onClick={() => file.current?.click()}>{logoUrl || draft.logo instanceof File ? t('Replace the logo…') : t('Upload a logo…')}</Button>
          {(logoUrl || draft.logo instanceof File) && draft.logo !== 'remove' && <Button variant="ghost" onClick={() => set({ logo: 'remove' })}>{t('Remove the logo')}</Button>}
          <input ref={file} type="file" accept="image/png,image/jpeg" hidden onChange={(e) => { pickLogo(e.target.files?.[0]); e.target.value = '' }} />
        </div>
        <p className="ne-hint">{t('The mark is always centred in a square badge. Logo is the WittyMediaServer logo until you upload your own: a PNG or JPEG up to 2 MB, shown fitted inside the badge. It follows the name’s colours unless you choose fixed ones.')}</p>

      </div>

      <div className="ne-preview" ref={frame} aria-label={t('Preview')}>
        <div className="ne-lab">{t('Live preview, everywhere the name appears')}</div>
        <Place label={t('Side menu')} cls="side"><Brand name={shown} parts={parts} /></Place>
        <Place label={t('Top bar and TV bar')} cls="bar"><Brand name={shown} parts={parts} /><span className="ne-pill on">{t('Home')}</span><span className="ne-pill">{t('Libraries')}</span><span className="ne-pill">{t('Live TV')}</span></Place>
        <Place label={t('Rail header, when open')} cls="rail"><Badge /><Brand name={shown} parts={parts} /></Place>
        <Place label={t('Sign-in')} cls="sign"><div className="ne-sign"><Brand name={shown} parts={parts} /><span className="ne-btn">{t('Sign in')}</span></div></Place>
        <div className="ne-two">
          <Place label={t('On a light look')} cls="light"><Brand name={shown} parts={parts} /></Place>
          <Place label={t('On a black look')} cls="dark"><Brand name={shown} parts={parts} /></Place>
        </div>
      </div>
    </div>
  )
}

function Place({ label, cls, children }: { label: string; cls: string; children: React.ReactNode }) {
  return <div className={'ne-place ' + cls}><span className="ne-place-label">{label}</span>{children}</div>
}

// A letter's chip shows its own colour.
function partChipStyle(c: BrandColor | undefined): CSSProperties | undefined {
  if (!c || c.kind === 'fg') return undefined
  if (c.kind === 'gradient') return { ['--chip' as string]: (c.stops || [])[0] } as CSSProperties
  if (c.kind === 'static') return { ['--chip' as string]: c.hex } as CSSProperties
  return { ['--chip' as string]: c.kind === 'accent' ? 'var(--brand-accent)' : `var(${cssVarOf(c.token || 'ok')})` } as CSSProperties
}

