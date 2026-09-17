// LorePopover: 正文世界书词条悬浮简报卡 (严格对齐设计原型)
import React from "react";
import { useUi } from "../stores/uiStore";
import "./LorePopover.css";

export const LorePopover: React.FC = () => {
  const { lorePopover, hideLorePop, injectComposerText, setWorldbookModalOpen } = useUi();

  if (!lorePopover || !lorePopover.visible) return null;

  const handleQuote = () => {
    injectComposerText(`*关于【${lorePopover.title}】的线索* `);
    hideLorePop();
  };

  const handleOpenWorldbook = () => {
    hideLorePop();
    setWorldbookModalOpen(true);
  };

  return (
    // 位置来自命中词的屏幕坐标，属于数据驱动几何，仍以内联 left/top 传入
    <div
      className="lore-popover"
      id="lore-popover"
      style={{ left: `${lorePopover.x}px`, top: `${lorePopover.y}px` }}
      onMouseLeave={hideLorePop}
    >
      <div className="lore-pop-tag">📖 世界书词条</div>
      <div className="lore-pop-title" id="lore-pop-title">
        {lorePopover.title}
      </div>
      <div className="lore-pop-desc" id="lore-pop-desc">
        {lorePopover.desc}
      </div>
      <div className="lore-pop-footer">
        <span className="lore-pop-keys" id="lore-pop-keys">
          触发词：{lorePopover.keys}
        </span>
        <div className="lore-pop-btn-group">
          <button className="lore-pop-btn-quote" onClick={handleQuote}>
            引用至输入框 ↵
          </button>
          <button className="lore-pop-link" onClick={handleOpenWorldbook}>
            查看全貌 →
          </button>
        </div>
      </div>
    </div>
  );
};
