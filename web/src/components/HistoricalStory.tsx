import React, { memo, useState } from "react";
import type { OptionView, StoryMessage, TurnPhase } from "../stores/storyTypes";
import { renderInlineMarkdown } from "../lib/inlineMarkdown";
import { DEFAULT_PLAYER_NAME, replaceMacros } from "../lib/characterMacros";
import { useStory } from "../stores/storyStore";
import { TurnActions } from "./TurnActions";
import { TurnEvidence } from "./TurnEvidence";
import { TTSPlayButton } from "./TTSButton";
import { TurnIllustration } from "./TurnIllustration";
import { IllustrationModal } from "./IllustrationModal";
import "./HistoricalStory.css";

/** 历史已提交回合的思考过程折叠组件 */
const CommittedThinkingSection: React.FC<{ thinking: string }> = ({ thinking }) => {
  const [open, setOpen] = useState(false);
  return (
    <div className={`thinking-container committed ${open ? "is-expanded" : "is-collapsed"}`}>
      <div className="thinking-toggle-bar" role="button" tabIndex={0} aria-expanded={open}
        onClick={() => setOpen((prev) => !prev)}
        onKeyDown={event => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); setOpen(prev => !prev); } }}>
        <div className="thinking-bar-main">
          <span className="thinking-icon">💭</span>
          <span className="thinking-title">思考过程 (Reasoning)</span>
        </div>
        <span className="thinking-expand-hint">
          {open ? "收起 ▲" : "展开 ▼"}
        </span>
      </div>
      {open && (
        <div className="thinking-content-body">
          {thinking}
        </div>
      )}
    </div>
  );
};


interface HistoricalStoryProps {
  storyMessages: StoryMessage[];
  characterName: string;
  phase: TurnPhase;
  viewMode: string;
  handleCopy: (text: string) => Promise<void>;
  handleQuote: (speaker: string, text: string) => void;
  chooseOption: (option: OptionView) => Promise<void>;
}

