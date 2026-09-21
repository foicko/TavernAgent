import React, { useEffect, useState } from "react";
import type { TextBlock } from "../app/types";
import { extractTTSContent } from "../lib/dialogueExtractor";
import { ttsPlayer } from "../lib/ttsPlayer";
import { useTTSSettings } from "../stores/ttsSettingsStore";

export interface TTSPlayButtonProps {
  /** 结构化回合块（优先） */
  blocks?: TextBlock[];
  /** 纯文本回退（如开场白或无 blocks 的文本） */
  fallbackText?: string;
  /** 兼容旧版调用的 text 字段 */
  text?: string;
  voice?: string;
  className?: string;
}

export const TTSPlayButton: React.FC<TTSPlayButtonProps> = ({
  blocks,
  fallbackText,
  text,
  voice,
  className = "",
}) => {
  const [isPlaying, setIsPlaying] = useState(false);
  const [noDialogueNotice, setNoDialogueNotice] = useState(false);
  const dialogueOnly = useTTSSettings((s) => s.dialogueOnly);

  const rawText = fallbackText ?? text ?? "";
  const extracted = extractTTSContent({ blocks, text: rawText }, { dialogueOnly });
  const playTargetText = extracted.text.trim();

  useEffect(() => {
    return ttsPlayer.subscribe((cur) => {
      setIsPlaying(Boolean(playTargetText && cur === playTargetText));
    });
  }, [playTargetText]);

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();

    if (isPlaying) {
      ttsPlayer.stop();
      return;
    }

    if (dialogueOnly && !extracted.hasDialogue) {
      setNoDialogueNotice(true);
      setTimeout(() => setNoDialogueNotice(false), 2200);
      return;
    }

    if (playTargetText) {
      void ttsPlayer.play(playTargetText, voice);
    }
  };

  let title = isPlaying ? "停止朗读" : dialogueOnly ? "朗读角色台词" : "朗读正文";
  let label = isPlaying ? "⏹ 停止" : "🔊 朗读";

  if (noDialogueNotice) {
    label = "🔇 仅旁白";
    title = "本段无角色对白（已在语音设置中开启“只读角色台词”）";
  } else if (!isPlaying && dialogueOnly && !extracted.hasDialogue) {
    title = "本段仅有旁白，无角色对白（只读台词模式）";
  }

  return (
    <button
      type="button"
      className={`turn-tool-btn tts-tool-btn ${isPlaying ? "is-playing" : ""} ${noDialogueNotice ? "no-dialogue-notice" : ""} ${className}`.trim()}
      onClick={handleClick}
      title={title}
      aria-label={title}
    >
      {label}
    </button>
  );
};
