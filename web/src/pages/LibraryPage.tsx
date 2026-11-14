// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense } from 'react'
import { Link, useParams } from 'react-router-dom'
import { EmptyState, Spinner } from '../components/common'
import { ModuleMissing } from '../components/ModuleMissing'
import { useApp } from '../state'
import AudioLibrary from './AudioLibrary'
import BooksLibrary from './Books'
import VideoLibrary from './library/VideoLibrary'

// Loaded when opened: most servers have only some of these libraries.
const PhotosLibrary = lazy(() => import('./photos/PhotosLibrary'))
const ShortsLibrary = lazy(() => import('./creators/ShortsLibrary'))
const ChannelsLibrary = lazy(() => import('./creators/ChannelsLibrary'))
const RadioLibrary = lazy(() => import('./RadioLibrary'))
const GamesLibrary = lazy(() => import('./games/Library'))

const AUDIO = ['music', 'audiobooks', 'podcasts']

export default function LibraryPage() {
  const { id } = useParams()
  const { libraries } = useApp()
  const lib = libraries.find((l) => String(l.id) === id)
  if (lib && AUDIO.includes(lib.type)) return <AudioLibrary key={lib.id} lib={lib} />
  if (lib?.type === 'books') return <BooksLibrary key={lib.id} lib={lib} />
  if (lib?.type === 'photos') return <Suspense fallback={<Spinner />}><PhotosLibrary key={lib.id} lib={lib} /></Suspense>
  if (lib?.type === 'shorts') return <Suspense fallback={<Spinner />}><ShortsLibrary key={lib.id} lib={lib} /></Suspense>
  if (lib?.type === 'channels' || lib?.type === 'musicvideos') return <Suspense fallback={<Spinner />}><ChannelsLibrary key={lib.id} lib={lib} /></Suspense>
  if (lib?.type === 'radio') return <Suspense fallback={<Spinner />}><RadioLibrary key={lib.id} lib={lib} /></Suspense>
  // A gaming PC's games: WittyRigs draws them (its takeover of the kind "rig"); without it, it's missing.
  if (lib?.type === 'games' && lib.console === 'rig') return <ModuleMissing ids={['wittyrigs']} />
  if (lib?.type === 'games') return <Suspense fallback={<Spinner />}><GamesLibrary key={lib.id} lib={lib} /></Suspense>
  if (!lib) return <EmptyState title="Library not found" action={<Link to="/settings/libraries" className="btn">Manage libraries</Link>} />
  return <VideoLibrary key={lib.id} lib={lib} />
}
