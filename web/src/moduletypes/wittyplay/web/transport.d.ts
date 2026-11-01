export type TransportKind = 'webrtc' | 'ws-h264' | 'ws-jpeg';
export type Mode = 'auto' | TransportKind;
export declare const TRANSPORT_LABELS: Record<TransportKind, string>;
export type Stats = {
    rttMs?: number;
    fps?: number;
    kbps?: number;
    jitterMs?: number;
    lost?: number;
    dropped?: number;
    codec?: string;
    decodeMs?: number;
};
export type AudioBuffer_ = 'lowest' | 'balanced' | 'smooth';
export type PlayEvent = {
    type: string;
    port?: number;
    strength?: number;
    text?: string;
    effect?: number;
    aspect?: number;
    width?: number;
    height?: number;
    turns?: number;
    index?: number;
    count?: number;
};
export interface Transport {
    readonly kind: TransportKind;
    readonly element: HTMLVideoElement | HTMLCanvasElement;
    connect(): Promise<void>;
    send(msg: Uint8Array): void;
    close(): void;
    stats(): Promise<Stats>;
    needsUnmute(): boolean;
    unmute(): void;
    setVolume(volume: number, muted: boolean): void;
    setAudioBuffer(b: AudioBuffer_): void;
    captureStream(): MediaStream | null;
    onEvent?: (e: PlayEvent) => void;
    onLost?: () => void;
}
export type Signal = {
    answer(sdp: string, prefer?: string): Promise<string>;
    wsUrl(video: string): string;
};
export declare function plan(mode: Mode, allowed: string[]): TransportKind[];
export declare function make(kind: TransportKind, signal: Signal): Transport;
