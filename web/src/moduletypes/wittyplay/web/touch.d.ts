// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { type Control, type ControlsData, type Layout, type Orientation } from './consoles';
export declare function layoutFor(c: ControlsData, o: Orientation): Layout;
export declare const clamp: (v: number, lo: number, hi: number) => number;
export declare function dpadMask(dx: number, dy: number): number;
export type Rect = {
    w: number;
    h: number;
};
export declare const isStick: (c: Control) => boolean;
export declare function onStick(pt: {
    x: number;
    y: number;
}, controls: Control[], layout: Layout, r: Rect): boolean;
export declare function touchStick(pt: {
    x: number;
    y: number;
} | null, controls: Control[], layout: Layout, r: Rect): {
    x: number;
    y: number;
};
export declare function touchMask(points: {
    x: number;
    y: number;
}[], controls: Control[], layout: Layout, r: Rect): number;
