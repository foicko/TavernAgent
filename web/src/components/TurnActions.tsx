// 回合操作：重新生成 / 改正文 / 改输入 / 分叉 / 候选切换（技术契约 §7）。
//
// 四条路径都不改写已提交节点：
//   重新生成、改输入 → 从 parent(N) 派生候选分支，新回复与 N 成为兄弟
//   改正文          → 后端落为纯叙事候选（零状态变化，丢弃原提议）
//   分叉            → 只建分支，不生成内容
import { useState } from "react";
import type { StoryMessage } from "../stores/storyStore";
import { storyBusy, useStory } from "../stores/storyStore";
import "./TurnActions.css";

export function TurnActions({ msg }: { msg: StoryMessage }) {
  const busy = useStory(storyBusy);
  const regenerate = useStory((s) => s.regenerate);
  const editPlayerInput = useStory((s) => s.editPlayerInput);
  const editAssistantText = useStory((s) => s.editAssistantText);
  const forkFrom = useStory((s) => s.forkFrom);
  const groups = useStory((s) => s.view?.candidateGroups);
  const viewAt = useStory((s) => s.viewAt);

  const [mode, setMode] = useState<null | "text" | "input">(null);
  const [draft, setDraft] = useState("");

  const group = msg.parentId ? groups?.[msg.parentId] : undefined;
  const hasCandidates = !!(group && group.length > 1);
  const currentIdx = hasCandidates ? group.findIndex((n) => n.nodeId === msg.id) : -1;

  if (mode === "text") {
    return (
      <div className="turn-editor">
        <div>
          ✏️ 改正文 · 将落为纯叙事候选（不改变世界状态，原回合保持不变）
        </div>
        <textarea
          value={draft}
          rows={3}
          onChange={(e) => setDraft(e.target.value)}
          placeholder="改写这一轮的叙述内容..."
        />
        <div>
          <button
            className="btn-solid"
            disabled={busy || !draft.trim()}
            onClick={() => {
              void editAssistantText(msg.id, [{ kind: "narration", text: draft.trim() }]);
              setMode(null);
            }}
          >
            保存为候选
          </button>
          <button
            className="btn-quiet"
            onClick={() => setMode(null)}
          >
            取消
          </button>
        </div>
      </div>
    );
  }

  if (mode === "input") {
    return (
      <div className="turn-editor">
        <div>
          💬 改输入 · 将从上一轮派生新分支并重新生成演义（原时间线保留）
        </div>
        <textarea
          value={draft}
          rows={2}
          onChange={(e) => setDraft(e.target.value)}
          placeholder="修改你当时的输入..."
        />
        <div>
          <button
            className="btn-solid"
            disabled={busy || !draft.trim()}
            onClick={() => {
              void editPlayerInput(msg.id, draft);
              setMode(null);
            }}
          >
            派生并生成
          </button>
          <button
            className="btn-quiet"
            onClick={() => setMode(null)}
          >
            取消
          </button>
        </div>
      </div>
    );
  }

  const blocksText = msg.blocks.map((b) => b.text).join("\n");

  return (
    <div className="turn-actions-bar">
      {/* 左侧：候选版本切换器 */}
      {hasCandidates && currentIdx >= 0 ? (
        <div>
          <button
            className="btn-quiet"
            disabled={busy || currentIdx <= 0}
            onClick={() => void viewAt(group[currentIdx - 1].nodeId)}
            title="查看上一候选版本"
          >
            ‹
          </button>
          <span>
            版本 {currentIdx + 1} / {group.length}
          </span>
          <button
            className="btn-quiet"
            disabled={busy || currentIdx >= group.length - 1}
            onClick={() => void viewAt(group[currentIdx + 1].nodeId)}
            title="查看下一候选版本"
          >
            ›
          </button>
        </div>
      ) : (
        <div />
      )}

      {/* 右侧：操作按钮群 */}
      <div>
        <button
          className="btn-quiet"
          disabled={busy}
          onClick={() => void regenerate(msg.id)}
          title="以父节点状态为基准重写文本，保留本次掷骰结果"
        >
          🔄 重新生成
        </button>
        {!!msg.checks?.length && (
          <button className="btn-quiet" disabled={busy} onClick={() => void regenerate(msg.id, true)} title="新建候选分支，重新掷骰并演绎新结果">
            🎲 重新掷骰
          </button>
        )}
        <button
          className="btn-quiet"
          disabled={busy}
          onClick={() => {
            setDraft(blocksText);
            setMode("text");
          }}
          title="手动修改此轮叙事（存为叙事候选）"
        >
          ✏️ 改正文
        </button>
        {msg.inputText && (
          <button
            className="btn-quiet"
            disabled={busy}
            onClick={() => {
              setDraft(msg.inputText ?? "");
              setMode("input");
            }}
            title="修改玩家在这一轮的输入并重新推演"
          >
            💬 改输入
          </button>
        )}
        <button
          className="btn-quiet"
          disabled={busy}
          onClick={() => void forkFrom(msg.id)}
          title="从本节点切出一条新时间线分支"
        >
          🌿 分叉
        </button>
      </div>
    </div>
  );
}

