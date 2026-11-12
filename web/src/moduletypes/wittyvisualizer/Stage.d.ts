import type { VisualizerHost } from './host.ts';
import { Session, type Place } from './session.ts';
import './visualizer.css';
export declare const currentSession: () => Session;
export type StageProps = {
    host: VisualizerHost;
    owner?: string | number;
    media: () => HTMLMediaElement | null;
    place: Place;
    onPlace?: (p: Place) => void;
    onClose: () => void;
};
export declare function VisualizerStage({ host, owner, media, place, onPlace, onClose }: StageProps): import("react").JSX.Element;
