import { useEffect, useState } from 'react'
import type { SessionInfo } from './Player'
import type { SettingsHost } from './settings'
import type { Stats, Transport } from './transport'

// The numbers on screen. Off by default. They ask the server about once a second, and
// only while they're showing.

type ServerStats = { gameFps: number; sentFps: number; skipped: number; encodedFps: number; encodeMs: number; kbps: number; droppedFrames: number; width: number; height: number; core: string; encoder?: string; speed: number; rewindBytes?: number; rewindFrames?: number }

export function Osd({ host, session, transport, values }: { host: SettingsHost; session: SessionInfo | null; transport: Transport | null; values: Record<string, unknown> }) {
  const [client, setClient] = useState<Stats>({})
  const [server, setServer] = useState<ServerStats | null>(null)
  useEffect(() => {
    if (!session) return
    let stop = false
    const tick = () => {
      transport?.stats().then((s) => { if (!stop) setClient(s) }).catch(() => {})
      host.request<ServerStats>('GET', `/play/${session.id}/stats`).then((s) => { if (!stop) setServer(s) }).catch(() => {})
    }
    tick()
    const iv = setInterval(tick, 1000)
    return () => { stop = true; clearInterval(iv) }
  }, [host, session?.id, transport]) // eslint-disable-line react-hooks/exhaustive-deps
  const on = (k: string) => values['osd.' + k] === true
  const round = (n: number) => Math.round(n * 10) / 10
  const lines: string[] = []
  if (on('fps') && client.fps != null) lines.push(`${client.fps} fps shown`)
  if (on('gameFps') && server) lines.push(`${Math.round(server.gameFps)} fps game` + (server.skipped ? `, ${server.skipped} skipped` : ''))
  if (on('latency') && client.rttMs != null) lines.push(`${client.rttMs} ms network` + (client.jitterMs != null ? `, ${client.jitterMs} ms jitter` : ''))
  if (on('encode') && server) lines.push(`${round(server.encodeMs)} ms encode` + (client.decodeMs != null ? `, ${client.decodeMs} ms decode` : ''))
  if (on('bitrate')) { const k = client.kbps ?? server?.kbps; if (k != null) lines.push(`${Math.round(k)} kbps`) }
  if (on('dropped')) {
    const parts = []
    if (server) parts.push(`${server.droppedFrames} at the server`)
    if (client.dropped != null) parts.push(`${client.dropped} here`)
    if (client.lost) parts.push(`${client.lost} packets lost`)
    if (parts.length) lines.push(`Dropped: ${parts.join(', ')}`)
  }
  if (on('resolution') && server && server.width) lines.push(`${server.width}×${server.height}`)
  if (on('core')) {
    const name = server?.core || session?.core
    if (name) lines.push(name)
    const via = [transport?.kind, server?.encoder, client.codec].filter(Boolean).join(', ')
    if (via) lines.push(via)
  }
  if (server?.rewindFrames) lines.push(`Rewind: ${server.rewindFrames} steps, ${Math.ceil((server.rewindBytes || 0) / 1048576)} MB`)
  const corner = String(values['osd.corner'] || 'tl')
  const size = String(values['osd.size'] || 'normal')
  return (
    <div className={`wp-osd wp-osd-${corner} wp-osd-${size}`} aria-hidden="true">
      {lines.map((l, i) => <span key={i}>{l}</span>)}
      {lines.length === 0 && <span>Measuring…</span>}
    </div>
  )
}
