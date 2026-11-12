import { type ControlsData, type Layout, type Orientation } from './consoles';
export declare function useOrientation(): Orientation;
export type OnPoint = (clientX: number, clientY: number, phase: 'down' | 'move' | 'up') => void;
export declare function TouchControls({ data, orientation, onMask, onStick, onPoint }: {
    data: ControlsData;
    orientation: Orientation;
    onMask: (mask: number) => void;
    onStick: (x: number, y: number) => void;
    onPoint?: OnPoint;
}): import("react").JSX.Element;
export declare function TouchEditor({ data, orientation, onSave, onClose }: {
    data: ControlsData;
    orientation: Orientation;
    onSave: (l: Layout | null) => Promise<void>;
    onClose: () => void;
}): import("react").JSX.Element;
