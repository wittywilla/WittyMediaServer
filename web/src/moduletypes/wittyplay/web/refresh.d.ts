export declare function measureRefresh(done: (hz: number) => void): void;
export declare function rateOf(gaps: number[]): number;
export declare function screenNow(): {
    hz?: number;
    width?: number;
    height?: number;
};
