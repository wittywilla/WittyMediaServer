/** A book as GET /api/items lists it: only what the shelf uses. */
export interface ShelfBook {
    id: number;
    title: string;
    sortTitle?: string;
    year?: number | null;
    artist?: string;
    album?: string;
    track?: number | null;
    reader?: string;
    container?: string;
    reading?: string;
    poster?: string;
    updatedAt?: number;
    addedAt?: number;
    runtimeSec?: number;
    progress?: {
        positionSec: number;
        durationSec?: number;
        watched?: boolean;
    } | null;
}
export type ShelfTab = 'books' | 'series' | 'authors';
export type ShelfSort = 'title' | 'added' | 'year' | 'author';
export type ShelfFormat = 'all' | 'epub' | 'pdf' | 'cbz';
export type ShelfStatus = 'all' | 'reading' | 'finished';
export interface ShelfState {
    tab: ShelfTab;
    sort: ShelfSort;
    desc: boolean;
    format: ShelfFormat;
    status?: ShelfStatus;
    q: string;
    group: string | null;
}
export declare const STATUSES: {
    id: ShelfStatus;
    label: string;
}[];
export declare const FORMATS: {
    id: ShelfFormat;
    label: string;
}[];
export declare const SORTS: {
    id: ShelfSort;
    label: string;
}[];
/** The group of books with no series, or no author. */
export declare const NONE = "\0";
/** How far through a book is, 0 to 100. */
export declare function percentRead(b: ShelfBook): number;
/** The books a choice shows, in its order. Pure, so it is tested on its own. */
export declare function shelfBooks(all: ShelfBook[], s: Pick<ShelfState, 'sort' | 'desc' | 'format' | 'status' | 'q' | 'tab' | 'group'>): ShelfBook[];
export interface ShelfGroup {
    name: string;
    books: ShelfBook[];
}
/** Series or authors, each with its books; books with neither come last under NONE. */
export declare function shelfGroups(books: ShelfBook[], tab: 'series' | 'authors'): ShelfGroup[];
