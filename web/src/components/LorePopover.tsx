// LorePopover: 正文世界书词条悬浮简报卡（锚定在命中词坐标，纯展示，不抢焦点）
import React from "react";
import { useUi } from "../stores/uiStore";
import { Button } from "../ui/Button";
import { Icon } from "../ui/Icon";
import { Popover } from "../ui/Popover";
import "./LorePopover.css";

export const LorePopover: React.FC = () => {
  const lorePopover = useUi((s) => s.lorePopover);
  const hideLorePop = useUi((s) => s.hideLorePop);
  const injectComposerText = useUi((s) => s.injectComposerText);
  const setWorldbookModalOpen = useUi((s) => s.setWorldbookModalOpen);

  const open = !!lorePopover?.visible;

  const handleQuote = () => {
    if (!lorePopover) return;
    injectComposerText(`*关于【${lorePopover.title}】的线索* `);
    hideLorePop();
  };

  const handleOpenWorldbook = () => {
    hideLorePop();
    setWorldbookModalOpen(true);
  };

  return (
    <Popover
      open={open}
      onClose={hideLorePop}
      anchor={lorePopover ? { x: lorePopover.x, y: lorePopover.y } : null}
      label="世界书词条"
      side="bottom"
      align="start"
      offset={0}
      autoFocus={false}
      returnFocus={false}
      onPointerLeave={hideLorePop}
      width={290}
      className="lore-card"
    >
      {lorePopover ? (
        <>
          <div className="lore-card__tag">
            <Icon name="book" size={12} /> 世界书词条
          </div>
          <div className="lore-card__title" id="lore-pop-title">
            {lorePopover.title}
          </div>
          <div className="lore-card__desc" id="lore-pop-desc">
            {lorePopover.desc}
          </div>
          <div className="lore-card__footer">
            <span className="lore-card__keys" id="lore-pop-keys">
              触发词：{lorePopover.keys}
            </span>
            <div className="lore-card__actions">
              <Button size="sm" variant="secondary" onClick={handleQuote}>
                <Icon name="cornerDownLeft" size={12} /> 引用
              </Button>
              <Button size="sm" variant="ghost" onClick={handleOpenWorldbook}>
                查看全貌 <Icon name="arrowRight" size={12} />
              </Button>
            </div>
          </div>
        </>
      ) : null}
    </Popover>
  );
};
