// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What `play` from another device needs of the app to open things here (device.ts): the
// router for the players that are pages, and the audio player, which has none.
import { useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAudio } from '../audio/player'
import { useApp } from '../state'
import { setRemoteOpener } from './device'

const PLAYER_PAGE = /^\/(play|read|game)\//

export function useRemoteOpener() {
  const nav = useNavigate()
  const audio = useAudio()
  const { libraries } = useApp()
  // The latest of each, so the opener never has to be handed over again.
  const now = useRef({ nav, audio, libraries })
  now.current = { nav, audio, libraries }
  useEffect(() => {
    setRemoteOpener({
      // One player taking over from another takes its place in the history too, so that Back
      // on this device leaves the player rather than stepping back through everything it was sent.
      go: (to, replace) => now.current.nav(to, { replace: replace || (PLAYER_PAGE.test(to) && PLAYER_PAGE.test(location.pathname)) }),
      playAudio: (items, index, mode, at) => now.current.audio.play(items, index, mode, at),
      libraryType: (id) => now.current.libraries.find((l) => l.id === id)?.type,
    })
    return () => setRemoteOpener(null)
  }, [])
}
