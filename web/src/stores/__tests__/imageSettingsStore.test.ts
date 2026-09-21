// @vitest-environment jsdom
import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { cleanup } from "@testing-library/react";
import { useImageSettings } from "../imageSettingsStore";

describe("imageSettingsStore", () => {
  beforeEach(() => {
    useImageSettings.getState().resetToDefaults();
    if (typeof window !== "undefined" && window.localStorage) {
      window.localStorage.clear();
    }
  });

  afterEach(cleanup);

  it("initializes with default image settings", () => {
    const state = useImageSettings.getState();
    expect(state.engine).toBe("comfyui");
    expect(state.size).toBe("1024x576");
    expect(state.stylePreset).toBe("anime");
    expect(state.customModel).toBe("image_qwen_image_2_1_t2i");
    expect(state.customBaseUrl).toBe("http://127.0.0.1:8188");
  });

  it("applies presets correctly", () => {
    // Apply OpenAI preset
    useImageSettings.getState().applyOpenAIPreset();
    let state = useImageSettings.getState();
    expect(state.engine).toBe("openai");
    expect(state.customBaseUrl).toBe("https://api.openai.com/v1");
    expect(state.customModel).toBe("dall-e-3");

    // Apply SD WebUI preset
    useImageSettings.getState().applySDWebUIPreset();
    state = useImageSettings.getState();
    expect(state.engine).toBe("sd-webui");
    expect(state.customBaseUrl).toBe("http://127.0.0.1:7860");
    expect(state.customModel).toBe("sd-xl");

    // Apply ComfyUI preset
    useImageSettings.getState().applyComfyUIPreset();
    state = useImageSettings.getState();
    expect(state.engine).toBe("comfyui");
    expect(state.customBaseUrl).toBe("http://127.0.0.1:8188");
    expect(state.customModel).toBe("image_qwen_image_2_1_t2i");
  });

  it("attaches and removes illustrations for turns", () => {
    const img = {
      id: "img_test_123",
      url: "/api/v1/images/img_test_123.png",
      prompt: "tavern scene",
      mimeType: "image/png",
      createdAt: "2026-09-21T12:00:00Z",
    };

    useImageSettings.getState().attachIllustration("turn_1", img);
    expect(useImageSettings.getState().turnIllustrations["turn_1"]).toEqual(img);

    useImageSettings.getState().removeIllustration("turn_1");
    expect(useImageSettings.getState().turnIllustrations["turn_1"]).toBeUndefined();
  });
});
