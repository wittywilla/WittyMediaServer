import { type SettingsHost } from './settings';
import './play.css';
export declare function GameSettingsScreen({ host, consoles, itemId, itemTitle, consoleId, onError }: {
    host: SettingsHost;
    consoles: {
        id: string;
        name: string;
        short: string;
    }[];
    itemId?: number;
    itemTitle?: string;
    consoleId?: string;
    onError: (m: string) => void;
}): import("react").JSX.Element;
