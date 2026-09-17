import { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import type { DirectorAction, DirectorBeat, DirectorDraft, DirectorPlan } from "../app/types";
import { directorBusy, storyBusy, useStory } from "../stores/storyStore";
import { directorScopeKey } from "../stores/slices/directorSlice";
import { owns, scope } from "../stores/storyHelpers";

const blankPlan = (): DirectorPlan => ({ title: "", guidance: "", beats: [] });
const statusText = { active: "执行中", paused: "已暂停", completed: "已完成" };

export function DirectorStatusBar({ onOpen }: { onOpen: () => void }) {
  const summary = useStory(s => s.view?.director);
  const error = useStory(s => s.directorError);
  const disabled = useStory(s => !!s.viewNodeId || storyBusy(s));
  const command = useStory(s => s.commandDirector);
  if (!summary) return null;
  return <div className="director-status" aria-label="导演进度">
    <div className="director-status-main">
      <button type="button" className="btn-quiet" onClick={onOpen}>🎬 {summary.title} · {statusText[summary.status]}</button>
      <span>{summary.status === "completed" ? "阶段已全部结束，可自由续写" : `当前：${summary.currentBeatTitle}`} · {summary.completed}/{summary.total}</span>
      {summary.status !== "completed" && <button type="button" className="btn-quiet" disabled={disabled}
        onClick={() => void command(summary.status === "paused" ? "resume" : "pause").catch(() => undefined)}>{summary.status === "paused" ? "恢复大纲" : "暂停大纲"}</button>}
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
  const dialog = useRef<HTMLDivElement>(null);
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
    const previous = document.activeElement as HTMLElement | null;
    dialog.current?.querySelector<HTMLButtonElement>("button")?.focus();
    return () => { mounted.current = false; previous?.focus(); };
  }, []);

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
    <div ref={dialog} className="director-dialog" role="dialog" aria-modal="true" aria-labelledby="director-title"
      onKeyDown={event => {
        if (event.key === "Escape") { event.stopPropagation(); void close(); }
        if (event.key === "Tab") {
          const items = Array.from(dialog.current?.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled),textarea:not(:disabled),select:not(:disabled),[tabindex="0"]') ?? []);
          const first = items[0], last = items.at(-1);
          if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
          else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
        }
      }}>
      <header className="director-header">
        <div><p className="director-eyebrow">STORY DIRECTOR</p><h2 id="director-title">导演模式</h2><p>先商量故事的方向，再让角色走进下一幕。</p></div>
        <button type="button" className="btn-quiet" onClick={() => void close()}>返回剧情 ×</button>
      </header>
      {readonly && <div className="director-notice">正在查看历史大纲，此处只读。回到当前节点或从此处分叉后，可以调整未来安排。</div>}
      {active && <div className="director-active-summary">
        <strong>{active.plan.title}</strong><span>{statusText[active.status]} · {Object.keys(active.progress).length}/{active.plan.beats.length} 阶段</span>
        <div className="director-actions">
          {active.status !== "completed" && <><button className="btn-quiet" disabled={busy} onClick={() => void run(active.status === "paused" ? "resume" : "pause")}>{active.status === "paused" ? "恢复大纲" : "暂停大纲"}</button>
            <button className="btn-quiet" disabled={busy} onClick={() => void run("complete", active.currentBeatId)}>标记当前阶段完成</button>
            <button className="btn-quiet" disabled={busy} onClick={() => void run("skip", active.currentBeatId)}>跳过当前阶段</button></>}
        </div>
        {active.warning && <p className="director-warning">{active.warning}</p>}
        {!!Object.keys(active.progress).length && <details><summary>已结束阶段与完成依据</summary>
          {active.plan.beats.filter(b => active.progress[b.beatId]).map(b => <div className="director-evidence" key={b.beatId}>
            <span>{b.title} · {active.progress[b.beatId].status === "skipped" ? "已跳过" : "已完成"}</span>
            {active.progress[b.beatId].evidence?.map((e, i) => <blockquote key={i}>{e.quote}</blockquote>)}
            <button className="btn-quiet" disabled={busy} onClick={() => void run("rewind", b.beatId)}>回退到「{b.title}」</button>
          </div>)}
        </details>}
      </div>}
      {(error || saveIssue) && <div className="director-error" role="alert">
        <p>{saveIssue || error}</p><button className="btn-quiet" onClick={() => void useStory.getState().reloadView().then(load)}>刷新状态</button>
        {saveIssue && <><button className="btn-quiet" onClick={() => void keepLocal()}>保留我的编辑并保存</button><button className="btn-quiet" onClick={() => void loadSaved()}>载入已保存草稿</button></>}
      </div>}
      {notice && <p className="director-notice" role="status">{notice}</p>}
      {!workspace ? <p className="director-notice">{loading ? "正在读取导演工作区…" : "暂时无法读取，请刷新重试。"}</p> : <div className="director-columns">
        <section className="director-discussion" aria-label="导演讨论">
          <h3>讨论走向</h3><p className="director-muted">这里的讨论独立保存。描述你想发生什么，或粘贴已有的大纲。</p>
          <div className="director-chat" aria-live="polite">
            {!workspace.requests.length && <div className="director-empty"><span>✦</span><p>希望下一幕发生什么？</p><small>例如：先让两人试探彼此，再因旧信产生误会，最后留下和解的机会。</small></div>}
            {workspace.requests.map(request => <div key={request.requestId} className="director-exchange">
              <article className="director-chat-user"><strong>你</strong><p>{request.text}</p></article>
              <article className="director-chat-assistant"><strong>导演助手</strong>
                <p>{request.status === "generating" ? "正在梳理剧情安排…" : request.reply || request.error || "讨论已停止"}</p>
                {request.candidate && !request.draftApplied && <><p className="director-muted">这份建议尚未载入；你的草稿或剧情已更新。</p>
                  <button className="btn-quiet" disabled={readonly || saving} onClick={() => { version.current = workspace.draft?.version ?? 0; baseRevision.current = active?.plan.revisionId ?? ""; setSaveIssue(""); edit(request.candidate!); }}>载入此建议</button></>}
                {["failed", "cancelled", "interrupted"].includes(request.status) && <button className="btn-quiet" disabled={readonly || !!running || sending} onClick={() => void send(request.text)}>重试讨论</button>}
              </article>
            </div>)}
          </div>
          {!readonly && <form className="director-chat-composer" onSubmit={event => { event.preventDefault(); void send(); }}>
            <label htmlFor="director-message">与导演讨论</label><textarea id="director-message" value={text} maxLength={8000} rows={3} placeholder="描述目标、转折、节奏，或想避免的情节…" onChange={event => setText(event.target.value)} disabled={!!running || sending} />
            <div className="director-actions">{running ? <button type="button" className="btn-quiet" onClick={() => void cancel(running.requestId).catch(() => undefined)}>停止讨论</button> : <button type="submit" className="director-primary" disabled={!text.trim() || sending || !ready}>{sending ? "发送中…" : "发送给导演"}</button>}</div>
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
                {!readonly && <div className="director-actions"><button type="button" aria-label={`上移阶段 ${index + 1}`} disabled={locked || index <= firstMovable} onClick={() => moveBeat(index, -1)}>↑</button><button type="button" aria-label={`下移阶段 ${index + 1}`} disabled={locked || index < firstMovable || index === plan.beats.length - 1} onClick={() => moveBeat(index, 1)}>↓</button><button type="button" aria-label={`删除阶段 ${index + 1}`} disabled={locked || current} onClick={() => edit({ ...editor.current, beats: editor.current.beats.filter(b => b.beatId !== beat.beatId) })}>删除</button></div>}
              </div>
              <label>阶段名称<input aria-label={`阶段 ${index + 1} 名称`} value={beat.title} maxLength={200} disabled={locked} onChange={event => updateBeat(beat.beatId, { title: event.target.value })} /></label>
              <label>剧情安排<textarea aria-label={`阶段 ${index + 1} 安排`} value={beat.instruction} rows={3} maxLength={4000} disabled={locked} onChange={event => updateBeat(beat.beatId, { instruction: event.target.value })} /></label>
              <label>完成条件<textarea aria-label={`阶段 ${index + 1} 完成条件`} value={beat.completionCriteria} rows={2} maxLength={2000} disabled={locked} placeholder="什么真正发生之后，才能进入下一阶段？" onChange={event => updateBeat(beat.beatId, { completionCriteria: event.target.value })} /></label>
            </article>;
          })}</div>
          {!readonly && <button type="button" className="director-add" disabled={!ready || commanding || plan.beats.length >= 32} onClick={() => edit({ ...editor.current, beats: [...editor.current.beats, { beatId: crypto.randomUUID(), title: "", instruction: "", completionCriteria: "" }] })}>＋ 添加阶段</button>}
        </section>
      </div>}
      <footer className="director-footer"><p>{readonly ? "历史进度随查看节点还原。" : "确认后从下一回合生效。每阶段可以跨多轮，完成后恢复自由续写。"}</p>
        {!readonly && <button type="button" className="director-primary" disabled={!valid || !ready || busy || saving || !!saveIssue} onClick={() => void run("activate")}>{commanding ? "正在启用…" : replace ? "确认替换并启用" : "确认启用大纲"}</button>}
      </footer>
    </div>
  </div>, document.body);
}
