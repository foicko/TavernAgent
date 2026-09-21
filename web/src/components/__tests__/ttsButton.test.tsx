// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, act, cleanup } from "@testing-library/react";
import { TTSPlayButton } from "../TTSButton";
import { ttsPlayer } from "../../lib/ttsPlayer";
import { useTTSSettings } from "../../stores/ttsSettingsStore";
import type { TextBlock } from "../../app/types";

class MockAudio {
  src: string;
  currentTime = 0;
  play = vi.fn().mockResolvedValue(undefined);
  pause = vi.fn();
  onended: (() => void) | null = null;
  onerror: (() => void) | null = null;

  constructor(src: string) {
    this.src = src;
  }
}

describe("TTSPlayButton", () => {
  afterEach(cleanup);
  beforeEach(() => {
    useTTSSettings.getState().resetToDefaults();
    ttsPlayer.stop();
    vi.restoreAllMocks();
    window.Audio = MockAudio as unknown as typeof Audio;
  });

  it("plays only dialogue blocks and filters out narration when dialogueOnly is true", async () => {
    const playSpy = vi.spyOn(ttsPlayer, "play").mockImplementation(async () => {});
    const blocks: TextBlock[] = [
      { kind: "narration", text: "外面正下着暴风雨。" },
      { kind: "dialogue", speakerId: "hero", text: "我们必须立刻出发！" },
      { kind: "narration", text: "他紧了紧斗篷。" },
    ];

    render(<TTSPlayButton blocks={blocks} fallbackText="全量文本" />);

    const btn = screen.getByRole("button", { name: "朗读角色台词" });
    expect(btn).toBeTruthy();
    expect(btn.textContent).toContain("🔊 朗读");

    await act(async () => {
      fireEvent.click(btn);
    });

    // Only character dialogue is passed to player
    expect(playSpy).toHaveBeenCalledWith("我们必须立刻出发！", undefined);
  });

  it("shows notice when clicked on a turn containing only narration with dialogueOnly=true", async () => {
    const playSpy = vi.spyOn(ttsPlayer, "play").mockImplementation(async () => {});
    const blocks: TextBlock[] = [
      { kind: "narration", text: "四周万籁俱寂，只有风吹过树梢。" },
    ];

    render(<TTSPlayButton blocks={blocks} />);

    const btn = screen.getByRole("button", { name: "本段仅有旁白，无角色对白（只读台词模式）" });
    expect(btn).toBeTruthy();

    await act(async () => {
      fireEvent.click(btn);
    });

    // Should NOT call play
    expect(playSpy).not.toHaveBeenCalled();
    // Notice shown
    expect(btn.textContent).toContain("🔇 仅旁白");
  });

  it("reads full text when dialogueOnly is set to false", async () => {
    useTTSSettings.getState().setDialogueOnly(false);
    const playSpy = vi.spyOn(ttsPlayer, "play").mockImplementation(async () => {});

    render(<TTSPlayButton text="Test text" />);

    const btn = screen.getByRole("button", { name: "朗读正文" });
    expect(btn).toBeTruthy();

    await act(async () => {
      fireEvent.click(btn);
    });

    expect(playSpy).toHaveBeenCalledWith("Test text", undefined);
  });

  it("updates button label when playing matching text", async () => {
    const stopSpy = vi.spyOn(ttsPlayer, "stop");
    render(<TTSPlayButton text="「Playing line」" />);

    // In dialogueOnly mode, target text extracted is "Playing line"
    act(() => {
      // @ts-expect-error accessing private property for test
      ttsPlayer.currentText = "Playing line";
      // @ts-expect-error accessing private property for test
      ttsPlayer.notify();
    });

    const btn = screen.getByRole("button", { name: "停止朗读" });
    expect(btn.textContent).toContain("⏹ 停止");
    expect(btn.classList.contains("is-playing")).toBe(true);

    await act(async () => {
      fireEvent.click(btn);
    });

    expect(stopSpy).toHaveBeenCalled();
  });
});
