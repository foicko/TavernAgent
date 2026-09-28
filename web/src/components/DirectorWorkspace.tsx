import { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useDialogFocus } from "../lib/useDialogFocus";
import type { DirectorAction, DirectorBeat, DirectorDraft, DirectorPlan } from "../app/types";
import { directorBusy, storyBusy, useStory } from "../stores/storyStore";
import { directorScopeKey } from "../stores/slices/directorSlice";
import { owns, scope } from "../stores/storyHelpers";
import { Button, IconButton } from "../ui/Button";
import { Icon } from "../ui/Icon";
import { EmptyState } from "../ui/EmptyState";

const blankPlan = (): DirectorPlan => ({ title: "", guidance: "", beats: [] });
const statusText: Record<string, string> = { active: "执行中", paused: "已暂停", completed: "已完成" };

export function DirectorStatusBar({ onOpen }: { onOpen: () => void }) {
  const summary = useStory(s => s.view?.director);
  const error = useStory(s => s.directorError);
  const disabled = useStory(s => !!s.viewNodeId || storyBusy(s));
  const command = useStory(s => s.commandDirector);
  if (!summary) return null;
  return <div className="director-status" aria-label="导演进度">
    <div className="director-status-main">
      <Button size="sm" variant="outline" onClick={onOpen}><Icon name="film" size={13} /> {summary.title} · {statusText[summary.status] ?? summary.status}</Button>
      <span>{summary.status === "completed" ? "阶段已全部结束，可自由续写" : `当前：${summary.currentBeatTitle}`} · {summary.completed}/{summary.total}</span>
      {summary.status !== "completed" && <Button size="sm" variant="ghost" disabled={disabled}
        onClick={() => void command(summary.status === "paused" ? "resume" : "pause").catch(() => undefined)}>{summary.status === "paused" ? "恢复大纲" : "暂停大纲"}</Button>}
    </div>
    {(summary.warning || error) && <p role={error ? "alert" : "status"}>{error || summary.warning}</p>}
  </div>;
}

