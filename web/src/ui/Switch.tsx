import { useId, type ReactNode } from "react";
import "./Switch.css";

export interface SwitchProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  /** 可访问名与可见文案（hint 作为描述，不并入可访问名）。 */
  label: ReactNode;
  /** 补充说明，显示在 label 下方。 */
  hint?: ReactNode;
  id?: string;
  disabled?: boolean;
  className?: string;
}

/** 开关：渲染真实 checkbox，因此 getByLabelText / type=checkbox 语义可继续使用。 */
export function Switch({ checked, onChange, label, hint, id, disabled = false, className = "" }: SwitchProps) {
  const autoId = useId();
  const inputId = id ?? autoId;
  const hintId = `${inputId}-hint`;

  return (
    <div className={["ui-switch-wrap", className].filter(Boolean).join(" ")}>
      <label className={["ui-switch", disabled ? "ui-switch--disabled" : ""].filter(Boolean).join(" ")} htmlFor={inputId}>
        <input
          id={inputId}
          type="checkbox"
          checked={checked}
          disabled={disabled}
          aria-describedby={hint ? hintId : undefined}
          onChange={(event) => onChange(event.target.checked)}
        />
        <span className="ui-switch__track" aria-hidden="true" />
        <span className="ui-switch__text">{label}</span>
      </label>
      {hint ? (
        <span className="ui-switch__hint" id={hintId}>
          {hint}
        </span>
      ) : null}
    </div>
  );
}
