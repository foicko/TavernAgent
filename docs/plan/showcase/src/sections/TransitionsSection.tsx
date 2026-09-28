import React, { useState } from "react";

interface Props {
  onOpenSource: (title: string, path: string, snippet: string) => void;
}

export const TransitionsSection: React.FC<Props> = ({ onOpenSource }) => {
  // 1. Spring Physics Playground
  const [stiffness, setStiffness] = useState(300);
  const [damping, setDamping] = useState(20);
  const [mass, setMass] = useState(1);
  const [bounceActive, setBounceActive] = useState(false);

  const triggerBounce = () => {
    setBounceActive(true);
    setTimeout(() => setBounceActive(false), 600);
  };

  // 2. Card Resize / FLIP Expand
  const [isCardExpanded, setIsCardExpanded] = useState(false);

  // 3. Error Shake
  const [isShaking, setIsShaking] = useState(false);
  const triggerShake = () => {
    setIsShaking(true);
    setTimeout(() => setIsShaking(false), 500);
  };

  // 4. Number Pop-in
  const [badgeCount, setBadgeCount] = useState(3);
  const [badgePopping, setBadgePopping] = useState(false);
  const incrementBadge = () => {
    setBadgeCount((prev) => prev + 1);
    setBadgePopping(true);
    setTimeout(() => setBadgePopping(false), 300);
  };

  return (
    <section className="showcase-section" id="transitions-primitives">
      <div className="showcase-section-header">
        <div>
          <div className="showcase-section-title-wrap">
            <h2 className="showcase-section-title">物理弹簧与高级转场 (Physics & Transitions)</h2>
            <span className="showcase-source-tag">transitions.dev</span>
          </div>
          <p className="showcase-section-desc">
            遵循真实物理学定律的动效曲线系统：弹性阻尼运动参数调节、FLIP 尺寸无缝变形、错误回弹抖动与数值弹跳。
          </p>
        </div>
      </div>

      <div className="showcase-grid">
        {/* 1. Spring Physics Playground */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>⚡ 弹簧物理参数演练场</span>
                <span className="showcase-source-tag">spring-curve.tsx</span>
              </h4>
              <p>动态调整刚度 (Stiffness)、阻尼 (Damping) 与质量 (Mass)</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "spring-curve.tsx",
                  "reference-components/transitions/skills/transitions-dev/_root.css",
                  `/* Transitions.dev Spring Curves */\n:root {\n  --ease-spring-snappy: cubic-bezier(0.16, 1, 0.3, 1);\n  --ease-spring-bouncy: cubic-bezier(0.34, 1.56, 0.64, 1);\n  --ease-spring-smooth: cubic-bezier(0.22, 1, 0.36, 1);\n  --duration-spring: 450ms;\n}`
                )
              }
            >
              查看源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div className="spring-playground-box">
              <div
                className={`spring-sample-target ${bounceActive ? "bouncing" : ""}`}
                onClick={triggerBounce}
                style={{
                  transitionDuration: `${Math.round(8000 / stiffness)}ms`,
                }}
              >
                {bounceActive ? "✨ 弹性过冲与回落中..." : "点击体验物理弹跳"}
              </div>

              {/* Sliders */}
              <div style={{ display: "flex", flexDirection: "column", gap: "8px", marginTop: "8px" }}>
                <div style={{ display: "flex", justifyContent: "space-between", fontSize: "12px", color: "var(--text-muted)" }}>
                  <span>刚度 (Stiffness): {stiffness}</span>
                  <input
                    type="range"
                    min="100"
                    max="600"
                    step="20"
                    value={stiffness}
                    onChange={(e) => setStiffness(Number(e.target.value))}
                    style={{ width: "130px" }}
                  />
                </div>
                <div style={{ display: "flex", justifyContent: "space-between", fontSize: "12px", color: "var(--text-muted)" }}>
                  <span>阻尼 (Damping): {damping}</span>
                  <input
                    type="range"
                    min="5"
                    max="40"
                    step="1"
                    value={damping}
                    onChange={(e) => setDamping(Number(e.target.value))}
                    style={{ width: "130px" }}
                  />
                </div>
                <div style={{ display: "flex", justifyContent: "space-between", fontSize: "12px", color: "var(--text-muted)" }}>
                  <span>质量 (Mass): {mass}</span>
                  <input
                    type="range"
                    min="0.5"
                    max="3"
                    step="0.5"
                    value={mass}
                    onChange={(e) => setMass(Number(e.target.value))}
                    style={{ width: "130px" }}
                  />
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* 2. Card Resize / FLIP Morph */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>📐 顺滑几何形变与尺寸伸缩</span>
                <span className="showcase-source-tag">01-card-resize.md</span>
              </h4>
              <p>基于 FLIP 技术的平滑折叠展开，无文字重排撕裂感</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "01-card-resize.md",
                  "reference-components/transitions/skills/transitions-dev/01-card-resize.md",
                  `### 01-card-resize\nTransition Technique: FLIP (First, Last, Invert, Play)\nUse clip-path or grid-template-rows 0fr -> 1fr with transition: grid-template-rows 300ms cubic-bezier(0.16, 1, 0.3, 1).\nEnsures inner elements retain crisp text rendering without blur.`
                )
              }
            >
              查看源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div
              style={{
                width: isCardExpanded ? "340px" : "260px",
                background: "var(--bg-surface)",
                border: "1px solid var(--border-light)",
                borderRadius: "var(--radius-xl)",
                padding: "16px",
                boxShadow: isCardExpanded ? "var(--elevation-3)" : "var(--shadow-sm)",
                transition: "all 0.38s cubic-bezier(0.16, 1, 0.3, 1)",
                overflow: "hidden",
                cursor: "pointer",
              }}
              onClick={() => setIsCardExpanded(!isCardExpanded)}
            >
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                <div style={{ display: "flex", alignItems: "center", gap: "10px" }}>
                  <span style={{ fontSize: "20px" }}>🗺</span>
                  <div>
                    <h5 style={{ fontSize: "14px", fontWeight: 600, color: "var(--text-main)" }}>迷雾之森</h5>
                    <p style={{ fontSize: "11.5px", color: "var(--text-muted)" }}>危险等级：S</p>
                  </div>
                </div>
                <span style={{ fontSize: "12px", color: "var(--text-dim)", transform: isCardExpanded ? "rotate(180deg)" : "rotate(0deg)", transition: "transform 0.3s ease" }}>
                  ▼
                </span>
              </div>

              {isCardExpanded && (
                <div style={{ marginTop: "14px", paddingTop: "14px", borderTop: "1px dashed var(--border-light)", fontSize: "12.5px", color: "var(--text-muted)", lineHeight: 1.6 }}>
                  <p>常年被不祥的紫瘴笼罩，古代法师行会曾在此建立地下秘密工坊。进入前需通过意志检定（DC 15）。</p>
                  <div style={{ display: "flex", gap: "6px", marginTop: "10px" }}>
                    <span style={{ padding: "3px 8px", borderRadius: "var(--radius-sm)", background: "var(--bg-subtle)", fontSize: "11px" }}>环境：剧毒</span>
                    <span style={{ padding: "3px 8px", borderRadius: "var(--radius-sm)", background: "var(--bg-subtle)", fontSize: "11px" }}>宝物：星界符石</span>
                  </div>
                </div>
              )}
            </div>
            <p style={{ fontSize: "12px", color: "var(--text-dim)", marginTop: "16px" }}>
              点击卡片切换展开 / 收起尺寸
            </p>
          </div>
        </div>

        {/* 3. Error Shake */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>💢 错误与拒绝回弹抖动</span>
                <span className="showcase-source-tag">12-error-state-shake.md</span>
              </h4>
              <p>检定失败、非法操作、密码错误时的非侵入式微震反馈</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "12-error-state-shake.md",
                  "reference-components/transitions/skills/transitions-dev/12-error-state-shake.md",
                  `@keyframes errorShake {\n  0%, 100% { transform: translateX(0); }\n  20%, 60% { transform: translateX(-6px); }\n  40%, 80% { transform: translateX(6px); }\n}\n.shake-element {\n  animation: errorShake 400ms cubic-bezier(0.36, 0.07, 0.19, 0.97) both;\n}`
                )
              }
            >
              查看源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div
              className={`spring-sample-target ${isShaking ? "shaking" : ""}`}
              onClick={triggerShake}
              style={{
                width: "280px",
                background: isShaking ? "var(--color-danger-text)" : "var(--bg-surface)",
                color: isShaking ? "#ffffff" : "var(--text-main)",
                border: "1px solid var(--border-light)",
                boxShadow: "var(--shadow-sm)",
              }}
            >
              {isShaking ? "❌ 检定失败！(D20 掷出 1)" : "点击触发错误抖动"}
            </div>
          </div>
        </div>

        {/* 4. Number Pop-in & Badge Jump */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>🔔 徽标数值弹跳 (Badge Pop)</span>
                <span className="showcase-source-tag">03-notification-badge.md</span>
              </h4>
              <p>消息未读数更新、经验值与状态加成弹跳登场</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "03-notification-badge.md",
                  "reference-components/transitions/skills/transitions-dev/03-notification-badge.md",
                  `.badge-pop {\n  animation: badgePop 300ms cubic-bezier(0.34, 1.56, 0.64, 1);\n}\n@keyframes badgePop {\n  0% { transform: scale(0.6); opacity: 0; }\n  70% { transform: scale(1.2); }\n  100% { transform: scale(1); opacity: 1; }\n}`
                )
              }
            >
              查看源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div style={{ display: "flex", alignItems: "center", gap: "24px" }}>
              <div style={{ position: "relative" }}>
                <button
                  style={{
                    width: "52px",
                    height: "52px",
                    borderRadius: "var(--radius-full)",
                    background: "var(--bg-surface)",
                    border: "1px solid var(--border-light)",
                    fontSize: "22px",
                    cursor: "pointer",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    boxShadow: "var(--shadow-sm)",
                  }}
                  onClick={incrementBadge}
                >
                  🔔
                </button>
                <span
                  style={{
                    position: "absolute",
                    top: "-4px",
                    right: "-4px",
                    minWidth: "22px",
                    height: "22px",
                    padding: "0 6px",
                    borderRadius: "var(--radius-full)",
                    background: "var(--accent-solid)",
                    color: "var(--accent-contrast)",
                    fontSize: "12px",
                    fontWeight: 700,
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    transform: badgePopping ? "scale(1.4)" : "scale(1)",
                    transition: "transform 0.25s cubic-bezier(0.34, 1.56, 0.64, 1)",
                  }}
                >
                  {badgeCount}
                </span>
              </div>
              <button
                onClick={incrementBadge}
                style={{
                  padding: "8px 14px",
                  borderRadius: "var(--radius-md)",
                  background: "var(--bg-subtle)",
                  border: "1px solid var(--border-light)",
                  fontSize: "13px",
                  color: "var(--text-main)",
                  cursor: "pointer",
                }}
              >
                +1 增加通知数
              </button>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};
