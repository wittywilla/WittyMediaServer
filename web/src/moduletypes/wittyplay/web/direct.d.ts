export declare const hasDirect: () => boolean;
export type DirectFrame = {
    rtp: number;
    received: number;
    drawn: number;
};
export declare class DirectVideo {
    private signal;
    readonly canvas: HTMLCanvasElement;
    onFrame?: (f: DirectFrame) => void;
    framesDrawn: number;
    private ctx;
    private decoder;
    private needKey;
    private closed;
    private receivedAt;
    private lastAsk;
    constructor(signal: (b: ArrayBuffer) => void);
    start(): Promise<void>;
    private restart;
    tellServer(): void;
    private askForKey;
    pump(readable: ReadableStream<any>): Promise<void>;
    private draw;
    close(): void;
}
