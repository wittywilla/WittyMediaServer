export type PadFamily = 'xbox' | 'playstation' | 'nintendo' | 'generic';
export declare function familyOf(id: string): PadFamily;
export type ProblemData = {
    needCore?: boolean;
    otherCore?: boolean;
    needBios?: boolean;
    encrypted?: boolean;
    file?: string;
    rig?: boolean;
    connect?: boolean;
    gone?: boolean;
    status?: number;
};
export type Explained = {
    title: string;
    body: string;
    fix?: 'settings';
    retry?: boolean;
    defaults?: boolean;
};
export declare function explain(message: string, d: ProblemData | null | undefined, isAdmin: boolean, consoleName: string): Explained;
