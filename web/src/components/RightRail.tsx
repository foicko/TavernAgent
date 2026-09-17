import React, { useEffect } from "react";
import { useUi, type TachieExpression } from "../stores/uiStore";
import { useStory } from "../stores/storyStore";
import { CHARACTER_PRESETS, type InventoryItemPreset } from "../lib/characterPresets";
import { StoryCheckpointSection } from "./StoryCheckpointSection";
import { characterPresentation } from "../lib/characterPresentation";
import { primaryCharacterId, affectionPercent } from "../lib/characterState";
import { DossierModal } from "./DossierModal";
import { PromisesModal } from "./PromisesModal";
import { MemoryBankModal } from "./MemoryBankModal";
import "./RightRail.css";

export const RightRail: React.FC = () => {
  const {
    isRightRailCollapsed,
    toggleRightRail,
    activeCharKey,
    tachieExpression,
    setTachieExpression,
    setFullTachieOpen,
    characterOverrides,
    meterDeltas,
    showInvBubble,
    setDossierModalOpen,
    setPromisesModalOpen,
    setMemoryModalOpen,
  } = useUi();

  const view = useStory((s) => s.view);
  const hud = useStory((s) => s.hud);
  const backendMemories = useStory((s) => s.memories);
  const memoryUsage = useStory((s) => s.memoryUsage);
  const memoryPaging = useStory((s) => s.memoryPaging);

  const char = characterPresentation(activeCharKey, view);

  // 1. 心境三维量表映射（优先使用后端真实世界状态，否则使用预设）
  const characterId = primaryCharacterId(hud, activeCharKey);
  const liveRel = hud && characterId ? hud.relationships[characterId] ?? { affection: 0, trust: 0, alertness: 0 } : undefined;

  const meters = liveRel
    ? {
        label1: "心意眷顾 (Affection)",
        val1: `${liveRel.affection > 0 ? "+" : ""}${liveRel.affection}`,
        width1: `${affectionPercent(liveRel.affection)}%`,
        label2: "信任沉淀 (Trust)",
        val2: `${liveRel.trust}%`,
        width2: `${Math.min(100, Math.max(0, liveRel.trust))}%`,
        label3: "防备戒心 (Alert)",
        val3: `${liveRel.alertness}%`,
        width3: `${Math.min(100, Math.max(0, liveRel.alertness))}%`,
      }
    : view ? CHARACTER_PRESETS.custom.meters : characterOverrides[activeCharKey ?? ""]?.meters || char.meters;

  // 实时模型情绪与立绘神情差分联动
  const liveMood = hud && characterId ? hud.moods[characterId] : undefined;

  useEffect(() => {
    if (!liveMood?.moodCode) return;
    const code = liveMood.moodCode.toLowerCase();
    if (code.includes("alert") || code.includes("angry") || code.includes("guard") || code.includes("shock")) {
      setTachieExpression("alert");
    } else if (code.includes("smile") || code.includes("happy") || code.includes("joy") || code.includes("relieved")) {
      setTachieExpression("smile");
    } else if (code.includes("whisper") || code.includes("blush") || code.includes("soft") || code.includes("moved") || code.includes("sad")) {
      setTachieExpression("whisper");
    } else if (code.includes("calm") || code.includes("neutral") || code.includes("quiet")) {
      setTachieExpression("calm");
    }
  }, [liveMood?.moodCode, setTachieExpression]);

  // 2. 背包与信物映射
  const liveInventory: InventoryItemPreset[] = view
    ? Object.values(hud?.items ?? {}).filter(item => item.quantity > 0 && item.ownerId === "player").map((item) => ({
        id: item.instanceId,
        name: item.name,
        title: item.name,
        status: item.keepsake ? "核心信物" : `数量: ${item.quantity}`,
        icon: item.keepsake ? "⏱️" : "📦",
        acts: [
          { key: "inspect", label: `仔细端详 ${item.name}`, text: `我将【${item.name}】握在手心仔细端详，思索其背后的渊源与线索...` },
          { key: "show", label: `向对方出示 ${item.name}`, text: `我将【${item.name}】轻轻置于桌面推向对方眼前，观察其神色变化。` },
          ...(view.actions ?? []).filter(action => action.itemIds?.includes(item.instanceId)).map(action => ({
            key: action.actionId, label: action.label || action.actionId, text: action.label || `我尝试使用【${item.name}】。`, actionRef: action.actionId,
          })),
        ],
      }))
    : char.inventory;

  const exprFilters: Record<TachieExpression, string> = {
    alert: "grayscale(10%) contrast(110%)",
    calm: "grayscale(10%)",
    smile: "grayscale(4%) brightness(104%)",
    whisper: "grayscale(0%) brightness(106%)",
  };

  const exprTitles: Record<TachieExpression, string> = {
    alert: "🎭 神态：警觉审视",
    calm: "🎭 神态：冷静自持",
    smile: "🎭 神态：微露释怀",
    whisper: "🎭 神态：动容微温",
  };

  const deltas1 = meterDeltas.filter((d) => d.meterIndex === 1);
  const deltas2 = meterDeltas.filter((d) => d.meterIndex === 2);
  const deltas3 = meterDeltas.filter((d) => d.meterIndex === 3);

  const livePromises = hud?.promises ? Object.values(hud.promises) : [];
  const activePromiseCount = view ? livePromises.filter(p => p.state === "active").length : 1;
  const memoryCount = view ? (memoryPaging.total || memoryUsage?.used || backendMemories.length) : char.memories.length;

  return (
    <aside className={`rail-right ${isRightRailCollapsed ? "collapsed" : ""}`} id="rail-right">
      {/* Right Rail Header Bar */}
      <div className="rail-right-header">
        <div className="rail-right-title">
          <span className="indicator-dot" />
          <span>角色档案 · 实时立绘</span>
        </div>
        <button className="rail-right-collapse-btn" onClick={toggleRightRail} title="收起角色档案栏 (Ctrl+])">
          <span>收起</span>
          <span>›</span>
        </button>
      </div>

      {/* 1. Real-time Character Portrait Stage (实时半身立绘舞台) */}
      <div className="tachie-hero-stage">
        <div className="tachie-viewport">
          <img
            id="tachie-img"
            src={char.avatar}
            alt={char.name}
            className="tachie-image tachie-image-live"
            style={{ "--rr-tachie-filter": exprFilters[tachieExpression] } as React.CSSProperties}
          />
          {/* 底部水墨羽化渐隐蒙版 */}
          <div className="tachie-gradient-mask" />

          {/* 顶部状态栏与全屏预览浮钮 */}
          <div className="tachie-top-bar">
            <span className="tachie-expression-badge" id="tachie-expr-tag">
              {exprTitles[tachieExpression]}
            </span>
            <button
              type="button"
              className="tachie-zoom-btn"
              onClick={() => setFullTachieOpen(true)}
              title="展开高清全身立绘与图鉴"
            >
              <span>⛶ 全身</span>
            </button>
          </div>

          {/* 底部微交互浮层：表情差分切换器 */}
          <div className="tachie-bottom-bar">
            <div className="tachie-expr-switcher" title="实时切换角色神情差分">
              <span
                className={`expr-switch-btn ${tachieExpression === "alert" ? "active" : ""}`}
                onClick={() => setTachieExpression("alert")}
              >
                警觉
              </span>
              <span
                className={`expr-switch-btn ${tachieExpression === "calm" ? "active" : ""}`}
                onClick={() => setTachieExpression("calm")}
              >
                冷静
              </span>
              <span
                className={`expr-switch-btn ${tachieExpression === "smile" ? "active" : ""}`}
                onClick={() => setTachieExpression("smile")}
              >
                释怀
              </span>
              <span
                className={`expr-switch-btn ${tachieExpression === "whisper" ? "active" : ""}`}
                onClick={() => setTachieExpression("whisper")}
              >
                动容
              </span>
            </div>
          </div>
        </div>

        {/* 角色姓名与身份铭牌 */}
        <div className="tachie-nameplate">
          <div className="tachie-name-row">
            <span className="tachie-main-name" id="dossier-name">
              {char.shortName || char.name}
            </span>
            <span className="tachie-stage-pill" id="dossier-stage-pill">
              {char.stagePill}
            </span>
          </div>
          {char.shortName && char.shortName !== char.name && (
            <div className="dossier-full-name-sub" title={char.name}>
              全名：{char.name}
            </div>
          )}
          <div className="dossier-identity" id="dossier-role">
            {char.role}
          </div>
          {char.dossier?.tags && char.dossier.tags.length > 0 && (
            <div className="dossier-tags-row">
              {char.dossier.tags.slice(0, 8).map((t, idx) => (
                <span key={idx} className="dossier-tag-pill">
                  #{t}
                </span>
              ))}
            </div>
          )}
          <div className="dossier-badges-row">
            <span className="dossier-badge" id="dossier-badge-state">
              <span className="dossier-badge-icon">🎭</span>
              <span>{char.stateBadge}</span>
            </span>
            <span className="dossier-badge" id="dossier-badge-alliance">
              <span className="dossier-badge-icon">🏛️</span>
              <span>{char.allianceBadge}</span>
            </span>
            <span className="dossier-badge" id="dossier-badge-voice">
              <span className="dossier-badge-icon">🎙️</span>
              <span>{char.voiceBadge}</span>
            </span>
          </div>
        </div>
      </div>

      {/* 2. Secondary Modals Entry Navigation Group (二级页面快捷导航磁贴组) */}
      <div className="hud-nav-tile-group">
        {/* 角色设定与世界观 */}
        <button
          type="button"
          className="hud-nav-tile"
          onClick={() => setDossierModalOpen(true)}
          title="点击展开完整角色人设、世界观与对话范例"
        >
          <div className="hud-nav-tile-icon">📜</div>
          <div className="hud-nav-tile-body">
            <div className="hud-nav-tile-title">角色设定与世界观</div>
            <div className="hud-nav-tile-subtitle">人设性格 · 场景规则 · 范例与说明</div>
          </div>
          <div className="hud-nav-tile-badge">详情</div>
          <div className="hud-nav-tile-arrow">›</div>
        </button>

        {/* 主动契约与承诺 */}
        <button
          type="button"
          className="hud-nav-tile"
          onClick={() => setPromisesModalOpen(true)}
          title="点击查看约定、许诺与违诺状态"
        >
          <div className="hud-nav-tile-icon">🤝</div>
          <div className="hud-nav-tile-body">
            <div className="hud-nav-tile-title">主动契约与承诺</div>
            <div className="hud-nav-tile-subtitle">
              {view ? `${activePromiseCount} 条生效约见 · 违诺状态跟踪` : `${char.pledge.title} · ${char.pledge.status}`}
            </div>
          </div>
          <div className="hud-nav-tile-badge">{view ? `${activePromiseCount} 生效` : char.pledge.status}</div>
          <div className="hud-nav-tile-arrow">›</div>
        </button>

        {/* 认知记忆库 */}
        <button
          type="button"
          className="hud-nav-tile"
          onClick={() => setMemoryModalOpen(true)}
          title="点击查看并修订亲历观察、推测记忆与心智星图"
        >
          <div className="hud-nav-tile-icon">🧠</div>
          <div className="hud-nav-tile-body">
            <div className="hud-nav-tile-title">认知记忆库</div>
            <div className="hud-nav-tile-subtitle">亲历观察 · 反思推演 · 分支独立 CoW</div>
          </div>
          <div className="hud-nav-tile-badge">{memoryCount} 条</div>
          <div className="hud-nav-tile-arrow">›</div>
        </button>
      </div>

      {/* 3. Minimalist Meters (极简素雅三维刻度) */}
      <div className="hud-block">
        <div className="hud-block-title">
          <span>心境三维量表</span>
          <span>实时投影</span>
        </div>

        <div className="meter-box-minimal">
          {/* Meter 1 */}
          <div className="meter-unit">
            <div className="meter-info-line">
              <span id="meter-label-1">{meters.label1}</span>
              <div className="meter-value-line">
                {deltas1.map((d) => (
                  <span key={d.id} className={`meter-delta-float ${d.isPositive ? "positive" : "negative"}`}>
                    {d.text} {d.isPositive ? "▲" : "▼"}
                  </span>
                ))}
                <span id="meter-num-aff">{meters.val1}</span>
              </div>
            </div>
            <div className="meter-bar-track">
              <div className="meter-bar-fill" id="meter-fill-aff" style={{ width: meters.width1 }} />
            </div>
          </div>

          {/* Meter 2 */}
          <div className="meter-unit">
            <div className="meter-info-line">
              <span id="meter-label-2">{meters.label2}</span>
              <div className="meter-value-line">
                {deltas2.map((d) => (
                  <span key={d.id} className={`meter-delta-float ${d.isPositive ? "positive" : "negative"}`}>
                    {d.text} {d.isPositive ? "▲" : "▼"}
                  </span>
                ))}
                <span id="meter-num-trust">{meters.val2}</span>
              </div>
            </div>
            <div className="meter-bar-track">
              <div className="meter-bar-fill" id="meter-fill-trust" style={{ width: meters.width2 }} />
            </div>
          </div>

          {/* Meter 3 */}
          <div className="meter-unit">
            <div className="meter-info-line">
              <span id="meter-label-3">{meters.label3}</span>
              <div className="meter-value-line">
                {deltas3.map((d) => (
                  <span key={d.id} className={`meter-delta-float ${d.isPositive ? "positive" : "negative"}`}>
                    {d.text} {d.isPositive ? "▲" : "▼"}
                  </span>
                ))}
                <span id="meter-num-alert">{meters.val3}</span>
              </div>
            </div>
            <div className="meter-bar-track">
              <div className="meter-bar-fill" id="meter-fill-alert" style={{ width: meters.width3 }} />
            </div>
          </div>
        </div>
      </div>

      {/* 4. Carried Items & Keepsakes (背包与信物) */}
      <div className="hud-block">
        <div className="hud-block-title">
          <span>携带物品与信物</span>
          <span>{liveInventory.length} 件</span>
        </div>

        <div className="item-slot-grid">
          {liveInventory.length === 0 ? (
            <div className="hud-empty-card hud-empty-card--full">
              <span className="empty-card-icon">🎒</span>
              <span className="empty-card-text">随身包裹空空如也</span>
            </div>
          ) : (
            liveInventory.map((item) => (
              <div
                key={item.id}
                className="item-slot"
                onClick={(e) => {
                  const rect = e.currentTarget.getBoundingClientRect();
                  showInvBubble(item.id, item, rect.left + rect.width / 2, rect.top);
                }}
                title="点击呼出信物动作"
              >
                <div className="slot-icon-quiet">{item.icon}</div>
                <div className="slot-name-label">{item.name}</div>
                <div className="slot-status-label">{item.status}</div>
              </div>
            ))
          )}
        </div>
      </div>

      {/* 5. 演义交接快照与前情回顾 (LiveAgent Context Compaction Handoff) */}
      <StoryCheckpointSection />

      {/* 6. Mounted Modals (二级页面弹窗) */}
      <DossierModal />
      <PromisesModal />
      <MemoryBankModal />
    </aside>
  );
};
