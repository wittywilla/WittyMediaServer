// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
export declare const FFT_SIZE = 2048;
export declare const SPECTRUM_BINS = 512;
export declare const WAVE_POINTS = 512;
export declare const HISTORY_ROWS = 64;
export declare const HISTORY_RATE = 64;
export declare const AUDIO_ROWS: number;
export declare const MIN_BEAT_GAP = 0.24;
export type AudioFrame = {
    texture: Uint8Array;
    historyRow: number;
    historyMoved: number;
    bass: number;
    mid: number;
    treble: number;
    level: number;
    beat: number;
    beatCount: number;
    beatAges: [number, number, number, number];
    isSilent: boolean;
};
export declare class Fft {
    readonly n: number;
    private cos;
    private sin;
    private rev;
    constructor(n: number);
    run(re: Float64Array, im: Float64Array): void;
}
export declare class Analyzer {
    private fft;
    private window;
    private re;
    private im;
    private mono;
    private mags;
    private smooth;
    private peaks;
    private bandBass;
    private bandMid;
    private bandTreble;
    private bandLevel;
    private lows;
    private lowsAt;
    private lowsHead;
    private lastLow;
    private lastLowInput;
    private clock;
    private lastBeat;
    private specPeak;
    private binLo;
    private binHi;
    private rate;
    private historyDue;
    private beats;
    readonly frame: AudioFrame;
    sensitivity: number;
    minBeatGap: number;
    constructor();
    private setRate;
    process(left: Float32Array, right: Float32Array, rate: number, dtSec: number): void;
    private bandPower;
    private bands;
    private spectrumRow;
    private waves;
    private detectBeat;
}
