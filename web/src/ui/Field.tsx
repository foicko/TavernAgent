import {
  createContext,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
  type InputHTMLAttributes,
  type ReactNode,
  type SelectHTMLAttributes,
} from "react";
import "./Field.css";

interface FieldContextValue {
  id?: string;
  describedBy?: string;
}

const FieldContext = createContext<FieldContextValue>({});

/** 取字段 id / aria-describedby：显式传入优先，否则沿用外层 Field 自动生成的。 */
export function useField(): FieldContextValue {
  return useContext(FieldContext);
}

export interface FieldProps {
  label: string;
  /** 不传则自动生成，并注入给内部控件（无需手写 htmlFor）。 */
  id?: string;
  hint?: ReactNode;
  error?: string | null;
  children: ReactNode;
  className?: string;
}

export function Field({ label, id, hint, error, children, className = "" }: FieldProps) {
  const autoId = useId();
  const fieldId = id ?? autoId;
  const hintId = `${fieldId}-hint`;
  const errorId = `${fieldId}-error`;
  const describedBy = error ? errorId : hint ? hintId : undefined;

  return (
    <div className={["ui-field", error ? "ui-field--invalid" : "", className].filter(Boolean).join(" ")}>
      <label className="ui-field__label" htmlFor={fieldId}>
        {label}
      </label>
      <FieldContext.Provider value={{ id: fieldId, describedBy }}>{children}</FieldContext.Provider>
      {!error && hint ? (
        <span className="ui-field__hint" id={hintId}>
          {hint}
        </span>
      ) : null}
      {error ? (
        <span className="ui-field__error" id={errorId} role="alert">
          {error}
        </span>
      ) : null}
    </div>
  );
}

type TextInputProps = Omit<InputHTMLAttributes<HTMLInputElement>, "className"> & { className?: string };

export function TextInput({ id, className = "", ...rest }: TextInputProps) {
  const field = useField();
  return (
    <input
      {...rest}
      id={id ?? field.id}
      aria-describedby={rest["aria-describedby"] ?? field.describedBy}
      className={["ui-input", className].filter(Boolean).join(" ")}
    />
  );
}

type SelectProps = Omit<SelectHTMLAttributes<HTMLSelectElement>, "className"> & { className?: string };

export function Select({ id, className = "", children, ...rest }: SelectProps) {
  const field = useField();
  return (
    <select
      {...rest}
      id={id ?? field.id}
      aria-describedby={rest["aria-describedby"] ?? field.describedBy}
      className={["ui-select", className].filter(Boolean).join(" ")}
    >
      {children}
    </select>
  );
}

export interface NumberInputProps
  extends Omit<InputHTMLAttributes<HTMLInputElement>, "value" | "onChange" | "type" | "min" | "max" | "className"> {
  value: number | undefined;
  onValueChange: (value: number | undefined) => void;
  /** 右侧单位（如 "k" / "tokens"），仅作视觉提示。 */
  suffix?: string;
  className?: string;
}

/**
 * 数字输入：内部保留字符串草稿，因此可以被清空（不会"删空即变 0"）。
 * 只要能解析成有限数就立刻回传；留空回传 undefined。取值范围校验由表单负责，
 * 以免边输入边被夹紧。
 */
export function NumberInput({
  value,
  onValueChange,
  suffix,
  id,
  className = "",
  onBlur,
  ...rest
}: NumberInputProps) {
  const field = useField();
  const [draft, setDraft] = useState(() => (value === undefined ? "" : String(value)));
  const focused = useRef(false);

  useEffect(() => {
    if (!focused.current) setDraft(value === undefined ? "" : String(value));
  }, [value]);

  const handleChange = (raw: string) => {
    if (!/^\d*\.?\d*$/.test(raw)) return;
    setDraft(raw);
    const trimmed = raw.trim();
    if (trimmed === "" || trimmed === ".") {
      onValueChange(undefined);
      return;
    }
    const parsed = Number(trimmed);
    if (Number.isFinite(parsed)) onValueChange(parsed);
  };

  const input = (
    <input
      {...rest}
      id={id ?? field.id}
      aria-describedby={rest["aria-describedby"] ?? field.describedBy}
      className={["ui-input", "ui-input--mono", className].filter(Boolean).join(" ")}
      type="text"
      inputMode="decimal"
      value={draft}
      onFocus={() => {
        focused.current = true;
      }}
      onChange={(event) => handleChange(event.target.value)}
      onBlur={(event) => {
        focused.current = false;
        setDraft(value === undefined ? "" : String(value));
        onBlur?.(event);
      }}
    />
  );

  if (!suffix) return input;

  return (
    <div className="ui-input-group">
      {input}
      <span className="ui-input-group__suffix">{suffix}</span>
    </div>
  );
}
