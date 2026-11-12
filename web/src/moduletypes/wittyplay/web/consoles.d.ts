import type { Binding, KeyMap, PadMap, Profile, ProfileOptions } from './mapping';
export declare const SELECT = 2, START = 3, UP = 4, DOWN = 5, LEFT = 6, RIGHT = 7;
export type Control = {
    key: string;
    id: number;
    label: string;
    kind: 'dpad' | 'stick' | 'face' | 'shoulder' | 'meta';
    stick?: boolean;
};
export type Target = {
    id: number;
    label: string;
    group: 'dpad' | 'face' | 'shoulder' | 'meta' | 'stick';
};
export type PadBinding = Binding;
export type { KeyMap, PadMap };
export type Placement = {
    x: number;
    y: number;
    size: number;
    opacity: number;
    hidden?: boolean;
};
export type Layout = Record<string, Placement>;
export type Orientation = 'landscape' | 'portrait';
export type Background = {
    version: number;
    fit: 'cover' | 'contain';
    x: number;
    y: number;
    dim: number;
};
export type PadShape = {
    art: string;
    sticks?: boolean;
    stickFirst?: boolean;
    pointer?: boolean;
};
export type ControlsData = {
    console: string;
    pad: PadShape;
    controls: Control[];
    targets: Target[];
    defaults: {
        gamepad: PadMap;
        keyboard: KeyMap;
        remote: KeyMap;
        touch: Record<Orientation, Layout>;
        options: ProfileOptions;
    };
    saved: {
        gamepad: Record<string, PadMap>;
        keyboard: KeyMap | null;
        touch: Partial<Record<Orientation, Layout>>;
    };
    game: number;
    profiles: Profile[];
    background: Background;
};
export type SaveControl = {
    kind: 'gamepad' | 'keyboard' | 'touch';
    key?: string;
    orientation?: Orientation;
    data: PadMap | KeyMap | Layout | null;
};
export declare const padKey: (id: string) => string;
