import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { GeneratedImageResult } from "../app/api";

export type ImageEngine = "comfyui" | "openai" | "sd-webui";
export type ImageStylePreset = "anime" | "fantasy" | "cinematic" | "realistic";
export type ImageAspectSize = "1024x576" | "1024x1024" | "576x1024";

export interface ImageSettingsState {
  engine: ImageEngine;
  customBaseUrl: string;
  customApiKey: string;
  customModel: string;
  size: ImageAspectSize;
  stylePreset: ImageStylePreset;

  // 剧情回合关联插画映射: turnId / nodeId -> GeneratedImageResult
  turnIllustrations: Record<string, GeneratedImageResult>;

  setEngine: (engine: ImageEngine) => void;
  setCustomBaseUrl: (url: string) => void;
  setCustomApiKey: (key: string) => void;
  setCustomModel: (model: string) => void;
  setSize: (size: ImageAspectSize) => void;
  setStylePreset: (style: ImageStylePreset) => void;

  attachIllustration: (turnId: string, image: GeneratedImageResult) => void;
  removeIllustration: (turnId: string) => void;

  applyComfyUIPreset: () => void;
  applySiliconFlowPreset: () => void;
  applyOpenAIPreset: () => void;
  applySDWebUIPreset: () => void;
  resetToDefaults: () => void;
}

export const DEFAULT_IMAGE_SETTINGS = {
  engine: "comfyui" as ImageEngine,
  customBaseUrl: "http://127.0.0.1:8188",
  customApiKey: "",
  customModel: "image_qwen_image_2_1_t2i",
  size: "1024x576" as ImageAspectSize, // 16:9 故事场景最佳画幅
  stylePreset: "anime" as ImageStylePreset,
};

export const useImageSettings = create<ImageSettingsState>()(
  persist(
    (set) => ({
      ...DEFAULT_IMAGE_SETTINGS,
      turnIllustrations: {},

      setEngine: (engine) => set({ engine }),
      setCustomBaseUrl: (customBaseUrl) => set({ customBaseUrl }),
      setCustomApiKey: (customApiKey) => set({ customApiKey }),
      setCustomModel: (customModel) => set({ customModel }),
      setSize: (size) => set({ size }),
      setStylePreset: (stylePreset) => set({ stylePreset }),

      attachIllustration: (turnId, image) =>
        set((state) => ({
          turnIllustrations: {
            ...state.turnIllustrations,
            [turnId]: image,
          },
        })),

      removeIllustration: (turnId) =>
        set((state) => {
          const next = { ...state.turnIllustrations };
          delete next[turnId];
          return { turnIllustrations: next };
        }),

      applyComfyUIPreset: () =>
        set({
          engine: "comfyui",
          customBaseUrl: "http://127.0.0.1:8188",
          customModel: "image_qwen_image_2_1_t2i",
        }),

      applySiliconFlowPreset: () =>
        set({
          engine: "openai",
          customBaseUrl: "https://api.siliconflow.cn/v1",
          customModel: "black-forest-labs/FLUX.1-schnell",
        }),

      applyOpenAIPreset: () =>
        set({
          engine: "openai",
          customBaseUrl: "https://api.openai.com/v1",
          customModel: "dall-e-3",
        }),

      applySDWebUIPreset: () =>
        set({
          engine: "sd-webui",
          customBaseUrl: "http://127.0.0.1:7860",
          customModel: "sd-xl",
        }),

      resetToDefaults: () =>
        set((state) => ({
          ...DEFAULT_IMAGE_SETTINGS,
          turnIllustrations: state.turnIllustrations,
        })),
    }),
    {
      name: "tavernagent_image_settings",
    }
  )
);
