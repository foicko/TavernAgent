import React, { useState, useEffect } from "react";
import { useUi } from "../stores/uiStore";
import { useStory, storyBusy } from "../stores/storyStore";
import { characterPresentation } from "../lib/characterPresentation";
import { Modal } from "../ui/Modal";
import "./MemoryBankModal.css";

export const MemoryBankModal: React.FC = () => {
  const {
    memoryModalOpen,
    setMemoryModalOpen,
    activeCharKey,
    setMindGraphOpen,
    notifyQuiet,
  } = useUi();

  const view = useStory((s) => s.view);
  const backendMemories = useStory((s) => s.memories);
  const reviseMemory = useStory((s) => s.reviseMemory);
  const memoryUsage = useStory((s) => s.memoryUsage);
  const memoryError = useStory((s) => s.memoryError);
  const memoryLoading = useStory((s) => s.memoryLoading);
  const memoryPaging = useStory((s) => s.memoryPaging);
  const loadMemories = useStory((s) => s.loadMemories);
  const organizeMemories = useStory((s) => s.organizeMemories);
  const viewNodeId = useStory((s) => s.viewNodeId);
  const [memoryBusy, setMemoryBusy] = useState(false);
  const storyIsBusy = useStory(storyBusy);
  const memoryReadOnly = memoryBusy || memoryLoading || !!memoryError || storyIsBusy || !!viewNodeId;

  async function changeMemory(id: string, patch: { content?: string; pinned?: boolean; hidden?: boolean }, message: string) {
    if (memoryReadOnly) return false;
    setMemoryBusy(true);
    try { await reviseMemory(id, patch); notifyQuiet(message); return true; }
    catch (error) { notifyQuiet(error instanceof Error ? error.message : "记忆操作失败，请刷新后重试"); return false; }
    finally { setMemoryBusy(false); }
  }

  const [memoryFilter, setMemoryFilter] = useState<string>("all");
  const [memorySearch, setMemorySearch] = useState({ scope: "", text: "" });
  const [memoryPage, setMemoryPage] = useState({ scope: "", index: 0 });
  const [pinnedIds, setPinnedIds] = useState<Set<string>>(() => new Set());
  const [hiddenIds, setHiddenIds] = useState<Set<string>>(() => new Set());
  const [expandedIds, setExpandedIds] = useState<Set<string>>(() => new Set());
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editDraft, setEditDraft] = useState<string>("");
  const [customContents, setCustomContents] = useState<Record<string, string>>({});
  const [editedIds, setEditedIds] = useState<Set<string>>(() => new Set());
  const char = characterPresentation(activeCharKey, view);

  const toggleExpand = (id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  useEffect(() => {
    setEditingId(null);
    setEditDraft("");
  }, [view?.sessionId, view?.branch.branchId, viewNodeId, activeCharKey]);

  const hasBackendMemories = view !== null;
  const displayMemories = hasBackendMemories
    ? backendMemories.map((bm) => {
        const kindMap: Record<string, { label: string; category: string }> = {
          observed: { label: "亲历", category: "episodic" },
          inferred: { label: "推测", category: "inference" },
          reported: { label: "转述", category: "episodic" },
          secret: { label: "秘密", category: "secret" },
        };
        const km = kindMap[bm.kind] || { label: bm.kind, category: "episodic" };
        return {
          id: bm.memoryId,
          category: km.category,
          categoryLabel: km.label,
          turnTag: bm.sourceNodeId ? `节点 #${bm.sourceNodeId.slice(-6)}` : "",
          confidence: Math.round(Math.min(1, Math.max(0, bm.confidence ?? 0)) * 100),
          recalled: bm.effective,
          edited: !!bm.supersedes,
          content: customContents[bm.memoryId] || bm.content,
          entities: bm.entityIds || [],
          valence: bm.kind === "observed" ? "直感观察" : "反思推演",
          isPinned: bm.pinned,
          isHidden: bm.hidden,
          isBackend: true,
          evidence: bm.evidence,
          protected: bm.protected,
          merged: bm.mergedFrom?.length ?? 0,
        };
      })
    : char.memories.map((m) => ({
        id: m.id,
        category: m.category,
        categoryLabel: m.categoryLabel || m.category,
        turnTag: m.turnTag,
        confidence: m.confidence,
        recalled: m.recalled,
        edited: editedIds.has(m.id) || m.edited,
        content: customContents[m.id] || m.content,
        entities: m.entities,
        valence: m.valence,
        isPinned: pinnedIds.has(m.id),
        isHidden: hiddenIds.has(m.id),
        isBackend: false,
        evidence: undefined,
        protected: false,
        merged: 0,
      }));

  const visibleMemories = displayMemories.filter((m) => !m.isHidden);
  const searchScope = [view?.sessionId || activeCharKey || "preview", view?.branch.branchId, viewNodeId].join(":");
  const searchText = memorySearch.scope === searchScope ? memorySearch.text : "";

  useEffect(() => {
    if (!view || (searchText === memoryPaging.search && memoryFilter === memoryPaging.kind)) return;
    const timer = setTimeout(() => void loadMemories({ search: searchText, kind: memoryFilter }), 250);
    return () => clearTimeout(timer);
  }, [view, searchText, memoryFilter, memoryPaging.search, memoryPaging.kind, loadMemories]);

  const filteredMemories = displayMemories.filter((m) => {
    if (searchText.trim() && !(m.content + " " + m.entities.join(" ")).toLowerCase().includes(searchText.trim().toLowerCase())) return false;
    if (memoryFilter === "hidden") return m.isHidden;
    if (m.isHidden) return false;
    if (memoryFilter === "all") return true;
    return m.category === memoryFilter;
  });

  const pageScope = [searchScope, view?.branch.branchId, viewNodeId, memoryFilter, searchText].join(":");
  const memoryTotal = hasBackendMemories ? memoryPaging.total : filteredMemories.length;
  const pageCount = Math.max(1, Math.ceil(memoryTotal / 40));
  const pageIndex = hasBackendMemories ? memoryPaging.index : Math.min(memoryPage.scope === pageScope ? memoryPage.index : 0, pageCount - 1);
  const pagedMemories = hasBackendMemories ? filteredMemories : filteredMemories.slice(pageIndex * 40, (pageIndex + 1) * 40);

  const memoryCount = (kind: string) =>
    hasBackendMemories && memoryPaging.counts[kind] !== undefined
      ? memoryPaging.counts[kind]
      : kind === "hidden"
      ? displayMemories.length - visibleMemories.length
      : kind === "all"
      ? visibleMemories.length
      : visibleMemories.filter((m) => m.category === kind).length;

  const allExpanded = pagedMemories.length > 0 && pagedMemories.every((m) => expandedIds.has(m.id));
  const toggleAllCollapse = () => {
    if (allExpanded) {
      setExpandedIds(new Set());
    } else {
      setExpandedIds(new Set(pagedMemories.map((m) => m.id)));
    }
  };

  const close = () => setMemoryModalOpen(false);

  return (
    <Modal
      open={memoryModalOpen}
      onClose={close}
      label="认知记忆库"
      id="memory-bank-modal"
      dialogClassName="memory-modal-window"
      closeLabel="关闭认知记忆库"
      size="xl"
      title={
        <span id="hud-block-memory">
          🧠 认知记忆库
        </span>
      }
      headerActions={
        <div className="hud-block-title-actions">
          <span className="mem-badge-cow">分支独立</span>
          <button
            type="button"
            className="mem-graph-cue-btn mem-collapse-all-btn"
            onClick={toggleAllCollapse}
            title={allExpanded ? "全部收起至紧凑摘要" : "展开全部记忆条目与证据"}
          >
            {allExpanded ? "⤡ 全部折叠" : "⤢ 全部展开"}
          </button>
          {view && (
            <button
              className="mem-graph-cue-btn"
              disabled={memoryReadOnly}
              onClick={async () => {
                setMemoryBusy(true);
                try { await organizeMemories(); } finally { setMemoryBusy(false); }
              }}
            >
              整理
            </button>
          )}
          <button
            className="mem-graph-cue-btn"
            onClick={() => {
              close();
              setMindGraphOpen(true);
            }}
            title="全景心智星图预览"
          >
            🕸️ 星图
          </button>
        </div>
      }
      footer={
        <div className="btn-row">
          <button type="button" className="btn-solid" onClick={close}>
            关闭
          </button>
        </div>
      }
    >
      {memoryError && (
        <div className="story-map-error" role="alert">
          记忆读取失败：{memoryError}。{backendMemories.length > 0 ? "仍显示上次读取的记录。" : "请重试以查看记忆。"}
          <button className="btn-quiet" disabled={memoryLoading} onClick={() => void loadMemories()}>
            重试读取记忆
          </button>
        </div>
      )}

      <div className="memory-modal-subbar">
        {memoryLoading && <div className="memory-quota" role="status">正在读取记忆…</div>}
        {memoryUsage && (
          <div className="memory-modal-quota-box memory-quota" role="status">
            普通记忆 {memoryUsage.used}/{memoryUsage.limit} · 受保护 {memoryUsage.protected} ·{" "}
            {{ Normal: "充足", Notice: "留意容量", Degraded: "建议整理", Critical: "容量紧张" }[memoryUsage.tier]}
            <progress value={memoryUsage.used} max={memoryUsage.limit} aria-label="记忆容量" />
          </div>
        )}
        {viewNodeId && <div className="memory-quota">正在查看历史记忆；返回当前进度后可修订。</div>}

        <div className="memory-modal-search-wrap">
          <input
            className="lore-search-input memory-search memory-modal-search"
            aria-label="筛选记忆正文或实体"
            placeholder="筛选记忆正文或实体…"
            value={searchText}
            onChange={(event) => setMemorySearch({ scope: searchScope, text: event.target.value })}
          />
        </div>
      </div>

      <div className="memory-filter-row memory-modal-filters" id="memory-filter-row">
        <button
          className={`mem-filter-btn memory-modal-filter-btn ${memoryFilter === "all" ? "active" : ""}`}
          onClick={() => setMemoryFilter("all")}
        >
          生效 ({memoryCount("all")})
        </button>
        <button
          className={`mem-filter-btn memory-modal-filter-btn ${memoryFilter === "episodic" ? "active" : ""}`}
          onClick={() => setMemoryFilter("episodic")}
        >
          👁️ 亲历 ({memoryCount("episodic")})
        </button>
        <button
          className={`mem-filter-btn memory-modal-filter-btn ${memoryFilter === "inference" ? "active" : ""}`}
          onClick={() => setMemoryFilter("inference")}
        >
          🧠 推测 ({memoryCount("inference")})
        </button>
        <button
          className={`mem-filter-btn memory-modal-filter-btn ${memoryFilter === "secret" ? "active" : ""}`}
          onClick={() => setMemoryFilter("secret")}
        >
          🔒 秘密 ({memoryCount("secret")})
        </button>
        <button
          className={`mem-filter-btn memory-modal-filter-btn ${memoryFilter === "hidden" ? "active" : ""}`}
          onClick={() => setMemoryFilter("hidden")}
        >
          隐藏 ({memoryCount("hidden")})
        </button>
      </div>

      {pageCount > 1 && (
        <nav className="memory-pagination" aria-label="记忆分页">
          <button
            className="btn-quiet"
            disabled={memoryLoading || pageIndex === 0}
            onClick={() => (hasBackendMemories ? void loadMemories({ page: "previous" }) : setMemoryPage({ scope: pageScope, index: pageIndex - 1 }))}
          >
            上一页
          </button>
          <span>{pageIndex + 1} / {pageCount} 页 · {memoryTotal} 条</span>
          <button
            className="btn-quiet"
            disabled={memoryLoading || (hasBackendMemories ? !memoryPaging.nextCursor : pageIndex + 1 >= pageCount)}
            onClick={() => (hasBackendMemories ? void loadMemories({ page: "next" }) : setMemoryPage({ scope: pageScope, index: pageIndex + 1 }))}
          >
            下一页
          </button>
        </nav>
      )}

      <div className="memory-bank-stream memory-modal-stream" id="memory-bank-stream">
        {filteredMemories.length === 0 ? (
          <div className="memory-empty-card">
            <div className="memory-empty-icon">🧠</div>
            <div className="memory-empty-title">
              {memoryError ? "暂时无法读取记忆" : memoryLoading ? "正在读取记忆…" : memoryFilter === "hidden" ? "没有隐藏的记忆" : "此分类暂无生效记忆"}
            </div>
            <div className="memory-empty-desc">随剧情推进积累亲历与推测。可在设置里打开「后台记忆」，让它自动整理认知记录。</div>
          </div>
        ) : (
          pagedMemories.map((m) => {
            const isPinned = m.isPinned;
            const isHidden = m.isHidden;
            const isEditing = editingId === m.id;
            const isExpanded = isEditing || expandedIds.has(m.id);
            const isEdited = m.edited;
            const currentContent = m.content;

            const togglePin = () => {
              if (m.isBackend) {
                void changeMemory(m.id, { pinned: !m.isPinned }, !m.isPinned ? "已置顶记忆" : "已取消置顶");
              } else {
                setPinnedIds((prev) => {
                  const next = new Set(prev);
                  if (next.has(m.id)) {
                    next.delete(m.id);
                    notifyQuiet("已取消记忆置顶");
                  } else {
                    next.add(m.id);
                    notifyQuiet("已置顶该条认知记忆 (优先送入上下文) 📌");
                  }
                  return next;
                });
              }
            };

            const toggleHide = () => {
              if (m.isBackend) {
                void changeMemory(m.id, { hidden: !m.isHidden }, !m.isHidden ? "已隐藏当前分支的记忆" : "已恢复记忆");
              } else {
                setHiddenIds((prev) => {
                  const next = new Set(prev);
                  if (next.has(m.id)) {
                    next.delete(m.id);
                    notifyQuiet("已恢复该记忆可见性");
                  } else {
                    next.add(m.id);
                    notifyQuiet("已在当前世界线屏蔽该记忆 👁️");
                  }
                  return next;
                });
              }
            };

            const startEdit = () => {
              setEditingId(m.id);
              setEditDraft(currentContent);
              setExpandedIds((prev) => new Set(prev).add(m.id));
            };

            const cancelEdit = () => {
              setEditingId(null);
            };

            const saveEdit = async () => {
              if (editDraft.trim()) {
                if (m.isBackend) {
                  if (!await changeMemory(m.id, { content: editDraft.trim() }, "已保存当前分支的修订")) return;
                } else {
                  setCustomContents((prev) => ({ ...prev, [m.id]: editDraft.trim() }));
                  setEditedIds((prev) => new Set(prev).add(m.id));
                  notifyQuiet("已成功保存 CoW 纠偏记忆 ✍️");
                }
              }
              setEditingId(null);
            };

            return (
              <div
                key={m.id}
                className={`memory-item-card category-${m.category} ${m.recalled ? "recalled" : ""} ${isPinned ? "pinned" : ""} ${isHidden ? "hidden" : ""} ${isExpanded ? "is-expanded" : "is-collapsed"}`}
                id={`card-${m.id}`}
              >
                <div className="mem-item-top">
                  <div className="mem-badge-group">
                    <span className="mem-type-badge">{m.categoryLabel || m.category}</span>
                    <span className="mem-turn-tag">{m.turnTag || ""}</span>
                    {m.confidence ? <span className="mem-confidence-tag">{m.confidence}%</span> : null}
                    {m.recalled && !isHidden && (
                      <span className="mem-recalled-badge" title="在当前查看路径上生效；召回仍按相关性选择">
                        本路径生效
                      </span>
                    )}
                    {isHidden && <span className="mem-turn-tag" title="隐藏记录不会进入生成上下文">已隐藏</span>}
                    {isEdited && (
                      <span className="mem-cow-edited-tag" title="当前分支已发生 CoW 纠偏">
                        已修订
                      </span>
                    )}
                  </div>
                  <div className="mem-item-actions">
                    <button
                      className={`mem-act-btn ${isPinned ? "pinned-active" : ""}`}
                      onClick={togglePin}
                      disabled={m.isBackend && memoryReadOnly}
                      title="置顶强化召回"
                    >
                      {isPinned ? "📌 已置顶" : "📌 置顶"}
                    </button>
                    <button className="mem-act-btn" disabled={m.isBackend && memoryReadOnly} onClick={startEdit} title="纠正当前分支的记忆">
                      纠正
                    </button>
                    <button className="mem-act-btn" disabled={m.isBackend && memoryReadOnly} onClick={toggleHide} title="在此时间线屏蔽">
                      {isHidden ? "恢复" : "隐藏"}
                    </button>
                    <button
                      type="button"
                      className={`mem-act-btn mem-toggle-expand-btn ${isExpanded ? "active" : ""}`}
                      onClick={(e) => {
                        e.stopPropagation();
                        toggleExpand(m.id);
                      }}
                      title={isExpanded ? "收起此条记忆" : "展开完整记忆与证据"}
                      aria-label={isExpanded ? "收起此条记忆" : "展开完整记忆与证据"}
                      aria-expanded={isExpanded}
                    >
                      <span>{isExpanded ? "收起" : "展开"}</span>
                      <span className="mem-expand-arrow">{isExpanded ? "▲" : "▼"}</span>
                    </button>
                  </div>
                </div>

                {isEditing ? (
                  <div className="mem-inline-editor">
                    <textarea
                      className="mem-inline-textarea"
                      aria-label="修订记忆内容"
                      disabled={m.isBackend && memoryReadOnly}
                      value={editDraft}
                      onChange={(e) => setEditDraft(e.target.value)}
                      placeholder="输入修订后的记忆事实与潜台词..."
                    />
                    <div className="mem-inline-actions mem-inline-actions--spaced">
                      <button className="probe-btn mem-edit-btn" onClick={cancelEdit}>
                        取消
                      </button>
                      <button
                        className="probe-btn primary mem-edit-btn"
                        disabled={(m.isBackend && memoryReadOnly) || !editDraft.trim()}
                        onClick={saveEdit}
                      >
                        保存修订
                      </button>
                    </div>
                  </div>
                ) : (
                  <div
                    className={`mem-item-content ${!isExpanded ? "mem-item-content--clamped" : ""}`}
                    onClick={!isExpanded ? () => toggleExpand(m.id) : undefined}
                    title={!isExpanded ? "点击展开详情" : undefined}
                  >
                    {currentContent}
                  </div>
                )}

                {(m.evidence || m.protected || m.merged > 0) && (
                  <details className="memory-evidence">
                    <summary>
                      {m.protected ? "受保护 · " : ""}证据与来源{m.merged > 0 ? ` · 合并 ${m.merged} 条` : ""}
                    </summary>
                    {m.evidence?.sourceQuote && <blockquote>{m.evidence.sourceQuote}</blockquote>}
                    {m.evidence?.reasoning && <p>推理依据：{m.evidence.reasoning}</p>}
                    {m.evidence?.autoDowngraded && <p>引文不足，置信度已自动调低。</p>}
                    {m.protected && <p>自动整理会保留与信物、有效誓言、秘密相关或已置顶的记忆。</p>}
                  </details>
                )}

                <div className="mem-item-footer">
                  <div className="mem-entity-tags">
                    {(m.entities || []).map((ent) => (
                      <span key={ent} className="mem-entity-pill">
                        🏷️ {ent}
                      </span>
                    ))}
                  </div>
                  {m.valence && <span className="mem-valence-pill">{m.valence}</span>}
                </div>
              </div>
            );
          })
        )}
      </div>
    </Modal>
  );
};
