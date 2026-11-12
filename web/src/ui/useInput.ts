// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useSyncExternalStore } from 'react'
import { inputSnapshot, subscribeInput } from './inputKind'

// Which input is in use (ui/inputKind.ts), for a component to draw itself by.
export const useInput = () => useSyncExternalStore(subscribeInput, inputSnapshot)
