import './filebrowser.css';
export type Entry = {
    name: string;
    path: string;
    dir: boolean;
    kind?: string;
    disc?: 'dvd' | 'bluray';
    size?: number;
    modified?: number;
    hidden?: boolean;
};
export type Place = {
    name: string;
    path: string;
    group: string;
    label?: string;
    free?: number;
    total?: number;
};
export type Listing = {
    path: string;
    parent: string | null;
    sep: string;
    entries?: Entry[];
    places?: Place[];
    counts?: Record<string, number>;
    truncated?: boolean;
};
export type FileBrowserProps = {
    list: (path: string, hidden: boolean) => Promise<Listing>;
    onPick: (paths: string[]) => void;
    onClose: () => void;
    multiple?: boolean;
    title?: string;
    where?: string;
    groupNames?: Record<string, string>;
};
export default function FileBrowser({ list: listFolder, onPick, onClose, multiple, title, where, groupNames }: FileBrowserProps): import("react").ReactPortal;
