import React, { useState } from "react";
import { useUi } from "../stores/uiStore";
import { useStory } from "../stores/storyStore";
import { characterPresentation } from "../lib/characterPresentation";
import { DossierMarkdown } from "../lib/dossierMarkdown";
import { DEFAULT_PLAYER_NAME } from "../lib/characterMacros";
import { Modal } from "../ui/Modal";
import "./DossierModal.css";

export const DossierModal: React.FC = () => {
  const { dossierModalOpen, setDossierModalOpen, activeCharKey } = useUi();
  const view = useStory((s) => s.view);
  const [activeTab, setActiveTab] = useState<"persona" | "scenario" | "examples" | "system" | "notes">("persona");

  if (!dossierModalOpen) return null;

  const char = characterPresentation(activeCharKey, view);
  const playerName = view?.state?.characters?.["player"]?.name || DEFAULT_PLAYER_NAME;

  const personaText = [char.dossier?.description, char.dossier?.personality].filter(Boolean).join("\n\n");
  const scenarioText = char.dossier?.scenario || "";
  const examplesText = char.dossier?.mesExample || "";
  const systemText = [char.dossier?.systemPrompt, char.dossier?.postHistoryInstructions].filter(Boolean).join("\n\n");
  const notesText = [
    char.dossier?.creator ? `**创作者**：${char.dossier.creator}` : "",
    char.dossier?.version ? `**版本规范**：${char.dossier.version}` : "",
    char.dossier?.creatorNotes ? `**创作者留言**：\n${char.dossier.creatorNotes}` : "",
  ].filter(Boolean).join("\n\n");

  const tabs: Array<{ id: "persona" | "scenario" | "examples" | "system" | "notes"; label: string; text: string }> = [
    { id: "persona", label: "人设与性格", text: personaText },
    { id: "scenario", label: "世界与场景", text: scenarioText },
    { id: "examples", label: "对话范例", text: examplesText },
    { id: "system", label: "系统规则", text: systemText },
    { id: "notes", label: "创作者说明", text: notesText },
  ];

  const currentTabItem = tabs.find((t) => t.id === activeTab) || tabs[0];
  const close = () => setDossierModalOpen(false);

  return (
    <Modal
      open={dossierModalOpen}
      onClose={close}
      label="角色设定与世界观"
      id="dossier-modal"
      dialogClassName="dossier-modal-window"
      closeLabel="关闭角色设定"
      size="lg"
      title={
        <span>
          📜 角色设定与世界观 · {char.shortName || char.name}
        </span>
      }
      footer={
        <div className="btn-row">
          <button type="button" className="btn-solid" onClick={close}>
            完成阅读
          </button>
        </div>
      }
    >
      <div className="dossier-modal-meta-row">
        <div className="dossier-modal-char-info">
          <img src={char.avatar} alt={char.name} className="dossier-modal-avatar-mini" />
          <div className="dossier-modal-names">
            <span className="dossier-modal-char-name">{char.name}</span>
            <span className="dossier-modal-char-role">{char.role}</span>
          </div>
        </div>
        {char.dossier?.tags && char.dossier.tags.length > 0 && (
          <div className="dossier-modal-tags">
            {char.dossier.tags.slice(0, 5).map((t, idx) => (
              <span key={idx} className="dossier-modal-tag-chip">
                #{t}
              </span>
            ))}
          </div>
        )}
      </div>

      <div className="dossier-modal-tabs">
        {tabs.map((t) => {
          const hasText = !!t.text.trim();
          return (
            <button
              key={t.id}
              type="button"
              className={`dossier-modal-tab-btn ${activeTab === t.id ? "active" : ""}`}
              onClick={() => setActiveTab(t.id)}
              title={hasText ? t.label : `${t.label} (暂无内容)`}
            >
              {t.label}
              {hasText && <span className="dossier-modal-tab-dot" />}
            </button>
          );
        })}
      </div>

      <div className="dossier-modal-content-scroll">
        {currentTabItem.text.trim() ? (
          <DossierMarkdown
            content={currentTabItem.text}
            context={{
              charName: char.shortName || char.name,
              userName: playerName,
            }}
          />
        ) : (
          <div className="dossier-modal-empty">
            <span>🍃 该项尚未录入具体内容</span>
          </div>
        )}
      </div>
    </Modal>
  );
};
