import React, { useState } from "react";

interface Props {
  onOpenSource: (title: string, path: string, snippet: string) => void;
}

export const MotionSection: React.FC<Props> = ({ onOpenSource }) => {
  const [activeDockIndex, setActiveDockIndex] = useState<number | null>(null);
  const [folderOpen, setFolderOpen] = useState(false);
  const [tiltPos, setTiltPos] = useState({ x: 0, y: 0 });

  const dockIcons = [
    { glyph: "📖", label: "故事" },
    { glyph: "🎭", label: "角色" },
    { glyph: "🌐", label: "世界书" },
    { glyph: "🎲", label: "检定" },
    { glyph: "⚙", label: "设置" },
  ];

  const handleMouseMove = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const x = ((e.clientX - rect.left) / rect.width - 0.5) * 20;
    const y = ((e.clientY - rect.top) / rect.height - 0.5) * -20;
    setTiltPos({ x, y });
  };

  const handleMouseLeave = () => {
    setTiltPos({ x: 0, y: 0 });
  };

  return (
    <section className="showcase-section" id="motion-primitives">
      <div className="showcase-section-header">
        <div>
          <div className="showcase-section-title-wrap">
            <h2 className="showcase-section-title">现代微动效与流畅交互 (Motion & Interactive)</h2>
            <span className="showcase-source-tag">beui.dev</span>
          </div>
          <p className="showcase-section-desc">
            基于 Tailwind CSS 与 Framer Motion 规范的高级动画组件：弹性悬浮停靠栏（Dock）、3D 陀螺仪视差卡片、文字流光遮罩与形变抽屉。
          </p>
        </div>
      </div>

      <div className="showcase-grid">
        {/* 1. Dock */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>🚢 弹性放大停靠栏</span>
                <span className="showcase-source-tag">Dock</span>
              </h4>
              <p>鼠标滑过时顺滑鱼眼放大与物理阻尼浮动</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "dock.tsx",
                  "reference-components/beui/motion/dock.tsx",
                  `// beui.dev - dock.tsx\nexport function Dock({ items }) {\n  return (\n    <motion.div className="dock-bar">\n      {items.map(item => (\n        <motion.div whileHover={{ scale: 1.25, y: -6 }} className="dock-item">\n          {item.icon}\n        </motion.div>\n      ))}\n    </motion.div>\n  );\n}`
                )
              }
            >
              源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div className="dock-demo-bar">
              {dockIcons.map((item, idx) => (
                <div
                  key={item.label}
                  className="dock-demo-item"
                  onMouseEnter={() => setActiveDockIndex(idx)}
                  onMouseLeave={() => setActiveDockIndex(null)}
                  title={item.label}
                >
                  {item.glyph}
                </div>
              ))}
            </div>
            {activeDockIndex !== null && (
              <span style={{ marginTop: "12px", fontSize: "12px", color: "var(--text-muted)" }}>
                正在聚焦：{dockIcons[activeDockIndex].label}
              </span>
            )}
          </div>
        </div>

        {/* 2. 3D Tilt Card */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>📐 3D 视差倾斜卡片</span>
                <span className="showcase-source-tag">TiltCard</span>
              </h4>
              <p>光标跟随旋转，亚像素高光反射质感</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "tilt-card.tsx",
                  "reference-components/beui/motion/tilt-card.tsx",
                  `// beui.dev - tilt-card.tsx\nexport function TiltCard({ children }) {\n  const [rotate, setRotate] = useState({ x: 0, y: 0 });\n  return (\n    <div\n      onMouseMove={e => calcTilt(e)}\n      style={{ transform: \`perspective(600px) rotateX(\${rotate.y}deg) rotateY(\${rotate.x}deg)\` }}\n    >\n      {children}\n    </div>\n  );\n}`
                )
              }
            >
              源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div
              className="tilt-card-demo"
              onMouseMove={handleMouseMove}
              onMouseLeave={handleMouseLeave}
              style={{
                transform: `perspective(600px) rotateX(${tiltPos.y}deg) rotateY(${tiltPos.x}deg)`,
              }}
            >
              <div>
                <div style={{ fontSize: "11px", color: "var(--text-dim)", textTransform: "uppercase" }}>
                  Character Dossier
                </div>
                <div style={{ fontSize: "15px", fontWeight: 600, color: "var(--text-main)", marginTop: "2px" }}>
                  塞莱斯蒂娅 · 暗夜巡守
                </div>
              </div>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-end" }}>
                <span style={{ fontSize: "12px", color: "var(--text-muted)" }}>好感度: 78% · 信任</span>
                <span style={{ fontSize: "18px" }}>🗡</span>
              </div>
            </div>
          </div>
        </div>

        {/* 3. Text Shimmer */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>✨ 渐变流光文字</span>
                <span className="showcase-source-tag">TextShimmer</span>
              </h4>
              <p>高级极简产品常用的呼吸流动金属光泽</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "text-shimmer.tsx",
                  "reference-components/beui/motion/text-shimmer.tsx",
                  `// beui.dev - text-shimmer.tsx\nexport function TextShimmer({ text }) {\n  return (\n    <span className="shimmer-gradient-mask">\n      {text}\n    </span>\n  );\n}`
                )
              }
            >
              源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div style={{ textAlign: "center", display: "flex", flexDirection: "column", gap: "10px" }}>
              <div
                className="thinking-shimmer-text"
                style={{ fontSize: "20px", fontWeight: 600, letterSpacing: "-0.01em" }}
              >
                TavernAgent AI Engine Active
              </div>
              <div
                className="thinking-shimmer-text"
                style={{ fontSize: "13px", opacity: 0.85 }}
              >
                本地大模型与故事记忆引擎协同工作中…
              </div>
            </div>
          </div>
        </div>

        {/* 4. Card Folder */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>📁 折叠式风琴资料夹</span>
                <span className="showcase-source-tag">CardFolder</span>
              </h4>
              <p>适用于角色装备袋、物品栏与世界书分支</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "card-folder.tsx",
                  "reference-components/beui/motion/card-folder.tsx",
                  `// beui.dev - card-folder.tsx\nexport function CardFolder({ title, files }) {\n  const [open, setOpen] = useState(false);\n  return (\n    <div className="folder-accordion">\n      <button onClick={() => setOpen(!open)}>{title}</button>\n      {open && <div className="folder-files">...</div>}\n    </div>\n  );\n}`
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
                maxWidth: "320px",
                border: "1px solid var(--border-light)",
                borderRadius: "var(--radius-lg)",
                background: "var(--bg-surface)",
                overflow: "hidden",
              }}
            >
              <div
                style={{
                  padding: "12px 14px",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  cursor: "pointer",
                  background: "var(--bg-subtle)",
                }}
                onClick={() => setFolderOpen(!folderOpen)}
              >
                <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
                  <span>🗂</span>
                  <span style={{ fontSize: "13px", fontWeight: 600, color: "var(--text-main)" }}>
                    主线战役物品袋
                  </span>
                </div>
                <span style={{ fontSize: "11px", color: "var(--text-muted)" }}>
                  {folderOpen ? "收起 ▲" : "3 件物品 ▼"}
                </span>
              </div>
              {folderOpen && (
                <div style={{ padding: "10px 14px", display: "flex", flexDirection: "column", gap: "8px" }}>
                  <div style={{ display: "flex", justifyContent: "space-between", fontSize: "12.5px" }}>
                    <span>📜 遗落的契约卷轴</span>
                    <span style={{ color: "var(--text-dim)" }}>任务物品</span>
                  </div>
                  <div style={{ display: "flex", justifyContent: "space-between", fontSize: "12.5px" }}>
                    <span>🧪 净化微光药剂 x2</span>
                    <span style={{ color: "var(--text-dim)" }}>恢复 +30</span>
                  </div>
                  <div style={{ display: "flex", justifyContent: "space-between", fontSize: "12.5px" }}>
                    <span>🗝 锈蚀的地下室钥匙</span>
                    <span style={{ color: "var(--text-dim)" }}>可交互</span>
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};
