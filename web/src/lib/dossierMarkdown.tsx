// dossierMarkdown: 轻量纯 React 结构化人设与设定排版渲染器
// 具备宏替换、对白标签识别、段落折行、列表与强调解析能力，零注入面。
import React, { type ReactNode } from "react";
import { replaceMacros, type MacroContext } from "./characterMacros";
import { isContainerLine, parseAttrLine, parseBracketAttrGroup, type AttrPair } from "./dossierAttrs";

interface DossierMarkdownProps {
  content?: string;
  context: MacroContext;
  className?: string;
}

const INLINE_TOKEN_REGEX = /(\*\*[^*\n]+\*\*|\*[^*\n]+\*|`[^`\n]+`|\[[^\]\n]+\])/g;

function renderInline(text: string): ReactNode[] {
  if (!text) return [];
  return text.split(INLINE_TOKEN_REGEX).map((part, idx) => {
    if (part.startsWith("**") && part.endsWith("**") && part.length > 4) {
      return <strong key={idx}>{part.slice(2, -2)}</strong>;
    }
    if (part.startsWith("*") && part.endsWith("*") && part.length > 2) {
      return <em key={idx}>{part.slice(1, -1)}</em>;
    }
    if (part.startsWith("`") && part.endsWith("`") && part.length > 2) {
      return <code key={idx} className="dossier-code-inline">{part.slice(1, -1)}</code>;
    }
    if (part.startsWith("[") && part.endsWith("]") && part.length > 2) {
      return (
        <span key={idx} className="dossier-bracket-tag">
          {part}
        </span>
      );
    }
    return part;
  });
}

interface ParsedBlock {
  type: "h1" | "h2" | "h3" | "divider" | "quote" | "pill" | "list" | "attrs" | "paragraph";
  content: string;
  lines?: string[];
  attrs?: AttrPair[];
}

function parseBlocks(raw: string): ParsedBlock[] {
  // 规范化换行（兼顾包含字面量 \n 文本的 JSON 角色卡）
  const normalized = raw.replace(/\\r\\n/g, "\n").replace(/\\n/g, "\n");
  const lines = normalized.split(/\r?\n/);
  const blocks: ParsedBlock[] = [];
  let currentParagraph: string[] = [];
  let currentQuote: string[] = [];
  let currentList: string[] = [];
  let currentAttrs: AttrPair[] = [];

  const flushParagraph = () => {
    if (currentParagraph.length > 0) {
      blocks.push({ type: "paragraph", content: currentParagraph.join("\n") });
      currentParagraph = [];
    }
  };
  const flushQuote = () => {
    if (currentQuote.length > 0) {
      blocks.push({ type: "quote", content: currentQuote.join("\n") });
      currentQuote = [];
    }
  };
  const flushList = () => {
    if (currentList.length > 0) {
      blocks.push({ type: "list", content: "", lines: [...currentList] });
      currentList = [];
    }
  };
  const flushAttrs = () => {
    if (currentAttrs.length > 0) {
      blocks.push({ type: "attrs", content: "", attrs: [...currentAttrs] });
      currentAttrs = [];
    }
  };
  const flushAll = () => {
    flushParagraph();
    flushQuote();
    flushList();
    flushAttrs();
  };

  for (const line of lines) {
    const trimmed = line.trim();

    if (!trimmed) {
      // 空行不切断属性表：V2 卡常把每个属性各占一段，
      // 若在这里断开就会退化成一堆单行小表。
      flushParagraph();
      flushQuote();
      flushList();
      continue;
    }

    // 属性表的容器符号单独成行（社区卡常把整表包在 [ ] 里）：丢弃，
    // 否则面板上会平白多出一个「[」段落。
    if (isContainerLine(trimmed)) {
      flushParagraph();
      flushQuote();
      flushList();
      continue;
    }

    if (trimmed.startsWith("---") || trimmed.startsWith("***")) {
      flushAll();
      blocks.push({ type: "divider", content: trimmed });
      continue;
    }

    if (trimmed.startsWith("# ")) {
      flushAll();
      blocks.push({ type: "h1", content: trimmed.slice(2).trim() });
      continue;
    }
    if (trimmed.startsWith("## ")) {
      flushAll();
      blocks.push({ type: "h2", content: trimmed.slice(3).trim() });
      continue;
    }
    if (trimmed.startsWith("### ")) {
      flushAll();
      blocks.push({ type: "h3", content: trimmed.slice(4).trim() });
      continue;
    }

    // 整行围成的一组属性：[头衔: k（"v"），k2（"v2"）]
    const attrGroup = parseBracketAttrGroup(trimmed);
    if (attrGroup) {
      flushParagraph();
      flushQuote();
      flushList();
      if (attrGroup.head) {
        flushAttrs();
        blocks.push({ type: "pill", content: attrGroup.head });
      }
      currentAttrs.push(...attrGroup.pairs);
      continue;
    }

    // 单行属性：{"Key": ("v")} / Key：（"v" + "v"）/ Key：值
    const attr = parseAttrLine(trimmed);
    if (attr) {
      flushParagraph();
      flushQuote();
      flushList();
      currentAttrs.push(attr);
      continue;
    }

    if (/^\[[^\]]+\]$/.test(trimmed)) {
      flushAll();
      blocks.push({ type: "pill", content: trimmed.slice(1, -1).trim() });
      continue;
    }

    if (/^\s*[-*•]\s+/.test(line)) {
      flushParagraph();
      flushQuote();
      flushAttrs();
      currentList.push(line.replace(/^\s*[-*•]\s+/, ""));
      continue;
    } else if (currentList.length > 0) {
      flushList();
    }

    if (trimmed.startsWith(">")) {
      flushParagraph();
      flushList();
      flushAttrs();
      currentQuote.push(trimmed.replace(/^>\s?/, ""));
      continue;
    } else if (currentQuote.length > 0) {
      flushQuote();
    }

    flushAttrs();
    currentParagraph.push(line);
  }

  flushAll();
  return blocks;
}

export const DossierMarkdown: React.FC<DossierMarkdownProps> = ({
  content,
  context,
  className = "dossier-markdown-body",
}) => {
  if (!content || !content.trim()) {
    return <div className="dossier-empty-note">暂无相关设定描述</div>;
  }

  const processed = replaceMacros(content, context);
  const blocks = parseBlocks(processed);

  return (
    <div className={className}>
      {blocks.map((block, bIdx) => {
        switch (block.type) {
          case "divider":
            return (
              <div key={bIdx} className="dossier-divider">
                <span>{block.content.replace(/^[-*]+\s*/, "").replace(/\s*[-*]+$/, "")}</span>
              </div>
            );
          case "h1":
            return <h4 key={bIdx} className="dossier-h1">{renderInline(block.content)}</h4>;
          case "h2":
            return <h5 key={bIdx} className="dossier-h2">{renderInline(block.content)}</h5>;
          case "h3":
            return <h6 key={bIdx} className="dossier-h3">{renderInline(block.content)}</h6>;
          case "pill":
            return (
              <div key={bIdx} className="dossier-section-pill">
                <span className="pill-dot">●</span>
                <span>{block.content}</span>
              </div>
            );
          case "quote":
            return (
              <blockquote key={bIdx} className="dossier-blockquote">
                {renderInline(block.content)}
              </blockquote>
            );
          case "list":
            return (
              <ul key={bIdx} className="dossier-ul">
                {block.lines?.map((line, lIdx) => (
                  <li key={lIdx}>{renderInline(line)}</li>
                ))}
              </ul>
            );
          case "attrs":
            return (
              <div key={bIdx} className="dossier-attr-grid">
                {block.attrs?.map((attr, aIdx) => (
                  <div key={aIdx} className="dossier-attr-row">
                    <span className="dossier-attr-key">{renderInline(attr.key)}</span>
                    <span className="dossier-attr-value">{renderInline(attr.value)}</span>
                  </div>
                ))}
              </div>
            );
          case "paragraph":
          default: {
            const lines = block.content.split("\n");
            return (
              <p key={bIdx} className="dossier-p">
                {lines.map((line, lIdx) => (
                  <React.Fragment key={lIdx}>
                    {renderInline(line)}
                    {lIdx < lines.length - 1 && <br />}
                  </React.Fragment>
                ))}
              </p>
            );
          }
        }
      })}
    </div>
  );
};
