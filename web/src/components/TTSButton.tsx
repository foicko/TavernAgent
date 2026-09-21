import React, { useEffect, useState } from "react";
import type { TextBlock } from "../app/types";
import { characterPresentation } from "../lib/characterPresentation";
import { primaryCharacterId } from "../lib/characterState";
import { extractTTSContent } from "../lib/dialogueExtractor";
import {
  buildDirectorInstruction,
  enrichDialogueWithAudioTags,
  isMiMoEngine,
  type DirectorContext,
} from "../lib/ttsDirector";
import { ttsPlayer } from "../lib/ttsPlayer";
import { useStory } from "../stores/storyStore";
import { useTTSSettings } from "../stores/ttsSettingsStore";
import { useUi } from "../stores/uiStore";

export interface TTSPlayButtonProps {
  /** 结构化回合块（优先） */
  blocks?: TextBlock[];
  /** 纯文本回退（如开场白或无 blocks 的文本） */
  fallbackText?: string;
  /** 兼容旧版调用的 text 字段 */
  text?: string;
  voice?: string;
  className?: string;
  /** 可选：特定回合的心境覆盖（如已持久化的回合情绪） */
  mood?: { moodCode?: string; text?: string };
}

export const TTSPlayButton: React.FC<TTSPlayButtonProps> = ({
  blocks,
  fallbackText,
  text,
  voice,
  className = "",
  mood,
}) => {
  const [isPlaying, setIsPlaying] = useState(false);
  const [noDialogueNotice, setNoDialogueNotice] = useState(false);

  const view = useStory((s) => s.view);
  const hud = useStory((s) => s.hud);
  const activeCharKey = useUi((s) => s.activeCharKey);
  const tachieExpression = useUi((s) => s.tachieExpression);

  const dialogueOnly = useTTSSettings((s) => s.dialogueOnly);
  const directorMode = useTTSSettings((s) => s.directorMode);
  const engine = useTTSSettings((s) => s.engine);
  const customModel = useTTSSettings((s) => s.customModel);
  const customBaseUrl = useTTSSettings((s) => s.customBaseUrl);

  const rawText = fallbackText ?? text ?? "";
  const extracted = extractTTSContent({ blocks, text: rawText }, { dialogueOnly });
  const playTargetText = extracted.text.trim();

  useEffect(() => {
    return ttsPlayer.subscribe((cur) => {
      if (!playTargetText || !cur) {
        setIsPlaying(false);
        return;
      }
      setIsPlaying(
        cur === playTargetText ||
          cur.includes(playTargetText) ||
          playTargetText.includes(cur)
      );
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

    if (!playTargetText) return;

    const isMiMo = isMiMoEngine({ engine, customModel, customBaseUrl });
    if (directorMode && isMiMo) {
      const char = characterPresentation(activeCharKey, view);
      const characterId =
        primaryCharacterId(hud, activeCharKey) ||
        primaryCharacterId(view?.state ?? null, activeCharKey);

      // 提取真实大模型判定的实时心境与关系量表
      const liveMood =
        mood ||
        (hud && characterId ? hud.moods[characterId] : undefined) ||
        (view?.state && characterId ? view.state.moods[characterId] : undefined);

      const liveRel =
        (hud && characterId ? hud.relationships[characterId] : undefined) ||
        (view?.state && characterId ? view.state.relationships[characterId] : undefined);

      // 提取真实场景信息，杜绝捏造臆测
      const sceneLocation =
        view?.state?.scene?.title || view?.outline?.scene || undefined;
      const sceneStage = view?.director?.currentBeatTitle || undefined;
      const turnNarration = blocks
        ? blocks
            .filter((b) => b.kind === "narration")
            .map((b) => b.text)
            .join(" ")
        : "";

      const directorCtx: DirectorContext = {
        characterName: char.name || char.shortName || "当前角色",
        characterRole: char.role || undefined,
        characterPersona:
          char.dossier?.personality ||
          char.dossier?.description ||
          char.modalDesc ||
          undefined,
        characterTags: char.dossier?.tags,
        sceneLocation,
        sceneStage,
        moodText: liveMood?.text,
        moodCode: liveMood?.moodCode || tachieExpression,
        relationship: liveRel
          ? {
              affection: liveRel.affection,
              trust: liveRel.trust,
              alertness: liveRel.alertness,
            }
          : undefined,
        turnNarration,
      };

      const enrichedText = enrichDialogueWithAudioTags(playTargetText, directorCtx);
      const instruction = buildDirectorInstruction(directorCtx);
      void ttsPlayer.play(enrichedText, { voice, instruction });
    } else {
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
