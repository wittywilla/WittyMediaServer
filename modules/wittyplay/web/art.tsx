import type { Target } from './consoles'

// Pictures of controllers, drawn for this project: generic shapes that suit each kind
// of console, with no maker's logo and nothing traced from a real product. Colours come
// from the theme. The buttons themselves are real <button>s laid over the drawing, so
// that remotes, keyboards and screen readers reach them like any other.

export type Slot = { t: number; x: number; y: number; w: number; h: number; shape: 'round' | 'pill' | 'pad' | 'stick'; arrow?: 'up' | 'down' | 'left' | 'right' }
export type Art = { body: string; wells?: { x: number; y: number; r: number }[]; cross?: { x: number; y: number; s: number }[]; marks?: { x: number; y: number; text: string }[]; slots: Slot[] }

// The picture's coordinates.
export const ART_W = 640, ART_H = 400

// RetroPad numbers.
const B = 0, Y = 1, SELECT = 2, START = 3, UP = 4, DOWN = 5, LEFT = 6, RIGHT = 7, A = 8, X = 9, L = 10, R = 11, L2 = 12, R2 = 13, L3 = 14, R3 = 15
const LS_L = 16, LS_R = 17, LS_U = 18, LS_D = 19, RS_L = 20, RS_R = 21, RS_U = 22, RS_D = 23

const round = (t: number, x: number, y: number, s = 56): Slot => ({ t, x, y, w: s, h: s, shape: 'round' })
const pill = (t: number, x: number, y: number, w = 74, h = 30): Slot => ({ t, x, y, w, h, shape: 'pill' })
const dpad = (x: number, y: number, s: number): Slot[] => [
  { t: UP, x, y: y - s, w: s, h: s, shape: 'pad', arrow: 'up' }, { t: DOWN, x, y: y + s, w: s, h: s, shape: 'pad', arrow: 'down' },
  { t: LEFT, x: x - s, y, w: s, h: s, shape: 'pad', arrow: 'left' }, { t: RIGHT, x: x + s, y, w: s, h: s, shape: 'pad', arrow: 'right' },
]
const stick = (ids: number[], x: number, y: number, off: number, s: number): Slot[] => [
  { t: ids[0], x: x - off, y, w: s, h: s, shape: 'stick', arrow: 'left' }, { t: ids[1], x: x + off, y, w: s, h: s, shape: 'stick', arrow: 'right' },
  { t: ids[2], x, y: y - off, w: s, h: s, shape: 'stick', arrow: 'up' }, { t: ids[3], x, y: y + off, w: s, h: s, shape: 'stick', arrow: 'down' },
]
const diamond = (x: number, y: number, d: number, s = 52): Slot[] => [round(Y, x - d, y, s), round(X, x, y - d, s), round(B, x, y + d, s), round(A, x + d, y, s)]

const ARTS: Record<string, Art> = {
  // A flat pad with two buttons.
  two: {
    body: 'M72 118 H568 Q604 118 604 154 V270 Q604 306 568 306 H72 Q36 306 36 270 V154 Q36 118 72 118 Z',
    cross: [{ x: 160, y: 212, s: 42 }],
    slots: [...dpad(160, 212, 42), pill(SELECT, 280, 256), pill(START, 368, 256), round(B, 454, 232, 66), round(A, 540, 192, 66), pill(L, 120, 90, 96, 34), pill(R, 520, 90, 96, 34)],
  },
  // A stick on a base, with one button.
  one: {
    body: 'M86 164 H554 Q590 164 590 200 V332 Q590 368 554 368 H86 Q50 368 50 332 V200 Q50 164 86 164 Z',
    wells: [{ x: 176, y: 266, r: 34 }],
    slots: [...dpad(176, 266, 58), pill(SELECT, 330, 226, 84), pill(START, 330, 306, 84), round(B, 470, 266, 84)],
  },
  // A rounded pad with four buttons in a diamond, and two shoulders.
  four: {
    body: 'M170 104 A108 108 0 0 0 170 320 C246 320 268 288 320 288 C372 288 394 320 470 320 A108 108 0 0 0 470 104 C394 104 372 136 320 136 C268 136 246 104 170 104 Z',
    cross: [{ x: 170, y: 212, s: 40 }],
    slots: [...dpad(170, 212, 40), pill(SELECT, 284, 212, 66), pill(START, 356, 212, 66), ...diamond(470, 212, 50), pill(L, 150, 76, 100, 34), pill(R, 490, 76, 100, 34)],
  },
  // A wide pad with six buttons in two rows.
  six: {
    body: 'M124 112 Q320 84 516 112 Q612 128 612 226 Q612 330 538 330 Q486 330 468 276 Q320 248 172 276 Q154 330 102 330 Q28 330 28 226 Q28 128 124 112 Z',
    cross: [{ x: 142, y: 208, s: 40 }],
    slots: [...dpad(142, 208, 40), pill(SELECT, 276, 160, 66), pill(START, 356, 160, 66), round(L, 374, 218, 52), round(X, 448, 196, 52), round(R, 522, 174, 52), round(Y, 396, 280, 52), round(B, 470, 258, 52), round(A, 544, 236, 52)],
  },
  // Three handles: a D-pad on the left, a stick in the middle, buttons on the right.
  handle: {
    body: 'M92 92 Q320 62 548 92 Q622 102 612 172 L588 302 Q580 344 546 338 Q516 332 506 292 L482 216 Q424 200 386 206 L362 342 Q352 374 320 374 Q288 374 278 342 L254 206 Q216 200 158 216 L134 292 Q124 332 94 338 Q60 344 52 302 L28 172 Q18 102 92 92 Z',
    cross: [{ x: 118, y: 166, s: 34 }],
    wells: [{ x: 320, y: 250, r: 30 }],
    marks: [{ x: 530, y: 134, text: 'C' }],
    slots: [...dpad(118, 166, 34), ...stick([LS_L, LS_R, LS_U, LS_D], 320, 250, 38, 32), pill(START, 320, 142, 66), round(Y, 412, 176, 46), round(B, 456, 216, 46),
      ...stick([RS_L, RS_R, RS_U, RS_D], 530, 134, 34, 32), pill(L, 96, 64, 96, 34), pill(R, 544, 64, 96, 34), pill(L2, 320, 342, 54, 30)],
  },
  // Two grips and two sticks.
  dual: {
    body: 'M150 92 H490 Q562 92 592 172 L626 306 Q640 366 592 370 Q552 374 522 340 L482 312 H158 L118 340 Q88 374 48 370 Q0 366 14 306 L48 172 Q78 92 150 92 Z',
    cross: [{ x: 150, y: 164, s: 34 }],
    wells: [{ x: 236, y: 262, r: 26 }, { x: 404, y: 262, r: 26 }],
    slots: [...dpad(150, 164, 34), ...diamond(490, 164, 40, 44), pill(SELECT, 280, 126, 58, 26), pill(START, 360, 126, 58, 26), ...stick([LS_L, LS_R, LS_U, LS_D], 236, 262, 32, 28), ...stick([RS_L, RS_R, RS_U, RS_D], 404, 262, 32, 28),
      round(L3, 236, 262, 28), round(R3, 404, 262, 28), pill(L, 118, 66, 86, 30), pill(R, 522, 66, 86, 30), pill(L2, 118, 28, 86, 30), pill(R2, 522, 28, 86, 30)],
  },
}

