// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { type ReactNode } from 'react';
import { type Ann, type BookMeta } from './host';
export type { Ann } from './host';
export declare const NAMED: Record<string, string>;
export declare const QUICK: string[];
export declare const hex: (c: string) => string;
export declare const colorName: (c: string) => string;
export declare function fill(c: string, a?: number): string;
export declare function useAnnotations(): {
    anns: Ann[];
    create: (a: Partial<Ann>) => Promise<Ann | null>;
    add: (a: Partial<Ann>) => Promise<Ann>;
    update: (a: Ann, patch: Partial<Ann>) => Promise<void>;
    remove: (a: Ann, undoable?: boolean) => Promise<void>;
    undo: () => Promise<void>;
    redo: () => Promise<void>;
    canUndo: boolean;
    canRedo: boolean;
};
export declare function useColorLabels(): readonly [Record<string, string>, (color: string, label: string) => void];
export declare function Sheet({ title, actions, onClose, children }: {
    title: string;
    actions?: ReactNode;
    onClose: () => void;
    children: ReactNode;
}): import("react").JSX.Element;
export declare function Field({ label, value, children }: {
    label: string;
    value?: ReactNode;
    children: ReactNode;
}): import("react").JSX.Element;
export declare function Range({ value, min, max, step, onChange, label, valueText, dir }: {
    value: number;
    min: number;
    max: number;
    step?: number;
    onChange: (v: number) => void;
    label: string;
    valueText?: string;
    dir?: 'ltr' | 'rtl';
}): import("react").JSX.Element;
export declare function Palette({ current, labels, onPick }: {
    current: string;
    labels: Record<string, string>;
    onPick: (c: string) => void;
}): import("react").JSX.Element;
export declare function Define({ word, onClose }: {
    word: string;
    onClose: () => void;
}): import("react").JSX.Element;
export declare function cite(it: BookMeta, quote: string, place: string): string;
export declare function exportNotes(it: BookMeta, anns: Ann[], labels: Record<string, string>, heading: (c: number) => string): void;
export declare function NotesPanel({ anns, labels, setLabel, place, open, remove }: {
    anns: Ann[];
    labels: Record<string, string>;
    setLabel: (c: string, v: string) => void;
    place: (chapter: number) => string;
    open: (a: Ann) => void;
    remove: (a: Ann) => void;
}): import("react").JSX.Element;
export declare function SelectionBar({ x, y, y2, quote, labels, onHighlight, onNote, onDefine, onCite, onSearch, onRead, onClose }: {
    x: number;
    y: number;
    y2?: number;
    quote: string;
    labels: Record<string, string>;
    onHighlight: (c: string) => void;
    onNote: () => void;
    onDefine?: (w: string) => void;
    onCite: () => void;
    onSearch: (q: string) => void;
    onRead?: () => void;
    onClose: () => void;
}): import("react").JSX.Element;
export declare function HighlightEditor({ ann, x, y, labels, onColor, onNote, onCite, onRemove, onDone }: {
    ann: Ann;
    x: number;
    y: number;
    labels: Record<string, string>;
    onColor: (c: string) => void;
    onNote: (n: string) => void;
    onCite: () => void;
    onRemove: () => void;
    onDone: () => void;
}): import("react").JSX.Element;
