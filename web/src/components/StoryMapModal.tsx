import { useEffect, useMemo, useRef, useState } from "react";
import { api } from "../app/api";
import type { GraphView, PlotNode, SessionView, TextBlock } from "../app/types";
import { storyBusy, useStory } from "../stores/storyStore";
import { Modal } from "../ui/Modal";

export function StoryOutlineSection({ view }: { view: SessionView }) {
  const outline = view.outline;
  return <section className="story-outline" aria-label="剧情进展">
    <h3>剧情进展</h3>
    {outline?.scene && <p>当前场景 · {outline.scene}</p>}
    {!!outline?.milestones?.length && <><h4>已达成里程碑</h4><ul>{outline.milestones.map(entry =>
      <li key={entry.milestoneId}>{entry.description}</li>)}</ul></>}
    {!!outline?.goals?.length && <><h4>角色目标</h4><ul>{outline.goals.map(entry =>
      <li key={entry.goalId}><b>{view.state?.characters[entry.characterId]?.name || entry.characterId}</b> · {entry.text}</li>)}</ul></>}
    {!outline?.milestones?.length && !outline?.goals?.length && <p className="story-map-muted">尚无已记录的里程碑或目标，推进故事后会随当前分支更新。</p>}
  </section>;
}

function nodeText(node: PlotNode): string {
  try {
    const content = JSON.parse(node.contentJson);
    return content.openingText || [content.inputText, ...(content.blocks || []).map((block: TextBlock) => block.text)].filter(Boolean).join("\n\n") || "记忆与状态维护节点";
  } catch { return "此节点没有可展示的正文。"; }
}

function nodeLabel(kind: string, turn?: number) {
  return kind === "root" ? "故事起点" : kind === "memory_change" ? "记忆整理 / 修订" : kind === "director_event" ? "导演安排 / 进度调整" : "回合 " + (turn ?? "—");
}

