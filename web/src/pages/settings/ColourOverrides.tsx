// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useId, useMemo, useState, type CSSProperties } from 'react'
import { ApiError } from '../../api'
import { checkContrast, draftTokens, normalizeColour, NOT_A_COLOUR, overlay, pickerValue, sameColours, tokenErrors, tokenGroups, tokenLabel, withPicked, type Colours, type ContrastIssue, type Draft, type TokenInfo } from '../../themeColours'
import { tokenVar } from '../../themeOverrides'
import { Button } from '../../ui/Button'
import { Segmented } from '../../ui/Choice'
import { Input } from '../../ui/Field'
import { Notice } from '../../ui/Surface'
import './engineColours.css'

// One set of colour overrides of the theme engine, being edited: the server's own (for
// everyone following its theme) or one module's. Each colour the server takes has a row with
// a picker, the colour as text and a way back to the theme's. What a change does to the
// text's contrast is said as it is typed, and each look of the theme is drawn with it.

// A look of the theme: dark or light, the colours under these overrides, and which of the
// colours that are otherwise worked out from others something below has set outright.
export type Look = { scheme: 'dark' | 'light'; base: Colours; set: string[] }
export type Warning = ContrastIssue & { variant?: string }

const capital = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

// The colours every theme's stylesheet sets itself. The rest are worked out from these (a
// hairline is a tenth of the text colour), so the drawing sets them only when someone has.
const OUTRIGHT = ['bg', 'bg2', 'bg3', 'bg4', 'fg', 'fg2', 'fg3', 'accent', 'accentHover', 'accentFg', 'accentText', 'brandAccent', 'danger', 'ok', 'warn', 'border', 'controlLine', 'overlay']
const rgbOf = (c: string) => [1, 3, 5].map((i) => parseInt(c.slice(i, i + 2), 16)).join(' ')

function previewVars(look: Look, tokens: Colours): CSSProperties {
  const all = overlay(look.base, tokens)
  const vars: Record<string, string> = { colorScheme: look.scheme }
  for (const name of new Set([...OUTRIGHT, ...look.set, ...Object.keys(tokens)])) {
    const c = normalizeColour(all[name] || '')
    if (c) vars[tokenVar(name)] = c
  }
  for (const [name, twin] of [['bg', '--bg-rgb'], ['accent', '--accent-rgb']]) {
    const c = normalizeColour(all[name] || '')
    if (c) vars[twin] = rgbOf(c)
  }
  return vars as CSSProperties
}

// A small piece of the app in one look, with the colours as they'd be: the page, a panel and
// a card on it, the three weights of text, a link, the accent button, a field's outline, a
// selected row and the three status colours.
function LookPreview({ look, tokens, caption }: { look: Look; tokens: Colours; caption: string }) {
  return (
    <figure className="ec-look">
      <div className="ec-preview" style={previewVars(look, tokens)} role="img" aria-label={caption + ', drawn with these colours'}>
        <div className="ec-panel">
          <b>Title</b>
          <span className="ec-fg2">Secondary text</span>
          <span className="ec-fg3">A hint or caption</span>
          <span className="ec-link">A link</span>
        </div>
        <div className="ec-page">
          <div className="ec-card">
            <b>On a card</b>
            <span className="ec-fg2">Secondary text</span>
            <span className="ec-fg3">A hint</span>
            <span className="ec-link">A link</span>
          </div>
          <div className="ec-line">
            <span className="btn primary sm">Play</span>
            <span className="ec-field">Field</span>
            <span className="ec-picked">Selected</span>
          </div>
          <div className="ec-line ec-status">
            <span className="ec-danger">Error</span>
            <span className="ec-ok">Saved</span>
            <span className="ec-warn">Warning</span>
          </div>
        </div>
      </div>
      <figcaption>{caption}</figcaption>
    </figure>
  )
}

