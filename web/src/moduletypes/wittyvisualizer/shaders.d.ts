// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { type EffectFile } from './format.ts';
import type { PaletteDef } from './engine.ts';
export declare const PALETTES: PaletteDef[];
export declare const BUILTIN_EFFECTS: EffectFile[];
export declare const fullscreenVert: () => string;
export declare const guardFrag: () => string;
export declare const presentFrag: () => string;
export declare const quadFrag: () => string;
export declare const effectFrag: (e: EffectFile) => string;
export declare const quadVert: (e: EffectFile) => string;
