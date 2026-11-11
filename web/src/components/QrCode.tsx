// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useMemo } from 'react'
import { qrPath } from '../ui/qr'

// A QR code drawn in the page itself: what it says is never sent anywhere to be
// made into a picture. Always dark on light, whatever the theme, because
// cameras read it that way.
export function QrCode({ text, label }: { text: string; label: string }) {
  const code = useMemo(() => { try { return qrPath(text) } catch { return null } }, [text])
  if (!code) return null
  return (
    <svg className="qr" role="img" aria-label={label} viewBox={`0 0 ${code.size} ${code.size}`} shapeRendering="crispEdges">
      <rect className="qr-paper" width={code.size} height={code.size} />
      <path className="qr-ink" d={code.d} />
    </svg>
  )
}