// Stable history is independent of live draft tokens and thinking deltas.
export const HistoricalStory = memo(function HistoricalStory({ storyMessages, characterName, phase, viewMode, handleCopy, handleQuote, chooseOption }: HistoricalStoryProps) {
  // 与实时流共用同一套宏替换：历史回合里同样不该出现 {{char}}/{{user}}。
  const playerName = useStory((s) => s.view?.state?.characters?.["player"]?.name) || DEFAULT_PLAYER_NAME;
  const expand = (text?: string) => replaceMacros(text ?? "", { charName: characterName, userName: playerName });

  const [illustrationTarget, setIllustrationTarget] = useState<{
    turnId: string;
    blocks?: StoryMessage["blocks"];
    fallbackText?: string;
  } | null>(null);

  return <>{storyMessages.map((msg, idx) => (
            <div key={msg.id} className={`story-turn ${msg.role === "opening" ? "opening-turn" : ""}`}>
              {/* 开场序章节点 */}
              {msg.role === "opening" ? (
                <div className="dialogue-turn-wrap">
                  <div className="turn-quick-toolbar">
                    <TTSPlayButton blocks={msg.blocks} fallbackText={msg.blocks.map((b) => b.text).join("\n")} />
                    <button
                      type="button"
                      className="turn-tool-btn"
                      onClick={() => setIllustrationTarget({ turnId: msg.id, blocks: msg.blocks, fallbackText: msg.blocks.map((b) => b.text).join("\n") })}
                      title="生成本幕场景插画"
                    >
                      🎨 生图
                    </button>
                    <button
                      className="turn-tool-btn"
                      onClick={() => handleCopy(msg.blocks.map((b) => b.text).join("\n"))}
                      title="复制文学正文"
                    >
                      ⎘ 复制
                    </button>
                  </div>
                  <div className="dialogue-meta-header">
                    <div className="speaker-label">
                      <span>✦ 命运序章 · 场景起步</span>
                    </div>
                    <span className="turn-time-stamp">开场白</span>
                  </div>
                  <div className="dialogue-content-text">
                    {msg.blocks.map((b, bi) => (
                      <p key={bi} className="block-narration">
                        {b.text && renderInlineMarkdown(expand(b.text))}
                      </p>
                    ))}
                  </div>
                  <TurnIllustration
                    turnId={msg.id}
                    onOpenModal={() => setIllustrationTarget({ turnId: msg.id, blocks: msg.blocks, fallbackText: msg.blocks.map((b) => b.text).join("\n") })}
                  />
                </div>
              ) : (
                /* 回合节点：玩家输入卡 + 角色文学回复卡 */
                <>
                  {msg.inputText && (
                    <div className="player-turn-container">
                      <div className="player-bubble">
                        <div className="player-bubble-header">
                          <span className="player-id-badge">✦ 玩家行动 · 第一人称</span>
                          <span className="player-turn-time">回合 #{msg.turnNumber ?? idx}</span>
                        </div>
                        <div className="player-speech-text">{renderInlineMarkdown(expand(msg.inputText))}</div>
                      </div>
                    </div>
                  )}

                  <div className="dialogue-turn-wrap">
                    <div className="turn-quick-toolbar">
                      <TTSPlayButton blocks={msg.blocks} fallbackText={msg.blocks.map((b) => b.text).join("\n")} />
                      <button
                        type="button"
                        className="turn-tool-btn"
                        onClick={() => setIllustrationTarget({ turnId: msg.id, blocks: msg.blocks, fallbackText: msg.blocks.map((b) => b.text).join("\n") })}
                        title="生成本幕场景插画"
                      >
                        🎨 生图
                      </button>
                      <button
                        className="turn-tool-btn"
                        onClick={() => handleQuote(characterName, msg.blocks.map((b) => b.text).join("\n"))}
                        title="引用此句并带入输入框"
                      >
                        ❝ 引用
                      </button>
                      <button
                        className="turn-tool-btn"
                        onClick={() => handleCopy(msg.blocks.map((b) => b.text).join("\n"))}
                        title="复制文学正文"
                      >
                        ⎘ 复制
                      </button>

                    </div>
                    <div className="dialogue-meta-header">
                      <div className="speaker-label">
                        <span>{characterName}</span>
                      </div>
                      <span className="turn-time-stamp">回合 #{msg.turnNumber ?? idx}</span>
                    </div>
                    {msg.thinking && <CommittedThinkingSection thinking={msg.thinking} />}
                    <div className="dialogue-content-text">
                      {msg.blocks.map((b, bi) => {
                        if (b.kind === "dialogue") {
                          return (
                            <p key={bi} className="block-dialogue">
                              「{renderInlineMarkdown(expand(b.text.replace(/^[「"']|[」"']$/g, "")))}」
                            </p>
                          );
                        }
                        if (b.kind === "inner_monologue") {
                          return (
                            <p
                              key={bi}
                              className="block-monologue"
                            >
                              （{renderInlineMarkdown(expand(b.text.replace(/^[（(]|[）)]$/g, "")))}）
                            </p>
                          );
                        }
                        return (
                          <p key={bi} className="block-narration">
                            {renderInlineMarkdown(expand(b.text))}
                          </p>
                        );
                      })}
                    </div>

                    <TurnIllustration
                      turnId={msg.id}
                      onOpenModal={() => setIllustrationTarget({ turnId: msg.id, blocks: msg.blocks, fallbackText: msg.blocks.map((b) => b.text).join("\n") })}
                    />

                    {/* 本轮依据：检定 / 状态变化 / 参考记忆（为什么这样演） */}
                    {!msg.draft && <TurnEvidence msg={msg} />}

                    {/* 回合微操作与候选切换条 (技术契约 §7) */}
                    {!msg.draft && <TurnActions msg={msg} />}
                    {/* 最新回合意图抉择卡 */}
                    {idx === storyMessages.length - 1 &&
                      phase === "idle" &&
                      msg.options &&
                      msg.options.length > 0 &&
                      viewMode === "studio" && (
                        <div className="decision-deck-box">
                          <div className="decision-deck-header">
                            <span>✦ 剧情抉择分支（点击直接推进，或填入输入框）</span>
                            <span>多意图推演</span>
                          </div>
                          <div className="decision-grid choice-grid">
                            {msg.options.map((opt) => {
                              const intentBadges: Record<string, { label: string; icon: string; border: string }> = {
                                aggressive: { label: "强势威慑", icon: "⚔️", border: "#e57373" },
                                clever: { label: "机智试探", icon: "💡", border: "#64b5f6" },
                                emotional: { label: "共情抚慰", icon: "🤝", border: "#81c784" },
                                chaotic: { label: "出其不意", icon: "🎲", border: "#ba68c8" },
                              };
                              const b = intentBadges[opt.intent] || {
                                label: opt.intent,
                                icon: "✦",
                                border: "var(--border-mid)",
                              };
                              return (
                                <div
                                  key={opt.optionId}
                                  className="decision-card-btn"
                                  role="button"
                                  tabIndex={0}
                                  onClick={() => void chooseOption(opt)}
                                  onKeyDown={event => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); void chooseOption(opt); } }}
                                  style={{ "--decision-accent": b.border } as React.CSSProperties}
                                  title="点击推进该抉择"
                                >
                                  <div>
                                    <span>
                                      {b.icon} {b.label}
                                    </span>
                                    <span>推进 ↵</span>
                                  </div>
                                  <div>
                                    {opt.text}
                                  </div>
                                </div>
                              );
                            })}
                          </div>
                        </div>
                      )}
                  </div>
                </>
              )}
            </div>
          ))}
          {illustrationTarget && (
            <IllustrationModal
              open={Boolean(illustrationTarget)}
              onClose={() => setIllustrationTarget(null)}
              turnId={illustrationTarget.turnId}
              blocks={illustrationTarget.blocks}
              fallbackText={illustrationTarget.fallbackText}
              characterName={characterName}
            />
          )}
        </>;
});