export function StoryMapModal({ view, onClose }: { view: SessionView; onClose: () => void }) {
  const [target, setTarget] = useState(view.viewNodeId || view.branch.headNodeId);
  const [selected, setSelected] = useState(target);
  const [graph, setGraph] = useState<GraphView | null>(null);
  const [detail, setDetail] = useState<PlotNode | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);
  const [loading, setLoading] = useState(false);
  const [acting, setActing] = useState(false);
  const storyIsBusy = useStory(storyBusy);
  const busy = acting || storyIsBusy;
  const viewport = useRef<HTMLDivElement>(null);
  const sessionId = view.sessionId;

  useEffect(() => {
    let active = true;
    setLoading(true); setError(null); setGraph(null);
    api.getGraph(sessionId, { nodeId: target, up: 6, down: 2 }).then(result => {
      if (active) setGraph(result);
    }).catch(err => { if (active) setError(String(err instanceof Error ? err.message : err)); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [sessionId, target, revision]);

  useEffect(() => {
    let active = true;
    setDetail(null); setDetailError(null);
    api.getNode(selected).then(result => {
      if (active && result.node.sessionId === sessionId) setDetail(result.node);
    }).catch(err => { if (active) setDetailError(String(err instanceof Error ? err.message : err)); });
    return () => { active = false; };
  }, [sessionId, selected, revision]);

  const layout = useMemo(() => {
    const nodes = [...(graph?.nodes || [])].sort((a, b) => a.depth - b.depth || (a.parentId || "").localeCompare(b.parentId || "") || a.nodeId.localeCompare(b.nodeId));
    const firstDepth = nodes[0]?.depth || 0;
    const rows = new Map<number, number>();
    return nodes.map(node => {
      const row = rows.get(node.depth) || 0;
      rows.set(node.depth, row + 1);
      return { ...node, x: 24 + (node.depth - firstDepth) * 196, y: 24 + row * 88 };
    });
  }, [graph]);
  const positions = new Map(layout.map(node => [node.nodeId, node]));
  const branchNames = new Map((view.branches || []).map(branch => [branch.headNodeId, branch.name]));
  const width = Math.max(600, ...layout.map(node => node.x + 180));
  const height = Math.max(320, ...layout.map(node => node.y + 84));
  useEffect(() => {
    const center = layout.find(node => node.nodeId === target);
    if (center && viewport.current) {
      viewport.current.scrollLeft = Math.max(0, center.x - viewport.current.clientWidth / 2 + 80);
      viewport.current.scrollTop = Math.max(0, center.y - viewport.current.clientHeight / 2 + 30);
    }
  }, [layout, target]);

  const selectedBranch = view.branches?.find(branch => branch.headNodeId === selected);
  const perform = async (action: () => Promise<void>) => {
    setActing(true); setDetailError(null);
    try {
      await action();
      const current = useStory.getState();
      if (current.view?.sessionId !== sessionId) return;
      if (current.error) setDetailError(current.error);
      else onClose();
    } catch (err) { setDetailError(String(err instanceof Error ? err.message : err)); }
    finally { setActing(false); }
  };

  return (
    <Modal
      label="剧情分支图"
      title="剧情分支图"
      closeLabel="关闭剧情分支图"
      size="xl"
      flush
      dialogClassName="story-map-modal"
      onClose={onClose}
    >
      <div className="story-map-toolbar">
        <label>定位分支 <select aria-label="定位分支" value={view.branches?.find(branch => branch.headNodeId === target)?.branchId || ""} onChange={event => {
          const branch = view.branches?.find(item => item.branchId === event.target.value);
          if (branch) { setTarget(branch.headNodeId); setSelected(branch.headNodeId); }
        }}><option value="" disabled>节点附近</option>{(view.branches || [view.branch]).map(branch => <option key={branch.branchId} value={branch.branchId}>{branch.name}</option>)}</select></label>
        <button className="btn-quiet" onClick={() => { setTarget(view.branch.headNodeId); setSelected(view.branch.headNodeId); setRevision(r => r + 1); }}>当前分支头</button>
        <span className="story-map-muted">{loading ? "加载中…" : "已加载 " + layout.length + " 个节点"}</span>
      </div>
      {error && <div className="story-map-error" role="alert">{error} <button onClick={() => setRevision(r => r + 1)}>重试</button></div>}
      {graph?.truncated && <p className="story-map-notice">当前仅展示附近的部分节点（最多 300 个）。选择节点后展开附近，或从菜单定位其他分支。</p>}
      <div className="story-map-body">
        <div className="story-map-viewport" ref={viewport} aria-label="分支节点" aria-busy={loading}>
          <div className="story-map-canvas" style={{ width, height }}>
            <svg width={width} height={height} aria-hidden="true">{layout.map(node => {
              const parent = positions.get(node.parentId || "");
              return parent ? <path key={node.nodeId} d={"M" + (parent.x + 160) + "," + (parent.y + 30) + " C" + (parent.x + 178) + "," + (parent.y + 30) + " " + (node.x - 18) + "," + (node.y + 30) + " " + node.x + "," + (node.y + 30)} /> : null;
            })}</svg>
            {layout.map(node => <button key={node.nodeId} className={"story-map-node" + (selected === node.nodeId ? " selected" : "")} style={{ left: node.x, top: node.y }} onClick={() => setSelected(node.nodeId)} aria-pressed={selected === node.nodeId}>
              <strong>{nodeLabel(node.kind, node.turnNumber)}{branchNames.has(node.nodeId) ? " · " + branchNames.get(node.nodeId) : ""}</strong>
              <span>{node.nodeId === view.branch.headNodeId ? "当前分支头 · " : ""}{node.childCount || 0} 个子节点{node.isCandidate ? " · 候选" : ""}</span>
            </button>)}
          </div>
        </div>
        <aside className="story-map-detail" aria-label="节点详情">
          <h3>{detail ? nodeLabel(detail.kind, detail.turnNumber) : "加载节点…"}</h3>
          {selectedBranch && <p className="story-map-muted">分支 · {selectedBranch.name}</p>}
          {detailError && <p className="story-map-error" role="alert">{detailError} <button onClick={() => setRevision(r => r + 1)}>重试</button></p>}
          <div className="story-map-excerpt">{detail && nodeText(detail)}</div>
          <div className="story-map-actions">
            <button className="btn-quiet" onClick={() => { setTarget(selected); setRevision(r => r + 1); }}>展开此节点附近</button>
            <button className="btn-quiet" disabled={busy || !detail} onClick={() => void perform(() => useStory.getState().viewAt(selected))}>查看历史快照</button>
            <button className="btn-quiet" disabled={busy || !detail} onClick={() => void perform(() => useStory.getState().forkFrom(selected))}>从此处开辟分支</button>
            {selectedBranch && <button className="btn-quiet" disabled={busy} onClick={() => void perform(() => useStory.getState().switchBranch(selectedBranch.branchId))}>切换到此分支</button>}
            {detail?.kind === "turn" && <button className="btn-quiet" disabled={busy} onClick={() => void perform(() => useStory.getState().regenerate(selected))}>重新生成此回合</button>}
          </div>
        </aside>
      </div>
    </Modal>
  );
}
