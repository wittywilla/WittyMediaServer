// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Brand, BrandMark } from './common'
import SpecPage from '../spec/SpecPage'
import { handledIds } from '../spec/model'
import { unreachableSpec } from '../spec/specs/unreachable'
import { useApp } from '../state'

const EVERY = 15

// What a browser shows when the server it came from can't be reached on its first look: what is being tried, and
// when it tries again by itself. Each try is a real request for the server's status.
export default function Unreachable() {
  const { retryStatus, loading } = useApp()
  const [left, setLeft] = useState(EVERY)
  const [tries, setTries] = useState(1)
  useEffect(() => {
    if (loading) return
    setLeft(EVERY)
    const t = setInterval(() => setLeft((n) => n - 1), 1000)
    return () => clearInterval(t)
  }, [loading, tries])
  useEffect(() => { if (left <= 0 && !loading) { setTries((n) => n + 1); retryStatus() } }, [left, loading, retryStatus])
  const page = unreachableSpec({ address: window.location.origin, trying: loading, seconds: Math.max(left, 0), tries })
  return (
    <div className="center">
      <div className="auth-spec">
        <div className="brand"><BrandMark /><Brand name="" /></div>
        <SpecPage page={page} handled={handledIds(page)} onAction={() => { setTries((n) => n + 1); retryStatus() }} />
      </div>
    </div>
  )
}