export function DirectorWorkspace({ onClose }: { onClose: () => void }) {
  const view = useStory(s => s.view)!;
  const viewNodeId = useStory(s => s.viewNodeId);
  const workspaceScope = useStory(s => s.directorScope);
  const workspaceData = useStory(s => s.directorView);
  const loading = useStory(s => s.directorLoading);
  const error = useStory(s => s.directorError);
  const saving = useStory(s => s.directorSaving);
  const commanding = useStory(s => s.directorCommanding);
  const load = useStory(s => s.loadDirector);
  const saveDraft = useStory(s => s.saveDirectorDraft);
  const command = useStory(s => s.commandDirector);
  const discuss = useStory(s => s.discussDirector);
  const cancel = useStory(s => s.cancelDirector);
  const scopeKey = directorScopeKey(view.sessionId, view.branch.branchId, viewNodeId);
  const workspace = workspaceScope === scopeKey ? workspaceData : null;
  const readonly = !!viewNodeId || !!workspace?.readOnly;
  const active = workspace?.state;
  const running = workspace?.requests.find(r => r.status === "generating");
  const runningRequestId = running?.requestId;
  const busy = useStory(directorBusy) || commanding || readonly;
  const storageKey = `tavernagent_director_draft_${view.sessionId}_${view.branch.branchId}`;
  const [plan, setPlan] = useState<DirectorPlan>(blankPlan);
  const [dirty, setDirty] = useState(false);
  const [editTick, setEditTick] = useState(0);
  const [replace, setReplace] = useState(false);
  const [text, setText] = useState("");
  const [sending, setSending] = useState(false);
  const [saveIssue, setSaveIssue] = useState("");
  const [notice, setNotice] = useState("");
  const [ready, setReady] = useState(false);
  // 注册进统一模态栈（B10-R1）：Esc / Tab 焦点边界 / 关闭后焦点归还交给 lib/useDialogFocus。
  const closeRef = useRef<() => void>(() => {});
  const dialog = useDialogFocus(true, () => closeRef.current());
  const editor = useRef(plan);
  const dirtyRef = useRef(false);
  const version = useRef(0);
  const baseRevision = useRef("");
  const changes = useRef(0);
  const initialized = useRef(false);
  const inFlight = useRef<Promise<DirectorDraft> | null>(null);
  const mounted = useRef(true);
  const owner = useRef(scope());
  const stillHere = useCallback(() => mounted.current && owns(owner.current), []);

  const backup = useCallback(() => {
    try { localStorage.setItem(storageKey, JSON.stringify({ plan: editor.current, version: version.current, baseRevisionId: baseRevision.current })); } catch { /* Server autosave remains available. */ }
  }, [storageKey]);
  const clearBackup = useCallback(() => { try { localStorage.removeItem(storageKey); } catch { /* Storage can be disabled. */ } }, [storageKey]);

  useEffect(() => { void load(); }, [load, view.sessionId, view.branch.branchId, viewNodeId, view.branch.headNodeId]);
  useEffect(() => {
    mounted.current = true;
    dialog.ref.current?.querySelector<HTMLButtonElement>("button")?.focus();
    return () => { mounted.current = false; };
  }, [dialog.ref]);

  useEffect(() => {
    if (!workspace || dirtyRef.current) return;
    let initial = workspace.draft?.plan ?? workspace.state?.plan ?? blankPlan();
    let initialVersion = workspace.draft?.version ?? 0;
    let initialBase = workspace.draft?.baseRevisionId ?? workspace.state?.plan.revisionId ?? "";
    if (!initialized.current && !readonly) {
      try {
        const saved = JSON.parse(localStorage.getItem(storageKey) || "null") as { plan: DirectorPlan; version: number; baseRevisionId: string } | null;
        if (saved?.plan && Array.isArray(saved.plan.beats) && typeof saved.plan.title === "string" && typeof saved.version === "number") {
          if (JSON.stringify(saved.plan) !== JSON.stringify(initial)) {
            initial = saved.plan; initialVersion = saved.version; initialBase = saved.baseRevisionId;
            dirtyRef.current = true; setDirty(true); setNotice("已恢复尚未同步的本地编辑。");
            if (initialVersion !== (workspace.draft?.version ?? 0)) setSaveIssue("已保存草稿也发生了变化，请选择保留本地编辑或载入已保存草稿。");
          } else { localStorage.removeItem(storageKey); }
        }
      } catch { /* Ignore malformed or unavailable local backups. */ }
    }
    initialized.current = true;
    editor.current = initial; version.current = initialVersion; baseRevision.current = initialBase;
    setPlan(initial); setReady(true);
  }, [workspace, readonly, storageKey]);

  const saveNow = useCallback(async function saveNow(): Promise<void> {
    if (readonly || !dirtyRef.current) return;
    if (inFlight.current) { await inFlight.current; return saveNow(); }
    const current = useStory.getState();
    if (current.view?.sessionId !== view.sessionId || current.view.branch.branchId !== view.branch.branchId || current.viewNodeId) return;
    const tick = changes.current;
    const promise = saveDraft(editor.current, version.current, baseRevision.current);
    inFlight.current = promise;
    try {
      const saved = await promise;
      version.current = saved.version;
      if (tick === changes.current) { dirtyRef.current = false; if (stillHere()) { clearBackup(); setDirty(false); } }
      else if (stillHere()) backup();
      if (stillHere()) setSaveIssue("");
    } catch (failure) {
      if (stillHere()) setSaveIssue(failure instanceof Error ? failure.message : "保存失败，本地编辑已保留");
      throw failure;
    } finally { inFlight.current = null; }
    if (dirtyRef.current && stillHere()) await saveNow();
  }, [readonly, saveDraft, view.sessionId, view.branch.branchId, stillHere, clearBackup, backup]);

  useEffect(() => {
    if (!dirty || !ready || readonly || saveIssue) return;
    const timer = setTimeout(() => { void saveNow().catch(() => undefined); }, 500);
    return () => clearTimeout(timer);
  }, [editTick, dirty, ready, readonly, saveIssue, saveNow]);

  // 讨论推进由 store 负责对账（见 directorSlice.attachDirectorRequest）：
  // 组件不再自己轮询，面板关闭时讨论也能走到终态。
  useEffect(() => { if (!runningRequestId) return; void load(); }, [runningRequestId, load]);

  const edit = (next: DirectorPlan) => {
    editor.current = next; changes.current++; dirtyRef.current = true;
    setPlan(next); setDirty(true); setEditTick(changes.current); backup();
  };
  const updateBeat = (beatId: string, patch: Partial<DirectorBeat>) => edit({ ...editor.current, beats: editor.current.beats.map(b => b.beatId === beatId ? { ...b, ...patch } : b) });
  const moveBeat = (index: number, direction: number) => {
    const beats = [...editor.current.beats]; [beats[index], beats[index + direction]] = [beats[index + direction], beats[index]];
    edit({ ...editor.current, beats });
  };
  const close = async () => { try { await saveNow(); } catch { /* Durable browser backup survives closing the workspace. */ } if (stillHere()) onClose(); };
  closeRef.current = () => void close();
  const run = async (action: DirectorAction, beatId?: string) => {
    try {
      await saveNow();
      if (!stillHere()) return;
      await command(action, beatId, action === "activate" ? replace : undefined);
      if (stillHere()) {
        setNotice(action === "activate" ? "大纲已启用，从下一回合开始生效。" : "导演进度已更新。");
        if (action === "activate") setReplace(false);
      }
    }
    catch { /* Store error is shown in this workspace. */ }
  };
  const send = async (message = text) => {
    if (!message.trim() || running || sending || readonly) return;
    setSending(true);
    try { await saveNow(); if (!stillHere()) return; await discuss(message); if (stillHere()) setText(""); }
    catch { /* Preserve the input for retry. */ }
    finally { if (stillHere()) setSending(false); }
  };
  const loadSaved = async () => {
    await load();
    if (!stillHere()) return;
    const latest = useStory.getState().directorView;
    if (!latest) return;
    const saved = latest.draft?.plan ?? latest.state?.plan ?? blankPlan();
    version.current = latest.draft?.version ?? 0; baseRevision.current = latest.draft?.baseRevisionId ?? latest.state?.plan.revisionId ?? "";
    editor.current = saved; dirtyRef.current = false; clearBackup(); setPlan(saved); setDirty(false); setSaveIssue("");
  };
  const keepLocal = async () => {
    await load();
    if (!stillHere()) return;
    const latest = useStory.getState().directorView;
    if (!latest) return;
    version.current = latest.draft?.version ?? 0; baseRevision.current = latest.state?.plan.revisionId ?? "";
    setSaveIssue(""); backup(); void saveNow().catch(() => undefined);
  };
  const valid = !!plan.title.trim() && plan.beats.length > 0 && plan.beats.every(b => b.title.trim() && b.instruction.trim() && b.completionCriteria.trim());
  const currentIndex = plan.beats.findIndex(b => b.beatId === active?.currentBeatId);
  const firstMovable = replace || !active ? 0 : currentIndex >= 0 ? currentIndex + 1 : Object.keys(active.progress).length;

  return createPortal(<div className="director-overlay">
    <div {...dialog} className="director-dialog" aria-labelledby="director-title">
      <header className="director-header">
        <div><h2 id="director-title">导演模式</h2><p>先商量故事的方向，再让角色走进下一幕。</p></div>
        <Button variant="outline" onClick={() => void close()}>返回剧情 <Icon name="close" size={13} /></Button>
      </header>
      {readonly && <div className="director-notice">正在查看历史大纲，此处只读。回到当前节点或从此处分叉后，可以调整未来安排。</div>}
      {active && <div className="director-active-summary">
        <strong>{active.plan.title}</strong><span>{statusText[active.status] ?? active.status} · {Object.keys(active.progress).length}/{active.plan.beats.length} 阶段</span>
        <div className="director-actions">
          {active.status !== "completed" && <><Button size="sm" variant="outline" disabled={busy} onClick={() => void run(active.status === "paused" ? "resume" : "pause")}>{active.status === "paused" ? "恢复大纲" : "暂停大纲"}</Button>
            <Button size="sm" variant="outline" disabled={busy} onClick={() => void run("complete", active.currentBeatId)}>标记当前阶段完成</Button>
            <Button size="sm" variant="ghost" disabled={busy} onClick={() => void run("skip", active.currentBeatId)}>跳过当前阶段</Button></>}
        </div>
        {active.warning && <p className="director-warning">{active.warning}</p>}
        {!!Object.keys(active.progress).length && <details><summary>已结束阶段与完成依据</summary>
          {active.plan.beats.filter(b => active.progress[b.beatId]).map(b => <div className="director-evidence" key={b.beatId}>
            <span>{b.title} · {active.progress[b.beatId].status === "skipped" ? "已跳过" : "已完成"}</span>
            {active.progress[b.beatId].evidence?.map((e, i) => <blockquote key={i}>{e.quote}</blockquote>)}
            <Button size="sm" variant="ghost" disabled={busy} onClick={() => void run("rewind", b.beatId)}><Icon name="undo" size={12} /> 回退到「{b.title}」</Button>
          </div>)}
        </details>}
      </div>}
      {(error || saveIssue) && <div className="director-error" role="alert">
        <p>{saveIssue || error}</p>
        <div className="director-actions">
          <Button size="sm" variant="outline" onClick={() => void useStory.getState().reloadView().then(load)}>刷新状态</Button>
          {saveIssue && <><Button size="sm" variant="outline" onClick={() => void keepLocal()}>保留我的编辑并保存</Button><Button size="sm" variant="outline" onClick={() => void loadSaved()}>载入已保存草稿</Button></>}
        </div>
      </div>}
      {notice && <p className="director-notice" role="status">{notice}</p>}
      {!workspace ? <p className="director-notice">{loading ? "正在读取导演工作区…" : "暂时无法读取，请刷新重试。"}</p> : <div className="director-columns">
        <section className="director-discussion" aria-label="导演讨论">
          <h3>讨论走向</h3><p className="director-muted">这里的讨论独立保存。描述你想发生什么，或粘贴已有的大纲。</p>
          <div className="director-chat" aria-live="polite">
            {!workspace.requests.length && (
              <EmptyState
                variant="plain"
                icon={<Icon name="sparkles" size={22} />}
                title="希望下一幕发生什么？"
                description="例如：先让两人试探彼此，再因旧信产生误会，最后留下和解的机会。"
              />
            )}
            {workspace.requests.map(request => <div key={request.requestId} className="director-exchange">
              <article className="director-chat-user"><strong>你</strong><p>{request.text}</p></article>
              <article className="director-chat-assistant"><strong>导演助手</strong>
                <p>{request.status === "generating" ? "正在梳理剧情安排…" : request.reply || request.error || "讨论已停止"}</p>
                {request.candidate && !request.draftApplied && <><p className="director-muted">这份建议尚未载入；你的草稿或剧情已更新。</p>
                  <Button size="sm" variant="outline" disabled={readonly || saving} onClick={() => { version.current = workspace.draft?.version ?? 0; baseRevision.current = active?.plan.revisionId ?? ""; setSaveIssue(""); edit(request.candidate!); }}>载入此建议</Button></>}
                {["failed", "cancelled", "interrupted"].includes(request.status) && <Button size="sm" variant="outline" disabled={readonly || !!running || sending} onClick={() => void send(request.text)}>重试讨论</Button>}
              </article>
            </div>)}
          </div>
          {!readonly && <form className="director-chat-composer" onSubmit={event => { event.preventDefault(); void send(); }}>
            <label htmlFor="director-message">与导演讨论</label><textarea id="director-message" value={text} maxLength={8000} rows={3} placeholder="描述目标、转折、节奏，或想避免的情节…" onChange={event => setText(event.target.value)} disabled={!!running || sending} />
            <div className="director-actions">{running ? <Button variant="outline" onClick={() => void cancel(running.requestId).catch(() => undefined)}>停止讨论</Button> : <Button type="submit" variant="primary" disabled={!text.trim() || sending || !ready}>{sending ? "发送中…" : "发送给导演"}</Button>}</div>
          </form>}
        </section>
        <section className="director-editor" aria-label="大纲编辑">
          <div className="director-section-heading"><h3>{readonly ? "当时的大纲" : "大纲草稿"}</h3><span role="status">{readonly ? "只读" : saving ? "保存中…" : dirty ? "有未同步编辑" : workspace.draft ? "已保存" : "等待填写"}</span></div>
          <label>大纲标题<input aria-label="大纲标题" value={plan.title} maxLength={200} disabled={readonly || !ready || commanding} placeholder="例如：雨夜重逢" onChange={event => edit({ ...editor.current, title: event.target.value })} /></label>
          <label>全局叙事要求<textarea aria-label="全局叙事要求" value={plan.guidance} rows={3} maxLength={4000} disabled={readonly || !ready || commanding} placeholder="贯穿各阶段的风格与约束；把未来的揭晓留在对应阶段。" onChange={event => edit({ ...editor.current, guidance: event.target.value })} /></label>
          {!readonly && active && <label className="director-replace"><input type="checkbox" checked={replace} onChange={event => setReplace(event.target.checked)} />整体替换大纲，从首阶段重新开始</label>}
          <div className="director-beats">{plan.beats.map((beat, index) => {
            const settled = !replace && !!active?.progress[beat.beatId];
            const current = !replace && active?.currentBeatId === beat.beatId;
            const locked = readonly || settled || commanding;
            return <article className={`director-beat ${current ? "current" : ""}`} key={beat.beatId}>
              <div className="director-beat-top"><span className="director-beat-number">{String(index + 1).padStart(2, "0")}</span><span>{current ? "当前阶段" : settled ? "已结束" : "待推进"}</span>
                {!readonly && <div className="director-actions">
                  <IconButton size="sm" label={`上移阶段 ${index + 1}`} disabled={locked || index <= firstMovable} onClick={() => moveBeat(index, -1)}><Icon name="arrowUp" size={13} /></IconButton>
                  <IconButton size="sm" label={`下移阶段 ${index + 1}`} disabled={locked || index < firstMovable || index === plan.beats.length - 1} onClick={() => moveBeat(index, 1)}><Icon name="arrowDown" size={13} /></IconButton>
                  <IconButton size="sm" label={`删除阶段 ${index + 1}`} disabled={locked || current} onClick={() => edit({ ...editor.current, beats: editor.current.beats.filter(b => b.beatId !== beat.beatId) })}><Icon name="trash" size={13} /></IconButton>
                </div>}
              </div>
              <label>阶段名称<input aria-label={`阶段 ${index + 1} 名称`} value={beat.title} maxLength={200} disabled={locked} onChange={event => updateBeat(beat.beatId, { title: event.target.value })} /></label>
              <label>剧情安排<textarea aria-label={`阶段 ${index + 1} 安排`} value={beat.instruction} rows={3} maxLength={4000} disabled={locked} onChange={event => updateBeat(beat.beatId, { instruction: event.target.value })} /></label>
              <label>完成条件<textarea aria-label={`阶段 ${index + 1} 完成条件`} value={beat.completionCriteria} rows={2} maxLength={2000} disabled={locked} placeholder="什么真正发生之后，才能进入下一阶段？" onChange={event => updateBeat(beat.beatId, { completionCriteria: event.target.value })} /></label>
            </article>;
          })}</div>
          {!readonly && <Button variant="outline" block className="director-add" disabled={!ready || commanding || plan.beats.length >= 32} onClick={() => edit({ ...editor.current, beats: [...editor.current.beats, { beatId: crypto.randomUUID(), title: "", instruction: "", completionCriteria: "" }] })}><Icon name="plus" size={13} /> 添加阶段</Button>}
        </section>
      </div>}
      <footer className="director-footer"><p>{readonly ? "历史进度随查看节点还原。" : "确认后从下一回合生效。每阶段可以跨多轮，完成后恢复自由续写。"}</p>
        {!readonly && <Button variant="primary" disabled={!valid || !ready || busy || saving || !!saveIssue} onClick={() => void run("activate")}>{commanding ? "正在启用…" : replace ? "确认替换并启用" : "确认启用大纲"}</Button>}
      </footer>
    </div>
  </div>, document.body);
}
