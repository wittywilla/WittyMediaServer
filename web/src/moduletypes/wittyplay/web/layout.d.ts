export type LayoutInput = {
    areaW: number;
    areaH: number;
    srcW: number;
    srcH: number;
    aspect: number;
    mode: string;
    custom: number;
    integer: boolean;
    cropX: number;
    cropY: number;
    turns: number;
    inset: number;
};
export type Layout = {
    boxW: number;
    boxH: number;
    picW: number;
    picH: number;
    scaleX: number;
    scaleY: number;
    scale: number;
};
export declare function computeLayout(i: LayoutInput): Layout;
