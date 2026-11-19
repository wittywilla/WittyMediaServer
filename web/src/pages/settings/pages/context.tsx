// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What every page of the editor shares: the catalogue the forms are built from, the
// libraries, the pages in force, and the draft (whether there is one, what it changes,
// publishing it, throwing it away, and looking at it in the app).

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, useSyncExternalStore, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { announce } from '../../../a11y'
import { t, tn } from '../../../i18n'
import { forgetPages, bump, getPreview, onPreview, setPreview } from '../../../sdui/client'
import { useApp } from '../../../state'
import { ask, Button, LoadError, Notice, Spinner, toast } from '../../../ui'
import { discardDraft, getCatalogue, getDiff, isConflict, listPages, mergeConflicts, message, previewToken, publishDraft } from './api'
import { describeChanges, pageName, setModelWords, type Catalogue, type Change, type LibraryRef, type PageEntry } from './editorModel.ts'

// The editor's model writes its sentences in the app's language from here on.
setModelWords({ t, tn })

type Status = { draft: boolean; pages: PageEntry[]; changes: Change[] }

type Editor = {
  cat: Catalogue
  libraries: LibraryRef[]
  status: Status
  // Reads the pages and the draft again, after anything that changed them.
  refresh: () => Promise<void>
  // "Someone else saved in between": true to make the change again on top of theirs.
  onConflict: () => Promise<boolean>
  // A token that shows the draft to this administrator, '' when there is no draft.
  token: () => Promise<string>
  // The page being edited has changes that aren't in the draft yet (what it is called), or ''.
  unsaved: React.RefObject<string>
  showInApp: (route: string) => Promise<void>
}

const Ctx = createContext<Editor | null>(null)

export function useEditor(): Editor {
  const e = useContext(Ctx)
  if (!e) throw new Error('The editor’s pages are drawn inside EditorProvider')
  return e
}

export const usePreviewing = () => useSyncExternalStore(onPreview, getPreview)

