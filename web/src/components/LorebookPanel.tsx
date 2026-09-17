import { useEffect, useState } from "react";
import { api } from "../app/api";
import type { LorebookPage, SessionView } from "../app/types";

const emptyPage = (): LorebookPage => ({ entries: [], hasMore: false, nextOffset: 0 });

export function LorebookPanel({ view }: { view: SessionView }) {
  const [query, setQuery] = useState("");
  const [offset, setOffset] = useState(0);
  const [revision, setRevision] = useState(0);
  const [page, setPage] = useState<LorebookPage>(emptyPage);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    setLoading(true); setError(null);
    const timer = window.setTimeout(() => {
      api.searchLorebook(view.sessionId, query, offset).then(result => {
        if (active) setPage(previous => ({ ...result, entries: offset ? [...previous.entries, ...result.entries] : result.entries }));
      }).catch(err => { if (active) setError(err instanceof Error ? err.message : String(err)); })
        .finally(() => { if (active) setLoading(false); });
    }, query ? 200 : 0);
    return () => { active = false; window.clearTimeout(timer); };
  }, [view.sessionId, query, offset, revision]);

  const secrets = (view.secrets || []).filter(secret => !query.trim() ||
    ((secret.title || "") + (secret.revealed ? secret.content || "" : "")).toLowerCase().includes(query.trim().toLowerCase()));

  return <section className="lorebook-panel" aria-label="会话世界书">
    <input className="lore-search-input" aria-label="检索会话世界书" placeholder="检索标题、触发词或正文…" value={query} maxLength={512} onChange={event => {
      setQuery(event.target.value); setOffset(0); setPage(emptyPage()); setLoading(true);
    }} />
    <h3>世界书词条 <small>已加载 {page.entries.length} 条</small></h3>
    <p className="story-map-muted">条目来自本次开局的角色卡；是否进入生成上下文由触发词决定。</p>
    {error && <p role="alert" className="story-map-error">{error} <button onClick={() => setRevision(value => value + 1)}>重试</button></p>}
    {!loading && !error && page.entries.length === 0 && <p className="story-map-muted">{query ? "没有匹配的世界书词条。" : "此会话没有公开世界书词条。"}</p>}
    {page.entries.map(match => <details className="lore-entry-card" key={match.bookIndex + ":" + match.entryIndex}>
      <summary className="lore-entry-top"><span className="lore-entry-title">{match.entry.title || match.entry.keys?.[0] || "未命名条目"}</span><span className="lore-toggle-pill">{match.entry.enabled ? "按词触发" : "已禁用"}</span></summary>
      <p className="story-map-muted">{match.bookName}</p>
      <div className="lore-keys-row">{(match.entry.keys || []).map((key, index) => <span className="lore-key-tag" key={index}>{key}</span>)}</div>
      {!!match.entry.secondaryKeys?.length && <p className="story-map-muted">还需触发：{match.entry.secondaryKeys.join(" / ")}</p>}
      <div className="lorebook-content">{match.entry.content}</div>
    </details>)}
    {loading && <p role="status">检索中…</p>}
    {page.hasMore && <button className="btn-quiet" disabled={loading} onClick={() => setOffset(page.nextOffset)}>加载更多词条</button>}
    <h3>剧情秘密</h3>
    {secrets.length === 0 && <p className="story-map-muted">{query ? "没有匹配的秘密标题。" : "此会话没有秘密条目。"}</p>}
    {secrets.map(secret => <article className="lore-entry-card" key={secret.secretId}>
      <div className="lore-entry-top"><span className="lore-entry-title">{secret.title || "剧情秘密"}</span><span className="lore-toggle-pill">{secret.revealed ? "已揭示" : "未揭示"}</span></div>
      <div className="lorebook-content">{secret.revealed ? secret.content : "推进剧情并达成条件后可查看。"}</div>
    </article>)}
  </section>;
}
