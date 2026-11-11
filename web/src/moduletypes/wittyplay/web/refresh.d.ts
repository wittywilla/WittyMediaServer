// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
export declare function measureRefresh(done: (hz: number) => void): void;
export declare function rateOf(gaps: number[]): number;
export declare function screenNow(): {
    hz?: number;
    width?: number;
    height?: number;
};
