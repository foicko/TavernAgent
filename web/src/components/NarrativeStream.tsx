import React, { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useUi } from "../stores/uiStore";
import { isTurnBusy, storyBusy, useStory } from "../stores/storyStore";
import { characterPresentation } from "../lib/characterPresentation";
import { renderInlineMarkdown } from "../lib/inlineMarkdown";
import { DEFAULT_PLAYER_NAME, replaceMacros } from "../lib/characterMacros";
import { HistoricalStory } from "./HistoricalStory";
import { Button } from "../ui/Button";
import { useShallow } from "zustand/react/shallow";
import "./NarrativeStream.css";

export const NarrativeStream: React.FC = () => {
  const {
    viewMode,
    switchViewMode,
    activeCharKey,
    toggleRightRail,
    openLeftTabAndExpand,
    injectComposerText,
    showLorePop,
    hideLorePop,
    notifyQuiet,
  } = useUi(useShallow(s => ({ viewMode: s.viewMode, switchViewMode: s.switchViewMode, activeCharKey: s.activeCharKey, toggleRightRail: s.toggleRightRail, openLeftTabAndExpand: s.openLeftTabAndExpand, injectComposerText: s.injectComposerText, showLorePop: s.showLorePop, hideLorePop: s.hideLorePop, notifyQuiet: s.notifyQuiet })));

  const {
    view,
    messages: storyMessages,
    phase,
    error,
    draftBlocks,
    thinkingText,
    lastInput,
    chooseOption,
    continueTurn,
    retry,
    regenerate,
    cancel,
    viewNodeId,
    backToHead,
    forkFrom,
    hasMoreHistory,
    loadOlder,
    loadingOlder,
    sessionLoading,
    pendingSessionId,
  } = useStory(useShallow(s => ({ view: s.view, messages: s.messages, phase: s.phase, error: s.error, draftBlocks: s.draftBlocks, thinkingText: s.thinkingText, lastInput: s.lastInput, chooseOption: s.chooseOption, continueTurn: s.continueTurn, retry: s.retry, regenerate: s.regenerate, cancel: s.cancel, viewNodeId: s.viewNodeId, backToHead: s.backToHead, forkFrom: s.forkFrom, hasMoreHistory: s.hasMoreHistory, loadOlder: s.loadOlder, loadingOlder: s.loadingOlder, sessionLoading: s.sessionLoading, pendingSessionId: s.pendingSessionId })));

  const char = characterPresentation(activeCharKey, view);
  // 导入卡的开场与正文普遍写 {{char}}/{{user}}：原样显示会把宏直接摊在故事里。
  const playerName = view?.state?.characters?.["player"]?.name || DEFAULT_PLAYER_NAME;
  const expand = (text?: string) => replaceMacros(text ?? "", { charName: char.shortName || char.name, userName: playerName });
  const scrollRef = useRef<HTMLDivElement>(null);
  const [thinkingOpen, setThinkingOpen] = useState(false);

  // 当进入新回合生成阶段时，确保思考栏默认保持单行精简展示
  useEffect(() => {
    if (phase === "generating") {
      setThinkingOpen(false);
    }
  }, [phase]);
  const latestTurn = useMemo(() => [...storyMessages].reverse().find(m => m.role === "assistant"), [storyMessages]);
  const wordCount = useMemo(() => storyMessages.reduce((count, m) => count + [...m.blocks.map(b => b.text).join("")].length, 0), [storyMessages]);
  const busy = useStory(storyBusy);
  const activeTurnNumber = (view?.headNode?.turnNumber ?? storyMessages.length) + 1;
  const currentSessionId = view?.sessionId;
  const isWaitingColdSession = sessionLoading && (!view || (pendingSessionId && view.sessionId !== pendingSessionId));
  const lastSessionIdRef = useRef<string | null>(null);

  // 打开会话或切换会话时：瞬间直接定位到底部，坚决杜绝从顶部向下滑动的动画
  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    if (currentSessionId && currentSessionId !== lastSessionIdRef.current) {
      lastSessionIdRef.current = currentSessionId;
      el.scrollTop = el.scrollHeight;
      if (typeof el.scrollTo === "function") {
        el.scrollTo({ top: el.scrollHeight, behavior: "instant" });
      }
      requestAnimationFrame(() => {
        if (scrollRef.current) {
          scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
        }
      });
    }
  }, [currentSessionId, storyMessages.length]);

  // 当故事内容或流式状态变化时自动滚动至最新
  useEffect(() => {
    if (loadingOlder) return;
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [storyMessages, draftBlocks, phase, thinkingText, lastInput, loadingOlder]);

  // 绑定世界书词条悬停监听
  useEffect(() => {
    const streamEl = scrollRef.current;
    if (!streamEl) return;

    const handleMouseEnter = (e: MouseEvent) => {
      const target = (e.target as HTMLElement).closest(".lore-term");
      if (!target) return;
      const title = target.getAttribute("data-lore-title") || target.textContent || "";
      const desc = target.getAttribute("data-lore-desc") || "世界书专有名词档案";
      const keys = target.getAttribute("data-lore-keys") || title;
      const rect = target.getBoundingClientRect();
      const popX = Math.max(16, Math.min(window.innerWidth - 300, rect.left));
      const popY = rect.top > 180 ? rect.top - 140 : rect.bottom + 10;
      showLorePop(title, desc, keys, popX, popY);
    };

    const handleMouseLeave = (e: MouseEvent) => {
      const target = (e.target as HTMLElement).closest(".lore-term");
      if (target) {
        setTimeout(hideLorePop, 260);
      }
    };

    streamEl.addEventListener("mouseover", handleMouseEnter);
    streamEl.addEventListener("mouseout", handleMouseLeave);

    return () => {
      streamEl.removeEventListener("mouseover", handleMouseEnter);
      streamEl.removeEventListener("mouseout", handleMouseLeave);
    };
  }, [showLorePop, hideLorePop]);

  const handleCopy = useCallback(async (text: string) => {
    try { await navigator.clipboard.writeText(text); notifyQuiet("已将正文复制到剪贴板"); }
    catch { notifyQuiet("复制失败，请手动选择正文"); }
  }, [notifyQuiet]);

  const handleQuote = useCallback((speaker: string, text: string) => {
    injectComposerText(`> ${speaker}：「${text}」\n\n`);
    notifyQuiet("已将原话转换为引文前置到输入框");
  }, [injectComposerText, notifyQuiet]);

  return (
    <>
      {/* Floating Quick Restore Button for Right Rail (when folded) */}
      <button
        className="floating-rail-trigger-right"
        id="floating-trigger-right"
        onClick={toggleRightRail}
        title="展开角色立绘与状态档案 (Ctrl+])"
      >
        <span className="floating-trigger-arrow">‹</span>
        {char.avatar ? (
          <img className="floating-trigger-avatar-img" src={char.avatar} alt={char.name} />
        ) : (
          <span className="floating-trigger-avatar">🎭</span>
        )}
        <span className="floating-trigger-name" id="floating-trigger-name">
          {char.shortName} 档案
        </span>
      </button>

      {/* Quiet Scene Telemetry Bar */}
      <div className="scene-bar">
        <div className="character-presence-deck">
          <img
            id="stage-avatar"
            src={char.avatar}
            alt={char.name}
            className="char-avatar-frame"
            onClick={() => openLeftTabAndExpand("characters")}
            title="点击切换对话角色或查看卡片"
          />
          <div className="char-presence-info">
            <div className="char-name-line">
              <span
                className="char-main-name"
                id="stage-char-name"
                onClick={() => openLeftTabAndExpand("characters")}
              >
                {char.shortName}
              </span>
              <span className="quiet-state-pill">{char.stateBadge}</span>
              <span className="quiet-state-pill">{char.voiceBadge}</span>
            </div>
            <div
              className="scene-sub-telemetry"
              id="stage-scene-telemetry"
              dangerouslySetInnerHTML={{ __html: char.telemetryHtml }}
              onClick={(e) => {
                // 事件委托：telemetryHtml 中的 data-open-tab 链接（静态可信预设，
                // 无用户内容注入面），打开对应左栏子页签。
                const tab = (e.target as HTMLElement).closest<HTMLElement>("[data-open-tab]")?.dataset.openTab;
                if (tab) openLeftTabAndExpand(tab as Parameters<typeof openLeftTabAndExpand>[0]);
              }}
            />
          </div>
        </div>

        <div className="scene-bar-actions">
          <button className="btn-quiet" onClick={() => openLeftTabAndExpand("characters")}>
            <span>切换角色</span>
          </button>
          <button className="btn-quiet" disabled={!latestTurn || busy} onClick={() => latestTurn && void regenerate(latestTurn.id)}>
            <span>重新讲述此轮</span>
          </button>
        </div>
      </div>

      {/* Narrative Flow (纯净小说流) */}
      <div className="stage-scroll-area" id="narrative-stream" ref={scrollRef}>
        {/* 纯净文学编年模式专属雅致题头 */}
        <div className="reading-mode-banner">
          <div className="reading-book-title">《{char.name} · 编年篇章》</div>
          <div className="reading-mode-desc">纯净文学编年模式已激活 · 专注文学阅读与剧情演义</div>
          <div className="reading-meta-stats">
            <span>已载入 {wordCount.toLocaleString()} 字</span>
            <span>•</span>
            <span>预估读毕 {Math.max(1, Math.ceil(wordCount / 450))} 分钟</span>
            <span>•</span>
            <span>第 {view?.headNode.turnNumber ?? 0} 轮</span>
          </div>
          <button className="reading-exit-pill" onClick={() => switchViewMode("studio")}>
            ← 返回跑团工作台 (Esc)
          </button>
        </div>

        {/* 若有更早的剧情历史（万级节点分页窗口），显示向上加载按钮 */}
        {view && hasMoreHistory && (
          <div>
            <button
              className="btn-quiet"
              onClick={() => void loadOlder()}
              disabled={loadingOlder}
            >
              {loadingOlder ? "正在加载更早历史..." : "↑ 查看更早的剧情历史"}
            </button>
          </div>
        )}

        {/* 历史快照只读浏览提示条 */}
        {view && viewNodeId && (
          <div className="history-read-banner">
            <span>📍 正在浏览历史快照节点（只读模式）</span>
            <div>
              <button className="probe-btn" onClick={() => void backToHead()}>
                返回最新进度
              </button>
              <button
                className="probe-btn primary"
                onClick={() => {
                  void forkFrom(viewNodeId, "平行分支_" + Date.now().toString().slice(-4));
                }}
              >
                🌿 从此处开辟新分支
              </button>
            </div>
          </div>
        )}

        {/* 1. 真实后端会话的故事流渲染 */}
        {view && storyMessages.length > 0 ? (
          <HistoricalStory storyMessages={storyMessages} characterName={char.shortName} phase={phase} viewMode={viewMode}
            handleCopy={handleCopy} handleQuote={handleQuote} chooseOption={chooseOption} />
        ) : isWaitingColdSession ? (
          /* 2. 切换未命中缓存的会话时的骨架屏 (消除突兀白屏或错位序幕卡闪烁) */
          <div className="narrative-loading-stage" data-testid="narrative-loading-skeleton">
            <div className="narrative-skeleton-card player-skeleton">
              <div className="skeleton-header">
                <div className="skeleton-line pulse" />
              </div>
              <div className="skeleton-body">
                <div className="skeleton-line pulse" />
              </div>
            </div>
            <div className="narrative-skeleton-card assistant-skeleton">
              <div className="skeleton-header">
                <div className="skeleton-avatar pulse" />
                <div className="skeleton-line pulse" />
              </div>
              <div className="skeleton-body">
                <div className="skeleton-line pulse" />
                <div className="skeleton-line pulse" />
                <div className="skeleton-line pulse" />
              </div>
            </div>
          </div>
        ) : (
          /* 3. 初始开场序幕卡片 (全新剧场就绪，无假对话) */
          <div className="story-turn initial-prologue-turn">
            <div className="dialogue-turn-wrap prologue-card">
              <div className="dialogue-meta-header">
                <div className="speaker-label">
                  <img src={char.avatar} alt={char.name} />
                  <div>
                    <div>{char.name}</div>
                    <div>{char.role}</div>
                  </div>
                </div>
                <span className="turn-time-stamp">
                  开场序幕 · 就绪
                </span>
              </div>

              <div className="dialogue-content-text">
                {renderInlineMarkdown(expand(char.prologue))}
              </div>

              <div className="prologue-guide-box">
                <div>
                  <span>🧭</span>
                  <span>互动起手引导</span>
                </div>
                <div>
                  暂无历史对话记录。你可以在下方输入框键入第一句对白或动作描述（如 <em>*环顾四周*</em> 或直接开口），由大模型推演真实因果与角色心境；亦可点击输入框上方的快捷动作宏直接起手。
                </div>
              </div>
            </div>
          </div>
        )}

        {/* 3. 活跃流式演义回合（玩家即时回显 + 角色渐进式回复） */}
        {(phase === "generating" || phase === "committed" || draftBlocks.length > 0 || (isTurnBusy(phase) && !!lastInput?.text)) && (
          <div className="story-turn active-streaming-turn">
            {/* 玩家输入即时卡片 (零延迟上屏) */}
            {lastInput?.text && (
              <div className="player-turn-container">
                <div className="player-bubble active-player-bubble">
                  <div className="player-bubble-header">
                    <span className="player-id-badge">✦ 玩家行动 · 第一人称</span>
                    <span className="player-turn-time">
                      回合 #{activeTurnNumber}
                      {phase === "generating" && <span className="player-sending-tag">（实时推演中）</span>}
                    </span>
                  </div>
                  {lastInput.actionRef && (
                    <div className="dice-badge-quiet">
                      🎲 行动检定：{view?.actions?.find((a) => a.actionId === lastInput.actionRef)?.label || lastInput.actionRef}
                    </div>
                  )}
                  <div className="player-speech-text">{renderInlineMarkdown(expand(lastInput.text))}</div>
                </div>
              </div>
            )}

            {/* 角色演义回复卡片 (统一结构与视觉语言，杜绝闪烁跳变) */}
            <div className="dialogue-turn-wrap is-streaming">
              <div className="dialogue-meta-header">
                <div className="speaker-label">
                  <span>{char.shortName}</span>
                  <span className="speaker-delivery">
                    {phase === "committed" ? "（回复已完成，正在同步选项…）" : "（大模型正在实时演义中... ⚡）"}
                  </span>
                </div>
                <span className="turn-time-stamp">
                  {phase === "committed" ? "同步中" : "生成中"}
                </span>
              </div>
              {/* 思维链实时思考展示（针对 DeepSeek-R1 / o1 等推理模型） */}
              {thinkingText && (
                <div className={`thinking-container ${thinkingOpen ? "is-expanded" : "is-collapsed"}`}>
                  <div
                    className="thinking-toggle-bar"
                    onClick={() => setThinkingOpen((prev) => !prev)}
                    title={thinkingOpen ? "点击折叠思考过程" : "点击展开完整思考过程"}
                  >
                    <div className="thinking-bar-main">
                      <span className="thinking-icon">💭</span>
                      <span className="thinking-title">
                        {draftBlocks.length === 0 || draftBlocks.every((b) => !b.text)
                          ? "深度推演与因果思索中..."
                          : "思考过程 (Reasoning)"}
                      </span>
                      {!thinkingOpen && (
                        <span className="thinking-ticker-text">
                          {thinkingText}
                        </span>
                      )}
                    </div>
                    <span className="thinking-expand-hint">
                      {thinkingOpen ? "收起 ▲" : "展开 ▼"}
                    </span>
                  </div>
                  {thinkingOpen && (
                    <div className="thinking-content-body">
                      {thinkingText}
                    </div>
                  )}
                </div>
              )}

              <div className="dialogue-content-text">
                {draftBlocks.length === 0 || draftBlocks.every((b) => !b.text) ? (
                  <p>
                    {thinkingText ? "正在整理思绪并撰写演义正文... ⚡" : "正在推演因果走势与角色心境... ⚡"}
                  </p>
                ) : (
                  draftBlocks.map((b, bi) => {
                    if (b.kind === "dialogue") {
                      return (
                        <p key={bi} className="block-dialogue">
                          「{renderInlineMarkdown(expand(b.text.replace(/^[「"']|[」"']$/g, "")))}」
                        </p>
                      );
                    }
                    if (b.kind === "inner_monologue") {
                      return (
                        <p key={bi} className="block-monologue">
                          （{renderInlineMarkdown(expand(b.text.replace(/^[（(]|[）)]$/g, "")))}）
                        </p>
                      );
                    }
                    return (
                      <p key={bi} className="block-narration">
                        {renderInlineMarkdown(expand(b.text))}
                      </p>
                    );
                  })
                )}
                <span className="typing-cursor" />
              </div>
            </div>
          </div>
        )}

        {/* 4. 截断续写控制栏 (truncated 状态) */}
        {phase === "truncated" && (
          <div className="story-turn truncated-turn">
            <div className="decision-deck-box">
              <div className="decision-deck-header">
                <span>
                  ⚡ 模型输出因上下文长度截断，等待确认
                </span>
                <span>状态机：awaiting_continuation</span>
              </div>
              <p>
                大模型已生成当前叙事片段。你可以让剧情顺滑续讲、重新发起本轮尝试、或放弃当前生成。
              </p>
              <div>
                <button
                  className="btn-primary-send"
                  onClick={() => void continueTurn()}
                >
                  ▶ 继续讲下去 (Continue)
                </button>
                <button
                  className="probe-btn"
                  onClick={() => void retry()}
                >
                  🔄 重新构思此轮 (Retry)
                </button>
                <button className="probe-btn" onClick={() => void cancel()}>
                  ✕ 放弃
                </button>
              </div>
            </div>
          </div>
        )}

        {/* 5. 错误提示卡 */}
        {(phase === "failed" || error) && (
          <div className="story-turn error-turn">
            <div className="decision-deck-box">
              <div className="error-turn__row">
                <span className="error-turn__icon" aria-hidden="true">✕</span>
                <div className="error-turn__text">
                  <span className="error-turn__title">生成受阻</span>
                  <p className="error-turn__detail">{error || "网络通信异常"}</p>
                </div>
                <Button
                  className="error-turn__retry"
                  variant="danger"
                  size="sm"
                  onClick={() => void retry()}
                >
                  重试
                </Button>
              </div>
            </div>
          </div>
        )}
      </div>
    </>
  );
};
