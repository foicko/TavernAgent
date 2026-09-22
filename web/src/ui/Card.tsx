import type { ButtonHTMLAttributes, HTMLAttributes, ReactNode } from "react";
import "./Card.css";

export type CardTone = "surface" | "inset" | "elevated";
export type CardPadding = "none" | "sm" | "md" | "lg";

interface CardBaseProps {
  tone?: CardTone;
  padding?: CardPadding;
  className?: string;
  children: ReactNode;
}

export interface CardProps extends CardBaseProps, Omit<HTMLAttributes<HTMLDivElement>, "className" | "children"> {
  /** 整卡可点击：渲染为 <button>，带 hover 抬升与按压反馈。 */
  interactive?: false;
}

export interface InteractiveCardProps
  extends CardBaseProps,
    Omit<ButtonHTMLAttributes<HTMLButtonElement>, "className" | "children"> {
  interactive: true;
}

function cardClasses(tone: CardTone, padding: CardPadding, interactive: boolean, className: string) {
  return ["ui-card", `ui-card--${tone}`, `ui-card--pad-${padding}`, interactive ? "ui-card--interactive" : "", className]
    .filter(Boolean)
    .join(" ");
}

/**
 * 卡片（shadcn card 结构：Header / Title / Description / Content / Footer）。
 * surface：白卡 + 发丝线；inset：下沉井；elevated：浮起阴影（仅用于浮层内容）。
 */
export function Card(props: CardProps | InteractiveCardProps) {
  if (props.interactive) {
    const { tone = "surface", padding = "md", className = "", children, interactive: _i, type, ...rest } = props;
    void _i;
    return (
      <button {...rest} type={type ?? "button"} className={cardClasses(tone, padding, true, className)}>
        {children}
      </button>
    );
  }
  const { tone = "surface", padding = "md", className = "", children, interactive: _i, ...rest } = props;
  void _i;
  return (
    <div {...rest} className={cardClasses(tone, padding, false, className)}>
      {children}
    </div>
  );
}

export function CardHeader({ className = "", children, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return <div {...rest} className={["ui-card__header", className].filter(Boolean).join(" ")}>{children}</div>;
}

export function CardTitle({ className = "", children, ...rest }: HTMLAttributes<HTMLHeadingElement>) {
  return <h3 {...rest} className={["ui-card__title", className].filter(Boolean).join(" ")}>{children}</h3>;
}

export function CardDescription({ className = "", children, ...rest }: HTMLAttributes<HTMLParagraphElement>) {
  return <p {...rest} className={["ui-card__description", className].filter(Boolean).join(" ")}>{children}</p>;
}

export function CardContent({ className = "", children, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return <div {...rest} className={["ui-card__content", className].filter(Boolean).join(" ")}>{children}</div>;
}

export function CardFooter({ className = "", children, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return <div {...rest} className={["ui-card__footer", className].filter(Boolean).join(" ")}>{children}</div>;
}
