import type { EffectFile } from './format.ts';
export type DrawInput = {
    audio: {
        texture: Uint8Array;
        historyRow: number;
        historyMoved: number;
        bass: number;
        mid: number;
        treble: number;
        level: number;
        beat: number;
        beatCount: number;
        beatAges: number[];
    };
    time: number;
    flow: number;
    dt: number;
    intensity: number;
    calm: boolean;
    palette: {
        a: number[];
        b: number[];
        c: number[];
        d: number[];
    };
    guard: {
        deadBand: number;
        rise: number;
    };
    a: string;
    b: string | null;
    mix: number;
    mixStyle: number;
};
export interface Renderer {
    readonly kind: 'webgl2' | 'canvas';
    readonly software: boolean;
    readonly label: string;
    prepare(e: EffectFile): {
        ok: true;
    } | {
        ok: false;
        error: string;
    } | null;
    supports(id: string): boolean;
    setSize(w: number, h: number): void;
    draw(f: DrawInput): void;
    release(id: string): void;
    dispose(): void;
}
export declare class GlRenderer implements Renderer {
    readonly kind: "webgl2";
    readonly software: boolean;
    readonly label: string;
    private gl;
    private parallel;
    private vao;
    private ubo;
    private audio;
    private uniforms;
    private guardProg;
    private presentProg;
    private quadFragShader;
    private fullVert;
    private guardTex;
    private guardFbo;
    private guardAt;
    private effects;
    private pending;
    private targets;
    private w;
    private h;
    private black;
    constructor(gl: WebGL2RenderingContext);
    private filters;
    private shader;
    private link;
    private bindNames;
    supports(): boolean;
    prepare(e: EffectFile): {
        ok: true;
    } | {
        ok: false;
        error: string;
    } | null;
    setSize(w: number, h: number): void;
    private target;
    private freeTarget;
    private sendUniforms;
    private drawEffect;
    private texOf;
    draw(f: DrawInput): void;
    release(id: string): void;
    dispose(): void;
}
export declare function trialCompile(e: EffectFile): Promise<string | null>;
export declare function createGl(canvas: HTMLCanvasElement): WebGL2RenderingContext;
