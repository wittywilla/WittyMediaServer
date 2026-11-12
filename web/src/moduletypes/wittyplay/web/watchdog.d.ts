export declare const STALL_MS = 1500;
type Stats = {
    getStats(): Promise<{
        forEach(fn: (s: any) => void): void;
    }>;
    readonly connectionState: string;
};
export declare function countFrames(v: any): {
    count: () => number;
    stop: () => void;
};
export declare class Watchdog {
    private timer;
    private leaving;
    private lastTick;
    private shown;
    private shownAt;
    private bytes;
    private bytesAt;
    private decoded;
    private decodedAt;
    private checking;
    private stalled;
    private done;
    private pc;
    private count;
    private lost;
    private onStall;
    private ownDecoded?;
    constructor(pc: Stats, count: () => number, lost: (why: string) => void, onStall: (stalled: boolean) => void, ownDecoded?: () => number);
    start(): void;
    stop(): void;
    state(st: string): void;
    private disconnected;
    private setStalled;
    tick(): void;
}
export {};
