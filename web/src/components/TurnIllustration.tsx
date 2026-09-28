import { useState } from "react";
import { createPortal } from "react-dom";
import { useDialogFocus } from "../lib/useDialogFocus";
import { useImageSettings } from "../stores/imageSettingsStore";
import { Button } from "../ui/Button";
import { Icon } from "../ui/Icon";
import "./IllustrationModal.css";

export interface TurnIllustrationProps {
  turnId: string;
  onOpenModal?: () => void;
}

export function TurnIllustration({ turnId, onOpenModal }: TurnIllustrationProps) {
  const illustration = useImageSettings((s) => s.turnIllustrations[turnId]);
  const [lightboxOpen, setLightboxOpen] = useState(false);
  // 灯箱接入统一模态栈（B10-R1）：Esc 关灯箱、焦点归还、打开期间背景 inert 都由栈负责。
  const lightbox = useDialogFocus(lightboxOpen, () => setLightboxOpen(false));

  if (!illustration) {
    return null;
  }

  return (
    <>
      <div className="turn-illustration-wrap">
        <img
          src={illustration.url}
          alt={illustration.prompt || "回合场景插画"}
          className="turn-illustration-img"
          onClick={() => setLightboxOpen(true)}
          title="点击放大查看全屏大图"
        />
        <div className="turn-illustration-bar">
          <span className="turn-illustration-label"><Icon name="image" size={13} /> 场景插画 · 已保存</span>
          <div className="turn-illustration-actions">
            <Button size="sm" variant="outline" onClick={() => setLightboxOpen(true)}>
              <Icon name="expand" size={12} /> 放大
            </Button>
            {onOpenModal && (
              <Button size="sm" variant="outline" onClick={onOpenModal}>
                <Icon name="refresh" size={12} /> 重绘
              </Button>
            )}
          </div>
        </div>
      </div>

      {lightboxOpen && typeof document !== "undefined" && createPortal(
        <div
          {...lightbox}
          className="lightbox-backdrop"
          aria-label={illustration.prompt || "全屏场景插画"}
          onClick={() => setLightboxOpen(false)}
        >
          <img
            src={illustration.url}
            alt={illustration.prompt || "全屏场景插画"}
            className="lightbox-img"
            onClick={(e) => e.stopPropagation()}
          />
        </div>,
        document.body
      )}
    </>
  );
}
