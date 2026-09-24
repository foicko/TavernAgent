// InventoryActionBubble: 背包信物轻量动作菜单（锚定在信物槽位坐标）
import React, { useEffect } from "react";
import { useUi } from "../stores/uiStore";
import { storyBusy, useStory } from "../stores/storyStore";
import type { InventoryAct } from "../lib/characterPresets";
import { Button } from "../ui/Button";
import { Popover } from "../ui/Popover";
import "./InventoryActionBubble.css";

export const InventoryActionBubble: React.FC = () => {
  const invBubble = useUi((s) => s.invBubble);
  const hideInvBubble = useUi((s) => s.hideInvBubble);
  const injectComposerText = useUi((s) => s.injectComposerText);
  const setWorldbookModalOpen = useUi((s) => s.setWorldbookModalOpen);
  const notifyQuiet = useUi((s) => s.notifyQuiet);
  const view = useStory(s => s.view);
  const viewNodeId = useStory(s => s.viewNodeId);
  const storyIsBusy = useStory(storyBusy);
  const readOnly = !!viewNodeId || storyIsBusy;
  useEffect(() => { hideInvBubble(); }, [view?.sessionId, view?.branch.branchId, viewNodeId, hideInvBubble]);

  const open = !!invBubble?.visible;

  const handleAct = (act: InventoryAct) => {
    if (readOnly) return;
    if (act.isModal) {
      hideInvBubble();
      setWorldbookModalOpen(true);
      return;
    }
    if (act.text) {
      injectComposerText(act.text);
      notifyQuiet(`已填入「${act.label}」，发送后推进剧情。`);
    }
    hideInvBubble();
  };

  return (
    <Popover
      open={open}
      onClose={hideInvBubble}
      anchor={invBubble ? { x: invBubble.x, y: invBubble.y } : null}
      label={invBubble ? `${invBubble.item.name} 动作` : "信物动作"}
      side="top"
      align="center"
      width={196}
      className="inv-bubble"
    >
      {invBubble ? (
        <>
          <div className="inv-bubble__title" id="item-bubble-title">
            {invBubble.item.name} · 动作
          </div>
          <div className="inv-bubble__actions" id="item-bubble-buttons">
            {invBubble.item.acts.map((act) => (
              <Button
                key={act.key}
                size="sm"
                variant="ghost"
                block
                className="inv-bubble__act"
                disabled={readOnly}
                onClick={() => handleAct(act)}
              >
                {act.label}
              </Button>
            ))}
          </div>
        </>
      ) : null}
    </Popover>
  );
};
