export type HistoryLike = {
    readonly state: unknown;
    pushState(data: unknown, unused: string): void;
    back(): void;
};
export declare class BackEntry {
    private history;
    private onBack;
    private wait;
    private pushed;
    private skips;
    private then;
    private timer;
    constructor(history: HistoryLike, onBack: () => boolean, wait?: number);
    opened(): void;
    closed(): void;
    popped(): void;
    release(then: () => void): void;
    get open(): boolean;
    private finish;
}
