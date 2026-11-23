// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Fragment, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import { t, tx } from '../../i18n'
import { IconCheck, IconDown } from '../../icons'
import { Button, Dialog } from '../../ui'
import { ranked, type Dir } from '../../ui/spatial'
import type { Facts, StepInfo } from './plan'

// The pieces every step of first-time setup is made from. Each works with a mouse, touch, a
// keyboard, a remote with only arrows and OK, and a screen reader, so a step doesn't have to think
// about any of that. Real buttons throughout: OK on a remote presses a button, and a native
// checkbox only answers to the space bar.

// What SetupWizard gives each step.
export type StepProps = {
  info: StepInfo
  facts: Facts
  // Ask again what's on this server, after something changed it.
  refresh: () => Promise<void>
  next: () => void
  back?: () => void
}

export function StepHead({ title, lead }: { title: string; lead?: ReactNode }) {
  return (
    <>
      <h1>{title}</h1>
      {lead && <p className="lead">{lead}</p>}
    </>
  )
}

// Every step that asks something says where the answer can be changed.
export function LaterNote({ info }: { info: StepInfo }) {
  if (!info.later) return null
  return <p className="su-later">{t('You can change this later in {where}.', { where: tx(info.later.where) })}</p>
}

// The row at the foot of a step: Back on the left, what happens next on the right.
export function StepActions({ back, note, children }: { back?: () => void; note?: ReactNode; children: ReactNode }) {
  return (
    <div className="oobe-actions su-actions">
      {back ? <Button variant="ghost" onClick={back}>{t('Back')}</Button> : <span />}
      {note && <span className="muted oobe-grow">{note}</span>}
      <span className="su-actions-go">{children}</span>
    </div>
  )
}

// A page of advanced setup: a short title, one sentence, what it asks, and Continue. What it asks saves as it's changed.
export function PanelStep({ title, lead, back, next, busy, children, go }: { title: string; lead: string; back?: () => void; next: () => void; busy?: boolean; children: ReactNode; go?: string }) {
  return (
    <section>
      <StepHead title={title} lead={lead} />
      {children}
      <StepActions back={back}><Button variant="primary" size="big" busy={busy} onClick={next}>{go || t('Continue')}</Button></StepActions>
    </section>
  )
}

// The optional choices of a step, kept out of the way: a Customize button beside the step's own, which opens them in a
// dialog. Everything in it already has the usual answer, so nobody has to open it to finish setup.
export function Customize({ title, children, label }: { title: string; children: ReactNode; label?: string }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <Button variant="ghost" onClick={() => setOpen(true)} aria-haspopup="dialog">{label || t('Customize')}</Button>
      {open && (
        <Dialog title={title} size="wide" className="su-customize" onClose={() => setOpen(false)}
          actions={<Button variant="primary" onClick={() => setOpen(false)}>{t('Done')}</Button>}>
          <div className="su-customize-body">{children}</div>
        </Dialog>
      )}
    </>
  )
}

// One part of a Customize dialog, folded until it's opened, with what it is in a few words.
export function Option({ title, text, children, open }: { title: string; text?: string; children: ReactNode; open?: boolean }) {
  return (
    <details className="su-option" open={open}>
      <summary><span className="su-option-words"><b>{title}</b>{text && <small>{text}</small>}</span><IconDown size={20} /></summary>
      <div className="su-option-body">{children}</div>
    </details>
  )
}

// One of a few, as cards. Tab is one stop. The arrows move between the cards as they're laid out, and
// OK or the space bar chooses. They don't choose as they go, as a remote has to pass through the
// cards to reach what's below them, and that mustn't change the answer. Past the edge they're left
// alone, so a remote moves on to what's next.
export type Card<T extends string> = { value: T; title: string; text?: string; pic?: ReactNode }