export const artFor = (name: string): Art => ARTS[name] || ARTS.dual

const arrowPath = { up: 'M12 6l7 11H5z', down: 'M12 18L5 7h14z', left: 'M6 12l11-7v14z', right: 'M18 12L7 19V5z' }

function Arrow({ dir }: { dir: keyof typeof arrowPath }) {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d={arrowPath[dir]} /></svg>
}

const crossPath = (x: number, y: number, s: number) => {
  const a = s * 0.54, b = s * 1.56
  return `M${x - a} ${y - b}h${2 * a}v${b - a}h${b - a}v${2 * a}h${-(b - a)}v${b - a}h${-2 * a}v${-(b - a)}h${-(b - a)}v${-2 * a}h${b - a}z`
}

type PictureProps = {
  art: string
  targets: Target[]
  active: number | null // the one being set
  describe: (t: Target) => string // what presses it, in words, for the button's label
  refs: React.MutableRefObject<Map<number, HTMLButtonElement>>
  onPick: (id: number) => void
  onFocusTarget?: (id: number) => void
}

// The picture. Lighting up follows the controller sixty times a second, so that's done
// directly on these buttons (data-on) rather than by drawing it all again.
export function ControllerPicture({ art, targets, active, describe, refs, onPick, onFocusTarget }: PictureProps) {
  const a = artFor(art)
  const byId = new Map(targets.map((t) => [t.id, t]))
  return (
    <div className="wps-art">
      <svg viewBox={`0 0 ${ART_W} ${ART_H}`} aria-hidden="true" focusable="false" preserveAspectRatio="xMidYMid meet">
        <path className="wps-body" d={a.body} />
        {a.cross?.map((c, i) => <path key={i} className="wps-cross" d={crossPath(c.x, c.y, c.s)} />)}
        {a.wells?.map((w, i) => <circle key={i} className="wps-well" cx={w.x} cy={w.y} r={w.r} />)}
        {a.marks?.map((m, i) => <text key={i} className="wps-mark" x={m.x} y={m.y} textAnchor="middle" dominantBaseline="central">{m.text}</text>)}
      </svg>
      {a.slots.map((s) => {
        const t = byId.get(s.t)
        if (!t) return null
        return (
          <button key={s.t} type="button" ref={(el) => { if (el) refs.current.set(s.t, el); else refs.current.delete(s.t) }}
            className={'wps-hot wps-hot-' + s.shape + (active === s.t ? ' listening' : '')} aria-label={`${t.label}: ${describe(t)}. Press to change.`}
            style={{ left: `${((s.x - s.w / 2) / ART_W) * 100}%`, top: `${((s.y - s.h / 2) / ART_H) * 100}%`, width: `${(s.w / ART_W) * 100}%`, height: `${(s.h / ART_H) * 100}%` }}
            onClick={() => onPick(s.t)} onFocus={() => onFocusTarget?.(s.t)}>
            {s.arrow ? <Arrow dir={s.arrow} /> : t.label}
          </button>
        )
      })}
    </div>
  )
}
