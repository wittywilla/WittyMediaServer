// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The page as a phone, a tablet, a desktop or a television would be sent it, drawn by the
// app's own renderer (sdui/SduiPage's PageView) at that device's real width and scaled to
// fit the pane. It is a still drawing: nothing in it can be pressed, and the pane scrolls it. What it shows is the
// draft as last saved, asked for with a preview token, which is how the server tells this
// administrator's request from everyone else's.

import { useEffect, useLayoutEffect, useMemo, useRef, useState, type CSSProperties } from 'react'
import { ApiError } from '../../../api'
import { fetchPage } from '../../../sdui/client'
import type { Device } from '../../../sdui/device'
import { PageView } from '../../../sdui/SduiPage'
import type { Document } from '../../../sdui/types'
import { Button, Field, Input, Segmented, Select, Spinner } from '../../../ui'
import { useEditor } from './context'
import { PREVIEW_FRAMES, pageKindSpec, pageName, previewDevice, previewInputs, previewLibrary, previewScale, words, type FormFactor, type Page } from './editorModel.ts'

const FORM_FACTORS: FormFactor[] = ['phone', 'tablet', 'desktop', 'tv']

export function Preview({ page, saved, dirty, onSave, onParams }: {
  page: Page
  saved: number            // goes up each time the page is saved to the draft: time to look again
  dirty: boolean
  onSave: () => void
  onParams?: (params: Record<string, string>) => void // what the page is being previewed with, for "open in the app"
}) {
  const { cat, libraries, status, token } = useEditor()
  const [formFactor, setFormFactor] = useState<FormFactor>('phone')
  const inputs = useMemo(() => previewInputs(cat, page.kind), [cat, page.kind])
  const [params, setParams] = useState<Record<string, string>>({})
  const [doc, setDoc] = useState<Document | null>(null)
  const [state, setState] = useState<'idle' | 'loading' | 'error'>('idle')
  const [error, setError] = useState('')
  const [again, setAgain] = useState(0)
  const pane = useRef<HTMLDivElement | null>(null)
  const [paneWidth, setPaneWidth] = useState(0)
  const sheet = useRef<HTMLDivElement | null>(null)
  const [sheetHeight, setSheetHeight] = useState(0)

  // A library page starts on a library of the kind it is for.
  const matchKey = JSON.stringify(page.match || {})
  useEffect(() => {
    setParams((p) => {
      if (!inputs.some((i) => i.kind === 'library') || p.library) return p
      const id = previewLibrary(page, libraries)
      return id ? { ...p, library: String(id) } : p
    })
  }, [inputs, libraries, matchKey]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { onParams?.(params) }, [params]) // eslint-disable-line react-hooks/exhaustive-deps

  useLayoutEffect(() => {
    const el = pane.current
    if (!el) return
    const read = () => setPaneWidth(el.clientWidth)
    read()
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(read)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const device = useMemo<Device>(() => previewDevice(formFactor, navigator.language || 'en'), [formFactor])
  const missing = inputs.filter((i) => i.required && !params[i.name])
  const ready = missing.length === 0
  const paramKey = JSON.stringify(params)

  useEffect(() => {
    if (!ready) { setDoc(null); setState('idle'); return }
    const stop = new AbortController()
    let live = true
    setState('loading')
    const ask = async (retry: boolean): Promise<Document> => {
      const t = status.draft ? await token() : ''
      try {
        return await fetchPage(page.id, device, params, { preview: t, diff: false, signal: stop.signal })
      } catch (e) {
        // A token that has run out (two hours) is asked for again, once.
        if (retry && t && e instanceof ApiError && e.status === 403) return ask(false)
        throw e
      }
    }
    ask(true).then((d) => { if (live) { setDoc(d); setState('idle') } }).catch((e) => {
      if (!live || stop.signal.aborted) return
      setDoc(null)
      setState('error')
      setError(e instanceof ApiError && e.status === 404
        ? 'There is nothing to draw here yet: the page isn’t in the draft, or what it is about (the library, the title) can’t be found.'
        : e instanceof Error ? e.message : String(e))
    })
    return () => { live = false; stop.abort() }
  }, [page.id, device, paramKey, saved, again, ready, status.draft]) // eslint-disable-line react-hooks/exhaustive-deps

  // The drawing is as tall as the page turns out to be (at least one screen of that device):
  // scaling doesn't change the room it takes, so its height is measured and the frame set to it.
  useLayoutEffect(() => {
    const el = sheet.current
    if (!el) return
    const read = () => setSheetHeight(el.offsetHeight)
    read()
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(read)
    ro.observe(el)
    return () => ro.disconnect()
  }, [doc, formFactor])

  const frame = PREVIEW_FRAMES[formFactor]
  const scale = previewScale(paneWidth, formFactor)
  // A television's page is laid out inside its safe area; the others get the app's own gutter,
  // which the drawing is told too (--gutter), so what bleeds to the edge of the page stops at the frame's.
  const safe = doc?.layout?.safeArea
  const tv = formFactor === 'tv'
  const pad = tv ? { top: safe?.top || 60, right: safe?.right || 80, bottom: safe?.bottom || 60, left: safe?.left || 80 } : { top: 16, right: formFactor === 'desktop' ? 32 : 16, bottom: 16, left: formFactor === 'desktop' ? 32 : 16 }
  const kind = pageKindSpec(cat, page.kind)

  return (
    <div className="pe-preview">
      <div className="pe-preview-bar">
        <Segmented label="Preview as" value={formFactor} onChange={setFormFactor} options={FORM_FACTORS.map((f) => ({ value: f, label: PREVIEW_FRAMES[f].label }))} />
        <Button size="sm" variant="ghost" onClick={() => setAgain((n) => n + 1)}>Refresh</Button>
      </div>
      {inputs.length > 0 && (
        <div className="pe-preview-with">
          {inputs.map((i) => (i.kind === 'library'
            ? <Field key={i.name} label="Preview with the library">
                <Select inputSize="sm" value={params.library || ''} onChange={(e) => setParams((p) => ({ ...p, library: e.target.value }))}>
                  <option value="">Choose a library…</option>
                  {libraries.map((l) => <option key={l.id} value={String(l.id)}>{l.name}</option>)}
                </Select>
              </Field>
            : <Field key={i.name} label={`Preview with the ${words(i.name).toLowerCase()}`} hint={`Its number, from the end of its address in the app (${(kind?.route || '').replace(`{${i.name}}`, '123')}).`}>
                <Input inputSize="sm" value={params[i.name] || ''} inputMode="numeric" placeholder="123" onChange={(e) => setParams((p) => ({ ...p, [i.name]: e.target.value.trim() }))} />
              </Field>))}
        </div>
      )}
      <p className="pe-preview-note" role="status">
        {!ready ? `Choose ${missing.map((m) => 'the ' + words(m.name).toLowerCase()).join(' and ')} to draw this page with.`
          : dirty ? <>This is the draft as last saved. <button type="button" className="linkish" onClick={onSave}>Save to see your latest changes</button></>
          : status.draft ? 'This is the draft, as that device would get it.' : 'This is the page as it is published.'}
        {doc?.page.variant ? ` Showing the variant “${doc.page.variant}”: you match its condition.` : ''}
        {doc?.layout?.truncated ? ' This device leaves the last sections out: the page is longer than it shows.' : ''}
      </p>
      <div className="pe-preview-pane" ref={pane}>
        {state === 'error' && <p className="pe-preview-empty" role="alert">{error}</p>}
        {state === 'loading' && !doc && <Spinner />}
        {doc && (
          <div className={'pe-frame ' + formFactor + (state === 'loading' ? ' stale' : '')} style={{ width: frame.width * scale, height: Math.max(frame.height, sheetHeight) * scale }}
            role="img" aria-label={`${pageName(cat, page)} as a ${formFactor === 'tv' ? 'television' : frame.label.toLowerCase()} would show it`}>
            <div className="pe-frame-page" ref={sheet} inert style={{ '--gutter': `${pad.left}px`, width: frame.width, minHeight: frame.height, transform: `scale(${scale})`, paddingTop: pad.top, paddingRight: pad.right, paddingBottom: pad.bottom, paddingLeft: pad.left } as CSSProperties}>
              <PageView doc={doc} device={device} width={frame.width - pad.left - pad.right} />
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
