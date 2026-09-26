// ContextMetricsPopover: 实时上下文窗口统计（仪表按钮 + 锚定浮层明细）
import React, { useState, useRef, useMemo } from "react";
import { useStory } from "../stores/storyStore";
import { useSettings } from "../stores/settingsStore";
import { useUi } from "../stores/uiStore";
import { CHARACTER_PRESETS } from "../lib/characterPresets";
import { calculateContextMetrics, formatDetailToken } from "../lib/tokenEstimator";
import { Button } from "../ui/Button";
import { Icon } from "../ui/Icon";
import { Popover } from "../ui/Popover";
import "./ContextMetricsPopover.css";

interface ContextMetricsPopoverProps {
  inputText?: string;
}

export const ContextMetricsPopover: React.FC<ContextMetricsPopoverProps> = ({ inputText = "" }) => {
  const [isOpen, setIsOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);

  const messages = useStory((s) => s.messages);
  const view = useStory((s) => s.view);
  const activeCharKey = useUi((s) => s.activeCharKey);
  const char = CHARACTER_PRESETS[activeCharKey ?? "custom"] || CHARACTER_PRESETS.custom;

  const providers = useSettings((s) => s.providers);
  const openSettings = useSettings((s) => s.openSettings);
  const primaryConfig = providers.find((p) => p.slot === "primary");

  // 实时聚合计算上下文指标
  const metrics = useMemo(() => {
    return calculateContextMetrics({
      char,
      messages,
      inputText,
      primaryConfig,
      view,
    });
  }, [char, messages, inputText, primaryConfig, view]);

  const close = () => setIsOpen(false);

  return (
    <div className="composer-context-wrapper">
      {/* 纯仪表：环形进度 + 占用百分比。它不选择模型，避免与模型胶囊混淆。 */}
      <button
        ref={triggerRef}
        type="button"
        className={`composer-context-trigger-btn ${isOpen ? "active" : ""}`}
        onClick={() => setIsOpen((prev) => !prev)}
        aria-label="实时上下文统计"
        aria-expanded={isOpen}
        aria-haspopup="dialog"
        title={`上下文占用 ${metrics.usedFormatted}/${metrics.limitFormatted}（${metrics.percentage}%）· 点击查看明细`}
      >
        {/* 环形进度状态图标 */}
        <svg className="context-donut-ring" viewBox="0 0 16 16" width="13" height="13" aria-hidden="true">
          <circle cx="8" cy="8" r="6" className="donut-track" />
          <circle
            cx="8"
            cy="8"
            r="6"
            className={`donut-fill ${metrics.percentage >= 85 ? "donut-fill--warn" : ""}`}
            style={{
              strokeDasharray: 37.7,
              strokeDashoffset: 37.7 * (1 - Math.min(1, metrics.percentage / 100)),
            }}
            transform="rotate(-90 8 8)"
          />
        </svg>

        <span className="context-meter-label">{metrics.percentage}%</span>
        <Icon name="chevronDown" size={11} className={`context-chevron ${isOpen ? "open" : ""}`} />
      </button>

      {/* 浮动卡片：上下文构成明细 */}
      <Popover
        open={isOpen}
        onClose={close}
        anchor={triggerRef}
        label="上下文详情"
        side="top"
        align="end"
        width={290}
        className="context-metrics-popover"
      >
        {/* 1. 头部：上下文窗口 123.4K/1M (12.3%) */}
        <div className="context-popover-header">
          <span className="context-popover-title">上下文窗口</span>
          <span className="context-popover-fraction">
            {metrics.usedFormatted}/{metrics.limitFormatted} ({metrics.percentage}%)
          </span>
        </div>
        {metrics.modelName ? (
          <div className="context-popover-model">主线模型：{metrics.modelName}</div>
        ) : null}

        {/* 2. 进度条 */}
        <div className="context-popover-progress-bar">
          <div
            className="context-popover-progress-fill"
            style={{ width: `${Math.min(100, Math.max(1.5, metrics.percentage))}%` }}
          />
        </div>

        {/* 3. 前缀缓存命中率 */}
        <div className="context-popover-cache-row">
          <span className="context-cache-label">平均前缀缓存命中</span>
          <span className="context-cache-val">~{metrics.cacheHitRate}%</span>
        </div>

        {/* 4. 实时上下文详细构成列表 */}
        <div className="context-details-section">
          <div className="context-details-divider" />
          <div className="context-details-list">
            <div className="context-detail-item">
              <span className="detail-item-name">
                <span className="detail-dot dot-system" />
                人设基底与规则
              </span>
              <span className="detail-item-val">~{formatDetailToken(metrics.systemTokens)}</span>
            </div>

            {metrics.memoryTokens > 0 && (
              <div className="context-detail-item">
                <span className="detail-item-name">
                  <span className="detail-dot dot-memory" />
                  记忆库与世界书
                </span>
                <span className="detail-item-val">~{formatDetailToken(metrics.memoryTokens)}</span>
              </div>
            )}

            <div className="context-detail-item">
              <span className="detail-item-name">
                <span className="detail-dot dot-history" />
                历史演义 ({metrics.turnCount} 轮)
              </span>
              <span className="detail-item-val">~{formatDetailToken(metrics.historyTokens)}</span>
            </div>

            <div className="context-detail-item">
              <span className="detail-item-name">
                <span className="detail-dot dot-draft" />
                当前输入框草稿
              </span>
              <span className="detail-item-val">~{formatDetailToken(metrics.draftTokens)}</span>
            </div>
          </div>

          {/* 5. 底部快捷模型配置 */}
          <div className="context-popover-footer">
            <Button
              size="sm"
              variant="secondary"
              block
              className="context-popover-settings-btn"
              onClick={() => {
                close();
                openSettings();
              }}
            >
              <Icon name="sliders" size={13} />
              <span>切换模型与上下文配额</span>
              <Icon name="arrowRight" size={12} className="context-popover-settings-arrow" />
            </Button>
          </div>
        </div>
      </Popover>
    </div>
  );
};
