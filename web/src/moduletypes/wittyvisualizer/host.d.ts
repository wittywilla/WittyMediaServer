// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
export type VisualizerHost = {
    loadSettings(): Promise<unknown>;
    saveSettings(settings: unknown): void;
};
