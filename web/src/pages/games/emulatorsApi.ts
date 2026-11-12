// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Standalone emulators: every request the pages make. WittyPlay's own routes are passed on by
// the server as they are, under /api/v1/modules/wittyplay; the two a game's page uses are the
// server's own. What comes back is described in emulatorsLogic.ts.
import { api, ApiError, serverBase } from '../../api'
import type { Connected, EmulatorHost, EmulatorSession, Hello } from '@wittywilla/wittyplay'
import { apiPath, type BackendSettings, type OwnProgram, type CatalogueEntry, type Diagnostics, type DockerStatus, type EmulatorSettingsView, type GameEmulator, type InstallJob, type RunningSession, type SettingsScope, type Values } from './emulatorsLogic'

const M = '/v1/modules/wittyplay/docker'
const id = encodeURIComponent

export const emulatorsApi = {
  // The backend, for administrators.
  status: () => api.get<DockerStatus>(`${M}/status`),
  diagnostics: () => api.get<Diagnostics>(`${M}/diagnostics`),
  settings: () => api.get<BackendSettings>(`${M}/settings`),
  // The whole object goes back, and the server answers with it as kept.
  saveSettings: (settings: Values) => api.put<Values>(`${M}/settings`, settings),

  // The catalogue, for administrators.
  catalogue: () => api.get<CatalogueEntry[]>(`${M}/emulators`),
  // license is the licence the administrator was shown, as the entry gives it: the server
  // refuses an install whose licence nobody has seen. An update is the same call.
  install: (emulator: string, license: string) => api.post<InstallJob>(`${M}/emulators/${id(emulator)}/install`, { license }),
  installJob: (emulator: string) => api.get<InstallJob>(`${M}/emulators/${id(emulator)}/install`),
  cancelInstall: (emulator: string) => api.del<void>(`${M}/emulators/${id(emulator)}/install`),
  remove: (emulator: string) => api.del<void>(`${M}/emulators/${id(emulator)}`),

  // One emulator's settings, for anyone signed in. game adds that game's own level.
  emulatorSettings: (emulator: string, game?: number) => api.get<EmulatorSettingsView>(`${M}/emulators/${id(emulator)}/settings${game ? `?game=${game}` : ''}`),
  // values holds only what changed; null clears a key.
  saveEmulatorSettings: (emulator: string, scope: SettingsScope, values: Record<string, unknown>, game?: number) =>
    api.put<EmulatorSettingsView>(`${M}/emulators/${id(emulator)}/settings`, { scope, ...(scope === 'game' ? { game } : {}), values }),

  // An administrator's own emulator programs, installed on this server by them. Replaced whole.
  own: () => api.get<{ own: OwnProgram[] | null; max: number }>(`${M}/own`),
  saveOwn: (list: OwnProgram[]) => api.put<{ own: OwnProgram[] | null; max: number }>(`${M}/own`, list),

  // Games running now: an administrator sees everyone's, anyone else their own.
  sessions: () => api.get<RunningSession[]>(`${M}/sessions`),
  connect: (sid: string, hello: Hello, offer: string) => api.post<Connected>(`${M}/sessions/${id(sid)}/connect`, { hello, offer }),
  pause: (sid: string) => api.post<void>(`${M}/sessions/${id(sid)}/pause`),
  resume: (sid: string) => api.post<void>(`${M}/sessions/${id(sid)}/resume`),
  stop: (sid: string) => api.del<void>(`${M}/sessions/${id(sid)}`),

  // A game's page: which emulators could play it, and starting one.
  forGame: (itemId: number) => api.get<{ emulators: GameEmulator[] }>(`/v1/games/${itemId}/emulators`).then((r) => r?.emulators || []),
  play: (itemId: number, emulator: string, device?: string) => api.post<EmulatorSession>(`/v1/games/${itemId}/play/emulator`, { emulator, ...(device ? { device } : {}) }),
}

// The server's code for a refusal ("in-use", "license", "no-session"), where it gave one.
export const codeOf = (e: unknown): string => (e instanceof ApiError && typeof e.data?.code === 'string' ? e.data.code : '')

// A refusal as the player wants it: with the status and the code beside the sentence, which is
// how it tells a game that has ended from a connection that failed.
function forPlayer(e: unknown): never {
  if (e instanceof ApiError) throw Object.assign(new Error(e.message), { status: e.status, code: codeOf(e) })
  throw e
}

// sessionHost is the player's way to one session, through the addresses the session itself
// gave. One that's missing answers as a refusal would, rather than guessing an address.
export function sessionHost(s: EmulatorSession, more: Pick<EmulatorHost, 'controls' | 'controllers' | 'report'> = {}): EmulatorHost {
  const at = (url: string | undefined, what: string) => {
    const path = apiPath(url)
    if (!path) throw new Error(`The server gave no way to ${what} this game.`)
    return path
  }
  return {
    ...more,
    connect: async (hello, offer) => { try { return await api.post<Connected>(at(s.connect, 'connect to'), { hello, offer }) } catch (e) { return forPlayer(e) } },
    stop: async () => { try { await api.del(at(s.stop, 'stop')) } catch (e) { forPlayer(e) } },
    pause: async () => { try { await api.post(at(s.pause, 'pause')) } catch (e) { forPlayer(e) } },
    resume: async () => { try { await api.post(at(s.resume, 'resume')) } catch (e) { forPlayer(e) } },
    // The desktop's address carries its own ticket, so the sign-in is kept out of it. An app
    // that keeps its pages apart from the server (WMS_SERVER) needs the server put in front.
    mediaUrl: (path) => serverBase() + path,
  }
}
