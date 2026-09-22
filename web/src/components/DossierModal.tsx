import React, { useState } from "react";
import { useUi } from "../stores/uiStore";
import { useStory } from "../stores/storyStore";
import { characterPresentation } from "../lib/characterPresentation";
import { DossierMarkdown } from "../lib/dossierMarkdown";
import { DEFAULT_PLAYER_NAME } from "../lib/characterMacros";
import { Avatar } from "../ui/Avatar";
import { Button } from "../ui/Button";
import { EmptyState } from "../ui/EmptyState";
import { Icon } from "../ui/Icon";
import { Modal } from "../ui/Modal";
import { Tag } from "../ui/Pill";
import { Tabs } from "../ui/Tabs";
import "./DossierModal.css";

type DossierTab = "persona" | "scenario" | "examples" | "system" | "notes";

export const DossierModal: React.FC = () => {
  const { dossierModalOpen, setDossierModalOpen, activeCharKey } = useUi();
  const view = useStory((s) => s.view);
  const [activeTab, setActiveTab] = useState<DossierTab>("persona");

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

  const tabs: Array<{ id: DossierTab; label: string; text: string }> = [
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
        <>
          <Icon name="scroll" size={16} /> 角色设定与世界观 · {char.shortName || char.name}
        </>
      }
      footer={
        <div className="btn-row">
          <Button variant="primary" onClick={close}>
            完成阅读
          </Button>
        </div>
      }
    >
      <div className="dossier-modal-meta-row">
        <div className="dossier-modal-char-info">
          <Avatar src={char.avatar} name={char.name} size={34} />
          <div className="dossier-modal-names">
            <span className="dossier-modal-char-name">{char.name}</span>
            <span className="dossier-modal-char-role">{char.role}</span>
          </div>
        </div>
        {char.dossier?.tags && char.dossier.tags.length > 0 && (
          <div className="dossier-modal-tags">
            {char.dossier.tags.slice(0, 5).map((t, idx) => (
              <Tag key={idx}>#{t}</Tag>
            ))}
          </div>
        )}
      </div>

      <Tabs
        label="档案分类"
        variant="underline"
        value={activeTab}
        onChange={setActiveTab}
        items={tabs.map((t) => {
          const hasText = !!t.text.trim();
          return {
            value: t.id,
            label: (
              <>
                {t.label}
                {hasText && <span className="dossier-modal-tab-dot" aria-hidden="true" />}
              </>
            ),
            title: hasText ? t.label : `${t.label} (暂无内容)`,
          };
        })}
      />

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
          <EmptyState icon={<Icon name="leaf" size={18} />} title="该项尚未录入具体内容" />
        )}
      </div>
    </Modal>
  );
};
