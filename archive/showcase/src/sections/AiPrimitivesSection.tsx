import React, { useState, useEffect } from "react";

interface Props {
  onOpenSource: (title: string, path: string, snippet: string) => void;
}

export const AiPrimitivesSection: React.FC<Props> = ({ onOpenSource }) => {
  const [thinkingExpanded, setThinkingExpanded] = useState(true);

  const fullText =
    "TavernAgent 融合了本地轻量模型推理、世界书动态向量检索以及人在回路（Human-in-the-loop）决策流，可为复杂的文字跑团提供极具临场感的叙事体验。";
  const [streamedLength, setStreamedLength] = useState(fullText.length);
  const [isStreaming, setIsStreaming] = useState(false);

  const restartStreaming = () => {
    setIsStreaming(true);
    setStreamedLength(0);
  };

  useEffect(() => {
    if (!isStreaming) return;
    if (streamedLength >= fullText.length) {
      setIsStreaming(false);
      return;
    }
    const timer = setTimeout(() => {
      setStreamedLength((prev) => Math.min(prev + 2, fullText.length));
    }, 45);
    return () => clearTimeout(timer);
  }, [isStreaming, streamedLength, fullText.length]);

  const [selectedOption, setSelectedOption] = useState<number | null>(0);
  const [customAnswer, setCustomAnswer] = useState("");
  const [approvalStep, setApprovalStep] = useState(1);

  const [toolsExpanded, setToolsExpanded] = useState(true);
  const [taskExpanded, setTaskExpanded] = useState(false);

  return (
    <section className="showcase-section" id="ai-primitives">
      <div className="showcase-section-header">
        <div>
          <div className="showcase-section-title-wrap">
            <h2 className="showcase-section-title">AI 原生交互原语 (AI-Native Primitives)</h2>
            <span className="showcase-source-tag">beautifului.dev</span>
          </div>
          <p className="showcase-section-desc">
            专为 AI Agent 与大模型设计的核心组件：折叠式深度思考、流式文本输出、人在回路决策卡片、工具芯片与任务执行流。
          </p>
        </div>
      </div>

      <div className="showcase-grid">
        {/* 1. Thinking State */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>🧠 深度思考状态</span>
                <span className="showcase-source-tag">ThinkingState</span>
              </h4>
              <p>树状逐步展开的思考逻辑与工具调用链</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "ThinkingState.tsx",
                  "reference-components/beautifului/primitives/ThinkingState.tsx",
                  `// beautifului.dev - ThinkingState\nexport function ThinkingState({ steps, status = "Thinking" }) {\n  const [expanded, setExpanded] = useState(true);\n  return (\n    <div className="thinking-container">\n      <button onClick={() => setExpanded(!expanded)}>\n        <span className="shimmer">{status}</span>\n      </button>\n      {expanded && <div className="steps-tree">...</div>}\n    </div>\n  );\n}`
                )
              }
            >
              源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div className="thinking-demo-box">
              <button
                className="thinking-trigger"
                onClick={() => setThinkingExpanded(!thinkingExpanded)}
                aria-expanded={thinkingExpanded}
              >
                <span>✦</span>
                <span className="thinking-shimmer-text">
                  {thinkingExpanded ? "已思考完毕（耗时 1.8 秒）" : "正在思考…"}
                </span>
                <span style={{ marginLeft: "auto", fontSize: "11px", opacity: 0.6 }}>
                  {thinkingExpanded ? "▲ 收起" : "▼ 展开"}
                </span>
              </button>

              {thinkingExpanded && (
                <div className="thinking-tree-content">
                  <div className="thinking-step-item">
                    <span>1. 检索世界书条目：埃尔维拉港口设定</span>
                    <span style={{ fontSize: "11px", color: "var(--color-success)" }}>✓ 命中</span>
                  </div>
                  <div className="thinking-step-item">
                    <span>2. 检定角色感知敏锐度 (d20 = 16 成功)</span>
                    <span style={{ fontSize: "11px", color: "var(--color-success)" }}>✓ 成功</span>
                  </div>
                  <div className="thinking-step-item">
                    <span>3. 综合主线上下文生成下一幕剧情</span>
                    <span style={{ fontSize: "11px", color: "var(--color-info)" }}>推理中</span>
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>

        {/* 2. Streaming Text */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>🌊 流式文本与引用</span>
                <span className="showcase-source-tag">StreamingText</span>
              </h4>
              <p>带平滑光标闪烁、来源微药丸与快捷重放</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "StreamingText.tsx",
                  "reference-components/beautifului/primitives/StreamingText.tsx",
                  `// beautifului.dev - StreamingText\nexport function StreamingText({ text, sources = [] }) {\n  return (\n    <div className="streaming-wrap">\n      <p>{text}<span className="cursor-blink" /></p>\n      <div className="sources-list">\n        {sources.map(s => <span key={s} className="source-chip">{s}</span>)}\n      </div>\n    </div>\n  );\n}`
                )
              }
            >
              源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div className="streaming-demo-box">
              <div className="streaming-text-bubble">
                {fullText.slice(0, streamedLength)}
                {isStreaming && <span className="streaming-cursor" />}
              </div>
              <div className="streaming-chips-row">
                <span className="source-chip">📚 世界书: 帝国史记</span>
                <span className="source-chip">🧠 记忆: 塞莱斯承诺</span>
                <button
                  className="showcase-src-link-btn"
                  onClick={restartStreaming}
                  style={{ marginLeft: "auto" }}
                >
                  {isStreaming ? "正在打字…" : "▶ 模拟流式生成"}
                </button>
              </div>
            </div>
          </div>
        </div>

        {/* 3. Approval Card */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>⚖ 人在回路确认卡</span>
                <span className="showcase-source-tag">ApprovalCard</span>
              </h4>
              <p>AI 执行关键情节或操作前的交互决断卡</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "ApprovalCard.tsx",
                  "reference-components/beautifului/primitives/ApprovalCard.tsx",
                  `// beautifului.dev - ApprovalCard\nexport function ApprovalCard({ title, options, onConfirm, onSkip }) {\n  const [selected, setSelected] = useState(null);\n  return (\n    <div className="approval-card">\n      <h4>{title}</h4>\n      {options.map((opt, i) => (\n        <button key={opt} onClick={() => setSelected(i)} className={selected === i ? "selected" : ""}>{opt}</button>\n      ))}\n    </div>\n  );\n}`
                )
              }
            >
              源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div className="approval-demo-card">
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <span style={{ fontSize: "13px", fontWeight: 600, color: "var(--text-main)" }}>
                  分支抉择：你打算如何应对潜伏者？
                </span>
                <span style={{ fontSize: "11px", color: "var(--text-dim)", fontFamily: "var(--font-mono)" }}>
                  {approvalStep}/3
                </span>
              </div>

              <div
                className={`approval-option-row ${selectedOption === 0 ? "selected" : ""}`}
                onClick={() => setSelectedOption(0)}
              >
                <span>🛡</span>
                <span>拔出佩剑，正面亮明身份并戒备</span>
              </div>
              <div
                className={`approval-option-row ${selectedOption === 1 ? "selected" : ""}`}
                onClick={() => setSelectedOption(1)}
              >
                <span>🕯</span>
                <span>隐入阴影，暗中观察其下一步动向</span>
              </div>
              <div
                className={`approval-option-row ${selectedOption === 2 ? "selected" : ""}`}
                onClick={() => setSelectedOption(2)}
              >
                <span>💬</span>
                <span>主动出声攀谈，试探其来意</span>
              </div>

              <input
                type="text"
                placeholder="或输入自定义行动…"
                value={customAnswer}
                onChange={(e) => {
                  setCustomAnswer(e.target.value);
                  if (e.target.value) setSelectedOption(null);
                }}
                className="showcase-search-input"
                style={{ width: "100%", height: "32px", fontSize: "12px", borderRadius: "var(--radius-sm)" }}
              />

              <div style={{ display: "flex", justifyContent: "flex-end", gap: "8px", marginTop: "4px" }}>
                <button
                  className="showcase-src-link-btn"
                  onClick={() => setApprovalStep((prev) => (prev > 1 ? prev - 1 : 1))}
                >
                  跳过
                </button>
                <button
                  className="showcase-src-link-btn"
                  style={{ background: "var(--accent-solid)", color: "var(--accent-contrast)" }}
                  onClick={() => setApprovalStep((prev) => (prev < 3 ? prev + 1 : 1))}
                >
                  确认行动
                </button>
              </div>
            </div>
          </div>
        </div>

        {/* 4. Tool Chips */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>🔧 工具调用芯片</span>
                <span className="showcase-source-tag">ToolChips</span>
              </h4>
              <p>代码改动、环境执行与多步骤折叠胶囊</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "ToolChips.tsx",
                  "reference-components/beautifului/primitives/ToolChips.tsx",
                  `// beautifului.dev - ToolChips\nexport function ToolChips({ toolCalls = [] }) {\n  const [expanded, setExpanded] = useState(false);\n  return (\n    <div className="tool-chips-root">\n      <button onClick={() => setExpanded(!expanded)}>\n        <span>{toolCalls.length} 个工具调用</span>\n      </button>\n      {expanded && <div className="chips-detail">...</div>}\n    </div>\n  );\n}`
                )
              }
            >
              源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div style={{ width: "100%", maxWidth: "340px", display: "flex", flexDirection: "column", gap: "8px" }}>
              <button
                className="thinking-trigger"
                onClick={() => setToolsExpanded(!toolsExpanded)}
                style={{ width: "100%" }}
              >
                <span>⚙</span>
                <span>4 个工具调用，2 次世界书查证</span>
                <span style={{ marginLeft: "auto", fontSize: "11px", opacity: 0.6 }}>
                  {toolsExpanded ? "收起" : "展开"}
                </span>
              </button>

              {toolsExpanded && (
                <div style={{ display: "flex", flexDirection: "column", gap: "6px", paddingLeft: "8px" }}>
                  <div className="source-chip" style={{ justifyContent: "space-between", padding: "6px 10px" }}>
                    <span>⚡ recall_memory(&quot;港口密信&quot;)</span>
                    <span style={{ color: "var(--color-success)", fontWeight: 500 }}>200 OK</span>
                  </div>
                  <div className="source-chip" style={{ justifyContent: "space-between", padding: "6px 10px" }}>
                    <span>📖 query_lorebook(&quot;圣殿骑士团&quot;)</span>
                    <span style={{ color: "var(--color-success)", fontWeight: 500 }}>已匹配</span>
                  </div>
                  <div className="source-chip" style={{ justifyContent: "space-between", padding: "6px 10px" }}>
                    <span>🎲 roll_dice(&quot;1d20+3&quot;) -&gt; 18</span>
                    <span style={{ color: "var(--color-info)", fontWeight: 500 }}>大成功</span>
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>

        {/* 5. Task Rows */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>📋 代理任务执行行</span>
                <span className="showcase-source-tag">TaskRows</span>
              </h4>
              <p>动态子任务执行队列、完成进度与耗时状态</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "TaskRows.tsx",
                  "reference-components/beautifului/primitives/TaskRows.tsx",
                  `// beautifului.dev - TaskRows\nexport function TaskRows({ tasks = [] }) {\n  return (\n    <div className="task-rows-wrap">\n      {tasks.map(t => (\n        <div key={t.id} className="task-row">\n          <span className={t.status}>{t.title}</span>\n        </div>\n      ))}\n    </div>\n  );\n}`
                )
              }
            >
              源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div style={{ width: "100%", maxWidth: "340px", display: "flex", flexDirection: "column", gap: "8px" }}>
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  padding: "10px 12px",
                  borderRadius: "var(--radius-lg)",
                  background: "var(--bg-surface)",
                  border: "1px solid var(--border-light)",
                  cursor: "pointer",
                }}
                onClick={() => setTaskExpanded(!taskExpanded)}
              >
                <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
                  <span style={{ color: "var(--color-success)" }}>●</span>
                  <span style={{ fontSize: "13px", fontWeight: 500, color: "var(--text-main)" }}>
                    完成主线场景状态核验
                  </span>
                </div>
                <span style={{ fontSize: "11px", color: "var(--text-muted)" }}>100%</span>
              </div>

              {taskExpanded && (
                <div style={{ paddingLeft: "16px", display: "flex", flexDirection: "column", gap: "4px" }}>
                  <div style={{ fontSize: "12px", color: "var(--text-muted)" }}>✓ 验证当前故事分支连续性</div>
                  <div style={{ fontSize: "12px", color: "var(--text-muted)" }}>✓ 同步好感度数值与记忆账本</div>
                  <div style={{ fontSize: "12px", color: "var(--text-muted)" }}>✓ 检查立绘表情差分图层</div>
                </div>
              )}
            </div>
          </div>
        </div>

        {/* 6. Prompt Bar */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>💬 悬浮流光输入栏</span>
                <span className="showcase-source-tag">PromptBar</span>
              </h4>
              <p>灵动悬浮、快捷动作芯片与模型切换器</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "PromptBar.tsx",
                  "reference-components/beautifului/primitives/PromptBar.tsx",
                  `// beautifului.dev - PromptBar\nexport function PromptBar({ onSubmit }) {\n  return (\n    <div className="floating-prompt-bar">\n      <textarea placeholder="Ask or describe your action..." />\n      <div className="prompt-actions">\n        <button>Model</button>\n        <button className="primary">Send</button>\n      </div>\n    </div>\n  );\n}`
                )
              }
            >
              源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div
              style={{
                width: "100%",
                maxWidth: "360px",
                padding: "12px",
                borderRadius: "var(--radius-xl)",
                background: "var(--bg-surface)",
                border: "1px solid var(--border-light)",
                boxShadow: "var(--shadow-md)",
                display: "flex",
                flexDirection: "column",
                gap: "10px",
              }}
            >
              <input
                type="text"
                placeholder="键入故事走向、角色对话或 TRPG 动作…"
                className="showcase-search-input"
                style={{ width: "100%", height: "34px", border: "none", background: "transparent" }}
                readOnly
                value="「你究竟在隐瞒什么？」我直视着她的眼睛问道。"
              />
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                <div style={{ display: "flex", gap: "4px" }}>
                  <span className="source-chip">「对白」</span>
                  <span className="source-chip">*动作*</span>
                  <span className="source-chip">🎲 检定</span>
                </div>
                <button
                  className="showcase-src-link-btn"
                  style={{ background: "var(--accent-solid)", color: "var(--accent-contrast)", borderRadius: "999px" }}
                >
                  发送 ↵
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};
