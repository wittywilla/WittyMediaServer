// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import React from 'react'
import ReactDom from 'react-dom'
import * as jsxRuntime from 'react/jsx-runtime'
import pdfWorkerUrl from 'pdfjs-dist/legacy/build/pdf.worker.min.mjs?url'

// What a module's web bundle shares with the app, and reads from one global because an
// inline import map would need the page's security policy loosened. A bundle has to use
// the app's React, or its hooks break, and pdf.js and its worker stay here so a bundle
// doesn't carry its own huge copy. web/scripts/build-module-bundle.mjs writes the other
// side of this. This file loads only once a bundle is actually going to be used.
//
// Adding here raises the module API (moduleBundles.ts); removing or changing anything
// breaks bundles already released.
export const moduleHost = {
  api: 4,
  react: React,
  reactDom: ReactDom,
  jsxRuntime,
  // The loader sets this to WittyInk's bundle before it loads a reader that's built on it.
  wittyink: undefined as Record<string, unknown> | undefined,
  // Large, so only a PDF loads it; pdfjsModule is filled in before a bundle that needs it.
  pdfjs: () => import('pdfjs-dist/legacy/build/pdf.mjs'),
  pdfjsModule: undefined as unknown,
  pdfWorkerUrl,
}

;(globalThis as unknown as { __wmsModuleHost: typeof moduleHost }).__wmsModuleHost = moduleHost
