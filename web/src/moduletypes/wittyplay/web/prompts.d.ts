import { type PadFamily } from './words';
import './prompts.css';
export type { PadFamily };
export type InputKind = 'pad' | 'keyboard' | 'remote' | 'pointer';
export type PromptButton = 'confirm' | 'back' | 'west' | 'north' | 'lb' | 'rb' | 'lt' | 'rt' | 'start' | 'select' | 'dpad' | 'stick';
type State = {
    kind: InputKind;
    family: PadFamily;
};
export declare function useInputKind(): State;
export declare function PadGlyph({ button, family }: {
    button: PromptButton;
    family: PadFamily;
}): import("react").JSX.Element;
export type Prompt = {
    button: PromptButton;
    label: string;
    key?: string;
};
export declare function PromptItem({ p, input }: {
    p: Prompt;
    input: State;
}): import("react").JSX.Element;
export declare function PromptBar({ items, always, className }: {
    items: Prompt[];
    always?: boolean;
    className?: string;
}): import("react").JSX.Element;
