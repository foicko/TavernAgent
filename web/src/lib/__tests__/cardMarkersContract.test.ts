import { describe, expect, it } from "vitest";
import { parseCardMarkers } from "../cardMarkers";
import { MARKER_CASES } from "./markerContractCases.generated";

// 契约测试：唯一来源是 testdata/contract/markers.json（Go 直接读，Kotlin 与本文件由脚本生成）。
describe("card marker contract (testdata/contract/markers.json)", () => {
  for (const tc of MARKER_CASES) {
    it(tc.id, () => {
      const assets: Record<string, string> = {};
      for (const key of tc.images) assets[key] = "/assets/" + key;
      const segs = parseCardMarkers(tc.text, assets);

      expect(segs.filter((s) => s.kind === "image").map((s) => s.alt)).toEqual(tc.images);

      const hud = segs
        .filter((s) => s.kind === "hud")
        .map((s) => (s.hint && s.hint.kind === "hud" ? s.hint.values : {}));
      expect(hud).toEqual(tc.hud);

      const sceneSeg = segs.find((s) => s.kind === "scene");
      const scene = sceneSeg && sceneSeg.hint && sceneSeg.hint.kind === "scene" ? sceneSeg.hint.text : "";
      expect(scene).toBe(tc.scene);

      const joined = segs
        .filter((s) => s.kind === "text")
        .map((s) => s.text ?? "")
        .join("");
      if (tc.varRemoved) expect(joined).not.toContain("[VAR:");
    });
  }
});
