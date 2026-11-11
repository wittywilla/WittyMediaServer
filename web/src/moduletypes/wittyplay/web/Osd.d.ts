// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import type { SessionInfo } from './Player';
import type { SettingsHost } from './settings';
import type { Transport } from './transport';
export declare function Osd({ host, session, transport, values, stampInput }: {
    host: SettingsHost;
    session: SessionInfo | null;
    transport: Transport | null;
    values: Record<string, unknown>;
    stampInput?: (next: (() => number) | null) => void;
}): import("react").JSX.Element;
