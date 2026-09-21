// @vitest-environment jsdom
import { beforeEach, afterEach, describe, expect, it } from "vitest";
import { cleanup } from "@testing-library/react";
import {
  DEFAULT_EMBEDDING_SETTINGS,
  useEmbeddingSettings,
} from "../embeddingSettingsStore";

describe("embeddingSettingsStore", () => {
  beforeEach(() => {
    useEmbeddingSettings.getState().resetToDefaults();
    if (typeof window !== "undefined" && window.localStorage) {
      window.localStorage.clear();
    }
  });

  afterEach(cleanup);

  it("initializes with bge-small-zh-v1.5 and Ollama defaults", () => {
    const state = useEmbeddingSettings.getState();
    expect(state.model).toBe("bge-small-zh-v1.5");
    expect(state.dimension).toBe(512);
    expect(state.baseUrl).toBe("http://127.0.0.1:11434/v1");
    expect(state.enabled).toBe(true);
    expect(state.hybridRRFWeight).toBe(0.5);
    expect(state.topK).toBe(5);
  });

  it("applies siliconflow preset correctly", () => {
    useEmbeddingSettings.getState().applySiliconFlow();
    const state = useEmbeddingSettings.getState();
    expect(state.provider).toBe("siliconflow");
    expect(state.baseUrl).toBe("https://api.siliconflow.cn/v1");
    expect(state.model).toBe("BAAI/bge-small-zh-v1.5");
    expect(state.dimension).toBe(512);
  });

  it("applies bge-m3 and nomic presets correctly", () => {
    useEmbeddingSettings.getState().applyBgeM3Ollama();
    expect(useEmbeddingSettings.getState().model).toBe("bge-m3");
    expect(useEmbeddingSettings.getState().dimension).toBe(1024);

    useEmbeddingSettings.getState().applyNomicOllama();
    expect(useEmbeddingSettings.getState().model).toBe("nomic-embed-text");
    expect(useEmbeddingSettings.getState().dimension).toBe(768);
  });

  it("applies openai preset and resets to defaults", () => {
    useEmbeddingSettings.getState().applyOpenAI();
    expect(useEmbeddingSettings.getState().model).toBe("text-embedding-3-small");
    expect(useEmbeddingSettings.getState().dimension).toBe(1536);

    useEmbeddingSettings.getState().resetToDefaults();
    expect(useEmbeddingSettings.getState().model).toBe(DEFAULT_EMBEDDING_SETTINGS.model);
    expect(useEmbeddingSettings.getState().dimension).toBe(DEFAULT_EMBEDDING_SETTINGS.dimension);
  });
});
