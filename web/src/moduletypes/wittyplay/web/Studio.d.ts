import type { Background, ControlsData } from './consoles';
import { type InputManager } from './input';
import { type ControllerData, type Profile } from './mapping';
export type ImportResult = {
    imported: Profile[];
    rejected: {
        index: number;
        name: string;
        error: string;
    }[];
    notes: string[];
};
export interface ProfileHost {
    list(): Promise<Profile[]>;
    create(p: Partial<Profile>): Promise<Profile>;
    update(id: number, p: Profile): Promise<Profile>;
    remove(id: number): Promise<void>;
    copy(id: number): Promise<Profile>;
    setActive(id: number, on: boolean): Promise<Profile>;
    exportFile(ids?: number[]): Promise<string>;
    importFile(text: string): Promise<ImportResult>;
    setBackground(b: Background): Promise<Background>;
    uploadBackground(f: File): Promise<Background>;
    removeBackground(): Promise<Background>;
    backgroundUrl(version: number): string;
}
export type PortChoice = {
    port: number;
    types: {
        id: number;
        name: string;
    }[];
    current: number;
};
type Props = {
    host: ProfileHost;
    data: ControlsData;
    known: ControllerData | null;
    input: InputManager;
    consoleName: string;
    gameId?: number;
    gameTitle?: string;
    consolePicker?: React.ReactNode;
    devices?: PortChoice[];
    setDevice?: (port: number, id: number) => Promise<void>;
    refresh: () => Promise<void>;
    onTouch?: () => void;
    onClose: () => void;
};
export declare function ControllerStudio(props: Props): import("react").JSX.Element;
export {};
