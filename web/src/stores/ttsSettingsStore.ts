import { create } from "zustand";
import { persist } from "zustand/middleware";

export type TTSEngine = "edge" | "openai" | "web-speech";

export interface TTSSettingsState {
  engine: TTSEngine;
  edgeVoice: string;
  customBaseUrl: string;
  customApiKey: string;
  customModel: string;
  customVoice: string;
  speed: number;
  dialogueOnly: boolean;

  setEngine: (engine: TTSEngine) => void;
  setEdgeVoice: (voice: string) => void;
  setCustomBaseUrl: (url: string) => void;
  setCustomApiKey: (key: string) => void;
  setCustomModel: (model: string) => void;
  setCustomVoice: (voice: string) => void;
  setSpeed: (speed: number) => void;
  setDialogueOnly: (dialogueOnly: boolean) => void;
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
      resetToDefaults: () => set({ ...DEFAULT_TTS_SETTINGS }),
    }),
    {
      name: "tavernagent_tts_settings",
    }
  )
);
