// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Where a page asks for one of the app's own rich views (docs/SDUI.md, "NativeView"):
// the TV guide's grid, the photo timeline and map, cover flow. Each is the view the web
// app has always had, unchanged. Where the view can't be shown here (the library isn't
// one this profile has), what the page put inside the NativeView is drawn instead.

import { Component, lazy, useEffect, useMemo, useState, type ComponentType, type ReactNode } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { api, type Album } from '../../api'
import { makePageHost } from '../../pages/ModulePage'
import { moduleView, type ModuleViewProps } from '../../moduleBundles'
import { Placeholder } from '../../components/common'
import { albumLink, coverUrl } from '../../pages/music/parts'
import CoverFlow, { type Cover } from '../../pages/music/CoverFlow'
import { useApp } from '../../state'

const GuidePage = lazy(() => import('../../pages/tv/GuidePage'))
const PhotosLibrary = lazy(() => import('../../pages/photos/PhotosLibrary'))

export default function NativeViewHost({ view, library, inert, children, props }: { view: string; library: string; inert: boolean; children: ReactNode; props?: Record<string, unknown> }) {
  const { libraries } = useApp()
  // A module's own view ("<module>.<name>"), drawn by its web bundle.
  if (view.includes('.') && !inert) return <ModuleViewHost view={view} props={props || {}}>{children}</ModuleViewHost>
  const lib = libraries.find((l) => String(l.id) === library)
  // A drawing of the page, in the editor: the view's name stands for it.
  if (inert) return <div className="sdui-native-stand" role="img" aria-label={`The app’s own view: ${view}`}><b>{NAMES[view] || view}</b><span className="muted">The app’s own view is shown here.</span></div>
  if (view === 'guide') return <div className="sdui-native" data-sdui-native="guide"><GuidePage /></div>
  if ((view === 'photoTimeline' || view === 'photoMap') && lib) return <div className="sdui-native" data-sdui-native={view}><Photos lib={lib} map={view === 'photoMap'} /></div>
  if (view === 'coverFlow' && lib) return <div className="sdui-native" data-sdui-native="coverFlow"><Covers libId={lib.id} tall={lib.type === 'audiobooks'} /></div>
  return <div className="sdui-stack">{children}</div>
}

const NAMES: Record<string, string> = { guide: 'TV guide', photoTimeline: 'Photo timeline', photoMap: 'Photo map', coverFlow: 'Cover flow' }

function Photos({ lib, map }: { lib: NonNullable<ReturnType<typeof useApp>['libraries'][number]>; map: boolean }) {
  const [params, setParams] = useSearchParams()
  // The map is one of the photo library's own tabs: open on it, once.
  useEffect(() => {
    if (map && !params.get('tab')) { const next = new URLSearchParams(params); next.set('tab', 'map'); setParams(next, { replace: true }) }
  }, [map]) // eslint-disable-line react-hooks/exhaustive-deps
  return <PhotosLibrary key={lib.id} lib={lib} />
}

function Covers({ libId, tall }: { libId: number; tall: boolean }) {
  const nav = useNavigate()
  const [albums, setAlbums] = useState<Album[] | null>(null)
  const [failed, setFailed] = useState(false)
  useEffect(() => {
    let live = true
    setAlbums(null); setFailed(false)
    api.get<Album[]>(`/music/albums?library=${libId}`).then((a) => { if (live) setAlbums(a || []) }).catch(() => { if (live) setFailed(true) })
    return () => { live = false }
  }, [libId])
  const covers = useMemo<Cover<Album>[]>(() => (albums || []).map((a) => ({ key: a.artist + '\u0000' + a.title, url: coverUrl(a.coverItem), label: [a.title, a.artist].filter(Boolean).join(', '), item: a })), [albums])
  if (failed) return <p className="sdui-error" role="alert">Couldn’t load this</p>
  if (!albums) return <div className="loading-box" role="status" aria-label="Loading"><div className="spinner" /></div>
  if (!covers.length) return <p className="muted sdui-empty">Nothing here yet.</p>
  return <CoverFlow covers={covers} tall={tall} onOpen={(c) => nav(albumLink(libId, c.item))} fallback={(c) => <Placeholder title={c.item.title || 'Unknown'} />} />
}

// A module's view: its bundle's component, given what it may use of the app and the
// NativeView's props, in the module's colours. Until it has loaded, and if it won't (or it
// fails while drawing), what the page put inside the NativeView is drawn instead.
function ModuleViewHost({ view, props, children }: { view: string; props: Record<string, unknown>; children: ReactNode }) {
  const { user } = useApp()
  const [C, setC] = useState<ComponentType<ModuleViewProps> | null | undefined>(undefined)
  useEffect(() => {
    let live = true
    setC(undefined)
    moduleView(view).then((c) => { if (live) setC(() => c) })
    return () => { live = false }
  }, [view])
  const module = view.slice(0, view.indexOf('.'))
  const host = useMemo(() => makePageHost(module, !!user?.isAdmin), [module, user?.isAdmin])
  if (C === undefined) return <div className="loading-box" role="status" aria-label="Loading"><div className="spinner" /></div>
  if (!C) return <div className="sdui-stack">{children}</div>
  return <div className="sdui-native" data-sdui-native={view}><ViewGuard fallback={children}><C {...props} host={host} view={view} /></ViewGuard></div>
}

class ViewGuard extends Component<{ fallback: ReactNode; children: ReactNode }, { broken: boolean }> {
  state = { broken: false }
  static getDerivedStateFromError() { return { broken: true } }
  componentDidCatch(e: unknown) { console.warn('A module view stopped:', e) }
  render() { return this.state.broken ? <div className="sdui-stack">{this.props.fallback}</div> : this.props.children }
}
