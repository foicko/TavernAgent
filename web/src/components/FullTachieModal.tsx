// FullTachieModal: 全身立绘图鉴、装束细节与人物设定大图灯箱
import React from "react";
import { useUi } from "../stores/uiStore";
import { characterPresentation } from "../lib/characterPresentation";
import { useStory } from "../stores/storyStore";
import { DossierMarkdown } from "../lib/dossierMarkdown";
import { DEFAULT_PLAYER_NAME } from "../lib/characterMacros";
import { Modal } from "../ui/Modal";
import "./FullTachieModal.css";

export const FullTachieModal: React.FC = () => {
  const { fullTachieOpen, setFullTachieOpen, activeCharKey } = useUi();
  const view = useStory((s) => s.view);
  const char = characterPresentation(activeCharKey, view);

  const playerName = view?.state?.characters?.["player"]?.name || DEFAULT_PLAYER_NAME;

  return (
    <Modal
      open={fullTachieOpen}
      id="full-tachie-modal"
      label="角色立绘与设定"
      closeLabel="关闭角色立绘"
      size="xl"
      flush
      dialogClassName="tachie-lightbox"
      onClose={() => setFullTachieOpen(false)}
    >
      {/* 左侧大图纵向立绘展台 */}
      <div className="tachie-lightbox-stage">
        <img className="tachie-lightbox-image" src={char.fullImg} alt={char.name} />
      </div>

      {/* 右侧角色设定详情 */}
      <div className="tachie-lightbox-dossier">
        <div>
          <span className="char-stage-pill tachie-lightbox-pill">{char.stagePill}</span>
          <h1 className="tachie-lightbox-name">{char.name}</h1>
          <div className="tachie-lightbox-role">{char.role}</div>
        </div>

        <div className="tachie-lightbox-badges">
          <span className="char-status-badge">{char.stateBadge}</span>
          <span className="char-status-badge">{char.allianceBadge}</span>
          <span className="char-status-badge">{char.voiceBadge}</span>
        </div>

        {char.dossier?.tags && char.dossier.tags.length > 0 && (
          <div className="tachie-lightbox-tags">
            {char.dossier.tags.map((tag, idx) => (
              <span key={idx} className="dossier-tag-pill">
                #{tag}
              </span>
            ))}
          </div>
        )}

        <div className="tachie-lightbox-body">
          {char.isCustom && (char.dossier?.description || char.dossier?.personality) ? (
            <DossierMarkdown
              content={[char.dossier.description, char.dossier.personality].filter(Boolean).join("\n\n")}
              context={{
                charName: char.shortName || char.name,
                userName: playerName,
              }}
            />
          ) : (
            <div dangerouslySetInnerHTML={{ __html: char.modalDesc }} />
          )}
        </div>

        <div className="tachie-lightbox-scenario">
          <div className="tachie-lightbox-scenario-title">当前场景世界观：</div>
          <div className="tachie-lightbox-scenario-body">
            {char.dossier?.scenario ? (
              <DossierMarkdown
                content={char.dossier.scenario}
                context={{
                  charName: char.shortName || char.name,
                  userName: playerName,
                }}
              />
            ) : (
              <span dangerouslySetInnerHTML={{ __html: char.breadcrumb }} />
            )}
          </div>
        </div>
      </div>
    </Modal>
  );
};
