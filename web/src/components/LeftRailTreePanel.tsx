import React from "react";
import { Icon } from "../ui/Icon";
import { StoryOutlineSection } from "./StoryMapModal";
import type { SessionView } from "../app/types";

export interface DisplayTreeNode {
  id: string;
  tag: string;
  desc: string;
  isActive: boolean;
  isViewing: boolean;
  isFork: boolean;
  aff: number;
  trust: number;
  alert: number;
  canRegenerate: boolean;
}

interface LeftRailTreePanelProps {
  currentView: SessionView | null;
  hasBackendNodes: boolean;
  displayTreeNodes: DisplayTreeNode[];
  probeNodeId: string | null;
  setProbeNodeId: (id: string | null) => void;
  branchConfirmId: string | null;
  setBranchConfirmId: (id: string | null) => void;
  forkFrom: (nodeId: string, label: string) => Promise<unknown>;
  viewAt: (nodeId: string) => Promise<unknown>;
  regenerate: (nodeId: string) => Promise<unknown>;
  notifyQuiet: (message: string) => void;
  liveRel?: { affection: number; trust: number; alertness: number };
  activateOnKey: (run: () => void) => (event: React.KeyboardEvent) => void;
  onOpenStoryMap: () => void;
  style?: React.CSSProperties;
}

