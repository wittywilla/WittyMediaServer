// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState } from 'react'
import { api, normalizeServer, serverBase, setServerBase } from '../api'
import { Brand, BrandMark } from './common'
import LegalNote from './LegalNote'
import SpecPage from '../spec/SpecPage'
import { handledIds } from '../spec/model'
import { connectSpec } from '../spec/specs/connect'
import { isSubmitKey } from '../spec/specs/submit'

// The apps' choice of server: the web pages come with the app, so it has to
// be told where the server is, at home or outside. In a browser this is never
// shown: the page came from the server.

// Plain HTTP is fine at home, or inside a VPN, and nowhere else.
const privateHost = (u: URL) => /^(localhost|127\.|10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.|100\.(6[4-9]|[7-9]\d|1[01]\d|12[0-7])\.|\[?f[cd]|\[?fe80|\[::1)/i.test(u.hostname) || !u.hostname.includes('.')

export default function ConnectServer({ onCancel }: { onCancel?: () => void }) {
  const [addr, setAddr] = useState(serverBase() || '')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const url = normalizeServer(addr)
  let insecure = false
  try { const u = new URL(url); insecure = u.protocol === 'http:' && !privateHost(u) } catch { /* not an address yet */ }
  const connect = async () => {
    setErr('')
    setBusy(true)
    const before = serverBase()
    setServerBase(url)
    try {
      const st = await api.get<{ name?: string }>('/status')
      if (st.name !== 'WittyMediaServer') throw new Error('Something answers at that address, but it isn’t a WittyMediaServer.')
      window.location.reload()
    } catch (e: any) {
      setServerBase(before)
      setErr(e.message)
      setBusy(false)
    }
  }
  const page = connectSpec({ address: addr, busy, error: err, insecure, hasCancel: !!onCancel })
  const act = (id: string) => { if (id === 'cancel') onCancel?.(); else if (!busy && addr.trim()) void connect() }
  const field = (_id: string, v: string | number | boolean | File) => setAddr(String(v))
  return (
    <div className="center">
      <div className="auth-spec" onKeyDown={(e) => { if (isSubmitKey(e)) { e.preventDefault(); act('connect') } }}>
        <div className="brand"><BrandMark /><Brand name="" /></div>
        <SpecPage page={page} handled={handledIds(page)} onAction={act} onField={field} />
        <LegalNote kind="connect" />
      </div>
    </div>
  )
}
