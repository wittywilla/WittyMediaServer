import { useMemo, useState, type KeyboardEvent } from 'react'
import { accentMask, cutsFor, gaps, isSpace, join, letters, pickFromMask, pieces, split } from '../brandName'
import { Segmented } from '../ui'

// The Colouring part of the rename window: every other word automatically, the
// words chosen by hand, or the whole name in one colour. Choosing by hand shows
// the words as buttons that colour them, and a second view where the gaps
// between letters are buttons that split a word there. Each row is one Tab
// stop, moved along with the arrows (and so with a remote).

export type Mode = 'auto' | 'pick' | 'one'

export const modeOf = (colouring: string): Mode => (colouring === 'one' ? 'one' : colouring.startsWith('pick:') ? 'pick' : 'auto')

// The choice being made: the mask says which characters are coloured, cuts
// where the words split.
export type Choice = { mode: Mode; cuts: Set<number>; mask: boolean[] }

export function startChoice(name: string, colouring: string): Choice {
  const mask = accentMask(name, colouring === 'one' ? 'auto' : colouring)
  return { mode: modeOf(colouring), cuts: cutsFor(name, mask), mask }
}

export function colouringOf(name: string, c: Choice): string {
  if (c.mode !== 'pick') return c.mode
  return pickFromMask(Array.from(name), c.mask)
}

// moveAlong: the arrows, Home and End within a row; at either end an arrow is
// left for the remote to carry on to what's beside.
function moveAlong(e: KeyboardEvent<HTMLElement>) {
  const items = Array.from(e.currentTarget.querySelectorAll<HTMLElement>('button'))
  const at = items.indexOf(document.activeElement as HTMLElement)
  let to = -1
  if (e.key === 'ArrowLeft' && at > 0) to = at - 1
  else if (e.key === 'ArrowRight' && at < items.length - 1) to = at + 1
  else if (e.key === 'Home') to = 0
  else if (e.key === 'End') to = items.length - 1
  if (to < 0) return
  e.preventDefault()
  items[to].focus()
}

const quote = (s: string) => `“${s}”`

export function BrandColouring({ name, choice, onChange }: { name: string; choice: Choice; onChange: (c: Choice) => void }) {
  const [view, setView] = useState<'colour' | 'split'>('colour')
  const [current, setCurrent] = useState(0)
  const r = useMemo(() => Array.from(name), [name])
  const splittable = useMemo(() => gaps(name), [name])
  const list = pieces(r, choice.cuts)
  const text = ([a, b]: [number, number]) => r.slice(a, b).join('')
  const setMode = (mode: Mode) => onChange({ ...choice, mode })

  const toggle = ([a, b]: [number, number]) => {
    const mask = choice.mask.slice()
    for (let i = a; i < b; i++) mask[i] = !choice.mask[a]
    onChange({ ...choice, mask })
  }
  const cutAt = (i: number) => {
    const [cuts, mask] = choice.cuts.has(i) ? join(r, choice.cuts, choice.mask, i) : split(r, choice.cuts, choice.mask, i)
    onChange({ ...choice, cuts, mask })
  }

  // The row's Tab stop is the button last used, or the first.
  let stop = 0, gapIndex = 0
  const last = (view === 'colour' ? list.length : splittable.size) - 1
  const tab = () => (stop++ === Math.min(current, last) ? 0 : -1)
  const used = (n: number) => () => setCurrent(n)

  return (
    <div className="brand-field brand-colouring" role="group" aria-labelledby="brand-colouring-label">
      <span id="brand-colouring-label">Colouring</span>
      <Segmented label="Colouring" value={choice.mode} onChange={setMode} options={[
        { value: 'auto', label: 'Automatic' },
        { value: 'pick', label: 'Choose the words' },
        { value: 'one', label: 'One colour' },
      ]} />
      {choice.mode === 'auto' && <span className="muted">Every other word is in the theme colour. Words are split at spaces and at capital letters.</span>}
      {choice.mode === 'one' && <span className="muted">The whole name is in one colour.</span>}
      {choice.mode === 'pick' && (
        <>
          <Segmented label="What choosing does" value={view} onChange={(v) => { setView(v); setCurrent(0) }} options={[
            { value: 'colour', label: 'Colour words' },
            { value: 'split', label: 'Split words' },
          ]} />
          <span className="muted" id="brand-pick-help">
            {view === 'colour' ? 'Choose a word to put it in the theme colour, or back. Changing the name starts again from the automatic split.' : 'Choose a gap between two letters to split the word there, or to join it again.'}
          </span>
          {view === 'colour' ? (
            <div className="brand-pick" role="group" aria-label="Words in the theme colour" aria-describedby="brand-pick-help" onKeyDown={moveAlong}>
              {list.map((p, n) => {
                // A word split in two shows as two buttons touching.
                const before = n > 0 && p[0] > 0 && !isSpace(r[p[0] - 1]) ? ' brand-pick-joined' : ''
                return (
                  <button key={p[0]} type="button" className={'brand-pick-word' + (choice.mask[p[0]] ? ' on' : '') + before} aria-pressed={choice.mask[p[0]]}
                    tabIndex={tab()} onFocus={used(n)} onClick={() => toggle(p)}>{text(p)}</button>
                )
              })}
            </div>
          ) : (
            <div className="brand-pick brand-pick-letters" role="group" aria-label="Where the words split" aria-describedby="brand-pick-help" onKeyDown={moveAlong}>
              {letters(name).map(([i, l]) => {
                const letter = <span key={'l' + i} className={'brand-pick-letter' + (choice.mask[i] ? ' on' : '')} aria-hidden="true">{isSpace(l) ? '\u00a0' : l}</span>
                if (!splittable.has(i)) return letter
                const cut = choice.cuts.has(i)
                const left = list.find(([, b]) => b === i), right = list.find(([a]) => a === i), whole = list.find(([a, b]) => a < i && i < b)
                const label = cut && left && right ? `Join ${quote(text(left))} and ${quote(text(right))}` : whole ? `Split ${quote(text(whole))} after ${quote(r.slice(whole[0], i).join(''))}` : 'Split here'
                const k = gapIndex++
                return [
                  <button key={'g' + i} type="button" className={'brand-pick-gap' + (cut ? ' on' : '')} aria-label={label} title={label}
                    tabIndex={tab()} onFocus={used(k)} onClick={() => cutAt(i)}><i /></button>,
                  letter,
                ]
              })}
            </div>
          )}
        </>
      )}
    </div>
  )
}