const warningText = (w: Warning, many: boolean) =>
  `${tokenLabel(w.text)} on ${tokenLabel(w.surface)}${many && w.variant ? ` in the ${w.variant} look` : ''}: ${w.ratio}:1`

function ColourRow({ t, value, theirs, error, serverError, short, onChange }: {
  t: TokenInfo; value: string; theirs: string[]; error: string; serverError: boolean; short: string[]; onChange: (v: string) => void
}) {
  const id = useId()
  const set = value.trim() !== ''
  const shown = normalizeColour(value) || theirs[0] || ''
  const notes = [id + 'h', error ? id + 'e' : '', short.length ? id + 'w' : ''].filter(Boolean).join(' ')
  return (
    <div className={'ec-row' + (set ? ' set' : '')}>
      <div className="ec-row-text">
        <div className="ec-row-label" id={id + 'l'}>{t.label}{set && <span className="set-badge">Changed</span>}</div>
        <div className="ec-row-hint" id={id + 'h'}>{t.hint}</div>
      </div>
      <div className="ec-row-ctl" role="group" aria-labelledby={id + 'l'}>
        <Input className="theme-colour ec-pick" type="color" aria-label={`${t.label}: pick a colour`} value={pickerValue(shown)}
          onChange={(e) => onChange(withPicked(e.target.value, shown))} />
        <Input className={'ec-code' + (theirs.length > 1 ? ' two' : '')} type="text" inputSize="sm" aria-label={`${t.label}: the colour as a code`} aria-describedby={notes} aria-invalid={error ? true : undefined}
          spellCheck={false} autoCapitalize="off" autoCorrect="off" autoComplete="off" maxLength={40}
          placeholder={theirs.join(' / ') || 'The theme’s'} value={value} onChange={(e) => onChange(e.target.value)} />
        <Button size="sm" variant="ghost" disabled={!set} aria-label={`${t.label}: use the theme’s colour`} onClick={() => onChange('')}>Reset</Button>
      </div>
      {error && <div className="ui-field-error ec-row-note" id={id + 'e'} role={serverError ? 'alert' : undefined}>{capital(error)}</div>}
      {short.length > 0 && <div className="ec-row-note ec-short" id={id + 'w'}>Hard to read: {short.join('; ')}. Text needs 4.5:1.</div>}
    </div>
  )
}

