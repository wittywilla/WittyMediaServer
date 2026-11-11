// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
export type DisplayValues = Record<string, unknown>;
type Media = HTMLVideoElement | HTMLCanvasElement;
export declare const reducedMotion: () => boolean;
export declare function wantsShader(v: DisplayValues): boolean;
export type DisplayMode = 'plain' | 'shader' | 'css';
export declare class DisplayEngine {
    private stage;
    onMode?: (m: DisplayMode) => void;
    private canvas;
    private overlay;
    private gl;
    private prog;
    private u;
    private tex;
    private newest;
    private haveFrame;
    private media;
    private values;
    private srcW;
    private srcH;
    private picW;
    private picH;
    private cropX;
    private cropY;
    private mode;
    private raf;
    private vfc;
    private running;
    private lost;
    private failed;
    constructor(stage: HTMLElement);
    attach(media: Media): void;
    update(values: DisplayValues, src: {
        width: number;
        height: number;
    }, size: {
        picW: number;
        picH: number;
    }): void;
    get currentMode(): DisplayMode;
    private apply;
    private setMode;
    private placeOverlay;
    private ensureGL;
    private setFilter;
    private resize;
    private start;
    private stop;
    private schedule;
    private ready;
    private draw;
    capture(): Promise<Blob | null>;
    destroy(): void;
}
export {};
