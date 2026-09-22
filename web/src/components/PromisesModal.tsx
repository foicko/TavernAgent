import React from "react";
import { useUi } from "../stores/uiStore";
import { useStory } from "../stores/storyStore";
import { characterPresentation } from "../lib/characterPresentation";
import { Button } from "../ui/Button";
import { EmptyState } from "../ui/EmptyState";
import { Icon } from "../ui/Icon";
import { Modal } from "../ui/Modal";
import { Pill, type PillTone } from "../ui/Pill";
import "./PromisesModal.css";

const STATE_PILLS: Record<string, { label: string; tone: PillTone }> = {
  proposed: { label: "口头提议", tone: "warning" },
  active: { label: "誓约生效中", tone: "success" },
  fulfilled: { label: "已践行达成", tone: "neutral" },
  broken: { label: "已违背", tone: "danger" },
  cancelled: { label: "已解除", tone: "neutral" },
};

export const PromisesModal: React.FC = () => {
  const { promisesModalOpen, setPromisesModalOpen, activeCharKey } = useUi();
  const view = useStory((s) => s.view);
  const hud = useStory((s) => s.hud);

  if (!promisesModalOpen) return null;

  const char = characterPresentation(activeCharKey, view);
  const livePromises = hud?.promises ? Object.values(hud.promises) : [];
  const close = () => setPromisesModalOpen(false);

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
        <>
          <Icon name="handshake" size={16} /> 主动契约与承诺
        </>
      }
      footer={
        <div className="btn-row">
          <Button variant="primary" onClick={close}>
            关闭
          </Button>
        </div>
      }
    >
      <p className="promises-modal-lead">
        展示与主角或相关同伴之间建立的主动约定、口头许诺与誓言。当达成或违背誓约时，关系将发生显著跃迁。
      </p>

      {livePromises.length > 0 ? (
        <div className="promises-modal-list">
          {livePromises.map((p) => {
            const pill = STATE_PILLS[p.state] || { label: p.state, tone: "info" as PillTone };
            return (
              <div key={p.promiseId} className="promises-modal-card">
                <div className="promises-modal-card-head">
                  <span className="promises-modal-type-title">
                    <Icon name="scroll" size={13} />
                    <span>约见 / 誓约</span>
                  </span>
                  <Pill tone={pill.tone} className="ui-pill--sm">{pill.label}</Pill>
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
        <EmptyState
          icon={<Icon name="scroll" size={18} />}
          title="当前分支暂无有效契约与承诺"
          description="在与角色的互动中，主动给出约定将触发契约机制并持久沉淀在世界账本中。"
        />
      ) : (
        <div className="promises-modal-list">
          <div className="promises-modal-card">
            <div className="promises-modal-card-head">
              <span className="promises-modal-type-title">
                <Icon name="scroll" size={13} />
                <span>{char.pledge.title}</span>
              </span>
              <Pill tone="success" className="ui-pill--sm">{char.pledge.status}</Pill>
            </div>
            <div className="promises-modal-text">{char.pledge.text}</div>
          </div>
        </div>
      )}
    </Modal>
  );
};
