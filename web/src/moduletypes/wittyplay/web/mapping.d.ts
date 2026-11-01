export declare const SELECT = 2, START = 3, UP = 4, DOWN = 5, LEFT = 6, RIGHT = 7, L2 = 12, R2 = 13;
export declare const STICK_LX_NEG = 16, TARGETS = 24;
export declare const HAT_UP = 1, HAT_RIGHT = 2, HAT_DOWN = 3, HAT_LEFT = 4;
export type Binding = {
    b: number;
} | {
    a: number;
    d: 1 | -1;
    r?: -1 | 0 | 1;
} | {
    h: number;
    d: 1 | 2 | 3 | 4;
};
export type PadMap = Record<number, Binding | null>;
export type KeyMap = Record<number, string>;
export type ProfileOptions = {
    deadzone?: number;
    threshold?: number;
    stickDpad?: boolean;
    turbo?: number[];
    turboHz?: number;
    noRumble?: boolean;
    calibration?: Record<number, [number, number, number]>;
};
export type Profile = {
    id: number;
    name: string;
    kind: 'gamepad' | 'keyboard';
    controller: string;
    controllerName?: string;
    console?: string;
    game?: number;
    gameTitle?: string;
    raw?: boolean;
    active: boolean;
    map?: PadMap;
    keys?: KeyMap;
    options: ProfileOptions;
    updatedAt?: number;
};
export type RawLayout = {
    buttons: (Binding | null)[];
    axes: number[];
};
export type KnownPad = {
    id: string;
    name: string;
    style: string;
    layout?: string;
    linuxLayout?: string;
};
export type ControllerData = {
    layouts: Record<string, RawLayout>;
    pads: KnownPad[];
};
export type PadLike = {
    id: string;
    index: number;
    mapping: string;
    connected?: boolean;
    timestamp?: number;
    buttons: ArrayLike<{
        pressed: boolean;
        value: number;
    }>;
    axes: ArrayLike<number>;
};
export declare function parseModel(id: string): {
    key: string;
    name: string;
    hasIds: boolean;
};
export declare const isLinux: (ua: string) => boolean;
export declare function knownPad(data: ControllerData | null, key: string): KnownPad | undefined;
export type Standing = 'standard' | 'known' | 'unknown';
export declare function standingOf(pad: PadLike, data: ControllerData | null, ua: string): {
    standing: Standing;
    layout: RawLayout | null;
    known?: KnownPad;
};
export declare function hatBits(v: number): number;
export declare function noteHat(pad: PadLike): boolean;
type Source = {
    kind: number;
    i: number;
    d: number;
    r: number;
};
type Entry = {
    target: number;
    src: Source;
};
export declare function throughLayout(b: Binding, pad: PadLike, layout: RawLayout | null): Binding | null;
export type Reading = {
    mask: number;
    lx: number;
    ly: number;
    rx: number;
    ry: number;
    l2: number;
    r2: number;
    held: Uint8Array;
};
export declare const newReading: () => Reading;
export type Compiled = {
    entries: Entry[];
    deadzone: number;
    threshold: number;
    stickDpad: Source[] | null;
    turbo: number;
    turboHz: number;
    calibration: Record<number, [number, number, number]> | null;
    noRumble: boolean;
    menu: (Source | null)[];
    prevMask: number;
};
export declare function compile(pad: PadLike, map: PadMap, layout: RawLayout | null, o: ProfileOptions): Compiled;
export declare function sourceValue(s: Source, pad: PadLike, cal: Compiled['calibration']): number;
export declare function readPad(c: Compiled, pad: PadLike, now: number, out: Reading): void;
export declare function menuHeld(c: Compiled, pad: PadLike): boolean;
export type Snapshot = {
    buttons: boolean[];
    axes: number[];
};
export declare function snapshot(pad: PadLike): Snapshot;
export declare function noteReleases(pad: PadLike, base: Snapshot): void;
export declare function findPress(pad: PadLike, base: Snapshot): Binding | null;
export declare function rawFromDefaults(defaults: PadMap, pad: PadLike): PadMap;
export declare function toStandard(b: Binding, layout: RawLayout | null, pad: PadLike): Binding | null;
export declare function resolveProfile(list: Profile[], kind: string, controller: string, consoleId: string, game: number, raw: boolean): Profile | null;
export declare function effectiveMap(defaults: PadMap, known: Set<number>, p: Profile | null): PadMap;
export declare function effectiveKeys(defaults: KeyMap, known: Set<number>, p: Profile | null): KeyMap;
export declare function effectiveOptions(defaults: ProfileOptions, p: Profile | null): ProfileOptions;
export declare function conflicts(map: PadMap): Map<number, number[]>;
export declare function keyConflicts(map: KeyMap): Map<number, number[]>;
export declare function describeSource(b: Binding | null | undefined, style?: string, raw?: boolean): string;
export declare const describeKey: (code?: string) => string;
export {};
