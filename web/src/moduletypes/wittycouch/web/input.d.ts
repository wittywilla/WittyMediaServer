// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
export type Send = (m: Uint8Array, reliable: boolean) => void;
export declare const MAX_PADS = 4;
type PadState = {
    buttons: number;
    lt: number;
    rt: number;
    lx: number;
    ly: number;
    rx: number;
    ry: number;
};
export declare function readPad(p: Gamepad): PadState;
export declare function padMessage(index: number, mask: number, s: PadState): Uint8Array;
export declare class Pads {
    private send;
    private onMenu;
    private onChange;
    private last;
    private lastSent;
    private timer;
    private menuSince;
    private paused;
    connected: number;
    constructor(send: Send, onMenu: () => void, onChange: (n: number) => void);
    start(): void;
    stop(): void;
    setPaused(on: boolean): void;
    private poll;
    rumble(index: number, low: number, high: number): void;
}
export declare function vkFor(code: string): number | undefined;
export declare function keyMessage(code: string, down: boolean, e: {
    shiftKey: boolean;
    ctrlKey: boolean;
    altKey: boolean;
    metaKey: boolean;
}): Uint8Array | null;
export declare function moveMessage(dx: number, dy: number): Uint8Array;
export declare function positionMessage(x: number, y: number, w: number, h: number): Uint8Array;
export declare function buttonMessage(button: number, down: boolean): Uint8Array;
export declare function scrollMessage(amount: number, horizontal: boolean): Uint8Array;
export declare const TOUCH_BUTTONS: {
    a: number;
    b: number;
    x: number;
    y: number;
    lb: number;
    rb: number;
    start: number;
    back: number;
    up: number;
    down: number;
    left: number;
    right: number;
};
export {};
