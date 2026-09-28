import React, { useState } from "react";

interface Props {
  onOpenSource: (title: string, path: string, snippet: string) => void;
}

export const ShadcnPrimitivesSection: React.FC<Props> = ({ onOpenSource }) => {
  // 1. Button Loading State
  const [btnLoading, setBtnLoading] = useState(false);
  const handleBtnClick = () => {
    setBtnLoading(true);
    setTimeout(() => setBtnLoading(false), 1200);
  };

  // 2. Switch & Checkbox
  const [switchChecked, setSwitchChecked] = useState(true);
  const [checkboxChecked, setCheckboxChecked] = useState(true);

  // 3. Input Text
  const [inputValue, setInputValue] = useState("TavernAgent AI");

  // 4. Accordion Open
  const [accordionOpen, setAccordionOpen] = useState<number | null>(0);

  return (
    <section className="showcase-section" id="shadcn-primitives">
      <div className="showcase-section-header">
        <div>
          <div className="showcase-section-title-wrap">
            <h2 className="showcase-section-title">基础规范原语 (Radix & Shadcn Primitives)</h2>
            <span className="showcase-source-tag">ui.shadcn.com</span>
          </div>
          <p className="showcase-section-desc">
            无障碍、高保真、可定制的现代标准组件基石：多态按钮、状态徽章、表单输入、开关控件与折叠手风琴。
          </p>
        </div>
      </div>

      <div className="showcase-grid">
        {/* 1. Button Variants */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>🔘 按钮形态矩阵 (Button Variants)</span>
                <span className="showcase-source-tag">button.tsx</span>
              </h4>
              <p>Default / Secondary / Outline / Destructive / Ghost / Loading</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "button.tsx",
                  "reference-components/shadcn-ui/ui/button.tsx",
                  `const buttonVariants = cva(\n  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50",\n  {\n    variants: {\n      variant: {\n        default: "bg-primary text-primary-foreground shadow hover:bg-primary/90",\n        destructive: "bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/90",\n        outline: "border border-input bg-background shadow-sm hover:bg-accent hover:text-accent-foreground",\n        secondary: "bg-secondary text-secondary-foreground shadow-sm hover:bg-secondary/80",\n        ghost: "hover:bg-accent hover:text-accent-foreground",\n        link: "text-primary underline-offset-4 hover:underline",\n      },\n      size: {\n        default: "h-9 px-4 py-2",\n        sm: "h-8 rounded-md px-3 text-xs",\n        lg: "h-10 rounded-md px-8",\n        icon: "h-9 w-9",\n      },\n    },\n    defaultVariants: {\n      variant: "default",\n      size: "default",\n    },\n  }\n)`
                )
              }
            >
              查看源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div style={{ display: "flex", flexWrap: "wrap", gap: "10px", justifyContent: "center" }}>
              {/* Default */}
              <button
                style={{
                  height: "36px",
                  padding: "0 16px",
                  borderRadius: "var(--radius-md)",
                  background: "var(--accent-solid)",
                  color: "var(--accent-contrast)",
                  border: "none",
                  fontSize: "13px",
                  fontWeight: 500,
                  cursor: "pointer",
                  boxShadow: "var(--shadow-sm)",
                }}
              >
                主要操作
              </button>

              {/* Secondary */}
              <button
                style={{
                  height: "36px",
                  padding: "0 16px",
                  borderRadius: "var(--radius-md)",
                  background: "var(--bg-subtle)",
                  color: "var(--text-main)",
                  border: "1px solid var(--border-light)",
                  fontSize: "13px",
                  fontWeight: 500,
                  cursor: "pointer",
                }}
              >
                次要按钮
              </button>

              {/* Outline */}
              <button
                style={{
                  height: "36px",
                  padding: "0 16px",
                  borderRadius: "var(--radius-md)",
                  background: "transparent",
                  color: "var(--text-main)",
                  border: "1px solid var(--border-mid)",
                  fontSize: "13px",
                  fontWeight: 500,
                  cursor: "pointer",
                }}
              >
                轮廓线
              </button>

              {/* Destructive */}
              <button
                style={{
                  height: "36px",
                  padding: "0 16px",
                  borderRadius: "var(--radius-md)",
                  background: "var(--color-danger-text)",
                  color: "#ffffff",
                  border: "none",
                  fontSize: "13px",
                  fontWeight: 500,
                  cursor: "pointer",
                }}
              >
                危险操作
              </button>

              {/* Loading */}
              <button
                onClick={handleBtnClick}
                disabled={btnLoading}
                style={{
                  height: "36px",
                  padding: "0 16px",
                  borderRadius: "var(--radius-md)",
                  background: "var(--bg-subtle)",
                  color: "var(--text-main)",
                  border: "1px solid var(--border-light)",
                  fontSize: "13px",
                  fontWeight: 500,
                  cursor: "pointer",
                  display: "flex",
                  alignItems: "center",
                  gap: "6px",
                }}
              >
                {btnLoading && (
                  <span
                    style={{
                      width: "12px",
                      height: "12px",
                      border: "2px solid var(--text-muted)",
                      borderTopColor: "transparent",
                      borderRadius: "50%",
                      animation: "spin 0.8s linear infinite",
                      display: "inline-block",
                    }}
                  />
                )}
                {btnLoading ? "处理中..." : "点击加载"}
              </button>
            </div>
          </div>
        </div>

        {/* 2. Badge & Status Pills */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>🏷 状态徽标与标签 (Badges & Pills)</span>
                <span className="showcase-source-tag">badge.tsx</span>
              </h4>
              <p>轻量元数据标记、运行状态指示与呼吸灯效果</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "badge.tsx",
                  "reference-components/shadcn-ui/ui/badge.tsx",
                  `const badgeVariants = cva(\n  "inline-flex items-center rounded-md border px-2.5 py-0.5 text-xs font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2",\n  {\n    variants: {\n      variant: {\n        default: "border-transparent bg-primary text-primary-foreground shadow hover:bg-primary/80",\n        secondary: "border-transparent bg-secondary text-secondary-foreground hover:bg-secondary/80",\n        destructive: "border-transparent bg-destructive text-destructive-foreground shadow hover:bg-destructive/80",\n        outline: "text-foreground",\n      },\n    },\n  }\n)`
                )
              }
            >
              查看源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div style={{ display: "flex", flexWrap: "wrap", gap: "10px", alignItems: "center", justifyContent: "center" }}>
              <span
                style={{
                  display: "inline-flex",
                  alignItems: "center",
                  gap: "6px",
                  padding: "4px 10px",
                  borderRadius: "var(--radius-full)",
                  background: "rgba(22, 163, 74, 0.12)",
                  color: "var(--color-success)",
                  fontSize: "12px",
                  fontWeight: 600,
                  border: "1px solid rgba(22, 163, 74, 0.2)",
                }}
              >
                <span style={{ width: "6px", height: "6px", borderRadius: "50%", background: "var(--color-success)" }} />
                运行正常 (Online)
              </span>

              <span
                style={{
                  padding: "4px 10px",
                  borderRadius: "var(--radius-full)",
                  background: "var(--accent-solid)",
                  color: "var(--accent-contrast)",
                  fontSize: "12px",
                  fontWeight: 600,
                }}
              >
                v1.2.0-beta
              </span>

              <span
                style={{
                  padding: "4px 10px",
                  borderRadius: "var(--radius-full)",
                  background: "var(--bg-subtle)",
                  color: "var(--text-muted)",
                  border: "1px solid var(--border-light)",
                  fontSize: "12px",
                  fontWeight: 500,
                }}
              >
                轻量模型: Qwen-2.5-7B
              </span>

              <span
                style={{
                  display: "inline-flex",
                  alignItems: "center",
                  gap: "6px",
                  padding: "4px 10px",
                  borderRadius: "var(--radius-full)",
                  background: "rgba(234, 88, 12, 0.12)",
                  color: "var(--color-warning)",
                  fontSize: "12px",
                  fontWeight: 600,
                  border: "1px solid rgba(234, 88, 12, 0.2)",
                }}
              >
                <span style={{ width: "6px", height: "6px", borderRadius: "50%", background: "var(--color-warning)" }} />
                内存占用较高
              </span>
            </div>
          </div>
        </div>

        {/* 3. Input & Switch Controls */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>🎛 表单输入与开关 (Input & Switch)</span>
                <span className="showcase-source-tag">input.tsx / switch.tsx</span>
              </h4>
              <p>无障碍微焦点环、自适应边框与平滑滑动反馈</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "switch.tsx",
                  "reference-components/shadcn-ui/ui/switch.tsx",
                  `const Switch = React.forwardRef<...>(({ className, ...props }, ref) => (\n  <SwitchPrimitives.Root\n    className={cn(\n      "peer inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:bg-primary data-[state=unchecked]:bg-input",\n      className\n    )}\n    ref={ref}\n    {...props}\n  >\n    <SwitchPrimitives.Thumb className="pointer-events-none block h-4 w-4 rounded-full bg-background shadow-lg ring-0 transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0" />\n  </SwitchPrimitives.Root>\n));`
                )
              }
            >
              查看源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div style={{ width: "100%", maxWidth: "320px", display: "flex", flexDirection: "column", gap: "16px" }}>
              <div>
                <label style={{ display: "block", fontSize: "12px", fontWeight: 600, color: "var(--text-main)", marginBottom: "6px" }}>
                  剧本标题 / 模组命名
                </label>
                <input
                  type="text"
                  value={inputValue}
                  onChange={(e) => setInputValue(e.target.value)}
                  style={{
                    width: "100%",
                    height: "38px",
                    padding: "0 12px",
                    borderRadius: "var(--radius-md)",
                    background: "var(--bg-surface)",
                    border: "1px solid var(--border-light)",
                    color: "var(--text-main)",
                    fontSize: "13px",
                    outline: "none",
                    boxShadow: "var(--shadow-sm)",
                    transition: "border-color 0.15s ease",
                  }}
                />
              </div>

              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                <span style={{ fontSize: "13px", color: "var(--text-main)" }}>启用动态世界书向量检索</span>
                <div
                  onClick={() => setSwitchChecked(!switchChecked)}
                  style={{
                    width: "42px",
                    height: "24px",
                    borderRadius: "var(--radius-full)",
                    background: switchChecked ? "var(--accent-solid)" : "var(--border-mid)",
                    position: "relative",
                    cursor: "pointer",
                    transition: "background 0.2s ease",
                  }}
                >
                  <div
                    style={{
                      position: "absolute",
                      top: "2px",
                      left: switchChecked ? "20px" : "2px",
                      width: "20px",
                      height: "20px",
                      borderRadius: "50%",
                      background: "var(--accent-contrast)",
                      boxShadow: "var(--shadow-sm)",
                      transition: "left 0.2s cubic-bezier(0.16, 1, 0.3, 1)",
                    }}
                  />
                </div>
              </div>

              <div style={{ display: "flex", alignItems: "center", gap: "10px", cursor: "pointer" }} onClick={() => setCheckboxChecked(!checkboxChecked)}>
                <input type="checkbox" checked={checkboxChecked} onChange={() => {}} style={{ cursor: "pointer" }} />
                <span style={{ fontSize: "13px", color: "var(--text-muted)" }}>检定前向主持人（DM）申请二次核准</span>
              </div>
            </div>
          </div>
        </div>

        {/* 4. Accordion */}
        <div className="showcase-card">
          <div className="showcase-card-header">
            <div className="showcase-card-meta">
              <h4>
                <span>📂 折叠面板 (Accordion)</span>
                <span className="showcase-source-tag">accordion.tsx</span>
              </h4>
              <p>优雅处理多段角色卡背景、技能树与长篇设定集</p>
            </div>
            <button
              className="showcase-src-link-btn"
              onClick={() =>
                onOpenSource(
                  "accordion.tsx",
                  "reference-components/shadcn-ui/ui/accordion.tsx",
                  `const AccordionItem = React.forwardRef<...>(({ className, ...props }, ref) => (\n  <AccordionPrimitive.Item ref={ref} className={cn("border-b", className)} {...props} />\n));\nconst AccordionTrigger = ...\nconst AccordionContent = ...`
                )
              }
            >
              查看源码
            </button>
          </div>
          <div className="showcase-card-stage">
            <div style={{ width: "100%", maxWidth: "340px", display: "flex", flexDirection: "column", gap: "8px" }}>
              {[
                { title: "角色基础属性 (D&D 5e)", content: "力量 16 (+3) | 敏捷 12 (+1) | 体质 14 (+2) | 智力 10 (+0)" },
                { title: "专长与特长", content: "幸运儿：每天可重掷 3 次 d20 检定；警觉：先攻加值 +5。" },
                { title: "随身背包清单", content: "火把 x5、精制品长剑、治疗药水 x2、羊皮纸地图碎片。" },
              ].map((item, index) => {
                const isOpen = accordionOpen === index;
                return (
                  <div
                    key={index}
                    style={{
                      border: "1px solid var(--border-light)",
                      borderRadius: "var(--radius-md)",
                      background: "var(--bg-surface)",
                      overflow: "hidden",
                    }}
                  >
                    <div
                      onClick={() => setAccordionOpen(isOpen ? null : index)}
                      style={{
                        padding: "12px 14px",
                        display: "flex",
                        justifyContent: "space-between",
                        alignItems: "center",
                        cursor: "pointer",
                        fontWeight: 500,
                        fontSize: "13px",
                        color: "var(--text-main)",
                      }}
                    >
                      <span>{item.title}</span>
                      <span style={{ fontSize: "11px", color: "var(--text-dim)", transform: isOpen ? "rotate(180deg)" : "rotate(0deg)", transition: "transform 0.2s ease" }}>
                        ▼
                      </span>
                    </div>
                    {isOpen && (
                      <div style={{ padding: "0 14px 12px", fontSize: "12.5px", color: "var(--text-muted)", lineHeight: 1.5 }}>
                        {item.content}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};
