import React from "react";
import { useUi } from "../stores/uiStore";
import { useStory } from "../stores/storyStore";
import { characterPresentation } from "../lib/characterPresentation";
import { Modal } from "../ui/Modal";
import "./PromisesModal.css";

export const PromisesModal: React.FC = () => {
  const { promisesModalOpen, setPromisesModalOpen, activeCharKey } = useUi();
  const view = useStory((s) => s.view);
  const hud = useStory((s) => s.hud);

  if (!promisesModalOpen) return null;

  const char = characterPresentation(activeCharKey, view);
  const livePromises = hud?.promises ? Object.values(hud.promises) : [];
  const close = () => setPromisesModalOpen(false);

  const statePillMap: Record<string, { label: string; color: string }> = {
    proposed: { label: "口头提议", color: "var(--accent-amber, #d97706)" },
    active: { label: "誓约生效中", color: "var(--accent-green, #10b981)" },
    fulfilled: { label: "已践行达成", color: "var(--text-dim)" },
    broken: { label: "已违背", color: "var(--color-warn-border, #ef4444)" },
    cancelled: { label: "已解除", color: "var(--text-dim)" },
  };

  return (
    <Modal
      open={promisesModalOpen}
      onClose={close}
      label="主动契约与承诺"
      id="promises-modal"
      dialogClassName="promises-modal-window"
      closeLabel="关闭契约弹窗"
      size="md"
      title={
        <span>
          🤝 主动契约与承诺
        </span>
      }
      footer={
        <div className="btn-row">
          <button type="button" className="btn-solid" onClick={close}>
            关闭
          </button>
        </div>
      }
    >
      <p className="promises-modal-lead">
        展示与主角或相关同伴之间建立的主动约定、口头许诺与誓言。当达成或违背誓约时，关系将发生显著跃迁。
      </p>

      {livePromises.length > 0 ? (
        <div className="promises-modal-list">
          {livePromises.map((p) => {
            const pill = statePillMap[p.state] || { label: p.state, color: "var(--accent-glow)" };
            return (
              <div key={p.promiseId} className="promises-modal-card">
                <div className="promises-modal-card-head">
                  <span className="promises-modal-type-title">
                    <span>📜</span>
                    <span>约见 / 誓约</span>
                  </span>
                  <span
                    className="promises-modal-status-badge"
                    style={{ color: pill.color, borderColor: pill.color }}
                  >
                    {pill.label}
                  </span>
                </div>
                <div className="promises-modal-text">{p.content}</div>
                {p.sourceNodeId && (
                  <div className="promises-modal-source">
                    来源节点 #{p.sourceNodeId.slice(0, 8)}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      ) : view ? (
        <div className="promises-modal-empty">
          <div className="promises-modal-empty-icon">📜</div>
          <div className="promises-modal-empty-title">当前分支暂无有效契约与承诺</div>
          <div>在与角色的互动中，主动给出约定将触发契约机制并持久沉淀在世界账本中。</div>
        </div>
      ) : (
        <div className="promises-modal-list">
          <div className="promises-modal-card">
            <div className="promises-modal-card-head">
              <span className="promises-modal-type-title">
                <span>📜</span>
                <span>{char.pledge.title}</span>
              </span>
              <span className="promises-modal-status-badge" style={{ color: "var(--accent-green, #10b981)" }}>
                {char.pledge.status}
              </span>
            </div>
            <div className="promises-modal-text">{char.pledge.text}</div>
          </div>
        </div>
      )}
    </Modal>
  );
};