export function EditorProvider({ children }: { children: ReactNode }) {
  const { libraries } = useApp()
  const navigate = useNavigate()
  const [cat, setCat] = useState<Catalogue | null>(null)
  const [status, setStatus] = useState<Status | null>(null)
  const [failed, setFailed] = useState(false)
  const [attempt, setAttempt] = useState(0)
  const unsaved = useRef('')
  const kept = useRef('')

  const refresh = useCallback(async () => {
    const list = await listPages(true)
    const changes = list.draft ? await getDiff('current', 'draft') : []
    if (!list.draft) kept.current = ''
    setStatus({ draft: list.draft, pages: list.pages, changes: changes || [] })
  }, [])

  useEffect(() => {
    let live = true
    setFailed(false)
    Promise.all([getCatalogue(), refresh()]).then(([c]) => { if (live) setCat(c) }).catch(() => { if (live) setFailed(true) })
    return () => { live = false }
  }, [attempt, refresh])

  const onConflict = useCallback(() => ask({
    title: t('Someone else saved in between'),
    body: t('The draft changed after you opened it, here in another window or by another administrator. Your change can be made again on top of theirs.'),
    confirm: t('Make my change again'), cancel: t('Not now'),
  }), [])

  const token = useCallback(async () => {
    if (kept.current) return kept.current
    try { kept.current = await previewToken() } catch { kept.current = '' }
    return kept.current
  }, [])

  const showInApp = useCallback(async (route: string) => {
    const tok = await token()
    if (!tok) { toast(t('There is no draft to look at yet. Save a change first.')); return }
    setPreview(tok)
    navigate(route || '/')
  }, [token, navigate])

  const libs = useMemo<LibraryRef[]>(() => libraries.map((l) => ({ id: l.id, name: l.name, type: l.type })), [libraries])
  const value = useMemo<Editor | null>(() => (cat && status ? { cat, libraries: libs, status, refresh, onConflict, token, unsaved, showInApp } : null), [cat, libs, status, refresh, onConflict, token, showInApp])

  if (failed) return <LoadError what={t('The page editor')} onRetry={() => setAttempt((n) => n + 1)} />
  if (!value) return <Spinner />
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

// The line that says where things stand: nothing waiting, or a draft with what it
// changes, and the three things to do with it.
export function DraftBar({ route }: { route?: string }) {
  const { cat, status, refresh, unsaved, showInApp } = useEditor()
  const previewing = usePreviewing()
  const [busy, setBusy] = useState('')
  const titles = useMemo(() => new Map(status.pages.map((p) => [p.id, p])), [status.pages])
  const lines = useMemo(() => describeChanges(status.changes, { titleOf: (id) => (titles.get(id) ? pageName(cat, titles.get(id)!) : id), isBuiltin: (id) => !!titles.get(id) && titles.get(id)!.source !== 'custom' }), [status.changes, titles, cat])

  const publish = async () => {
    const waiting = unsaved.current
    if (!await ask({
      title: t('Publish the draft?'),
      body: waiting
        ? t('Everyone gets these pages and this menu from now on. You can go back to how it was from History. Your latest changes to {what} aren’t saved to the draft yet, so they won’t be published.', { what: waiting })
        : t('Everyone gets these pages and this menu from now on. You can go back to how it was from History.'),
      confirm: t('Publish'),
    })) return
    setBusy('publish')
    try {
      try {
        await publishDraft()
      } catch (e) {
        // Someone published since this draft was begun, and changed some of the same things.
        const clash = mergeConflicts(e)
        if (!clash) throw e
        const whole = clash.includes('/')
        const named = clash.map((c) => c.replace(/^\/pages\//, '').replace(/^\/navigation$/, t('the menu')).replace(/^\/theme$/, t('the theme')).replace(/^\/personal$/, t('rearranging'))).join(', ')
        const keepDraft = await ask({
          title: t('Changed in both places'),
          body: whole
            ? t('What was published when this draft was begun is no longer kept, so the two can’t be merged. Publish the draft as it is, replacing what is published now?')
            : t('Since this draft was begun, someone published changes to {what}, and the draft changes them too. Use the draft’s version of those? Everything else from both is kept either way.', { what: named }),
          confirm: whole ? t('Publish the draft') : t('Use the draft’s'),
          cancel: whole ? t('Cancel') : t('Keep what’s published'),
        })
        if (whole && !keepDraft) return
        // Closing the question is not an answer: say what publishing now would do, and ask once more.
        if (!keepDraft && !await ask({ title: t('Publish, keeping what’s published for those?'), body: t('The draft’s other changes are published; {what} stay as they are now.', { what: named }), confirm: t('Publish') })) return
        await publishDraft(keepDraft ? 'draft' : 'published')
      }
      setPreview('')
      forgetPages(); bump()
      await refresh()
      toast(t('Published. Everyone has it now.'), { tone: 'ok' })
      announce(t('The draft was published'))
    } catch (e) {
      if (isConflict(e)) {
        await refresh()
        toast(t('The draft changed since you last looked. Check what it holds now, then publish.'), { tone: 'warn' })
      } else toast(message(e), { tone: 'danger' })
    } finally { setBusy('') }
  }

  const discard = async () => {
    if (!await ask({ title: t('Discard the draft?'), body: t('Everything in the draft is thrown away. What everyone sees now stays as it is.'), confirm: t('Discard the draft'), danger: true })) return
    setBusy('discard')
    try {
      await discardDraft()
      setPreview('')
      await refresh()
      toast(t('The draft was discarded'))
    } catch (e) { toast(message(e), { tone: 'danger' }) } finally { setBusy('') }
  }

  if (!status.draft) {
    return (
      <div className="pe-draft none" role="status">
        <span className="pe-draft-dot" aria-hidden="true" />
        <span>{t('Nothing is waiting to be published. What you save here goes to a draft first, which only you see until you publish it.')}</span>
      </div>
    )
  }
  return (
    <div className="pe-draftbar">
    <Notice tone="warn" title={lines.length ? tn('A draft is waiting with {count} change', 'A draft is waiting with {count} changes', lines.length) : t('A draft is waiting')}
      action={<span className="pe-draft-actions">
        {previewing
          ? <Button size="sm" variant="ghost" onClick={() => { setPreview(''); toast(t('The app shows what is published again')) }}>{t('Stop showing the draft')}</Button>
          : <Button size="sm" variant="ghost" onClick={() => void showInApp(route || '/')}>{t('Open the draft in the app')}</Button>}
        <Button size="sm" variant="ghost" busy={busy === 'discard'} disabled={!!busy} onClick={discard}>{t('Discard draft')}</Button>
        <Button size="sm" variant="primary" busy={busy === 'publish'} disabled={!!busy} onClick={publish}>{t('Publish')}</Button>
      </span>}>
      <span>{previewing ? t('Nobody else sees it until you publish. The app is showing you the draft.') : t('Nobody else sees it until you publish.')}</span>
      {lines.length > 0 && (
        <details className="pe-draft-what">
          <summary>{t('What the draft changes')}</summary>
          <ul>
            {lines.map((l) => <li key={l.title}>{l.title}{l.details.length > 0 && <ul>{l.details.map((d) => <li key={d}>{d}</li>)}</ul>}</li>)}
          </ul>
        </details>
      )}
    </Notice>
    </div>
  )
}
