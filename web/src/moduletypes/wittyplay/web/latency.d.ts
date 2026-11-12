export type FrameRecord = {
    key: boolean;
    seq: number;
    inputSeq: number;
    rtp: number;
    size: number;
    inputServer: number;
    inputHost: number;
    runStart: number;
    runEnd: number;
    serverIn: number;
    encIn: number;
    encOut: number;
    sent: number;
};
export type StageTimes = {
    core: number;
    toServer: number;
    queue: number;
    encode: number;
    send: number;
    network: number;
    decode: number;
    display: number;
    total: number;
};
export type InputTimes = StageTimes & {
    uplink: number;
    toRunner: number;
    waitCore: number;
    photon: number;
};
export declare const TIMING_PING = 241;
export declare const TIMING_PONG = 242;
export declare const TIMING_SUBSCRIBE = 243;
export declare const TIMING_RECORD = 244;
export declare const TIMING_LATENCY = 245;
export declare const TIMING_SHIFT = 246;
export declare const TIMING_KEYFRAME = 247;
export declare const TIMING_DIRECT = 248;
export declare function parseRecord(b: ArrayBuffer, toLocal: (unixNanos: bigint) => number): FrameRecord | null;
export declare class ClockSync {
    private best;
    private bestAt;
    private offset;
    private ready;
    ping(): ArrayBuffer;
    pong(b: ArrayBuffer): boolean;
    get synced(): boolean;
    get rttMs(): number;
    toLocal: (unixNanos: bigint) => number;
}
export declare class Series {
    private cap;
    private a;
    constructor(cap?: number);
    add(v: number): void;
    get count(): number;
    percentile(p: number): number;
    get median(): number;
    get p95(): number;
    clear(): void;
}
export type StageName = keyof StageTimes;
export type InputStageName = 'uplink' | 'toRunner' | 'waitCore' | 'photon';
export type Presented = {
    rtp?: number;
    seq?: number;
    received?: number;
    presented: number;
    displayAt: number;
};
export declare class LatencyMeter {
    readonly clock: ClockSync;
    readonly frames: Record<StageName, Series>;
    readonly presses: Record<InputStageName, Series>;
    framesSeen: number;
    private records;
    private bySeq;
    private waiting;
    private pressSeq;
    private pressAt;
    private lastInput;
    log: InputTimes[] | null;
    message(b: ArrayBuffer): void;
    bySequence(seq: number): FrameRecord | null;
    byRtp(rtp: number): FrameRecord | null;
    shown(p: Presented): void;
    press(at?: number): number;
    summary(): LatencySummary | null;
    private join;
    private finishPresses;
}
export type LatencySummary = {
    frames?: number;
    press?: number;
    total: number;
    core?: number;
    encode?: number;
    toNetwork?: number;
    network?: number;
    decode?: number;
    display?: number;
    worst: number;
};
export declare function adviceFor(s: LatencySummary, c: {
    fps: number;
    gameFps?: number;
    rttMs?: number;
    jitterMs?: number;
    lost?: number;
}): string;
export declare class PhaseLock {
    private send;
    private waits;
    private floor;
    private lastDecision;
    private lastShift;
    private settled;
    private gaveUp;
    private period;
    private lastDisplay;
    shifted: number;
    margin: number;
    constructor(send: (ms: number) => void, margin?: number);
    shown(p: Presented): void;
    private decide;
    private move;
}
export declare class SteadinessWatch {
    private say;
    private gaps;
    private last;
    private uneven;
    private since;
    constructor(say: (uneven: boolean) => void);
    shown(p: Presented): void;
}
export declare function watchVideo(v: HTMLVideoElement, onFrame: (p: Presented, meta: any) => void): () => void;
export declare const STAMP_BITS = 32;
export declare class StampReader {
    private c;
    private w;
    private h;
    constructor();
    colours(source: HTMLVideoElement | HTMLCanvasElement, srcW: number, srcH: number): {
        error: number;
        seen: number[][];
    } | null;
    read(source: HTMLVideoElement | HTMLCanvasElement, srcW: number, srcH: number): {
        seq: number;
        input: number;
    } | null;
}
