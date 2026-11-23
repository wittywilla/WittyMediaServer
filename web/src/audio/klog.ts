// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Karaoke's sound, written down as it happens, for finding a dropout: which set of parts started and stopped, when the
// parts were started again and why, the audio engine's state. Off unless asked for on this device:
//   localStorage.setItem('wms.debug.karaoke', '1')
// then read window.__wmsKaraoke (the last 2000 lines) or the console's debug messages.
let on: boolean | null = null
const enabled = () => {
  if (on === null) { try { on = localStorage.getItem('wms.debug.karaoke') === '1' } catch { on = false } }
  return on
}
export function klog(...parts: unknown[]) {
  if (!enabled()) return
  const w = window as unknown as { __wmsKaraoke?: string[] }
  const line = `${(performance.now() / 1000).toFixed(2)} ${parts.map(String).join(' ')}`
  const list = (w.__wmsKaraoke ??= [])
  list.push(line)
  if (list.length > 2000) list.shift()
  console.debug('[karaoke]', line)
}
