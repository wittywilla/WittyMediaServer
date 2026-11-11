// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
export declare class ArrivalDelay {
    private best;
    private bestAt;
    private worst;
    note(sentMs: number, arrivedMs: number): void;
    take(): number;
}
