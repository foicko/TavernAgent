// 演义交接快照与前情回顾区块（M4b）。
// 展示当前查看路径上生效的宏观剧情推进、角色心境暗流与未解悬念。
import { useMemo, useState } from "react";
import { useStory } from "../stores/storyStore";
import { parseStoryCheckpoint } from "../lib/storyCheckpoint";
import "./StoryCheckpointSection.css";

export function StoryCheckpointSection() {
  const view = useStory((s) => s.view);
  const [expanded, setExpanded] = useState(true);

  const activeSummary = view?.activeSummary;
  const checkpoint = useMemo(() => parseStoryCheckpoint(activeSummary?.text ?? ""), [activeSummary?.text]);
  if (!activeSummary || !activeSummary.text) {
    return null;
  }

  const { narrativeArc, mindsets, hiddenTension, openLoops, milestones } = checkpoint;

  return (
    <div className="hud-block story-checkpoint-section">
      <div
        className="hud-block-title story-checkpoint-toggle"
        role="button"
        tabIndex={0}
        aria-expanded={expanded}
        onClick={() => setExpanded(!expanded)}
        onKeyDown={(event) => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            setExpanded(!expanded);
          }
        }}
      >
        <span>📖 前情回顾与交接</span>
        <span className="story-checkpoint-toggle-hint">{expanded ? "收起 ▲" : "展开 ▼"}</span>
      </div>

      {expanded && (
        <div className="story-checkpoint-body">
          {narrativeArc && (
            <div className="checkpoint-card">
              <div className="checkpoint-card-header">
                <span className="checkpoint-card-tag">📜 剧情推进</span>
              </div>
              <p className="checkpoint-card-text">{narrativeArc}</p>
            </div>
          )}

          {(mindsets.length > 0 || hiddenTension) && (
            <div className="checkpoint-card">
              <div className="checkpoint-card-header">
                <span className="checkpoint-card-tag">🌊 角色心境与暗涌</span>
              </div>
              {mindsets.map((m, idx) => (
                <div key={idx} className="checkpoint-card-mindset">
                  {m.attr ? <span className="checkpoint-attr-pill">{m.attr}：</span> : null}
                  <span>{m.content}</span>
                </div>
              ))}
              {hiddenTension && (
                <div className="checkpoint-card-tension">
                  <span className="checkpoint-tension-label">暗流：</span>
                  <span>{hiddenTension}</span>
                </div>
              )}
            </div>
          )}

          {openLoops && (
            <div className="checkpoint-card">
              <div className="checkpoint-card-header">
                <span className="checkpoint-card-tag">⏳ 待解悬念与约定</span>
              </div>
              <pre className="checkpoint-card-pre">
                {openLoops}
              </pre>
            </div>
          )}

          {milestones && (
            <div className="checkpoint-card">
              <div className="checkpoint-card-header">
                <span className="checkpoint-card-tag">🚩 阶段里程碑</span>
              </div>
              <pre className="checkpoint-card-pre">
                {milestones}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
