import type { ReactNode } from "react";
import { createPortal } from "react-dom";
import { useDialogFocus } from "../lib/useDialogFocus";
import { IconButton } from "./Button";
import "./Modal.css";

export interface ModalProps {
  open?: boolean;
  onClose: () => void;
  /** 对话框可访问名（aria-label）。 */
  label: string;
  title?: ReactNode;
  description?: ReactNode;
  /** 头部右侧操作区（关闭按钮之前）。 */
  headerActions?: ReactNode;
  footer?: ReactNode;
  size?: "sm" | "md" | "lg" | "xl" | "full";
  /** 内容贴边：body 去掉内边距与间距，交给内容自绘布局（图谱、灯箱等）。 */
  flush?: boolean;
  /**
   * 是否可关闭：默认 true（Esc / 遮罩点击 / 右上关闭按钮）。
   * 置 false 用于必须先完成才能继续的阻塞式弹窗（如局域网配对）。
   */
  dismissible?: boolean;
  /**
   * 关闭按钮可见但禁用：用于"当前正处于不可中断的操作中"（如角色卡导入解析中）。
   * 传 true 时即使 dismissible 为 false 也会渲染关闭按钮，只是不可点。
   */
  closeDisabled?: boolean;
  /** 关闭按钮的可访问名，默认「关闭」。 */
  closeLabel?: string;
  /** 挂到遮罩上的 id，供既有 e2e 定位（如 #settings-modal）。 */
  id?: string;
  /** 加到遮罩上的类名。 */
  className?: string;
  /** 加到对话框上的类名（用于个别弹窗的尺寸微调）。 */
  dialogClassName?: string;
  children: ReactNode;
}

/**
 * 弹窗唯一实现：遮罩 + 焦点陷阱（复用 lib/useDialogFocus）+ 统一头部与关闭按钮。
 * Tab 焦点边界、Esc 与遮罩点击关闭、关闭后焦点回到触发元素，都在这里处理。
 */
export function Modal({
  open = true,
  onClose,
  label,
  title,
  description,
  headerActions,
  footer,
  size = "md",
  flush = false,
  dismissible = true,
  closeDisabled = false,
  closeLabel = "关闭",
  id,
  className = "",
  dialogClassName = "",
  children,
}: ModalProps) {
  const dialog = useDialogFocus(open, dismissible ? onClose : undefined);

  if (!open) return null;

  const content = (
    <div
      className={["ui-modal-backdrop", className].filter(Boolean).join(" ")}
      id={id}
      onClick={dismissible ? onClose : undefined}
    >
      <div
        {...dialog}
        aria-label={label}
        className={[
          "ui-modal",
          `ui-modal--${size}`,
          flush ? "ui-modal--flush" : "",
          dialogClassName,
        ]
          .filter(Boolean)
          .join(" ")}
        onClick={(event) => event.stopPropagation()}
      >
        <div className="ui-modal__head">
          <div className="ui-modal__text">
            {title ? <h2 className="ui-modal__title">{title}</h2> : null}
            {description ? <p className="ui-modal__description">{description}</p> : null}
          </div>
          {headerActions ? <div className="ui-modal__head-actions">{headerActions}</div> : null}
          {dismissible || closeDisabled ? (
            <IconButton label={closeLabel} disabled={closeDisabled} onClick={onClose}>
              ✕
            </IconButton>
          ) : null}
        </div>
        <div className="ui-modal__body">{children}</div>
        {footer ? <div className="ui-modal__foot">{footer}</div> : null}
      </div>
    </div>
  );

  if (typeof document === "undefined") {
    return content;
  }
  return createPortal(content, document.body);
}