export const LeftRailTreePanel: React.FC<LeftRailTreePanelProps> = ({
  currentView,
  hasBackendNodes,
  displayTreeNodes,
  probeNodeId,
  setProbeNodeId,
  branchConfirmId,
  setBranchConfirmId,
  forkFrom,
  viewAt,
  regenerate,
  notifyQuiet,
  liveRel,
  activateOnKey,
  onOpenStoryMap,
  style,
}) => {
  return (
    <div className="branch-timeline-tree rail-view-panel" id="view-tree" style={style}>
      {currentView && (
        <>
          <button className="story-map-open btn-quiet" onClick={onOpenStoryMap}>
            <Icon name="grid" size={13} /> 打开剧情分支图
          </button>
          <StoryOutlineSection view={currentView} />
        </>
      )}
      <div className="tree-lead-label">
        <span>分支拓扑 (Branch Graph)</span>
        <span className="tree-lead-hint">可点击节点回溯</span>
      </div>

      <div className="tree-node-stack">
        {displayTreeNodes.length > 0 && <div className="tree-vertical-line" />}

        {displayTreeNodes.length === 0 ? (
          <div className="tree-empty-card">
            <div className="tree-empty-icon"><Icon name="leaf" size={22} /></div>
            <div className="tree-empty-title">因果树未播种</div>
            <div className="tree-empty-desc">
              推进第一轮对话后，大模型将在此自动生长因果拓扑与历史快照。
            </div>
          </div>
        ) : (
          displayTreeNodes.map((node) => {
            const isOpen = probeNodeId === node.id;
            const isConfirming = branchConfirmId === node.id;
            return (
              <div
                key={node.id}
                className={`node-item ${node.isActive ? "active" : ""} ${node.isViewing ? "viewing" : ""} ${isOpen ? "opened" : ""} ${node.isFork ? "fork" : ""}`}
                role="button"
                tabIndex={0}
                aria-expanded={isOpen}
                onClick={() => {
                  setProbeNodeId(isOpen ? null : node.id);
                  setBranchConfirmId(null);
                }}
                onKeyDown={activateOnKey(() => {
                  setProbeNodeId(isOpen ? null : node.id);
                  setBranchConfirmId(null);
                })}
              >
                <div className="node-turn-tag">
                  <div className="node-tag-group">
                    <span className="node-tag-text">{node.tag}</span>
                    {node.isViewing && <span className="node-badge-viewing">当前视角</span>}
                  </div>
                  {node.isActive ? (
                    <span className="node-state-pill active">活跃中</span>
                  ) : node.isFork ? (
                    <span className="node-state-pill fork">已封存分支</span>
                  ) : (
                    <span className="node-state-pill">快照</span>
                  )}
                </div>

                <div className="node-turn-desc" title={node.desc}>
                  {node.desc}
                </div>

                {node.isViewing && liveRel && (
                  <div className="node-mini-metrics">
                    <span className="metric-chip aff" title="好感度">
                      <span>好感</span>
                      <b>{node.aff > 0 ? `+${node.aff}` : node.aff}</b>
                    </span>
                    <span className="metric-chip trust" title="信任度">
                      <span>信任</span>
                      <b>{node.trust}%</b>
                    </span>
                    <span className="metric-chip alert" title="戒备度">
                      <span>戒备</span>
                      <b>{node.alert}%</b>
                    </span>
                  </div>
                )}

                {isOpen && (
                  <div className="branch-probe-card" onClick={(e) => e.stopPropagation()}>
                    <div className="probe-head">
                      <span>快照控制</span>
                      <button
                        className="probe-close-btn"
                        onClick={() => {
                          setProbeNodeId(null);
                          setBranchConfirmId(null);
                        }}
                        title="收起选项"
                      >
                        <Icon name="close" size={12} />
                      </button>
                    </div>

                    {isConfirming ? (
                      <div className="probe-fork-confirm-box">
                        <div className="probe-fork-confirm-text">
                          <Icon name="leaf" size={12} /> 确定从<strong>【{node.tag}】</strong>派生新平行因果分支？
                        </div>
                        <div className="probe-fork-confirm-actions">
                          <button
                            className="probe-btn-confirm"
                            onClick={() => {
                              if (hasBackendNodes) {
                                void forkFrom(node.id, `平行分支_${node.tag}`);
                              } else {
                                notifyQuiet("请先开启故事，再开辟分支。");
                              }
                              setBranchConfirmId(null);
                              setProbeNodeId(null);
                            }}
                          >
                            <Icon name="check" size={12} /> 确认开辟
                          </button>
                          <button
                            className="probe-btn-cancel"
                            onClick={() => setBranchConfirmId(null)}
                          >
                            取消
                          </button>
                        </div>
                      </div>
                    ) : (
                      <div className="probe-actions-stack">
                        {node.isViewing ? (
                          <div className="probe-viewing-badge">
                            <span><Icon name="eye" size={12} /> 当前主窗口正在显示此节点</span>
                          </div>
                        ) : (
                          <button
                            className="probe-btn-primary"
                            onClick={() => {
                              if (hasBackendNodes) {
                                void viewAt(node.id);
                              } else {
                                notifyQuiet("请先开启故事，再查看历史快照。");
                              }
                              setProbeNodeId(null);
                            }}
                            title="切换主窗口视角至该历史快照"
                          >
                            <Icon name="eye" size={13} />
                            <span>回溯定位至此快照</span>
                          </button>
                        )}

                        <div
                          className="probe-secondary-grid probe-secondary-grid--dynamic"
                          style={{ "--lr-probe-cols": node.canRegenerate && hasBackendNodes ? "1fr 1fr" : "1fr" } as React.CSSProperties}
                        >
                          <button
                            className="probe-btn-sub"
                            onClick={() => setBranchConfirmId(node.id)}
                            title="从该快照分叉新时间线"
                          >
                            <Icon name="leaf" size={13} />
                            <span>开辟分支</span>
                          </button>
                          {node.canRegenerate && hasBackendNodes && (
                            <button
                              className="probe-btn-sub"
                              onClick={() => {
                                void regenerate(node.id);
                                notifyQuiet(`正在重新生成【${node.tag}】的变体回复...`);
                                setProbeNodeId(null);
                              }}
                              title="在兄弟节点上重跑大模型变体回复"
                            >
                              <Icon name="refresh" size={13} />
                              <span>重生变体</span>
                            </button>
                          )}
                        </div>
                      </div>
                    )}
                  </div>
                )}
              </div>
            );
          })
        )}
      </div>
    </div>
  );
};
