// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ttsPlayer } from "../ttsPlayer";
import * as api from "../../app/api";

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

describe("ttsPlayer", () => {
  beforeEach(() => {
    ttsPlayer.stop();
    vi.restoreAllMocks();
    window.Audio = MockAudio as unknown as typeof Audio;
  });

  it("notifies subscribers when starting and stopping playback", async () => {
    const states: (string | null)[] = [];
    const unsub = ttsPlayer.subscribe((cur) => states.push(cur));

    vi.spyOn(api, "synthesizeTTS").mockResolvedValue(new Blob(["mock-mp3"], { type: "audio/mpeg" }));
    global.URL.createObjectURL = vi.fn().mockReturnValue("blob:mock-url");

    await ttsPlayer.play("Hello world");
    expect(ttsPlayer.getCurrentText()).toBe("Hello world");

    ttsPlayer.stop();
    expect(ttsPlayer.getCurrentText()).toBeNull();

    unsub();
  });

  it("toggles off when playing the same text twice", async () => {
    vi.spyOn(api, "synthesizeTTS").mockResolvedValue(new Blob(["mock-mp3"], { type: "audio/mpeg" }));
    global.URL.createObjectURL = vi.fn().mockReturnValue("blob:mock-url");

    await ttsPlayer.play("Toggle me");
    expect(ttsPlayer.getCurrentText()).toBe("Toggle me");

    await ttsPlayer.play("Toggle me");
    expect(ttsPlayer.getCurrentText()).toBeNull();
  });

  it("passes instruction and retains tags when engine is MiMo and directorMode is true", async () => {
    const { useTTSSettings } = await import("../../stores/ttsSettingsStore");
    useTTSSettings.getState().applyMiMoPreset();
    useTTSSettings.getState().setDirectorMode(true);

    const synthSpy = vi.spyOn(api, "synthesizeTTS").mockResolvedValue(new Blob(["mock-mp3"], { type: "audio/mpeg" }));
    global.URL.createObjectURL = vi.fn().mockReturnValue("blob:mock-url");

    await ttsPlayer.play("(冷峻)你想带我走？[停顿片刻]滚出去。", {
      instruction: "【角色】岑家大当家\n【指导】极慢，实音重",
    });

    expect(synthSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        engine: "mimo",
        text: "(冷峻)你想带我走？[停顿片刻]滚出去。",
        instruction: "【角色】岑家大当家\n【指导】极慢，实音重",
      })
    );
  });

  it("automatically strips audio tags when engine is edge-tts to avoid reading tags aloud", async () => {
    const { useTTSSettings } = await import("../../stores/ttsSettingsStore");
    useTTSSettings.getState().resetToDefaults();

    const synthSpy = vi.spyOn(api, "synthesizeTTS").mockResolvedValue(new Blob(["mock-mp3"], { type: "audio/mpeg" }));
    global.URL.createObjectURL = vi.fn().mockReturnValue("blob:mock-url");

    await ttsPlayer.play("(冷峻)你想带我走？[停顿片刻]滚出去。");

    expect(synthSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        engine: "edge",
        text: "你想带我走？滚出去。",
        instruction: undefined,
      })
    );
  });
});

