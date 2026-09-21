import { useState, useEffect } from "react";
import { createPortal } from "react-dom";
import { useImageSettings } from "../stores/imageSettingsStore";
import { Button } from "../ui/Button";
import "./IllustrationModal.css";

export interface TurnIllustrationProps {
  turnId: string;
  onOpenModal?: () => void;
}

export function TurnIllustration({ turnId, onOpenModal }: TurnIllustrationProps) {
  const illustration = useImageSettings((s) => s.turnIllustrations[turnId]);
  const [lightboxOpen, setLightboxOpen] = useState(false);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && lightboxOpen) {
        setLightboxOpen(false);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [lightboxOpen]);

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
          <span className="turn-illustration-label">✦ 场景插画 · 已保存</span>
          <div className="turn-illustration-actions">
            <Button size="sm" variant="quiet" onClick={() => setLightboxOpen(true)}>
              🔍 放大
            </Button>
            {onOpenModal && (
              <Button size="sm" variant="quiet" onClick={onOpenModal}>
                🔄 重绘
              </Button>
            )}
          </div>
        </div>
      </div>

      {lightboxOpen && typeof document !== "undefined" && createPortal(
        <div className="lightbox-backdrop" onClick={() => setLightboxOpen(false)}>
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