export default function ColourOverrides({ looks, names, saved, checked, hasOwn, save, clear, clearLabel }: {
  looks: Look[]
  // The colour names the server takes.
  names: string[]
  // The overrides as saved.
  saved: Colours
  // What the server said about the saved ones (null: it wasn't asked about this theme).
  checked: Warning[] | null
  // Whether anything is saved, so there is something to take away.
  hasOwn: boolean
  // save sends the overrides; it throws with the server's words when it refuses them.
  save: (tokens: Colours) => Promise<void>
  clear: () => Promise<void>
  clearLabel: string
}) {
  const groups = useMemo(() => tokenGroups(names), [names])
  const [group, setGroup] = useState('')
  const [draft, setDraft] = useState<Draft>(saved)
  const [refused, setRefused] = useState<{ fields: Record<string, string>; other: string }>({ fields: {}, other: '' })
  const [busy, setBusy] = useState(false)
  // What's saved changed under the page (saved here, or another module chosen): start from it.
  const savedKey = JSON.stringify(saved)
  useEffect(() => { setDraft(saved); setRefused({ fields: {}, other: '' }) }, [savedKey]) // eslint-disable-line react-hooks/exhaustive-deps

  const { tokens, bad } = useMemo(() => draftTokens(draft), [draft])
  const dirty = bad.length > 0 || !sameColours(tokens, saved)
  const many = looks.length > 1
  // While something is being changed the warnings are worked out here, with the server's own
  // pairs and sums; once saved, what the server answered is shown.
  const live: Warning[] = useMemo(() => looks.flatMap((l) => checkContrast(overlay(l.base, tokens), tokens).map((w) => ({ ...w, variant: many ? l.scheme : undefined }))), [looks, tokens, many])
  const warnings = !dirty && checked ? checked : live
  // At a row: the text colour that's hard to read, and the surface when it was the surface that changed.
  const shortOf = (name: string) => warnings.filter((w) => w.text === name || (w.surface === name && name in tokens)).map((w) => warningText(w, many))

  const change = (name: string, value: string) => {
    setDraft((d) => ({ ...d, [name]: value }))
    setRefused((r) => (r.fields[name] || r.other ? { fields: Object.fromEntries(Object.entries(r.fields).filter(([k]) => k !== name)), other: '' } : r))
  }
  const run = async (work: () => Promise<void>) => {
    setBusy(true)
    setRefused({ fields: {}, other: '' })
    try { await work() } catch (e) {
      // The server's own words, before the app made a sentence of them: each part starts with
      // the colour it is about.
      const words = e instanceof ApiError && typeof e.data?.error === 'string' ? e.data.error : (e as Error).message
      const said = tokenErrors(words, names)
      setRefused(said)
      // The first colour the server refused may be in a group that isn't showing.
      const first = groups.find((g) => g.tokens.some((t) => said.fields[t.name]))
      if (first) setGroup(first.id)
    } finally { setBusy(false) }
  }

  const at = groups.find((g) => g.id === group) || groups[0]
  if (!at || looks.length === 0) return null
  const count = (ids: TokenInfo[]) => ids.filter((t) => (draft[t.name] || '').trim()).length
  return (
    <div className="ec">
      <div className="ec-edit">
        <Segmented className="ec-groups" label="Which colours" value={at.id} onChange={setGroup}
          options={groups.map((g) => ({ value: g.id, label: count(g.tokens) ? `${g.title} (${count(g.tokens)})` : g.title }))} />
        <div className="ec-rows">
          {at.tokens.map((t) => (
            <ColourRow key={t.name} t={t} value={draft[t.name] || ''} theirs={looks.map((l) => l.base[t.name]).filter(Boolean)}
              error={refused.fields[t.name] || (bad.includes(t.name) ? NOT_A_COLOUR : '')} serverError={!!refused.fields[t.name]}
              short={shortOf(t.name)} onChange={(v) => change(t.name, v)} />
          ))}
        </div>
      </div>
      <div className="ec-looks">
        {looks.map((l) => <LookPreview key={l.scheme} look={l} tokens={tokens} caption={many ? (l.scheme === 'dark' ? 'The dark look' : 'The light look') : 'How it looks'} />)}
      </div>
      <div className="ec-foot">
        {refused.other && <Notice tone="danger">{capital(refused.other)}</Notice>}
        {bad.length > 0 && <Notice tone="warn">{bad.map(tokenLabel).join(', ')}: {NOT_A_COLOUR}.</Notice>}
        {warnings.length > 0 && (
          <Notice tone="warn" title={warnings.length === 1 ? 'One piece of text would be hard to read' : `${warnings.length} pieces of text would be hard to read`}>
            <ul className="ec-warnings">
              {warnings.map((w) => <li key={`${w.variant}/${w.text}/${w.surface}`}>{warningText(w, many)}</li>)}
            </ul>
            Text needs a contrast of 4.5:1 to be easy to read. You can save anyway.
            {!dirty && checked && <span className="muted"> Checked by the server.</span>}
          </Notice>
        )}
        <div className="panel-actions">
          <Button variant="primary" busy={busy} disabled={!dirty || bad.length > 0} onClick={() => run(() => save(tokens))}>Save for everyone</Button>
          {dirty && <Button variant="ghost" disabled={busy} onClick={() => { setDraft(saved); setRefused({ fields: {}, other: '' }) }}>Undo changes</Button>}
          {hasOwn && <Button variant="ghost" disabled={busy} onClick={() => run(clear)}>{clearLabel}</Button>}
        </div>
      </div>
    </div>
  )
}
