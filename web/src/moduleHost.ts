// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import React from 'react'
import ReactDom from 'react-dom'
import * as jsxRuntime from 'react/jsx-runtime'
import * as reactRouterDom from 'react-router-dom'
import pdfWorkerUrl from 'pdfjs-dist/legacy/build/pdf.worker.min.mjs?url'
import { api, getToken, mediaUrl, serverBase } from './api'
import { useApp } from './state'
import * as ui from './ui'
import * as icons from './icons'
import { ModuleMissing } from './components/ModuleMissing'
import LegalNote from './components/LegalNote'
import { CardRouteContext } from './sdui/render/cardRoute'
import { GameArtContext, GlyphContext, RowLayoutContext, TabsContext } from './sdui/render/glyphs'
import { setPageRegions, usePageRegions } from './sdui/regions'
import { useTvMode } from './tvmode/mode'
import { focusFirst } from './tv'
import { rememberHost } from './moduleRegistry'

// What a module's web bundle shares with the app, and reads from one global because an
// inline import map would need the page's security policy loosened. A bundle has to use
// the app's React, or its hooks break, and the app's router, or its links and routes
// belong to nobody; pdf.js and its worker stay here so a bundle doesn't carry its own huge
// copy. WittyModule's builder (kit/web) writes the other side of this. This file loads only
// once a bundle is actually going to be used.
//
// Web host API 5 (module API 5, docs/MODULES-API5.md) adds the router, the app's API client,
// its state, its UI kit and icons, the SDUI renderer's contexts and TV input, and hostFor,
// which gives each module's register(host) a host of its own. WittyModule kit/web/host has
// the types; a test keeps them and this object the same.
//
// Adding here raises the web host API; removing or changing anything breaks bundles
// already released.
export const moduleHost = {
  api: 5,
  react: React,
  reactDom: ReactDom,
  jsxRuntime,
  reactRouterDom,
  // The loader sets this to WittyInk's bundle before it loads a reader that's built on it.
  wittyink: undefined as Record<string, unknown> | undefined,
  // Large, so only a PDF loads it; pdfjsModule is filled in before a bundle that needs it.
  pdfjs: () => import('pdfjs-dist/legacy/build/pdf.mjs'),
  pdfjsModule: undefined as unknown,
  pdfWorkerUrl,
}

/** What the server-facing parts of a module's host are: the app's own client. */
const http = {
  get: api.get,
  post: api.post,
  put: api.put,
  del: api.del,
  mediaUrl,
  token: getToken,
  base: serverBase,
}

/** Host features this app offers, and their versions. */
const offers: Record<string, number> = { web: 5, frames: 1, cards: 1, settings: 1, routes: 1 }

/** The host a module's register(host) and its components are given (web host API 5). */
export function hostFor(module: string) {
  const host = {
    version: 5,
    module,
    has: (feature: string, v = 1) => (offers[feature] || 0) >= v,
    /** The app's API client, with the person's own sign-in. */
    http,
    /** The module's own routes: /api/modules/<module><path>. */
    moduleApi: {
      get: <T,>(path: string) => api.get<T>(`/modules/${module}${path}`),
      post: <T,>(path: string, body?: unknown) => api.post<T>(`/modules/${module}${path}`, body),
      put: <T,>(path: string, body?: unknown) => api.put<T>(`/modules/${module}${path}`, body),
      del: <T,>(path: string) => api.del<T>(`/modules/${module}${path}`),
    },
    /** The app's state: who is signed in, the server, the app's settings. */
    useApp,
    useTvMode,
    toast: ui.toast,
    ask: ui.ask,
    ui,
    icons,
    ModuleMissing,
    LegalNote,
    sdui: { CardRouteContext, GameArtContext, GlyphContext, RowLayoutContext, TabsContext, setPageRegions, usePageRegions },
    input: { focusFirst },
  }
  rememberHost(module, host)
  return host
}

export type ModuleHost = ReturnType<typeof hostFor>

;(globalThis as unknown as { __wmsModuleHost: typeof moduleHost & { hostFor: typeof hostFor } }).__wmsModuleHost = Object.assign(moduleHost, { hostFor })
