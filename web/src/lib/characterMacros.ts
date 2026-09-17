// characterMacros: 展示层的角色卡宏替换。
//
// 分工（与 internal/application/macros.go 配套，改动请同步两处注释）：
//   - 身份宏 {{char}} / {{user}} 由**服务端在会话创建时**展开，写进世界状态、开场正文、
//     世界书与导出包；这里是第二道保险，覆盖旧会话与旧剧情包里遗留的原文。
//   - 语言宏 {{sub}} / {{obj}} / {{poss}} / <START> 只在本层展开：它们是中文措辞，
//     不该被写进数据层（否则存档就带上了语言）。
//   代价：模型侧仍会看到 {{sub}} 这类字面量，这是有意接受的取舍。

/** 玩家未命名时的统一称呼。不要再在各组件里各写一份。 */
export const DEFAULT_PLAYER_NAME = "旅人";
/** 角色名缺失时的统一占位。 */
export const DEFAULT_CHARACTER_NAME = "角色";

export interface MacroContext {
  charName: string;
  userName?: string;
}

export function replaceMacros(text: string, context: MacroContext): string {
  if (!text) return "";
  const char = context.charName || DEFAULT_CHARACTER_NAME;
  const user = context.userName || DEFAULT_PLAYER_NAME;

  return text
    // /gi 已经覆盖 {{Char}}/{{User}} 等大小写变体，不必再各写一条。
    .replace(/\{\{\s*char\s*\}\}/gi, char)
    .replace(/\{\{\s*user\s*\}\}/gi, user)
    .replace(/\{\{\s*sub\s*\}\}/gi, "他/她")
    .replace(/\{\{\s*obj\s*\}\}/gi, "他/她")
    .replace(/\{\{\s*poss\s*\}\}/gi, "的")
    // <START> 或 <start> 转换为直观规范的对白范例标记
    .replace(/<START>/gi, "\n\n--- 【示例对白开始】 ---\n\n");
}
