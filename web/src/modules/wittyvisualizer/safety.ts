// Flashing can harm people with photosensitive epilepsy, so the picture goes through a
// limiter that no effect can get around. WCAG 2.3.1 calls a change a flash only if it's a
// pair of opposing swings of at least 10% of the maximum relative luminance (or the
// equivalent in saturated red), and allows up to three a second.
//
// The guard keeps, for each tile of the screen, a memory of how dark that tile has lately
// been. It may show at most the memory plus a small dead band, and the memory rises slowly.
// Falling is free. So a swing inside any third of a second is at most
//
//     deadBand + rise / 3
//
// With the values below, that's under 0.1: a change fast enough to count as more than
// three flashes a second can't be large enough to count as a flash at all. shaders/passes.glsl
// runs this on the graphics card ("guard-frag"), and canvas2d.ts runs it here. WittyMediaPlayer
// has a copy for its CD screen's simple visuals (FlashLimiter in src/visual/visualizer.cpp), so
// a change to this rule goes there too.

export const GUARD = { deadBand: 0.04, rise: 0.15 }
export const GUARD_CALM = { deadBand: 0.02, rise: 0.05 }

// Relative luminance of a colour in screen values (0 to 1), with saturated red counted the
// way the guideline counts it: a change of 20/320 in R - G - B is a flash, so it's scaled
// to compare with the 0.1 for luminance.
export function flashMetric(r: number, g: number, b: number) {
  const lr = Math.pow(Math.max(r, 0), 2.2), lg = Math.pow(Math.max(g, 0), 2.2), lb = Math.pow(Math.max(b, 0), 2.2)
  const lum = 0.2126 * lr + 0.7152 * lg + 0.0722 * lb
  const sum = lr + lg + lb
  const red = sum > 0 && lr / sum >= 0.8 ? (lr - lg - lb) * 1.6 : 0
  return Math.max(lum, red)
}

export class FlashGuard {
  private memory = 0
  deadBand: number
  rise: number
  constructor(limits = GUARD) { this.deadBand = limits.deadBand; this.rise = limits.rise }
  // Takes this frame's brightness, gives what to multiply its light by.
  step(metric: number, dt: number) {
    this.memory = Math.min(metric, this.memory + this.rise * dt)
    const allowed = this.memory + this.deadBand
    return metric > allowed ? allowed / metric : 1
  }
}

// WCAG's definition, for checking a recording of the screen: how many flashes (pairs of
// opposing swings of at least 0.1, where the darker side is under 0.8) there are in the
// worst second. Takes one brightness value per frame, with the frame times in seconds.
export function worstFlashesPerSecond(values: number[], times: number[]) {
  const swings: number[] = [] // when each swing finished
  let low = values[0], high = values[0], dir = 0
  const big = (a: number, b: number) => Math.abs(a - b) >= 0.1 && Math.min(a, b) < 0.8
  for (let i = 1; i < values.length; i++) {
    const v = values[i]
    if (dir >= 0 && v > high) high = v
    if (dir <= 0 && v < low) low = v
    if (dir >= 0 && big(high, v) && v < high) { swings.push(times[i]); dir = -1; low = v }
    else if (dir <= 0 && big(low, v) && v > low) { swings.push(times[i]); dir = 1; high = v }
  }
  let worst = 0
  for (let i = 0; i < swings.length; i++) {
    let n = 0
    for (let j = i; j < swings.length && swings[j] - swings[i] < 1; j++) n++
    worst = Math.max(worst, Math.floor(n / 2))
  }
  return worst
}
