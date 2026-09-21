import { create } from "zustand";
import { persist } from "zustand/middleware";

export type TTSEngine = "edge" | "openai" | "mimo" | "web-speech";

export interface TTSSettingsState {
  engine: TTSEngine;
  edgeVoice: string;
  customBaseUrl: string;
  customApiKey: string;
  customModel: string;
  customVoice: string;
  speed: number;
  dialogueOnly: boolean;
  directorMode: boolean; // 情境感知演播模式 (宏观指导与细粒度标签)

  setEngine: (engine: TTSEngine) => void;
  setEdgeVoice: (voice: string) => void;
  setCustomBaseUrl: (url: string) => void;
  setCustomApiKey: (key: string) => void;
  setCustomModel: (model: string) => void;
  setCustomVoice: (voice: string) => void;
  setSpeed: (speed: number) => void;
  setDialogueOnly: (dialogueOnly: boolean) => void;
  setDirectorMode: (directorMode: boolean) => void;
  applyMiMoPreset: () => void;
  resetToDefaults: () => void;
}

export const DEFAULT_TTS_SETTINGS = {
  engine: "edge" as TTSEngine,
  edgeVoice: "zh-CN-XiaoxiaoNeural",
  customBaseUrl: "http://127.0.0.1:9880/v1",
  customApiKey: "",
  customModel: "tts-1",
  customVoice: "alloy",
  speed: 1.0,
  dialogueOnly: true, // 核心需求：只播报人物说的话，旁白不读（默认开启）
  directorMode: true, // 核心需求：情境感知演播（导演模式，默认开启）
};

export const useTTSSettings = create<TTSSettingsState>()(
  persist(
    (set) => ({
      ...DEFAULT_TTS_SETTINGS,
      setEngine: (engine) => set({ engine }),
      setEdgeVoice: (edgeVoice) => set({ edgeVoice }),
      setCustomBaseUrl: (customBaseUrl) => set({ customBaseUrl }),
      setCustomApiKey: (customApiKey) => set({ customApiKey }),
      setCustomModel: (customModel) => set({ customModel }),
      setCustomVoice: (customVoice) => set({ customVoice }),
      setSpeed: (speed) => set({ speed }),
      setDialogueOnly: (dialogueOnly) => set({ dialogueOnly }),
      setDirectorMode: (directorMode) => set({ directorMode }),
      applyMiMoPreset: () =>
        set({
          engine: "mimo",
          customBaseUrl: "https://api.xiaomimimo.com/v1",
          customModel: "mimo-v2.5-tts",
          customVoice: "mimo_default",
        }),
      resetToDefaults: () => set({ ...DEFAULT_TTS_SETTINGS }),
    }),
    {
      name: "tavernagent_tts_settings",
    }
  )
);
