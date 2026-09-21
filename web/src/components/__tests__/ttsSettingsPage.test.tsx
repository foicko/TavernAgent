// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/react";
import { TTSSettingsPage } from "../TTSSettingsPage";
import { useTTSSettings } from "../../stores/ttsSettingsStore";
import { ttsPlayer } from "../../lib/ttsPlayer";
import * as api from "../../app/api";

describe("TTSSettingsPage", () => {
  afterEach(cleanup);
  beforeEach(() => {
    useTTSSettings.getState().resetToDefaults();
    vi.restoreAllMocks();
    vi.spyOn(api, "getTTSVoices").mockResolvedValue([
      {
        id: "zh-CN-XiaoxiaoNeural",
        name: "晓晓 (温柔女声)",
        gender: "female",
        locale: "zh-CN",
        description: "温暖亲切",
      },
    ]);
  });

  it("renders default edge-tts settings and dialogueOnly switch", async () => {
    render(<TTSSettingsPage />);

    expect(screen.getByText("TTS 语音引擎")).toBeTruthy();
    expect(screen.getByText("Edge-TTS 声音配置")).toBeTruthy();

    const switchInput = screen.getByLabelText(/只播报角色对白/);
    expect(switchInput).toBeTruthy();
    expect((switchInput as HTMLInputElement).checked).toBe(true);

    // Toggle dialogueOnly switch
    fireEvent.click(switchInput);
    expect(useTTSSettings.getState().dialogueOnly).toBe(false);
  });

  it("switches to openai engine and configures local endpoint", async () => {
    render(<TTSSettingsPage />);

    const engineSelect = screen.getByLabelText("引擎类型");
    fireEvent.change(engineSelect, { target: { value: "openai" } });

    expect(useTTSSettings.getState().engine).toBe("openai");
    expect(screen.getByText("自定义端点与本地模型")).toBeTruthy();

    // Click local preset button
    const localBtn = screen.getByText("本地端点 (GPT-SoVITS / CosyVoice)");
    fireEvent.click(localBtn);

    expect(useTTSSettings.getState().customBaseUrl).toBe("http://127.0.0.1:9880/v1");
  });

  it("triggers test playback on click", async () => {
    const playSpy = vi.spyOn(ttsPlayer, "play").mockResolvedValue(undefined);
    render(<TTSSettingsPage />);

    const testBtn = screen.getByText("🔊 试听发音");
    fireEvent.click(testBtn);

    await waitFor(() => {
      expect(playSpy).toHaveBeenCalledWith("你好！很高兴能与你一同开启这段旅程。");
    });
  });
});
