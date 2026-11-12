export type VisualizerHost = {
    loadSettings(): Promise<unknown>;
    saveSettings(settings: unknown): void;
};
