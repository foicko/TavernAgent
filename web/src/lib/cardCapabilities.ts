// 把卡片声明的特性（RisuAI/charx 或原生 .tacard）汇总成一句人话，用于导入预览的兼容报告。
import type { CardCapabilities } from "../app/types";

export function describeCapabilities(caps?: CardCapabilities): string {
  if (!caps) return "";
  const parts: string[] = [];
  if (caps.hasModule) parts.push("内嵌模块（不执行）");
  if (caps.hasBackgroundHTML) parts.push("自定义主题");
  if (caps.hasDefaultVariables) parts.push("预置变量");
  if (caps.hasDepthPrompt) parts.push("深度提示");
  if (caps.viewScreen && caps.viewScreen !== "none") parts.push(`视图：${caps.viewScreen}`);
  const families = caps.markerFamilies ?? [];
  if (families.length > 0) parts.push(`状态标记：${families.join("、")}`);
  else if (caps.imgCmd) parts.push("图片指令");
  if (caps.imageAssetCount) parts.push(`图片 ${caps.imageAssetCount}`);
  if (caps.audioAssetCount) parts.push(`音频 ${caps.audioAssetCount}`);
  if (caps.lorebookEntries) parts.push(`世界书 ${caps.lorebookEntries}`);
  if (caps.lowLevelAccess) parts.push("⚠ 低层访问");
  if (parts.length === 0) return "";
  const label = caps.source === "tacard" ? "原生卡特性" : "RisuAI 特性";
  return `${label}：${parts.join(" · ")}`;
}
