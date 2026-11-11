// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
export type TapState = 'live' | 'pattern' | 'needs-gesture' | 'closed';
export interface AudioTap {
    state(): TapState;
    why(): string;
    rate(): number;
    read(left: Float32Array, right: Float32Array): void;
    resume(): Promise<void>;
    close(): void;
}
export declare function openTap(el: HTMLMediaElement | null): AudioTap;
