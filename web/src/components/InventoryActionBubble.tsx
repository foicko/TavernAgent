// InventoryActionBubble: 背包信物轻量交互气泡菜单 (严格对齐设计原型)
import React, { useEffect } from "react";
import { useUi } from "../stores/uiStore";
import { storyBusy, useStory } from "../stores/storyStore";
import type { InventoryAct } from "../lib/characterPresets";
import "./InventoryActionBubble.css";

export const InventoryActionBubble: React.FC = () => {
  const { invBubble, hideInvBubble, injectComposerText, setWorldbookModalOpen, notifyQuiet } = useUi();
  const view = useStory(s => s.view);
  const viewNodeId = useStory(s => s.viewNodeId);
  const storyIsBusy = useStory(storyBusy);
  const readOnly = !!viewNodeId || storyIsBusy;
  useEffect(() => { hideInvBubble(); }, [view?.sessionId, view?.branch.branchId, viewNodeId, hideInvBubble]);

  if (!invBubble || !invBubble.visible) return null;

  const handleAct = (act: InventoryAct) => {
    if (readOnly) return;
    if (act.isModal) {
      hideInvBubble();
      setWorldbookModalOpen(true);
      return;
    }
    if (act.text) {
      if (act.actionRef) useStory.getState().prepareAction(act.text, act.actionRef);
      else injectComposerText(act.text);
      notifyQuiet(`已填入「${act.label}」，发送后推进剧情。`);
    }
    hideInvBubble();
  };

  return (
    <>
      <div className="item-bubble-backdrop" onClick={hideInvBubble} />
      {/* 位置来自点击坐标，属于数据驱动几何，仍以内联 left/top 传入 */}
      <div
        className="item-action-bubble"
        id="item-action-bubble"
        style={{ left: `${invBubble.x}px`, top: `${invBubble.y}px` }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="item-bubble-title" id="item-bubble-title">
          {invBubble.item.icon} {invBubble.item.name} 动作
        </div>
        <div className="item-bubble-buttons" id="item-bubble-buttons">
          {invBubble.item.acts.map((act) => (
            <button
              key={act.key}
              className="item-action-btn"
              disabled={readOnly}
              onClick={() => handleAct(act)}
            >
              {act.label}
            </button>
          ))}
        </div>
      </div>
    </>
  );
};
