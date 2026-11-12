import type { ControlsData } from './consoles';
import type { ControllerData } from './mapping';
import type { Hello, StreamInfo } from './transport';
import type { Explained } from './words';
export type EmulatorBackend = 'docker+webrtc' | 'docker+kasmvnc' | 'local+webrtc' | 'host+webrtc';
export type EmulatorSession = {
    id: string;
    emulator: string;
    name: string;
    backend: EmulatorBackend | string;
    connect?: string;
    kasmUrl?: string;
    stop: string;
    pause?: string;
    resume?: string;
    width: number;
    height: number;
    fps: number;
    players: number;
    notices?: string[];
};
export type Connected = {
    stream: StreamInfo;
    sdp: string;
};
export interface EmulatorHost {
    connect(hello: Hello, offer: string): Promise<Connected>;
    stop(): Promise<void>;
    pause(): Promise<void>;
    resume(): Promise<void>;
    controls?(device: string): Promise<ControlsData>;
    controllers?(): Promise<ControllerData>;
    mediaUrl?(path: string): string;
    report?(level: 'error' | 'warn', message: string): void;
}
export type Showing = 'stream' | 'desktop' | 'none';
export declare function showingOf(s: Pick<EmulatorSession, 'backend' | 'connect' | 'kasmUrl'>): Showing;
export declare const canPause: (s: Pick<EmulatorSession, 'pause' | 'resume'>) => boolean;
export declare const playersOf: (s: Pick<EmulatorSession, 'players'>) => number;
export declare function isGone(e: unknown): boolean;
export type Phase = 'connecting' | 'playing' | 'reconnecting' | 'failed' | 'ended';
export type StreamState = {
    phase: Phase;
    attempt: number;
    retried: boolean;
    message: string;
};
export type StreamEvent = {
    type: 'connected';
} | {
    type: 'lost';
} | {
    type: 'error';
    gone: boolean;
    message: string;
} | {
    type: 'retry';
} | {
    type: 'ended';
    message?: string;
};
export declare const firstState: StreamState;
export declare function nextState(s: StreamState, e: StreamEvent): StreamState;
export declare function problemOf(phase: Phase | 'none', message: string, name: string): Explained | null;
export type Box = {
    left: number;
    top: number;
    width: number;
    height: number;
};
export declare function pictureBox(el: Box, picW: number, picH: number): Box;
export declare function pointOnPicture(el: Box, picW: number, picH: number, clientX: number, clientY: number): {
    x: number;
    y: number;
    inside: boolean;
};
export declare function desktopSrc(kasmUrl: string | undefined, mediaUrl?: (path: string) => string): string;
export declare function usualControls(): ControlsData;
