import type { ReaderHost } from './host';
export type Tool = 'pen' | 'pencil' | 'marker' | 'eraser';
export type DrawTool = Exclude<Tool, 'eraser'>;
export type Stroke = {
    t: DrawTool;
    w: number;
    o: number;
    a: number;
    pts: number[];
};
export declare const INK_COLORS: string[];
export declare const MARKER_COLORS: string[];
export type InkPrefs = {
    tool: Tool;
    color: Record<DrawTool, string>;
    size: Record<DrawTool, number>;
    opacity: Record<DrawTool, number>;
    pressure: boolean;
    penOnly: boolean;
    autoPen: boolean;
    snap: boolean;
    recent: string[];
};
export declare const INK_DEF: InkPrefs;
export declare function loadInk(host?: Pick<ReaderHost, 'settings'>): InkPrefs;
export declare function saveInk(p: InkPrefs, host?: Pick<ReaderHost, 'settings'>): void;
export declare function pointPressure(e: PointerEvent, tool: DrawTool, usePressure: boolean): number;
export declare function centerline(pts: number[]): string;
export declare function outline(pts: number[], w: number, varying: boolean): string;
export declare function hits(pts: number[], w: number, x: number, y: number, tol: number): boolean;
export declare const InkStroke: import("react").MemoExoticComponent<({ s, color, blend, grain }: {
    s: Stroke;
    color: string;
    blend: boolean;
    grain?: string;
}) => import("react").JSX.Element>;
export declare const InkDefs: ({ id, freq }: {
    id?: string;
    freq?: number;
}) => import("react").JSX.Element;
export declare function InkBar({ p, set, undo, redo, canUndo, canRedo, done, extra }: {
    p: InkPrefs;
    set: (v: Partial<InkPrefs>) => void;
    undo: () => void;
    redo: () => void;
    canUndo: boolean;
    canRedo: boolean;
    done: () => void;
    extra?: React.ReactNode;
}): import("react").JSX.Element;
