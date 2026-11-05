export declare class ArrivalDelay {
    private best;
    private bestAt;
    private worst;
    note(sentMs: number, arrivedMs: number): void;
    take(): number;
}