export function ChoiceCards<T extends string>({ label, value, onChange, cards, small }: { label: string; value: T; onChange: (v: T) => void; cards: Card<T>[]; small?: boolean }) {
  const box = useRef<HTMLDivElement | null>(null)
  const keys: Record<string, Dir> = { ArrowUp: 'up', ArrowDown: 'down', ArrowLeft: 'left', ArrowRight: 'right' }
  const buttons = () => Array.from(box.current?.querySelectorAll<HTMLElement>('[role=radio]') || [])
  const land = (i: number, e: KeyboardEvent) => {
    e.preventDefault()
    buttons()[i]?.focus()
  }
  const onKey = (e: KeyboardEvent) => {
    const all = buttons()
    const at = all.indexOf(document.activeElement as HTMLElement)
    if (at < 0) return
    if (e.key === 'Home') return land(0, e)
    if (e.key === 'End') return land(all.length - 1, e)
    const dir = keys[e.key]
    if (!dir) return
    const to = ranked(all[at].getBoundingClientRect(), all.filter((_, i) => i !== at), (el) => el.getBoundingClientRect(), dir)[0]
    if (to) land(all.indexOf(to), e)
  }
  const on = Math.max(0, cards.findIndex((c) => c.value === value))
  return (
    <div ref={box} className={'su-cards' + (small ? ' small' : '')} role="radiogroup" aria-label={label} data-nav-group onKeyDown={onKey}>
      {cards.map((c, i) => (
        <button key={c.value} type="button" role="radio" aria-checked={c.value === value} tabIndex={i === on ? 0 : -1}
          className={'su-card' + (c.value === value ? ' on' : '')} onClick={() => onChange(c.value)}>
          {c.pic}
          <span className="su-card-mark" aria-hidden="true">{c.value === value && <IconCheck size={16} />}</span>
          <b>{c.title}</b>
          {c.text && <span className="su-card-text">{c.text}</span>}
        </button>
      ))}
    </div>
  )
}

// A yes or no, as a whole row that can be pressed. A checkbox when it's saved with the step, a
// switch when it takes effect at once, as docs/DESIGN.md asks. Nothing else that can be pressed
// goes inside: a link belongs beside it.
export function TickRow({ checked, onChange, title, text, icon, kind = 'checkbox', disabled, note }: {
  checked: boolean; onChange?: (v: boolean) => void; title: ReactNode; text?: ReactNode; icon?: ReactNode; kind?: 'checkbox' | 'switch'; disabled?: boolean; note?: ReactNode
}) {
  return (
    <button type="button" role={kind} aria-checked={checked} aria-label={typeof title === 'string' ? title : undefined} aria-disabled={disabled || undefined} className={'su-tick' + (checked ? ' on' : '') + (disabled ? ' off' : '')}
      onClick={disabled ? undefined : () => onChange?.(!checked)}>
      {kind === 'checkbox'
        ? <span className="su-tick-box" aria-hidden="true">{checked && <IconCheck size={16} />}</span>
        : <span className={'set-switch su-tick-switch' + (checked ? ' on' : '')} aria-hidden="true"><i /></span>}
      {icon && <span className="su-tick-ic" aria-hidden="true">{icon}</span>}
      <span className="su-tick-words">
        <b>{title}</b>
        {text && <span className="su-tick-text">{text}</span>}
        {note && <small>{note}</small>}
      </span>
    </button>
  )
}

// A translated sentence with pieces of the page in it: each {name} left in the text is replaced by parts[name].
export function Fill({ text, parts }: { text: string; parts: Record<string, ReactNode> }) {
  return <>{text.split(/\{(\w+)\}/).map((bit, i) => <Fragment key={i}>{i % 2 ? parts[bit] ?? `{${bit}}` : bit}</Fragment>)}</>
}

// Where the setup code is, for someone setting the server up from another device.
export function WhereIsTheCode() {
  return (
    <div className="su-where">
      <p>{t('You’re on a different device from the server, so it asks for a code to prove the server is yours. The server wrote the code down in three places. You only need one:')}</p>
      <ul>
        <li><b>{t('The server’s window.')}</b> {t('It’s printed when the server starts, under “This server has no accounts yet”.')}</li>
        <li><b>Docker.</b> <Fill text={t('Run {logs} followed by the name of the container, such as {example}. On Linux, {journal} shows it too.')} parts={{ logs: <code>docker logs</code>, example: <code>docker logs wittymediaserver</code>, journal: <code>journalctl -u wittymediaserver</code> }} /></li>
        <li><b>{t('A file.')}</b> <Fill text={t('It’s in {file}, in the server’s data folder:')} parts={{ file: <code>setup-code.txt</code> }} />
          <ul>
            <li>Windows: <Fill text={t('{folder}, or {service} if it runs as a service')} parts={{ folder: <code>%LOCALAPPDATA%\WittyMediaServer</code>, service: <code>C:\ProgramData\WittyMediaServer</code> }} /></li>
            <li>{t('A Mac:')} <code>~/Library/Application Support/WittyMediaServer</code></li>
            <li>Linux: <code>/var/lib/wittymediaserver</code></li>
            <li>FreeBSD: <code>/var/db/wittymediaserver</code></li>
            <li>Docker: {t('the folder you gave it for its data')}</li>
          </ul>
        </li>
      </ul>
      <p>{t('Setting up on the server’s own screen needs no code: open this page there instead. The code changes each time the server starts, and the file goes away once setup is done.')}</p>
    </div>
  )
}
