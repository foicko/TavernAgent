import React, { useState, useEffect, useRef } from "react";

interface Props {
  onOpenSource: (title: string, path: string, snippet: string) => void;
}

export const CreativeSection: React.FC<Props> = ({ onOpenSource }) => {
  // 1. Fluid Orb State
  const [orbPulseSpeed, setOrbPulseSpeed] = useState<"normal" | "active" | "speaking">("normal");
  const [orbColor, setOrbColor] = useState<"mono" | "violet" | "cyan">("mono");

  // 2. Animated Counter State
  const [counterValue, setCounterValue] = useState(1482);
  const [isRolling, setIsRolling] = useState(false);

  const rollDice = () => {
    setIsRolling(true);
    const target = Math.floor(Math.random() * 9000) + 1000;
    let current = counterValue;
    const step = Math.ceil((target - current) / 12);
    let count = 0;
    const timer = setInterval(() => {
      count++;
      current += step;
      if (count >= 12) {
        clearInterval(timer);
        setCounterValue(target);
        setIsRolling(false);
      } else {
        setCounterValue(Math.floor(current + (Math.random() * 20 - 10)));
      }
    }, 40);
  };

  // 3. Gooey Nav State
  const [activeGooeyIndex, setActiveGooeyIndex] = useState(0);
  const gooeyTabs = ["概览", "角色卡", "世界设定", "事件流"];

  // 4. Gravity Letters
  const [lettersHovered, setLettersHovered] = useState<number | null>(null);
  const titleLetters = "TAVERN-AGENT".split("");

  // 5. Emoji Reaction Bursts
  const [bursts, setBursts] = useState<{ id: number; emoji: string; x: number; y: number }[]>([]);
  const triggerReaction = (emoji: string, e: React.MouseEvent) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const id = Date.now() + Math.random();
    setBursts((prev) => [...prev, { id, emoji, x: rect.width / 2, y: -10 }]);
    setTimeout(() => {
      setBursts((prev) => prev.filter((b) => b.id !== id));
    }, 1000);
  };

  return (
    <section className="showcase-section" id="creative-primitives">
      <div className="showcase-section-header">
        <div>
          <div className="showcase-section-title-wrap">
            <h2 className="showcase-section-title">创意与沉浸交互 (Creative & Sensory)</h2>
            <span className="showcase-source-tag">rareui.com</span>
          </div>
          <p className="showcase-section-desc">
            探索突破常规的感官动效：流体 WebGL 能量光球、粘性粘连导航条（Gooey）、重力文字碰撞与动态数字翻牌器。
          </p>
        </div>
      </div>

      <div className="showcase-grid">
        {/* 1. Fluid Orb */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>🔮 流体能量光球</span>
                <span className="showcase-source-tag">fluid-orb.tsx</span>
              </h4>
              <p>适合 AI 语音交互、灵感唤醒状态与沉浸式背景环境光</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "fluid-orb.tsx",
                  "reference-components/rareui/ui/fluid-orb.tsx",
                  `// WebGL Fragment Shader Raymarching Fluid Orb\nconst FRAG = \`\nuniform vec2 u_resolution;\nuniform float u_time;\nuniform vec3 u_color;\n\nfloat noise(vec2 p) { ... }\nvoid main() {\n  vec2 uv = (gl_FragCoord.xy * 2.0 - u_resolution) / min(u_resolution.x, u_resolution.y);\n  float d = length(uv) - 0.5 + noise(uv * 3.0 + u_time * 0.4) * 0.15;\n  vec3 col = mix(u_color, vec3(0.0), smoothstep(0.0, 0.4, d));\n  gl_FragColor = vec4(col, 1.0 - smoothstep(0.4, 0.5, d));\n}\`;`
                )
              }
            >
              查看源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div
              className="fluid-orb-container"
              style={{
                transform:
                  orbPulseSpeed === "speaking"
                    ? "scale(1.15)"
                    : orbPulseSpeed === "active"
                    ? "scale(1.05)"
                    : "scale(1)",
                transition: "transform 0.4s var(--ease-spring)",
              }}
            >
              <div
                className="fluid-orb-core"
                style={{
                  background:
                    orbColor === "violet"
                      ? "radial-gradient(circle, #8b5cf6 0%, #ec4899 50%, transparent 75%)"
                      : orbColor === "cyan"
                      ? "radial-gradient(circle, #06b6d4 0%, #3b82f6 50%, transparent 75%)"
                      : "radial-gradient(circle, var(--accent-solid) 0%, var(--text-muted) 50%, transparent 75%)",
                  animationDuration:
                    orbPulseSpeed === "speaking" ? "1.2s" : orbPulseSpeed === "active" ? "2s" : "3.5s",
                }}
              />
              <div className="fluid-orb-glass">
                {orbPulseSpeed === "speaking" ? "🎙" : orbPulseSpeed === "active" ? "✨" : "💤"}
              </div>
            </div>

            <div style={{ display: "flex", gap: "8px", marginTop: "24px" }}>
              {(["normal", "active", "speaking"] as const).map((s) => (
                <button
                  key={s}
                  onClick={() => setOrbPulseSpeed(s)}
                  style={{
                    padding: "4px 10px",
                    borderRadius: "var(--radius-full)",
                    fontSize: "12px",
                    border: "1px solid var(--border-light)",
                    background: orbPulseSpeed === s ? "var(--accent-solid)" : "var(--bg-surface)",
                    color: orbPulseSpeed === s ? "var(--accent-contrast)" : "var(--text-muted)",
                    cursor: "pointer",
                    transition: "all 0.15s ease",
                  }}
                >
                  {s === "normal" ? "待命" : s === "active" ? "思考" : "说话"}
                </button>
              ))}

              {(["mono", "violet", "cyan"] as const).map((c) => (
                <button
                  key={c}
                  onClick={() => setOrbColor(c)}
                  style={{
                    padding: "4px 8px",
                    borderRadius: "var(--radius-full)",
                    fontSize: "11px",
                    border: "1px solid var(--border-light)",
                    background: orbColor === c ? "var(--accent-solid)" : "var(--bg-surface)",
                    color: orbColor === c ? "var(--accent-contrast)" : "var(--text-muted)",
                    cursor: "pointer",
                    transition: "all 0.15s ease",
                  }}
                >
                  {c === "mono" ? "黑白" : c === "violet" ? "紫霓" : "青蓝"}
                </button>
              ))}
            </div>
          </div>
        </div>

        {/* 2. Gooey Navigation */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>💧 粘性粘连导航条</span>
                <span className="showcase-source-tag">gooey-nav.tsx</span>
              </h4>
              <p>利用 SVG feGaussianBlur + feColorMatrix 滤镜实现水滴粘连效果</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "gooey-nav.tsx",
                  "reference-components/rareui/ui/gooey-nav.tsx",
                  `<svg className="hidden">\n  <defs>\n    <filter id="gooey">\n      <feGaussianBlur in="SourceGraphic" stdDeviation="8" result="blur" />\n      <feColorMatrix in="blur" mode="matrix" values="1 0 0 0 0  0 1 0 0 0  0 0 1 0 0  0 0 0 19 -9" result="goo" />\n      <feComposite in="SourceGraphic" in2="goo" operator="atop" />\n    </filter>\n  </defs>\n</svg>`
                )
              }
            >
              查看源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div
              style={{
                display: "inline-flex",
                background: "var(--bg-subtle)",
                padding: "6px",
                borderRadius: "var(--radius-full)",
                border: "1px solid var(--border-light)",
                position: "relative",
              }}
            >
              {gooeyTabs.map((tab, idx) => (
                <button
                  key={tab}
                  onClick={() => setActiveGooeyIndex(idx)}
                  style={{
                    position: "relative",
                    zIndex: 2,
                    padding: "8px 18px",
                    borderRadius: "var(--radius-full)",
                    fontSize: "13px",
                    fontWeight: 500,
                    border: "none",
                    background: "transparent",
                    color: activeGooeyIndex === idx ? "var(--accent-contrast)" : "var(--text-muted)",
                    cursor: "pointer",
                    transition: "color 0.2s ease",
                  }}
                >
                  {tab}
                </button>
              ))}

              {/* Pill Slider with subtle gooey bounce */}
              <div
                style={{
                  position: "absolute",
                  top: "6px",
                  bottom: "6px",
                  left: `${6 + activeGooeyIndex * 70}px`,
                  width: "66px",
                  borderRadius: "var(--radius-full)",
                  background: "var(--accent-solid)",
                  transition: "left 0.35s cubic-bezier(0.34, 1.4, 0.64, 1), width 0.25s ease",
                  zIndex: 1,
                  boxShadow: "var(--shadow-sm)",
                }}
              />
            </div>
            <p style={{ fontSize: "12px", color: "var(--text-muted)", marginTop: "24px" }}>
              当前激活视口：<strong>{gooeyTabs[activeGooeyIndex]}</strong>（带物理过冲与弹性收缩）
            </p>
          </div>
        </div>

        {/* 3. Animated Rolling Counter */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>🔢 动态数字翻牌滚动</span>
                <span className="showcase-source-tag">animated-counter.tsx</span>
              </h4>
              <p>跑团掷骰检定、代币/Token 消耗监控与高频数值平滑跳动</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "animated-counter.tsx",
                  "reference-components/rareui/ui/animated-counter.tsx",
                  `export const AnimatedCounter = ({ value, duration = 1000 }) => {\n  const [displayValue, setDisplayValue] = useState(value);\n  useEffect(() => {\n    const controls = animate(displayValue, value, {\n      duration: duration / 1000,\n      ease: [0.16, 1, 0.3, 1],\n      onUpdate: (latest) => setDisplayValue(Math.round(latest))\n    });\n    return () => controls.stop();\n  }, [value]);\n  return <span>{displayValue.toLocaleString()}</span>;\n};`
                )
              }
            >
              查看源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div
              style={{
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                gap: "12px",
              }}
            >
              <div
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: "44px",
                  fontWeight: 700,
                  letterSpacing: "-0.04em",
                  color: "var(--text-main)",
                  display: "flex",
                  alignItems: "center",
                  gap: "4px",
                }}
              >
                <span>🎲</span>
                <span>{counterValue.toLocaleString()}</span>
                <span style={{ fontSize: "16px", color: "var(--text-dim)", fontWeight: 400 }}>tokens</span>
              </div>

              <div style={{ display: "flex", gap: "10px" }}>
                <button
                  onClick={rollDice}
                  disabled={isRolling}
                  style={{
                    padding: "8px 18px",
                    borderRadius: "var(--radius-md)",
                    background: "var(--accent-solid)",
                    color: "var(--accent-contrast)",
                    border: "none",
                    fontSize: "13px",
                    fontWeight: 500,
                    cursor: isRolling ? "not-allowed" : "pointer",
                    boxShadow: "var(--shadow-sm)",
                    transition: "all 0.15s ease",
                  }}
                >
                  {isRolling ? "滚动检定中..." : "重新掷骰 / 刷新数值"}
                </button>
                <button
                  onClick={() => setCounterValue(0)}
                  style={{
                    padding: "8px 14px",
                    borderRadius: "var(--radius-md)",
                    background: "var(--bg-subtle)",
                    color: "var(--text-muted)",
                    border: "1px solid var(--border-light)",
                    fontSize: "13px",
                    cursor: "pointer",
                  }}
                >
                  清零
                </button>
              </div>
            </div>
          </div>
        </div>

        {/* 4. Gravity & Physics Letters */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>🔤 重力与弹簧微动文字</span>
                <span className="showcase-source-tag">gravity-letters.tsx</span>
              </h4>
              <p>每个字符具备独立的物理弹簧碰撞，鼠标悬浮时产生连锁反应</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "gravity-letters.tsx",
                  "reference-components/rareui/ui/gravity-letters.tsx",
                  `export const GravityLetters = ({ text }: { text: string }) => {\n  return (\n    <div className="flex gap-1 overflow-hidden">\n      {text.split('').map((char, i) => (\n        <motion.span\n          key={i}\n          whileHover={{ y: -12, rotate: (i % 2 === 0 ? 1 : -1) * 8 }}\n          transition={{ type: 'spring', stiffness: 450, damping: 12 }}\n        >\n          {char}\n        </motion.span>\n      ))}\n    </div>\n  );\n};`
                )
              }
            >
              查看源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div
              style={{
                display: "flex",
                gap: "4px",
                userSelect: "none",
                cursor: "pointer",
              }}
            >
              {titleLetters.map((char, index) => {
                const isHovered = lettersHovered === index;
                const isAdjacent =
                  lettersHovered !== null && Math.abs(lettersHovered - index) === 1;
                return (
                  <span
                    key={index}
                    onMouseEnter={() => setLettersHovered(index)}
                    onMouseLeave={() => setLettersHovered(null)}
                    style={{
                      display: "inline-block",
                      fontSize: "30px",
                      fontWeight: 800,
                      fontFamily: "var(--font-mono)",
                      color: isHovered
                        ? "var(--accent-solid)"
                        : isAdjacent
                        ? "var(--text-main)"
                        : "var(--text-muted)",
                      transform: isHovered
                        ? "translateY(-16px) scale(1.3) rotate(-6deg)"
                        : isAdjacent
                        ? "translateY(-6px) scale(1.1) rotate(3deg)"
                        : "translateY(0) scale(1) rotate(0deg)",
                      transition:
                        "transform 0.28s cubic-bezier(0.34, 1.56, 0.64, 1), color 0.15s ease",
                    }}
                  >
                    {char}
                  </span>
                );
              })}
            </div>
            <p style={{ fontSize: "12px", color: "var(--text-dim)", marginTop: "20px" }}>
              将鼠标移到字符上方体验多段弹力波纹
            </p>
          </div>
        </div>

        {/* 5. Emoji Reaction Bursts */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>💥 表情爆发与微粒子反馈</span>
                <span className="showcase-source-tag">emoji-reaction.tsx</span>
              </h4>
              <p>点击跑团高光时刻、重要剧情分支的即时粒子动效</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "emoji-reaction.tsx",
                  "reference-components/rareui/ui/emoji-reaction.tsx",
                  `// Particle Burst Reaction\nconst triggerBurst = (emoji) => {\n  const particles = Array.from({ length: 8 }).map((_, i) => ({\n    angle: (i / 8) * Math.PI * 2,\n    velocity: 4 + Math.random() * 3,\n    life: 1.0\n  }));\n  // Render via canvas or CSS transforms\n};`
                )
              }
            >
              查看源码
            </button>
          </div>
          <div className="showcase-card-stage" style={{ position: "relative" }}>
            {bursts.map((b) => (
              <span
                key={b.id}
                style={{
                  position: "absolute",
                  left: "50%",
                  top: "40%",
                  fontSize: "26px",
                  pointerEvents: "none",
                  animation: "burstFly 0.9s cubic-bezier(0.16, 1, 0.3, 1) forwards",
                }}
              >
                {b.emoji}
              </span>
            ))}
            <div style={{ display: "flex", gap: "12px" }}>
              {["🔥 绝妙", "🎲 大成功", "💔 团灭", "✨ 顿悟"].map((item) => {
                const [emoji, label] = item.split(" ");
                return (
                  <button
                    key={item}
                    onClick={(e) => triggerReaction(emoji, e)}
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: "6px",
                      padding: "8px 14px",
                      borderRadius: "var(--radius-full)",
                      background: "var(--bg-surface)",
                      border: "1px solid var(--border-light)",
                      fontSize: "13px",
                      color: "var(--text-main)",
                      cursor: "pointer",
                      boxShadow: "var(--shadow-sm)",
                      transition: "all 0.15s ease",
                    }}
                  >
                    <span>{emoji}</span>
                    <span>{label}</span>
                  </button>
                );
              })}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};
