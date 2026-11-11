// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import type { DisplayValues } from './display';
export type HotAction = 'fastForward' | 'rewind' | 'slow' | 'quickSave' | 'quickLoad' | 'pause' | 'advance' | 'screenshot' | 'menu' | 'mute' | 'osd';
export declare const HOLD_ACTIONS: HotAction[];
export declare class Hotkeys {
    enabled: boolean;
    private keyOf;
    private padOf;
    private modifier;
    private combos;
    private held;
    private lastMask;
    private swallowed;
    private fire;
    private keyInUse;
    constructor(fire: (a: HotAction, down: boolean) => void, keyInUse: (code: string) => boolean);
    configure(v: DisplayValues): void;
    start(): void;
    stop(): void;
    private press;
    private letGo;
    private down;
    private up;
    filter(mask: number): number;
}
