// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import type { Target } from './consoles';
export type Slot = {
    t: number;
    x: number;
    y: number;
    w: number;
    h: number;
    shape: 'round' | 'pill' | 'pad' | 'stick';
    arrow?: 'up' | 'down' | 'left' | 'right';
};
export type Art = {
    body: string;
    wells?: {
        x: number;
        y: number;
        r: number;
    }[];
    cross?: {
        x: number;
        y: number;
        s: number;
    }[];
    marks?: {
        x: number;
        y: number;
        text: string;
    }[];
    slots: Slot[];
};
export declare const ART_W = 640, ART_H = 400;
export declare const artFor: (name: string) => Art;
type PictureProps = {
    art: string;
    targets: Target[];
    active: number | null;
    describe: (t: Target) => string;
    refs: React.MutableRefObject<Map<number, HTMLButtonElement>>;
    onPick: (id: number) => void;
    onFocusTarget?: (id: number) => void;
};
export declare function ControllerPicture({ art, targets, active, describe, refs, onPick, onFocusTarget }: PictureProps): import("react").JSX.Element;
export {};
