import React, { useState, useEffect } from "react";
import { AiPrimitivesSection } from "./sections/AiPrimitivesSection";
import { MotionSection } from "./sections/MotionSection";
import { CreativeSection } from "./sections/CreativeSection";
import { TransitionsSection } from "./sections/TransitionsSection";
import { ShadcnPrimitivesSection } from "./sections/ShadcnPrimitivesSection";
import "./styles/tokens.css";
import "./styles/showcase.css";

interface SourceDrawerData {
  title: string;
  path: string;
  snippet: string;
}

export const App: React.FC = () => {
  const [theme, setTheme] = useState<"light" | "dark">("light");
  const [activeCategory, setActiveCategory] = useState<string>("all");
  const [searchQuery, setSearchQuery] = useState("");
  const [drawerData, setDrawerData] = useState<SourceDrawerData | null>(null);
  const [copied, setCopied] = useState(false);

  // Sync theme attribute to <html>
  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
  }, [theme]);

  const toggleTheme = () => {
    setTheme((prev) => (prev === "light" ? "dark" : "light"));
  };

  const handleOpenSource = (title: string, path: string, snippet: string) => {
    setDrawerData({ title, path, snippet });
    setCopied(false);
  };

  const handleCopyCode = () => {
    if (!drawerData) return;
    navigator.clipboard.writeText(drawerData.snippet);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const categories = [
    { id: "all", label: "全部组件 (All)" },
    { id: "ai-primitives", label: "🧠 AI 原语 (beautifului.dev)" },
    { id: "motion-primitives", label: "🚢 现代动效 (beui.dev)" },
    { id: "creative-primitives", label: "🔮 创意感官 (rareui.com)" },
    { id: "transitions-primitives", label: "⚡ 物理弹簧 (transitions.dev)" },
    { id: "shadcn-primitives", label: "🔘 基础原语 (ui.shadcn.com)" },
  ];

  const scrollToCategory = (id: string) => {
    setActiveCategory(id);
    if (id === "all") {
      window.scrollTo({ top: 0, behavior: "smooth" });
      return;
    }
    const elem = document.getElementById(id);
    if (elem) {
      elem.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  };

  return (
    <div className="showcase-app">
      {/* Top Header */}
      <header className="showcase-header">
        <div className="showcase-brand">
          <div className="showcase-brand-badge">✨</div>
          <div className="showcase-title-group">
            <h1>TavernAgent UI Showcase</h1>
            <p>五大设计体系独立组件展厅 · 双配色系统</p>
          </div>
        </div>

        <div className="showcase-header-actions">
          <input
            type="text"
            className="showcase-search-input"
            placeholder="搜索组件 (如: Dock, 思考, 弹簧...)"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />

          <button className="showcase-theme-pill" onClick={toggleTheme}>
            {theme === "light" ? "⚪ 纯净纸白" : "⚫ 曜石墨黑"}
          </button>

          <a
            href="http://localhost:5173"
            target="_blank"
            rel="noopener noreferrer"
            style={{
              display: "inline-flex",
              alignItems: "center",
              gap: "6px",
              padding: "0 14px",
              height: "36px",
              borderRadius: "var(--radius-full)",
              background: "var(--accent-solid)",
              color: "var(--accent-contrast)",
              fontSize: "12.5px",
              fontWeight: 500,
              textDecoration: "none",
              boxShadow: "var(--shadow-sm)",
            }}
          >
            <span>跑团应用 (Web :5173)</span>
            <span>↗</span>
          </a>
        </div>
      </header>

      {/* Category Nav Tabs */}
      <nav className="showcase-nav-bar">
        {categories.map((cat) => (
          <button
            key={cat.id}
            className={`showcase-tab-btn ${activeCategory === cat.id ? "active" : ""}`}
            onClick={() => scrollToCategory(cat.id)}
          >
            {cat.label}
          </button>
        ))}
      </nav>

      {/* Main Content */}
      <main className="showcase-container">
        {(activeCategory === "all" || activeCategory === "ai-primitives") && (
          <AiPrimitivesSection onOpenSource={handleOpenSource} />
        )}

        {(activeCategory === "all" || activeCategory === "motion-primitives") && (
          <MotionSection onOpenSource={handleOpenSource} />
        )}

        {(activeCategory === "all" || activeCategory === "creative-primitives") && (
          <CreativeSection onOpenSource={handleOpenSource} />
        )}

        {(activeCategory === "all" || activeCategory === "transitions-primitives") && (
          <TransitionsSection onOpenSource={handleOpenSource} />
        )}

        {(activeCategory === "all" || activeCategory === "shadcn-primitives") && (
          <ShadcnPrimitivesSection onOpenSource={handleOpenSource} />
        )}
      </main>

      {/* Footer */}
      <footer
        style={{
          borderTop: "1px solid var(--border-light)",
          padding: "36px 32px",
          background: "var(--bg-surface)",
          marginTop: "60px",
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          gap: "12px",
          textAlign: "center",
        }}
      >
        <p style={{ fontSize: "14px", fontWeight: 600, color: "var(--text-main)" }}>
          TavernAgent 现代化设计体系与组件参考库
        </p>
        <p style={{ fontSize: "12px", color: "var(--text-muted)", maxWidth: "600px", lineHeight: 1.6 }}>
          基于 beautifului.dev、beui.dev、rareui.com、transitions.dev 与 ui.shadcn.com。全部 382+
          原件源码保存于项目根目录 <code>reference-components/</code> 中，可直接引入 React Web 端。
        </p>
      </footer>

      {/* Code Drawer / Inspector */}
      {drawerData && (
        <div className="showcase-drawer-backdrop" onClick={() => setDrawerData(null)}>
          <div className="showcase-drawer" onClick={(e) => e.stopPropagation()}>
            <div className="showcase-drawer-header">
              <div>
                <h3>{drawerData.title}</h3>
                <span className="source-chip" style={{ marginTop: "4px" }}>
                  📁 {drawerData.path}
                </span>
              </div>
              <button
                className="showcase-src-link-btn"
                onClick={() => setDrawerData(null)}
                style={{ fontSize: "14px", padding: "4px 10px" }}
              >
                ✕ 关闭
              </button>
            </div>

            <div className="showcase-drawer-body">
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <span style={{ fontSize: "12px", color: "var(--text-muted)" }}>
                  参考源码切片 (已下载至本地仓库):
                </span>
                <button
                  className="showcase-src-link-btn"
                  onClick={handleCopyCode}
                  style={{
                    background: copied ? "var(--color-success)" : "var(--accent-solid)",
                    color: "#ffffff",
                    borderColor: copied ? "var(--color-success)" : "var(--accent-solid)",
                  }}
                >
                  {copied ? "✓ 已复制到剪贴板" : "📋 复制源码"}
                </button>
              </div>

              <pre className="showcase-code-block">{drawerData.snippet}</pre>

              <div
                style={{
                  padding: "14px",
                  borderRadius: "var(--radius-lg)",
                  background: "var(--bg-subtle)",
                  border: "1px solid var(--border-light)",
                  fontSize: "12px",
                  lineHeight: 1.6,
                  color: "var(--text-muted)",
                }}
              >
                <strong style={{ color: "var(--text-main)" }}>💡 接入指南：</strong>
                <br />
                - 完整源码文件保存在 <code>{drawerData.path}</code>
                <br />
                - Web 端可直接从 <code>reference-components/</code> 复制到 <code>web/src/components/</code>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
