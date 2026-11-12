import type { Explained } from './words';
export type LoadStage = 'prepare' | 'start' | 'connect' | 'picture';
export declare function LoadingScreen({ title, consoleName, cover, stage, status, percent, onCancel }: {
    title: string;
    consoleName: string;
    cover?: string;
    stage: LoadStage;
    status: string;
    percent?: number;
    onCancel: () => void;
}): import("react").JSX.Element;
export declare function ProblemScreen({ title, cover, problem, onBack, onRetry, onDefaults, onSettings }: {
    title: string;
    cover?: string;
    problem: Explained;
    onBack: () => void;
    onRetry?: () => void;
    onDefaults?: () => void;
    onSettings?: () => void;
}): import("react").JSX.Element;
