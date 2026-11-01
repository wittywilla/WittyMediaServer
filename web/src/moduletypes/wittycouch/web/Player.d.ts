import './player.css';
export type StreamRequest = {
    width: number;
    height: number;
    fps: number;
    bitrate: number;
};
export type StreamInfo = {
    id: string;
    title: string;
    width?: number;
    height?: number;
    fps?: number;
};
export interface CouchHost {
    title: string;
    start(req: StreamRequest): Promise<StreamInfo>;
    answer(sid: string, sdp: string): Promise<string>;
    end(sid: string, quit: boolean): Promise<void>;
    close(): void;
}
export default function CouchPlayer({ host }: {
    host: CouchHost;
}): import("react").JSX.Element;
