// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
export declare const GUARD: {
    deadBand: number;
    rise: number;
};
export declare const GUARD_CALM: {
    deadBand: number;
    rise: number;
};
export declare function flashMetric(r: number, g: number, b: number): number;
export declare class FlashGuard {
    private memory;
    deadBand: number;
    rise: number;
    constructor(limits?: {
        deadBand: number;
        rise: number;
    });
    step(metric: number, dt: number): number;
}
export declare function worstFlashesPerSecond(values: number[], times: number[]): number;
