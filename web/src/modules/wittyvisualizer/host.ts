// What WittyVisualizer asks of the app around it. The server's web app keeps these per
// person (web/src/audio/visualizer.tsx); any other host can use local storage.
export type VisualizerHost = {
  loadSettings(): Promise<unknown>
  saveSettings(settings: unknown): void
}
