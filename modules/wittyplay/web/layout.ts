// Where the picture goes on the screen. No browser objects in here, so tests can feed
// it any shape of screen and game.

export type LayoutInput = {
  areaW: number; areaH: number // the room there is
  srcW: number; srcH: number // the game's own pixels
  aspect: number // what the game says its shape is; 0 when it doesn't
  mode: string // original | 4:3 | 16:9 | square | stretch | custom
  custom: number // used by custom
  integer: boolean
  cropX: number; cropY: number // percent off each side
  turns: number // quarter turns clockwise, from the game and the person's choice together
  inset: number // percent of the room kept for what's around the picture
}

export type Layout = {
  // The box on screen, in pixels: this is what's seen, after turning.
  boxW: number; boxH: number
  // The picture inside it before turning, cropped by its overflow.
  picW: number; picH: number
  // How much larger than the box's unturned shape the whole picture is, for the crop.
  scaleX: number; scaleY: number
  scale: number // screen pixels to a game pixel, for scanlines and grids
}

// shapeOf is width over height of the part that will be seen, before turning.
function shapeOf(i: LayoutInput): number {
  const keepW = 1 - (2 * i.cropX) / 100, keepH = 1 - (2 * i.cropY) / 100
  const own = i.srcW > 0 && i.srcH > 0 ? i.srcW / i.srcH : 4 / 3
  const pixelShape = (i.aspect > 0 ? i.aspect : own) / own // how wide a game pixel is
  let whole: number
  switch (i.mode) {
    case '4:3': whole = 4 / 3; break
    case '16:9': whole = 16 / 9; break
    case 'square': whole = own; break
    case 'custom': whole = i.custom > 0 ? i.custom : own; break
    default: whole = own * pixelShape
  }
  return (whole * keepW) / keepH
}

export function computeLayout(i: LayoutInput): Layout {
  const roomW = Math.max(1, i.areaW * (1 - i.inset / 100)), roomH = Math.max(1, i.areaH * (1 - i.inset / 100))
  const sideways = (i.turns & 1) === 1
  // What fits, thinking in the picture's own unturned directions.
  const fitW = sideways ? roomH : roomW, fitH = sideways ? roomW : roomH
  const keepW = 1 - (2 * i.cropX) / 100, keepH = 1 - (2 * i.cropY) / 100
  const srcW = Math.max(1, i.srcW * keepW), srcH = Math.max(1, i.srcH * keepH)
  let w: number, h: number
  if (i.mode === 'stretch') {
    w = fitW
    h = fitH
  } else {
    const shape = shapeOf(i)
    if (fitW / fitH > shape) {
      h = fitH
      w = h * shape
    } else {
      w = fitW
      h = w / shape
    }
  }
  if (i.integer && i.srcH > 0) {
    // A whole number of screen pixels for each game pixel, keeping the shape.
    const shape = w / h
    const n = Math.max(1, Math.floor(Math.min(h / srcH, (w / shape) / srcH)))
    h = n * srcH
    w = i.mode === 'square' ? n * srcW : h * shape
    if (w > fitW) {
      const k = fitW / w
      w *= k
      h *= k
    }
  }
  w = Math.max(1, Math.round(w))
  h = Math.max(1, Math.round(h))
  return { boxW: sideways ? h : w, boxH: sideways ? w : h, picW: w, picH: h, scaleX: 1 / keepW, scaleY: 1 / keepH, scale: h / srcH }
}
