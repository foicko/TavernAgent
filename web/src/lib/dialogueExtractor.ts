import type { TextBlock } from "../app/types";

export interface ExtractTTSContentOptions {
  dialogueOnly?: boolean;
}

export interface ExtractedTTSContent {
  text: string;
  hasDialogue: boolean;
  originalText: string;
}

/**
 * 正则匹配中英文小说/对白常见引号：
 * 1. 「...」 角标引号
 * 2. “...” 弯双引号
 * 3. 『...』 双角标引号
 * 4. "..." 直双引号
 */
const DIALOGUE_QUOTE_REGEX = /[「“『]([^」”』]+)[」”』]|"([^"\r\n]+)"/g;

/**
 * 从消息结构（TextBlock 数组或备用纯文本）中提取要朗读的文本。
 * 当 dialogueOnly 为 true（默认）时，过滤掉旁白 (narration) 与内心独白 (inner_monologue)，仅保留角色说的话。
 */
export function extractTTSContent(
  source: { blocks?: TextBlock[]; text?: string },
  options: ExtractTTSContentOptions = { dialogueOnly: true }
): ExtractedTTSContent {
  const dialogueOnly = options.dialogueOnly ?? true;

  // 1. 如果有结构化的 blocks，按 kind 精确过滤
  if (source.blocks && source.blocks.length > 0) {
    const originalText = source.blocks.map((b) => b.text).join("\n").trim();
    const hasDialogue = source.blocks.some((b) => b.kind === "dialogue");

    if (dialogueOnly) {
      const dialogueBlocks = source.blocks.filter((b) => b.kind === "dialogue");
      const text = dialogueBlocks
        .map((b) => b.text.trim())
        .filter(Boolean)
        .join("\n");
      return {
        text,
        hasDialogue: text.length > 0,
        originalText,
      };
    }

    return {
      text: originalText,
      hasDialogue,
      originalText,
    };
  }

  // 2. 纯文本回退（如开场白字符串、无 blocks 的普通文本）
  const originalText = (source.text ?? "").trim();
  if (!originalText) {
    return {
      text: "",
      hasDialogue: false,
      originalText: "",
    };
  }

  if (dialogueOnly) {
    const dialogues: string[] = [];
    let match: RegExpExecArray | null;
    const regex = new RegExp(DIALOGUE_QUOTE_REGEX.source, DIALOGUE_QUOTE_REGEX.flags);

    while ((match = regex.exec(originalText)) !== null) {
      const content = match[1] ?? match[2];
      if (content && content.trim()) {
        dialogues.push(content.trim());
      }
    }

    if (dialogues.length > 0) {
      return {
        text: dialogues.join("\n"),
        hasDialogue: true,
        originalText,
      };
    }

    // 未能匹配到任何引号对白，视为纯旁白/描写文本
    return {
      text: "",
      hasDialogue: false,
      originalText,
    };
  }

  return {
    text: originalText,
    hasDialogue: true,
    originalText,
  };
}
