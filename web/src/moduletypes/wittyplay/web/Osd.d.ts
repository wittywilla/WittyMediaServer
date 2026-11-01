import type { SessionInfo } from './Player';
import type { SettingsHost } from './settings';
import type { Transport } from './transport';
export declare function Osd({ host, session, transport, values }: {
    host: SettingsHost;
    session: SessionInfo | null;
    transport: Transport | null;
    values: Record<string, unknown>;
}): import("react").JSX.Element;
