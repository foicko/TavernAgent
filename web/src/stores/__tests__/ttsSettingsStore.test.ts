// @vitest-environment jsdom
import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { cleanup } from "@testing-library/react";
import { useTTSSettings, DEFAULT_TTS_SETTINGS } from "../ttsSettingsStore";

describe("ttsSettingsStore", () => {
  beforeEach(() => {
    useTTSSettings.getState().resetToDefaults();
    if (typeof window !== "undefined" && window.localStorage) {
      window.localStorage.clear();
    }
  });

  afterEach(cleanup);

  it("initializes with expected default values (including dialogueOnly = true)", () => {
    const state = useTTSSettings.getState();
    expect(state.engine).toBe("edge");
    expect(state.edgeVoice).toBe("zh-CN-XiaoxiaoNeural");
    expect(state.speed).toBe(1.0);
    expect(state.dialogueOnly).toBe(true);
    expect(state.customBaseUrl).toBe("http://127.0.0.1:9880/v1");
    expect(state.customModel).toBe("tts-1");
  });

  it("updates settings and resets back to defaults", () => {
    useTTSSettings.getState().setEngine("openai");
    useTTSSettings.getState().setCustomBaseUrl("http://localhost:5000/v1");
    useTTSSettings.getState().setCustomApiKey("sk-secret");
    useTTSSettings.getState().setCustomModel("cosyvoice");
    useTTSSettings.getState().setCustomVoice("shimmer");
    useTTSSettings.getState().setSpeed(1.5);
    useTTSSettings.getState().setDialogueOnly(false);

    let state = useTTSSettings.getState();
    expect(state.engine).toBe("openai");
    expect(state.customBaseUrl).toBe("http://localhost:5000/v1");
    expect(state.customApiKey).toBe("sk-secret");
    expect(state.customModel).toBe("cosyvoice");
    expect(state.customVoice).toBe("shimmer");
    expect(state.speed).toBe(1.5);
    expect(state.dialogueOnly).toBe(false);

    useTTSSettings.getState().resetToDefaults();
    state = useTTSSettings.getState();
    expect(state.engine).toBe(DEFAULT_TTS_SETTINGS.engine);
    expect(state.dialogueOnly).toBe(true);
    expect(state.speed).toBe(1.0);
  });
});
