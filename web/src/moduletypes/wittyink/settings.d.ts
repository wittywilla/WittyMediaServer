/** One of the choices, or the fallback. */
export declare function settingEnum<T extends string>(v: unknown, allowed: readonly T[], fallback: T): T;
/** A whole number within its range, or the fallback. */
export declare function settingInt(v: unknown, lo: number, hi: number, fallback: number): number;
export declare function settingBool(v: unknown, fallback: boolean): boolean;
/** A saver that sends the last values once nothing has changed for `wait` ms. */
export declare function laterSaver(save: ((values: Record<string, unknown>) => void) | undefined, wait?: number): (values: Record<string, unknown>) => void;
