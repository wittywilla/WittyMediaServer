import frame from './shaders/frame.glsl?raw'
import lib from './shaders/lib.glsl?raw'
import passes from './shaders/passes.glsl?raw'
import palettes from './effects/palettes.json'
import { parseEffect, type EffectFile } from './format.ts'
import type { PaletteDef } from './engine.ts'

// Puts shaders together for WebGL 2. The same files go through qt/CMakeLists.txt for the
// desktop app, with its own first lines, so the effects themselves are written once.

const files = import.meta.glob('./effects/*.glsl', { query: '?raw', import: 'default', eager: true }) as Record<string, string>

export const PALETTES = palettes.palettes as PaletteDef[]

export const BUILTIN_EFFECTS: EffectFile[] = Object.values(files)
  .map((text) => {
    const r = parseEffect(text, true)
    if (!r.ok) throw new Error(r.error)
    return r.effect
  })
  .sort((a, b) => a.name.localeCompare(b.name))

const section = (name: string) => {
  const m = new RegExp(`//@ ${name}\\r?\\n([\\s\\S]*?)(?=\\r?\\n//@ |$)`).exec(passes)
  if (!m) throw new Error('no shader section ' + name)
  return m[1] + '\n'
}

const HEADER = `#version 300 es
precision highp float;
precision highp int;
precision highp sampler2D;
#define WV_UBO layout(std140)
#define WV_SAMPLER(n) uniform sampler2D
#define WV_IN(n) in
#define WV_OUT(n) out
#define WV_VOUT(n) out
#define WV_VERTEX_ID gl_VertexID
`

export const fullscreenVert = () => HEADER + frame + section('fullscreen-vert')
export const guardFrag = () => HEADER + frame + section('composite') + section('guard-frag')
export const presentFrag = () => HEADER + frame + section('composite') + section('present-frag')
export const quadFrag = () => HEADER + section('quad-frag')
export const effectFrag = (e: EffectFile) => HEADER + (e.feedback ? '#define WV_FEEDBACK\n' : '') + frame + lib + section('effect-head') + e.source + '\n' + section('effect-tail')
export const quadVert = (e: EffectFile) => HEADER + '#define WV_QUAD\n' + frame + lib + e.source + '\n' + section('quad-vert')
