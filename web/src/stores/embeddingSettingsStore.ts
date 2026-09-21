import { create } from "zustand";
import { persist } from "zustand/middleware";

export type EmbeddingProviderType = "ollama" | "siliconflow" | "openai" | "custom";

export interface EmbeddingSettingsState {
  enabled: boolean;
  provider: EmbeddingProviderType;
  baseUrl: string;
  apiKey: string;
  model: string;
  dimension: number;
  hybridRRFWeight: number; // 0.0 ~ 1.0, 0.5 means equal weight for lexical and vector
  topK: number;

  setEnabled: (enabled: boolean) => void;
  setProvider: (provider: EmbeddingProviderType) => void;
  setBaseUrl: (baseUrl: string) => void;
  setApiKey: (apiKey: string) => void;
  setModel: (model: string) => void;
  setDimension: (dimension: number) => void;
  setHybridRRFWeight: (weight: number) => void;
  setTopK: (topK: number) => void;

  applyBgeSmallOllama: () => void;
  applySiliconFlow: () => void;
  applyBgeM3Ollama: () => void;
  applyNomicOllama: () => void;
  applyOpenAI: () => void;
  resetToDefaults: () => void;
}

export const DEFAULT_EMBEDDING_SETTINGS = {
  enabled: true,
  provider: "ollama" as EmbeddingProviderType,
  baseUrl: "http://127.0.0.1:11434/v1",
  apiKey: "",
  model: "bge-small-zh-v1.5",
  dimension: 512,
  hybridRRFWeight: 0.5,
  topK: 5,
};

export const useEmbeddingSettings = create<EmbeddingSettingsState>()(
  persist(
    (set) => ({
      ...DEFAULT_EMBEDDING_SETTINGS,

      setEnabled: (enabled) => set({ enabled }),
      setProvider: (provider) => set({ provider }),
      setBaseUrl: (baseUrl) => set({ baseUrl }),
      setApiKey: (apiKey) => set({ apiKey }),
      setModel: (model) => set({ model }),
      setDimension: (dimension) => set({ dimension }),
      setHybridRRFWeight: (hybridRRFWeight) => set({ hybridRRFWeight }),
      setTopK: (topK) => set({ topK }),

      applyBgeSmallOllama: () =>
        set({
          provider: "ollama",
          baseUrl: "http://127.0.0.1:11434/v1",
          model: "bge-small-zh-v1.5",
          dimension: 512,
        }),

      applySiliconFlow: () =>
        set({
          provider: "siliconflow",
          baseUrl: "https://api.siliconflow.cn/v1",
          model: "BAAI/bge-small-zh-v1.5",
          dimension: 512,
        }),

      applyBgeM3Ollama: () =>
        set({
          provider: "ollama",
          baseUrl: "http://127.0.0.1:11434/v1",
          model: "bge-m3",
          dimension: 1024,
        }),

      applyNomicOllama: () =>
        set({
          provider: "ollama",
          baseUrl: "http://127.0.0.1:11434/v1",
          model: "nomic-embed-text",
          dimension: 768,
        }),

      applyOpenAI: () =>
        set({
          provider: "openai",
          baseUrl: "https://api.openai.com/v1",
          model: "text-embedding-3-small",
          dimension: 1536,
        }),

      resetToDefaults: () => set(DEFAULT_EMBEDDING_SETTINGS),
    }),
    {
      name: "tavernagent_embedding_settings",
    }
  )
);
