// 卡片场景提示的读取工具（与组件解耦，便于单测）。
import type { UIHint } from "../app/types";

/** 取最近一条 scene 提示的标题；无则返回空串。 */
export function latestSceneTitle(messages: { uiHints?: UIHint[] }[]): string {
  for (let i = messages.length - 1; i >= 0; i--) {
    const hints = messages[i].uiHints;
    if (!hints) continue;
    for (let j = hints.length - 1; j >= 0; j--) {
      const h = hints[j];
      if (h.kind === "scene" && h.text) return h.text;
    }
  }
  return "";
}
