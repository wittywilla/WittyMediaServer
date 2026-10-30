import { useState } from 'react'
import { api, normalizeServer, serverBase, setServerBase } from '../api'
import { Brand, BrandMark } from './common'

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
  const connect = async (e: React.FormEvent) => {
    e.preventDefault()
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
  return (
    <div className="center">
      <form className="auth-card" onSubmit={connect}>
        <div className="brand"><BrandMark /><Brand name="" /></div>
        <p>Enter your server's address.</p>
        <div className="form">
          <label>Server address
            <input className="input" autoFocus inputMode="url" autoCapitalize="none" spellCheck={false} placeholder="https://media.example.com" value={addr} onChange={(e) => setAddr(e.target.value)} />
          </label>
          <small className="muted">At home, that's the address shown in the server's settings, such as http://192.168.1.10:9797. Away from home, it's the address from Settings → Remote Access.</small>
          {insecure && <div className="error" role="alert">This address isn't encrypted. Don't use it over the internet: your password could be read on the way. Use an https:// address or a VPN.</div>}
          {err && <div className="error" role="alert">{err}</div>}
          <button className="btn primary" disabled={busy || !addr.trim()} style={{ justifyContent: 'center' }}>{busy ? 'Connecting…' : 'Connect'}</button>
          {onCancel && <button type="button" className="btn ghost" onClick={onCancel} style={{ justifyContent: 'center' }}>Cancel</button>}
        </div>
      </form>
    </div>
  )
}
