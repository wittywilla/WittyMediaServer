import { type ControlsData, type SaveControl } from './consoles';
import type { ControllerData } from './mapping';
import { type SettingsHost } from './settings';
import { type PortChoice, type ProfileHost } from './Studio';
import { type Hello, type StreamInfo } from './transport';
import './play.css';
export type SessionInfo = {
    id: string;
    itemId: number;
    title: string;
    console: string;
    core: string;
    width: number;
    height: number;
    aspect: number;
    fps: number;
    sampleRate: number;
    resumed: boolean;
    encoder?: string;
    renderer?: string;
    turns?: number;
    disc?: number;
    discs?: number;
    modes: {
        id: string;
        stream?: number;
    }[];
    devices?: PortChoice[];
    safeMode?: boolean;
    maxSpeed?: number;
    notices?: string[];
};
export type SaveSlot = {
    slot: number;
    updatedAt: number;
    size: number;
    hasThumb: boolean;
    core?: string;
};
export type PrepareStatus = {
    state: 'ready' | 'waiting' | 'reading' | 'unpacking';
    done: number;
    total: number;
};
export interface PlayHost extends SettingsHost {
    itemId: number;
    title: string;
    consoleId: string;
    consoleName: string;
    resume: boolean;
    safeMode: boolean;
    startSlot?: number;
    cover?: string;
    openSettings?(): void;
    prepare?(): Promise<PrepareStatus>;
    start(resume: boolean, safeMode: boolean): Promise<SessionInfo>;
    relaunch(opts: {
        resume: boolean;
        safeMode: boolean;
    }): void;
    stop(sessionId: string): Promise<void>;
    answer(sessionId: string, sdp: string, prefer?: string): Promise<string>;
    wsUrl(sessionId: string, video: string): string;
    stream?(sessionId: string, hello: Hello, sdp: string): Promise<{
        stream: StreamInfo;
        sdp: string;
    }>;
    streamUrl?(sessionId: string): string;
    action(sessionId: string, type: string, slot?: number, on?: boolean, extra?: {
        port?: number;
        device?: number;
        value?: number;
        key?: string;
        text?: string;
    }): Promise<any>;
    controls(device: string): Promise<ControlsData>;
    saveControl(s: SaveControl): Promise<void>;
    controllers(): Promise<ControllerData>;
    profiles: ProfileHost;
    close(): void;
    report?(level: 'error' | 'warn', message: string): void;
}
export default function GamePlayer({ host }: {
    host: PlayHost;
}): import("react").JSX.Element;
export declare function ControllersPage({ profiles, controls, controllers, consoles, consoleId, onConsole, saveControl, onClose }: {
    profiles: ProfileHost;
    controls: (consoleId: string, device: string) => Promise<ControlsData>;
    controllers: () => Promise<ControllerData>;
    consoles: {
        id: string;
        short: string;
        name: string;
    }[];
    consoleId: string;
    onConsole: (id: string) => void;
    saveControl: (consoleId: string, s: SaveControl) => Promise<void>;
    onClose: () => void;
}): import("react").JSX.Element;
