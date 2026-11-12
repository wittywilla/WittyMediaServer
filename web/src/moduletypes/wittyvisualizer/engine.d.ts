export type Settings = {
    notice: boolean;
    on: boolean;
    effect: string | null;
    random: boolean;
    cycleSec: number;
    favourites: string[];
    favouritesOnly: boolean;
    intensity: number;
    sensitivity: number;
    calm: boolean | null;
    scale: number;
    stats: boolean;
    speed: number;
    colour: 'effect' | 'theme';
};
export declare const DEFAULT_SETTINGS: Settings;
export declare const CYCLE_CHOICES: number[];
export declare const SCALE_CHOICES: number[];
export declare function normalizeSettings(v: unknown): Settings;
export declare function moduleSettingValues(s: Settings): Record<string, unknown>;
export type CycleItem = {
    id: string;
    motion: 'gentle' | 'lively';
    cost: number;
};
export declare function eligible(all: CycleItem[], s: Pick<Settings, 'favourites' | 'favouritesOnly'>, calm: boolean, weak: boolean): CycleItem[];
export declare class Cycle {
    private queue;
    private past;
    current: string | null;
    private rand;
    constructor(rand?: () => number);
    private shuffled;
    next(ids: string[], random: boolean): string;
    previous(ids: string[], random: boolean): string;
    select(id: string): void;
}
export declare class Budget {
    scale: number;
    frameGap: number;
    fps: number;
    private avg;
    private held;
    private calmFor;
    private patience;
    private rose;
    private counted;
    private countedFor;
    readonly floor: number;
    readonly ceiling: number;
    constructor(start: number, floor: number, ceiling: number);
    frame(dt: number): boolean;
}
export type PaletteDef = {
    id: string;
    a: number[];
    b: number[];
    c: number[];
    d: number[];
};
export declare function accentPalette(hex: string): PaletteDef | null;
export declare class Palettes {
    private from;
    private to;
    private t;
    private beatsAtChange;
    private since;
    private list;
    private rand;
    readonly out: {
        a: number[];
        b: number[];
        c: number[];
        d: number[];
    };
    constructor(list: PaletteDef[], rand?: () => number);
    private blend;
    step(dt: number, beatCount: number, calm: boolean): void;
}
