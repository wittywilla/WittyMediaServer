export declare const FORMAT_VERSION = 1;
export declare const MAX_USER_BYTES: number;
export type EffectInfo = {
    id: string;
    name: string;
    description: string;
    author?: string;
    license?: string;
    tags: string[];
    cost: 1 | 2 | 3;
    motion: 'gentle' | 'lively';
    feedback: boolean;
    quads: number;
};
export type EffectFile = EffectInfo & {
    source: string;
    builtin: boolean;
};
export type Parsed = {
    ok: true;
    effect: EffectFile;
} | {
    ok: false;
    error: string;
};
export declare function parseEffect(text: string, builtin: boolean): Parsed;
export declare function validateUserShader(source: string): string | null;
