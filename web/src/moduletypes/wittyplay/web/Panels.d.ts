import { type Choice, type Level, type Preset, type Schema, type SettingsApi, type SettingsHost, type SettingsView } from './settings';
export type LevelChoice = {
    level: Level;
    label: string;
};
export declare function Cycler({ label, value, options, onChange, disabled }: {
    label: string;
    value: string;
    options: Choice[];
    onChange: (v: string) => void;
    disabled?: boolean;
}): import("react").JSX.Element;
export declare function Slider({ label, value, min, max, step, unit, onChange }: {
    label: string;
    value: number;
    min: number;
    max: number;
    step: number;
    unit?: string;
    onChange: (v: number) => void;
}): import("react").JSX.Element;
export declare function Back({ onClick, label }: {
    onClick: () => void;
    label?: string;
}): import("react").JSX.Element;
export declare function ConsoleChips({ consoles, chosen, onChoose, first, label }: {
    consoles: {
        id: string;
        short: string;
        name?: string;
    }[];
    chosen: string;
    onChoose: (id: string) => void;
    first?: string;
    label?: string;
}): import("react").JSX.Element;
export declare function LevelPicker({ levels, level, onChange }: {
    levels: LevelChoice[];
    level: Level;
    onChange: (l: Level) => void;
}): import("react").JSX.Element;
export declare function SettingsPanel({ schema, api, view, groups, level, levels, onLevel, consoleName, host, say, presets }: {
    schema: Schema;
    api: SettingsApi;
    view: SettingsView;
    groups: string[];
    level: Level;
    levels: LevelChoice[];
    onLevel: (l: Level) => void;
    consoleName: string;
    host: SettingsHost;
    say: (t: string) => void;
    presets?: Preset[];
}): import("react").JSX.Element;
export declare function CorePanel({ host, itemId, api, view, level, levels, onLevel, consoleName, say, onJson }: {
    host: SettingsHost;
    itemId: number;
    api: SettingsApi;
    view: SettingsView;
    level: Level;
    levels: LevelChoice[];
    onLevel: (l: Level) => void;
    consoleName: string;
    say: (t: string) => void;
    onJson: () => void;
}): import("react").JSX.Element;
export declare function JsonEditor({ host, api, view, scope, level, levels, onLevel, consoleName, title, say, itemId, onDirty }: {
    host: SettingsHost;
    api: SettingsApi;
    view: SettingsView;
    scope: {
        console?: string;
        item?: number;
    };
    level: Level;
    levels: LevelChoice[];
    onLevel: (l: Level) => void;
    consoleName: string;
    title: string;
    say: (t: string) => void;
    itemId?: number;
    onDirty?: (dirty: boolean) => void;
}): import("react").JSX.Element;
export declare function CheatsPanel({ host, itemId, title, say }: {
    host: SettingsHost;
    itemId: number;
    title: string;
    say: (t: string) => void;
}): import("react").JSX.Element;
export declare function PatchesPanel({ host, itemId, say, onRestart }: {
    host: SettingsHost;
    itemId: number;
    say: (t: string) => void;
    onRestart?: () => void;
}): import("react").JSX.Element;
export type SaveSlot = {
    slot: number;
    updatedAt: number;
    size: number;
    hasThumb: boolean;
    core?: string;
};
export declare const slotName: (n: number, schema?: Schema | null) => string;
export declare function SlotsPanel({ host, schema, itemId, kind, confirm, act, say, onDone, core }: {
    host: SettingsHost;
    schema: Schema | null;
    itemId: number;
    kind: 'save' | 'load';
    confirm: boolean;
    act: (n: number) => Promise<void>;
    say: (t: string) => void;
    onDone: (closed: boolean) => void;
    core?: string;
}): import("react").JSX.Element;
