export type Choice = {
    value: string;
    label: string;
};
export type SettingDef = {
    key: string;
    group: string;
    section?: string;
    label: string;
    help?: string;
    caution?: string;
    kind: 'toggle' | 'choice' | 'slider' | 'color' | 'bezel';
    default: unknown;
    min?: number;
    max?: number;
    step?: number;
    unit?: string;
    choices?: Choice[];
    shader?: boolean;
    motion?: boolean;
};
export type Preset = {
    name: string;
    description?: string;
    settings?: Record<string, unknown>;
    coreOptions?: Record<string, string>;
};
export type Schema = {
    settings: SettingDef[];
    presets: Preset[];
    levels: string[];
    maxSlot: number;
    undoSlot: number;
    prevAutoSlot: number;
};
export type CustomControl = {
    label: string;
    help?: string;
    type: 'toggle' | 'choice' | 'slider';
    option: string;
    on?: string;
    off?: string;
    choices?: Choice[];
};
export type Layer = {
    settings?: Record<string, unknown>;
    coreOptions?: Record<string, string>;
    controls?: CustomControl[];
    presets?: Preset[];
};
export type Resolved = {
    values: Record<string, unknown>;
    sources: Record<string, string>;
    coreOptions: Record<string, string>;
    coreSources: Record<string, string>;
    controls: CustomControl[];
    presets: Preset[];
};
export type Limits = {
    maxSpeed: number;
    rewindBytes: number;
    runAhead: boolean;
};
export type Issue = {
    line?: number;
    column?: number;
    path?: string;
    message: string;
};
export type Report = {
    errors: Issue[];
    ignored: Issue[];
};
export type SettingsView = {
    resolved: Resolved;
    layers: Record<string, Layer>;
    limits: Limits;
    console: string;
    item: number;
    ignored?: Issue[];
};
export type Level = 'person' | 'console' | 'game' | 'server' | 'serverConsole';
export type OptionInfo = {
    key: string;
    label: string;
    info?: string;
    category?: string;
    choices: string[];
    labels?: string[];
    default: string;
    hidden?: boolean;
};
export type CoreOptionsView = {
    core: string;
    known: boolean;
    options: OptionInfo[];
    values: Record<string, string>;
    sources: Record<string, string>;
    running: Record<string, string>;
    ignored: Issue[];
    controls: CustomControl[];
};
export interface SettingsHost {
    request<T>(method: string, path: string, body?: unknown): Promise<T>;
    upload<T>(path: string, body: Blob): Promise<T>;
    mediaUrl(path: string): string;
    isAdmin: boolean;
}
export declare const LEVEL_RANK: Record<string, number>;
export declare function levelName(level: string, consoleName?: string): string;
export declare function loadSchema(host: SettingsHost): Promise<Schema>;
export type Scope = {
    console?: string;
    item?: number;
};
export declare function useSettings(host: SettingsHost, scope: Scope, onError: (m: string) => void): {
    view: SettingsView;
    set: (level: Level, key: string, value: unknown) => void;
    setOption: (level: Level, key: string, value: string | null) => void;
    applyPreset: (level: Level, p: Preset) => Promise<SettingsView>;
    reset: (level: Level) => Promise<void>;
    saveDocument: (level: Level, document: string) => Promise<SettingsView>;
    reload: () => Promise<SettingsView>;
    flush: () => Promise<void>;
};
export type SettingsApi = ReturnType<typeof useSettings>;
export declare function layerDocument(l: Layer | undefined): string;
export declare function downloadBlob(blob: Blob, name: string): void;
export declare function pickFile(accept: string): Promise<File | null>;
export declare const fileName: (t: string) => string;
