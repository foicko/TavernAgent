// TurnEvidence：「这一轮为什么这样演」。
//
// 这是整套系统最被低估的资产：检定记了骰值/属性/修正/DC，记忆有来源引文与置信度，
// 状态变更全是从领域事件推导出来的。但读者过去只看得到「结果」，于是既分不清角色
// 是「不知道」还是「知道了没用上」，也无从发现记忆召回错了——那是最难自己察觉的
// 一类故障。
//
// 一条硬约束：这里**只展示服务端已提交的节点内容**，前端不做任何推断、补全或措辞
// 加工。依据一旦是猜出来的，它就不再是依据，反而会掩护真正的错误。
import { useState } from "react";
import type { StoryMessage } from "../stores/storyTypes";
import { useStory } from "../stores/storyStore";
import { getAttributeMeta, getOutcomeMeta } from "../lib/ruleCheck";
import { Icon } from "../ui/Icon";
import "./TurnEvidence.css";

const MEMORY_KIND_LABEL: Record<string, string> = {
  observed: "见证",
  reported: "转述",
  inferred: "推测",
  secret: "秘密",
};

export function TurnEvidence({ msg }: { msg: StoryMessage }) {
  const [open, setOpen] = useState(false);
  // 检定用的动作名从会话动作表里取（与输入台展示的是同一份口径）。
  const actions = useStory((s) => s.view?.actions);
  const changes = msg.changes ?? [];
  const memories = msg.injectedMemories ?? [];
  const checks = msg.checks ?? [];
  const note = (msg.inputNote ?? "").trim();
  const suppressed = msg.suppressedOptions ?? 0;
  if (changes.length === 0 && memories.length === 0 && checks.length === 0 && note === "" && suppressed === 0) return null;

  const counts = [
    checks.length > 0 ? `${checks.length} 次检定` : "",
    memories.length > 0 ? `${memories.length} 条记忆` : "",
  ].filter(Boolean);
  // 展开里只放“筹码看不到的明细”（检定算式与参考记忆）；变化筹码已经常驻，
  // 再在面板里列一遍就是同一信息占两处。
  const hasDetail = checks.length > 0 || memories.length > 0;

  return (
    <div className="turn-evidence">
      {/* 常驻一行：结果先看得见，细节再展开。 */}
      {(note !== "" || changes.length > 0 || checks.length > 0) && (
        <div className="turn-evidence-line">
          {suppressed > 0 && (
            // 收起台阶也要说一声：读者要能分清"这一轮本来没有抉择"与
            // "系统没有把选项摆出来"——两者对"我现在该做什么"含义完全不同。
            <span className="evidence-chip muted-chip" title="按「只在关键节点」规则收起">
              <Icon name="sparkles" size={11} /> <span className="evidence-chip-label">未摆出选项</span>
              <span className="evidence-chip-text">{suppressed} 条（本轮承接上一轮抉择）</span>
            </span>
          )}
          {note !== "" && (
            <span className="evidence-chip note-chip" title="本轮注记（非叙事）：只约束这一次演绎">
              <Icon name="note" size={11} /> <span className="evidence-chip-label">本轮注记</span>
              <span className="evidence-chip-text">{note}</span>
            </span>
          )}
          {checks.map((check) => {
            const attr = getAttributeMeta(check.attribute);
            const label = actions?.find((a) => a.actionId === check.actionId)?.label || `${attr.name}检定`;
            const meta = getOutcomeMeta(check.outcome);
            const tone = check.outcome === "critical_success" ? "is-up is-crit-up"
              : check.outcome === "success" ? "is-up"
                : check.outcome === "critical_failure" ? "is-down is-crit-down" : "is-down";
            const modSign = check.attributeModifier >= 0 ? "+" : "";
            const title = `${label}（${attr.name}）· 掷骰：d20 = ${check.natural}，修正 ${modSign}${check.attributeModifier}，总值 ${check.total} / DC ${check.dc}（${meta.label}）`;
            return (
              <span key={check.rollId} className={`evidence-chip dice-chip ${tone}`} title={title}>
                <Icon name="dice" size={11} />
                <span className="evidence-chip-label">{label}</span>
                <span className="evidence-chip-text">{meta.label}</span>
                {meta.isCrit && <span className="evidence-chip-crit-mark" aria-hidden="true">{meta.symbol}</span>}
              </span>
            );
          })}
          {changes.map((change, i) => {
            const tone = change.delta === undefined || change.delta === 0 ? "" : change.delta > 0 ? " is-up" : " is-down";
            return (
              <span key={`${change.kind}-${i}`} className={`evidence-chip change-chip${tone}`}>
                <span className="evidence-chip-label">{change.label}</span>
                <span className="evidence-chip-text">{change.text}</span>
              </span>
            );
          })}
        </div>
      )}

      {hasDetail && (
        <button
          type="button"
          className="turn-evidence-toggle"
          aria-expanded={open}
          onClick={() => setOpen((prev) => !prev)}
          title="展开本轮依据：检定明细与参考到的记忆"
        >
          {open ? "收起依据" : `展开依据（${counts.join(" · ")}）`} <Icon name={open ? "chevronUp" : "chevronDown"} size={11} />
        </button>
      )}

      {open && hasDetail && (
        <div className="turn-evidence-panel">
          {checks.length > 0 && (
            <section className="evidence-section">
              <h4>检定</h4>
              <div className="evidence-checks-list">
                {checks.map((check) => {
                  const attr = getAttributeMeta(check.attribute);
                  const label = actions?.find((a) => a.actionId === check.actionId)?.label || `${attr.name}检定`;
                  const meta = getOutcomeMeta(check.outcome);
                  const isNat20 = check.natural === 20;
                  const isNat1 = check.natural === 1;
                  const modSign = check.attributeModifier >= 0 ? "+" : "";
                  return (
                    <div key={check.rollId} className={`evidence-check-card ${meta.badgeClass}`}>
                      <div className="check-card-header">
                        <div className="check-card-title-group">
                          <Icon name="dice" size={12} className="check-card-dice-icon" />
                          <span className="check-card-label">{label}</span>
                          <span className="check-card-attr-badge">{attr.name} {attr.abbr}</span>
                        </div>
                        <span className={`check-outcome-badge ${meta.badgeClass}`}>
                          {meta.symbol} {meta.label}
                        </span>
                      </div>

                      <div className="check-card-formula">
                        <div className="check-card-formula-text">
                          {check.attribute}：d20 = {check.natural}，修正 {modSign}{check.attributeModifier}，总值 {check.total} / DC {check.dc} · {meta.label}
                        </div>
                        <div className="check-card-formula-pills" aria-label="骰点算式拆解">
                          <span className={`formula-pill ${isNat20 ? "is-nat20" : isNat1 ? "is-nat1" : ""}`}>
                            <span className="formula-pill-lbl">d20</span>
                            <strong>{check.natural}</strong>
                            {isNat20 && <span className="formula-tag">Nat 20</span>}
                            {isNat1 && <span className="formula-tag">Nat 1</span>}
                          </span>
                          <span className="formula-op">+</span>
                          <span className="formula-pill">
                            <span className="formula-pill-lbl">修正</span>
                            <strong>{modSign}{check.attributeModifier}</strong>
                          </span>
                          <span className="formula-op">=</span>
                          <span className={`formula-pill formula-total ${meta.isSuccess ? "pill-up" : "pill-down"}`}>
                            <span className="formula-pill-lbl">总值</span>
                            <strong>{check.total}</strong>
                          </span>
                          <span className="formula-vs">vs</span>
                          <span className="formula-pill formula-dc">
                            <span className="formula-pill-lbl">目标</span>
                            <strong>DC {check.dc}</strong>
                          </span>
                        </div>
                      </div>

                      {check.permanentEffect && (
                        <div className="evidence-permanent-effect">
                          <Icon name="sparkles" size={11} />
                          <span>永久影响：{check.permanentEffect}</span>
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            </section>
          )}

          {memories.length > 0 && (
            <section className="evidence-section">
              <h4>本轮参考的记忆</h4>
              {memories.map((memory) => (
                <div key={memory.memoryId} className="evidence-row">
                  {memory.kind && <span className="evidence-row-note">{MEMORY_KIND_LABEL[memory.kind] ?? memory.kind}</span>}
                  <span className="evidence-row-text">{memory.text}</span>
                </div>
              ))}
              <p className="evidence-footnote">
                这些是本次生成实际注入的记忆快照。角色没提到的事，未必是它忘了——也可能是这条记忆没被召回。
              </p>
            </section>
          )}
        </div>
      )}
    </div>
  );
}
