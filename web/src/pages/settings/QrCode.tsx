import { useMemo } from 'react'
import { qrMatrix } from './qr'

// A QR code, always dark on a white card whatever the theme (phones read those
// best). The text it holds must also be on the page for whoever can't scan it.
export default function QrCode({ text, label, size = 200 }: { text: string; label: string; size?: number }) {
  const m = useMemo(() => qrMatrix(text), [text])
  if (!m) return null
  const n = m.length, quiet = 4
  let d = ''
  m.forEach((row, y) => row.forEach((on, x) => { if (on) d += `M${x + quiet} ${y + quiet}h1v1h-1z` }))
  return (
    <svg className="rem-qr" role="img" aria-label={label} width={size} height={size} viewBox={`0 0 ${n + quiet * 2} ${n + quiet * 2}`} shapeRendering="crispEdges">
      <rect width="100%" height="100%" fill="#fff" />
      <path d={d} fill="#000" />
    </svg>
  )
}
