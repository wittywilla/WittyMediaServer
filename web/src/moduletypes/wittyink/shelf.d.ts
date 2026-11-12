import './shelf.css';
export * from './shelfsort';
/** What the app gives a module's page or view. */
export interface PageHost {
    request: <T = unknown>(method: string, path: string, body?: unknown) => Promise<T>;
    user: {
        admin: boolean;
    };
    store?: {
        get: (key: string) => Promise<unknown>;
        set: (key: string, value: unknown) => Promise<void>;
    };
    token: () => string;
    base: () => string;
}
/** The view wittyink.bookshelf. `library` is the NativeView's library property. */
export declare function Bookshelf({ host, library }: {
    host: PageHost;
    library?: string | number;
}): import("react").JSX.Element;
/** The views WittyInk's bundle draws, by view ID (docs/SDUI-MODULES.md, "Views"). */
export declare const views: {
    'wittyink.bookshelf': typeof Bookshelf;
};
