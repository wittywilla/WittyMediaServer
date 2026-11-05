import type { ReactNode } from 'react';
declare const P: Record<string, string>;
export declare const MenuIcon: ({ name }: {
    name: keyof typeof P | string;
}) => import("react").JSX.Element;
export type TileProps = {
    icon: string;
    label: string;
    sub?: string;
    onClick?: () => void;
    primary?: boolean;
    danger?: boolean;
    pressed?: boolean;
    disabled?: boolean;
    id: string;
    wide?: boolean;
};
export declare function Tile({ icon, label, sub, onClick, primary, danger, pressed, disabled, id, wide }: TileProps): import("react").JSX.Element;
export declare function MenuFrame({ title, sub, cover, children, foot }: {
    title: string;
    sub?: string;
    cover?: string;
    children: ReactNode;
    foot?: ReactNode;
}): import("react").JSX.Element;
export declare const TileGrid: ({ label, children }: {
    label: string;
    children: ReactNode;
}) => import("react").JSX.Element;
export {};
