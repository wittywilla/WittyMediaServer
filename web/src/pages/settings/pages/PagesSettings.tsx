// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Settings, Pages & menu: where an administrator decides what Home, the libraries, a
// title's page and the main menu hold, for everyone and every app. The server describes
// its pages as data (docs/SDUI.md) and this is the editor of that data: a list of the
// pages, an editor for one of them, the menu, a gallery of ready-made pages and sections,
// and the history. Loaded only when Settings opens it.

import { useEffect, useState } from 'react'
import { Route, Routes, useLocation } from 'react-router-dom'
import { Tabs } from '../../../ui'
import { PageHead } from '../ui'
import { getPresets } from './api'
import { DraftBar, EditorProvider } from './context'
import { editPath, type Preset } from './editorModel.ts'
import { HistoryAndFiles, PagesList, PresetsGallery } from './Lists'
import LayoutsEditor from './LayoutsEditor'
import NavigationEditor from './NavigationEditor'
import PageEditor from './PageEditor'
import './pages-editor.css'

type View = 'pages' | 'navigation' | 'layouts' | 'presets' | 'history'
const BASE = editPath('')
const TABS: { value: View; label: string; to: string }[] = [
  { value: 'pages', label: 'Pages', to: BASE },
  { value: 'navigation', label: 'The menu', to: `${BASE}/navigation` },
  { value: 'layouts', label: 'Looks', to: `${BASE}/layouts` },
  { value: 'presets', label: 'Ready-made', to: `${BASE}/presets` },
  { value: 'history', label: 'History and files', to: `${BASE}/history` },
]

export default function PagesSettings() {
  const { pathname } = useLocation()
  const [first, second] = pathname.slice(BASE.length).split('/').filter(Boolean)
  const view = !first ? 'pages' : !second && TABS.some((t) => t.value === first) ? first as View : null
  // The gallery is asked for when it is first wanted, and kept while Settings stays on these pages.
  const [presets, setPresets] = useState<Preset[] | null>(null)
  useEffect(() => {
    if (view !== 'presets' || presets) return
    getPresets().then(setPresets).catch(() => setPresets([]))
  }, [view, presets])
  return (
    <EditorProvider>
      {view && <Tabs label="Pages and menu" value={view} items={TABS} />}
      <Routes>
        <Route index element={<>
          <PageHead title="Pages & menu" desc="What Home, each library, a title’s page and the main menu hold, for everyone and in every app. Change a page, try it as a phone or a television would show it, and publish when it is right." />
          <DraftBar />
          <PagesList />
        </>} />
        <Route path="navigation" element={<NavigationEditor />} />
        <Route path="layouts" element={<LayoutsEditor />} />
        <Route path="presets" element={<>
          <PageHead title="Ready-made" desc="Pages and sections made for you to start from." />
          <DraftBar />
          <PresetsGallery presets={presets} />
        </>} />
        <Route path="history" element={<>
          <PageHead title="History and files" desc="What was published and when, going back to an earlier revision, and pages as files in and out." />
          <DraftBar />
          <HistoryAndFiles />
        </>} />
        {/* A page whose ID is one of the words above is reached by the longer address. */}
        <Route path="page/:pageId" element={<PageEditor />} />
        <Route path=":pageId" element={<PageEditor />} />
      </Routes>
    </EditorProvider>
  )
}
