import { type EmulatorHost, type EmulatorSession } from './emulator';
import './play.css';
import './emulator.css';
type Props = {
    host: EmulatorHost;
    session: EmulatorSession;
    title: string;
    onExit: () => void;
};
export declare function EmulatorPlayer({ host, session, title, onExit }: Props): import("react").JSX.Element;
export {};
