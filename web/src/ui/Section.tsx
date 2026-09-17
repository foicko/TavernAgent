import type { ElementType, ReactNode } from "react";
import "./Section.css";

export interface SectionProps {
  title: ReactNode;
  /** 有序区块的序号徽标；非流程化区块不要传。 */
  index?: number | string;
  description?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  headingLevel?: 2 | 3;
  className?: string;
}

export function Section({
  title,
  index,
  description,
  actions,
  children,
  headingLevel = 2,
  className = "",
}: SectionProps) {
  const Heading = `h${headingLevel}` as ElementType;
  return (
    <section className={["ui-section", className].filter(Boolean).join(" ")}>
      <div className="ui-section__head">
        <div className="ui-section__text">
          <Heading className="ui-section__heading">
            {index !== undefined ? <span className="ui-section__index">{index}</span> : null}
            {title}
          </Heading>
          {description ? <p className="ui-section__description">{description}</p> : null}
        </div>
        {actions ? <div className="ui-section__actions">{actions}</div> : null}
      </div>
      <div className="ui-section__body">{children}</div>
    </section>
  );
}
