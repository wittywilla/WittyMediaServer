// What the library transcoding API sends and takes. docs/CLIENT-API.md has the routes.

export type Policy = {
  preset: string
  container: string
  video: {
    codec: string; accept: string[] | null; quality: number; speed: string; maxHeight: number; hdr: string; dolbyVision: string
    eightBit: boolean; deinterlace: boolean; constantRate: boolean; threeD: string; extraVideo: string; minSavingPercent: number; skipBelowKbps: number
  }
  audio: {
    keep: string; languages: string[]; keepOriginal: boolean; untagged: string; untaggedAs: string; commentary: string; description: string; default: string
    codec: string; accept: string[] | null; lossless: string; objects: string; maxChannels: number; addStereo: boolean; stereoFirst: boolean
  }
  subtitles: { keep: string; languages: string[]; forced: string; sdh: string; untagged: string; images: string; text: string; sidecars: string }
}

export type PresetInfo = { id: string; name: string; summary: string; details: string[]; cost: string; policy: Policy }

export type Options = {
  presets: PresetInfo[]
  outputs: string[]
  saveMeta: boolean
  capabilities: { h264: boolean; hevc: boolean; av1: boolean; opus: boolean; toneMap: boolean; burnText: boolean; hardware: string[]; cores: number }
}

export type Settings = {
  enabled: boolean; acknowledged: boolean; maxJobs: number; threads: number; lowPriority: boolean; pauseWhileStreaming: boolean
  hoursOn: boolean; hoursFrom: number; hoursTo: number; useHardware: boolean; minFreeGb: number; settleSeconds: number
}

export type Rule = { libraryId: number; policy: Policy; output: string; allowWrites: boolean; auto: boolean; autoSince: number; saved: boolean }

export type Library = { id: number; name: string; type: string; items: number; readOnly: boolean; rule: Rule; blocked?: string }

export type PlanStep = { kind: string; action: string; from: string; to?: string; why?: string }

export type Plan = {
  verdict: string; reason: string; container: string; ext: string; steps: PlanStep[]; warnings: string[]; notes: string[]; encoder?: string; estBytes: number; estSec: number
}

export type Step = { plan: Plan }

export type Preview = { itemId: number; title: string; parts: { path: string; size: number; duration: number; plan: Plan }[] }

export type Sample = { seconds: number; took: number; speed: number; bytes: number; estFileBytes: number; estFileSec: number; note?: string }

export type Summary = {
  files: number; convert: number; remux: number; skip: number; refuse: number; done: number; failed: number; notWorth: number; postponed: number; changed: number
  bytesBefore: number; estBytes: number; estSec: number; bytesAfter: number; needsBytes: number
  drives?: { folder: string; free: number; needs: number; enough: boolean }[]
}

export type Run = { id: number; libraryId: number; dry: boolean; auto: boolean; policy: Policy; output: string; state: string; by?: string; createdAt: number; finishedAt?: number; summary: Summary }

export type FileRow = {
  id: number; runId: number; itemId: number; part: number; path: string; verdict: string; reason: string; state: string; message?: string
  sizeBefore: number; sizeAfter: number; estBytes: number; estSec: number; finalPath?: string; keptPath?: string; plan?: Plan
}

export type TdarrWorker = { id: string; file: string; percent: number; mode: string; fps: number; eta: string }
export type TdarrNode = { id: string; name: string; paused: boolean; workers: TdarrWorker[] }
export type TdarrLibrary = { id: string; name: string; folder: string }
export type TdarrReport = { ok: boolean; error?: string; version?: string; status?: string; nodes: TdarrNode[]; libraries: TdarrLibrary[] }
export type TdarrMapping = { libraryId: number; tdarrId: string; from: string; to: string }
export type TdarrSettings = { enabled: boolean; url: string; keySet: boolean; mappings: TdarrMapping[] }
