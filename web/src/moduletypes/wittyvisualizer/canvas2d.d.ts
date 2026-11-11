// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import type { EffectFile } from './format.ts';
import type { DrawInput, Renderer } from './gl.ts';
export declare class CanvasRenderer implements Renderer {
    private canvas;
    readonly kind: "canvas";
    readonly software = true;
    readonly label = "Plain canvas";
    private ctx;
    private probe;
    private guard;
    private plasma;
    private plasmaData;
    private w;
    private h;
    constructor(canvas: HTMLCanvasElement);
    supports(id: string): boolean;
    prepare(e: EffectFile): {
        ok: true;
        error?: undefined;
    } | {
        ok: false;
        error: string;
    };
    release(): void;
    dispose(): void;
    setSize(w: number, h: number): void;
    private color;
    draw(f: DrawInput): void;
    private bars;
    private scope;
    private dial;
    private stars;
    private waterfall;
    private molten;
}
