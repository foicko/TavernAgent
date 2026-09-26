// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
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
    audios.push(this);
  }
}

const audios: MockAudio[] = [];

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("ttsPlayer", () => {
  beforeEach(() => {
    ttsPlayer.stop();
    vi.restoreAllMocks();
    audios.length = 0;
    window.Audio = MockAudio as unknown as typeof Audio;
  });

  it("notifies subscribers when starting and stopping playback", async () => {
    const states: (string | null)[] = [];
    const unsub = ttsPlayer.subscribe((cur) => states.push(cur));

    vi.spyOn(api, "synthesizeTTS").mockResolvedValue(new Blob(["mock-mp3"], { type: "audio/mpeg" }));
    window.URL.createObjectURL = vi.fn().mockReturnValue("blob:mock-url");

    await ttsPlayer.play("Hello world");
    expect(ttsPlayer.getCurrentText()).toBe("Hello world");

    ttsPlayer.stop();
    expect(ttsPlayer.getCurrentText()).toBeNull();

    unsub();
  });

  it("toggles off when playing the same text twice", async () => {
    vi.spyOn(api, "synthesizeTTS").mockResolvedValue(new Blob(["mock-mp3"], { type: "audio/mpeg" }));
    window.URL.createObjectURL = vi.fn().mockReturnValue("blob:mock-url");

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
    window.URL.createObjectURL = vi.fn().mockReturnValue("blob:mock-url");

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
    window.URL.createObjectURL = vi.fn().mockReturnValue("blob:mock-url");

    await ttsPlayer.play("(冷峻)你想带我走？[停顿片刻]滚出去。");

    expect(synthSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        engine: "edge",
        text: "你想带我走？滚出去。",
        instruction: undefined,
      })
    );
  });

  describe("请求作废与缓存", () => {
    const speak = vi.fn();

    beforeEach(async () => {
      const { useTTSSettings } = await import("../../stores/ttsSettingsStore");
      useTTSSettings.getState().resetToDefaults();
      speak.mockReset();
      Object.defineProperty(window, "speechSynthesis", {
        value: { speak, cancel: vi.fn() },
        configurable: true,
      });
      vi.stubGlobal("SpeechSynthesisUtterance", class { constructor(public text: string) {} });
      let n = 0;
      window.URL.createObjectURL = vi.fn(() => `blob:url-${++n}`);
      window.URL.revokeObjectURL = vi.fn();
    });

    afterEach(() => {
      ttsPlayer.stop();
      vi.unstubAllGlobals();
      Reflect.deleteProperty(window, "speechSynthesis");
    });

    it("合成期间 stop，结果返回后不再播放", async () => {
      const pending = deferred<Blob>();
      vi.spyOn(api, "synthesizeTTS").mockReturnValue(pending.promise);

      const playing = ttsPlayer.play("停在加载中");
      ttsPlayer.stop();
      pending.resolve(new Blob(["x"]));
      await playing;

      expect(audios).toHaveLength(0);
      expect(ttsPlayer.getCurrentText()).toBeNull();
    });

    it("A 加载中点 B，只播放 B", async () => {
      const a = deferred<Blob>();
      const b = deferred<Blob>();
      vi.spyOn(api, "synthesizeTTS").mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise);

      const playA = ttsPlayer.play("段落A");
      const playB = ttsPlayer.play("段落B");
      b.resolve(new Blob(["b"]));
      await playB;
      a.resolve(new Blob(["a"]));
      await playA;

      expect(audios).toHaveLength(1);
      expect(audios[0].play).toHaveBeenCalled();
      expect(ttsPlayer.getCurrentText()).toBe("段落B");
    });

    it("合成失败时若已 stop，不降级为浏览器朗读", async () => {
      const pending = deferred<Blob>();
      vi.spyOn(api, "synthesizeTTS").mockReturnValue(pending.promise);

      const playing = ttsPlayer.play("失败也别出声");
      ttsPlayer.stop();
      pending.reject(new Error("offline"));
      await playing;

      expect(speak).not.toHaveBeenCalled();
    });

    it("缓存有上限，淘汰最久未用的条目并释放其 blob URL", async () => {
      // 单例的缓存会被前面的用例写入，这里换一份全新的模块实例，计数才确定
      vi.resetModules();
      const freshApi = await import("../../app/api");
      const { ttsPlayer: fresh } = await import("../ttsPlayer");
      vi.spyOn(freshApi, "synthesizeTTS").mockResolvedValue(new Blob(["x"]));
      for (let i = 0; i < 21; i++) {
        await fresh.play(`缓存段落-${i}`);
      }
      fresh.stop();
      // 第 21 段写入后超出上限 20，最久未用的第 1 段被淘汰
      expect(window.URL.revokeObjectURL).toHaveBeenCalledTimes(1);
      expect(window.URL.revokeObjectURL).toHaveBeenCalledWith("blob:url-1");
    });
  });
});

