// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
export type BookMeta = {
    title: string;
    author?: string;
    year?: number | null;
};
export type Ann = {
    id: number;
    kind: 'bookmark' | 'highlight' | 'ink';
    chapter: number;
    pos: number;
    quote: string;
    prefix: string;
    suffix: string;
    color: string;
    note: string;
    data?: string;
    created: number;
};
export type EpubInfo = {
    spine: string[];
    toc: {
        title: string;
        href: string;
    }[];
    words: number[];
};
export type SearchHit = {
    chapter: number;
    snippet: string;
    quote: string;
    prefix: string;
};
export type Definition = {
    word?: string;
    phonetic?: string;
    meanings?: {
        partOfSpeech: string;
        definitions: {
            definition: string;
            example?: string;
        }[];
    }[];
};
export interface ReaderHost {
    key: string;
    book: BookMeta;
    format: 'epub' | 'pdf' | 'cbz';
    reading?: string;
    start: number;
    saveProgress(position: number, total: number): void;
    annotations: {
        list(): Promise<Ann[]>;
        add(a: Partial<Ann>): Promise<Ann>;
        update(a: Ann, patch: Partial<Ann>): Promise<void>;
        remove(a: Ann): Promise<void>;
    };
    pdf?: {
        url?: string;
        data?: Uint8Array;
        wasmUrl?: string;
        iccUrl?: string;
    };
    comic?: {
        pages: number;
        pageUrl(n: number): string;
    };
    epub?: {
        info: EpubInfo;
        chapterUrl(path: string): string;
        search(q: string): Promise<SearchHit[]>;
    };
    define?(word: string): Promise<Definition[]>;
    toast(message: string): void;
    close(): void;
    report?(level: 'error' | 'warn', message: string, detail?: {
        stack?: string;
        page?: string;
    }): void;
    settings?: {
        get(module: string): Record<string, unknown>;
        save(module: string, values: Record<string, unknown>): void;
    };
}
export declare const HostContext: import("react").Context<ReaderHost>;
export declare function useHost(): ReaderHost;
