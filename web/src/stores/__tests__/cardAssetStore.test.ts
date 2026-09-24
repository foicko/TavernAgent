import { describe, expect, it } from "vitest";
import { emotionMapFromAssets } from "../cardAssetStore";
import type { CardAssetView } from "../../app/types";

const asset = (name: string, type: string, url = `u/${name}`): CardAssetView => ({ name, type, url });

describe("emotionMapFromAssets", () => {
  it("maps emotion assets to expression keys by name keywords", () => {
    const map = emotionMapFromAssets([
      asset("happy_1", "emotion"),
      asset("angry_1", "emotion"),
      asset("sad_1", "emotion"),
      asset("neutral_1", "emotion"),
      asset("room_bg", "x-risu-asset"),
      asset("main", "icon"),
    ]);
    expect(map.smile).toBe("u/happy_1");
    expect(map.alert).toBe("u/angry_1");
    expect(map.whisper).toBe("u/sad_1");
    expect(map.calm).toBe("u/neutral_1");
  });

  it("ignores non-emotion assets and first match wins", () => {
    const map = emotionMapFromAssets([
      asset("smile_scene", "x-risu-asset"),
      asset("smile_a", "emotion"),
      asset("smile_b", "emotion"),
    ]);
    expect(Object.keys(map)).toEqual(["smile"]);
    expect(map.smile).toBe("u/smile_a");
  });

  it("returns empty map when there are no emotion assets", () => {
    expect(emotionMapFromAssets([asset("main", "icon")])).toEqual({});
  });
});
