// MindGraphModal: 纯原生 HTML5 Canvas 物理引力排斥算法 (Force-Directed) 心智与实体星图
import React, { useEffect, useRef, useState, useCallback, useMemo } from "react";
import { useUi } from "../stores/uiStore";
import { useStory } from "../stores/storyStore";
import { characterPresentation } from "../lib/characterPresentation";
import { Modal } from "../ui/Modal";
import "./MindGraphModal.css";

/** 过滤胶囊：label 为可见文案，key 同时写入 activeFilter。 */
const FILTERS = [
  { key: "all", label: "全景星系" },
  { key: "entity", label: "🏷️ 仅实体" },
  { key: "episodic", label: "👁️ 亲历事实" },
  { key: "inference", label: "🧠 主观推测" },
  { key: "secret", label: "🔒 角色隐秘" },
] as const;

interface GraphNode {
  id: string;
  label: string;
  sub: string;
  type: "root" | "entity" | "episodic" | "inference" | "secret";
  icon?: string;
  radius: number;
  x: number;
  y: number;
  vx: number;
  vy: number;
  color: string;
  pinned?: boolean;
  raw?: unknown;
}

interface GraphLink {
  source: string;
  target: string;
  label: string;
  style: "solid" | "dashed" | "dotted";
}

export const MindGraphModal: React.FC = () => {
  const { mindGraphOpen, setMindGraphOpen, activeCharKey, theme } = useUi();
  const view = useStory((s) => s.view);
  const char = useMemo(() => characterPresentation(activeCharKey, view), [activeCharKey, view]);
  const backendMemories = useStory((s) => s.memories);

  const canvasRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  const [activeFilter, setActiveFilter] = useState<string>("all");
  const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);

  // 物理引力图数据
  const nodesRef = useRef<GraphNode[]>([]);
  const linksRef = useRef<GraphLink[]>([]);
  const animFrameRef = useRef<number | null>(null);

  // 视口平移与缩放
  const viewPosRef = useRef<{ x: number; y: number; scale: number }>({ x: 0, y: 0, scale: 1 });
  const isDraggingNodeRef = useRef<GraphNode | null>(null);
  const isPanningRef = useRef<boolean>(false);
  const panStartRef = useRef<{ x: number; y: number }>({ x: 0, y: 0 });

  // 切换会话或查看节点后清除旧检查器，避免显示另一条路径的记忆。
  useEffect(() => {
    setSelectedNode(null);
  }, [mindGraphOpen, view?.sessionId, view?.viewNodeId, view?.branch?.headNodeId]);

  // 初始化图谱数据
  const initGraphData = useCallback(
    (filter: string) => {
      const nodes: GraphNode[] = [];
      const links: GraphLink[] = [];

      // 1. Root Node (当前主角或当前会话)
      const rootNode: GraphNode = {
        id: view?.sessionId ? `session_${view.sessionId}` : `root_${char.key}`,
        label: view?.title || char.shortName,
        sub: view ? `当前分支: ${view.branch?.name || "main"}` : char.role,
        type: "root",
        radius: 36,
        x: 0,
        y: 0,
        vx: 0,
        vy: 0,
        color: theme === "dark" ? "#ffffff" : "#141414",
        pinned: true,
      };
      nodes.push(rootNode);

      // 2. Entity Nodes (优先使用后端真实世界状态角色，否则回退预设)
      const backendChars = view?.state?.characters
        ? Object.values(view.state.characters).filter((c) => c.characterId !== "player").slice(0, 50)
        : [];
      if (backendChars.length > 0) {
        const entRadius = 150;
        backendChars.forEach((ent, i) => {
          if (filter !== "all" && filter !== "entity") return;
          const angle = (i / backendChars.length) * Math.PI * 2;
          const rel = view?.state?.relationships?.[ent.characterId];
          const affStr = rel ? `好感 ${rel.affection > 0 ? "+" : ""}${rel.affection}` : "在场";
          const eNode: GraphNode = {
            id: ent.characterId,
            label: ent.name,
            sub: ent.description || "世界状态在场角色",
            type: "entity",
            icon: "👤",
            radius: 24,
            x: Math.cos(angle) * entRadius + (Math.random() - 0.5) * 20,
            y: Math.sin(angle) * entRadius + (Math.random() - 0.5) * 20,
            vx: 0,
            vy: 0,
            color: "var(--color-node-muted)",
            raw: ent,
          };
          nodes.push(eNode);
          links.push({
            source: rootNode.id,
            target: eNode.id,
            label: affStr,
            style: "solid",
          });
        });
      } else if (!view) {
        const entRadius = 150;
        char.entities.forEach((ent, i) => {
          if (filter !== "all" && filter !== "entity") return;
          const angle = (i / char.entities.length) * Math.PI * 2;
          const eNode: GraphNode = {
            id: ent.id,
            label: ent.label,
            sub: ent.desc,
            type: "entity",
            icon: ent.icon,
            radius: 24,
            x: Math.cos(angle) * entRadius + (Math.random() - 0.5) * 20,
            y: Math.sin(angle) * entRadius + (Math.random() - 0.5) * 20,
            vx: 0,
            vy: 0,
            color: "var(--color-node-muted)",
            raw: ent,
          };
          nodes.push(eNode);
          links.push({
            source: rootNode.id,
            target: eNode.id,
            label: ent.affinity,
            style: "solid",
          });
        });
      }

      // 3. Memory Nodes (优先使用后端真实记忆库，否则回退预设)
      const memRadius = 240;
      const memColors: Record<string, string> = {
        episodic: "#2e7d32",
        inference: "#1565c0",
        secret: "#c2185b",
      };

      if (backendMemories.length > 0) {
        const selectedMemories = backendMemories.filter(mem => mem.effective && !mem.hidden).filter(mem => {
          const category = mem.kind === "secret" ? "secret" : mem.kind === "inferred" ? "inference" : "episodic";
          return filter === "all" || filter === category;
        }).sort((a, b) => Number(b.pinned) - Number(a.pinned) || (b.importance ?? 0) - (a.importance ?? 0) || b.memoryId.localeCompare(a.memoryId)).slice(0, 100);
        selectedMemories.forEach((mem, i) => {
          const category = mem.kind === "secret" ? "secret" : mem.kind === "inferred" ? "inference" : "episodic";
          if (filter !== "all" && filter !== category) return;
          const angle = (i / selectedMemories.length) * Math.PI * 2 + 0.3;
          const label = mem.pinned
            ? "📌 置顶记忆"
            : mem.kind === "observed"
              ? "亲历记忆"
              : mem.kind === "secret" ? "角色隐秘" : mem.kind === "reported"
                ? "传闻"
                : "推断";
          const mNode: GraphNode = {
            id: mem.memoryId,
            label,
            sub: mem.content,
            type: category,
            radius: 18,
            x: Math.cos(angle) * memRadius + (Math.random() - 0.5) * 30,
            y: Math.sin(angle) * memRadius + (Math.random() - 0.5) * 30,
            vx: 0,
            vy: 0,
            color: memColors[category] || "#2e7d32",
            raw: mem,
          };
          nodes.push(mNode);

          let linked = false;
          if (mem.entityIds && mem.entityIds.length > 0) {
            mem.entityIds.forEach((eid) => {
              if (nodes.some((n) => n.id === eid)) {
                links.push({
                  source: eid,
                  target: mNode.id,
                  label: "关联",
                  style: "dashed",
                });
                linked = true;
              }
            });
          }
          if (!linked) {
            links.push({
              source: rootNode.id,
              target: mNode.id,
              label: "记忆",
              style: "dotted",
            });
          }
        });
      } else if (!view) {
        char.memories.forEach((mem, i) => {
          if (filter !== "all" && filter !== mem.category) return;
          const angle = (i / char.memories.length) * Math.PI * 2 + 0.3;
          const mNode: GraphNode = {
            id: mem.id,
            label: mem.categoryLabel,
            sub: mem.content,
            type: mem.category,
            radius: 18,
            x: Math.cos(angle) * memRadius + (Math.random() - 0.5) * 30,
            y: Math.sin(angle) * memRadius + (Math.random() - 0.5) * 30,
            vx: 0,
            vy: 0,
            color: memColors[mem.category],
            raw: mem,
          };
          nodes.push(mNode);

          let linked = false;
          char.entities.forEach((ent) => {
            if (mem.entities.some((eName) => ent.label.includes(eName) || eName.includes(ent.label))) {
              links.push({
                source: ent.id,
                target: mNode.id,
                label: mem.turnTag,
                style: "dashed",
              });
              linked = true;
            }
          });

          if (!linked) {
            links.push({
              source: rootNode.id,
              target: mNode.id,
              label: mem.turnTag,
              style: "dotted",
            });
          }
        });
      }

      // 4. Secret Nodes (从世界状态或会话中加载秘密)
      if (view?.secrets && view.secrets.length > 0) {
        view.secrets.slice(0, 50).forEach((sec, i) => {
          if (filter !== "all" && filter !== "secret") return;
          const angle = (i / view.secrets!.length) * Math.PI * 2 + 1.2;
          const sNode: GraphNode = {
            id: sec.secretId,
            label: sec.title || (sec.revealed ? "已揭示秘密" : "🔒 待解锁秘密"),
            sub: sec.revealed ? sec.content || "已解锁" : "剧情尚未达成揭示条件，内容尚未公开",
            type: "secret",
            radius: 20,
            x: Math.cos(angle) * 190 + (Math.random() - 0.5) * 20,
            y: Math.sin(angle) * 190 + (Math.random() - 0.5) * 20,
            vx: 0,
            vy: 0,
            color: "var(--color-danger-text)",
            raw: sec,
          };
          nodes.push(sNode);
          links.push({
            source: rootNode.id,
            target: sNode.id,
            label: sec.revealed ? "真相" : "隐秘",
            style: "dotted",
          });
        });
      }

      nodesRef.current = nodes;
      linksRef.current = links;
    },
    [char, theme, view, backendMemories]
  );

  useEffect(() => {
    if (mindGraphOpen) {
      initGraphData(activeFilter);
    }
  }, [mindGraphOpen, activeFilter, initGraphData]);

  // 物理模拟与绘制循环
  useEffect(() => {
    if (!mindGraphOpen) return;

    const canvas = canvasRef.current;
    const container = containerRef.current;
    if (!canvas || !container) return;

    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    let running = true;

    const resize = () => {
      const rect = container.getBoundingClientRect();
      const dpr = window.devicePixelRatio || 1;
      canvas.width = rect.width * dpr;
      canvas.height = rect.height * dpr;
      ctx.resetTransform();
      ctx.scale(dpr, dpr);
    };

    resize();
    window.addEventListener("resize", resize);

    const stepSimulation = () => {
      const nodes = nodesRef.current;
      const links = linksRef.current;

      // 1. 库仑斥力
      for (let i = 0; i < nodes.length; i++) {
        for (let j = i + 1; j < nodes.length; j++) {
          const a = nodes[i];
          const b = nodes[j];
          const dx = b.x - a.x;
          const dy = b.y - a.y;
          const distSq = dx * dx + dy * dy || 1;
          const dist = Math.sqrt(distSq);
          if (dist < 320) {
            const force = (320 - dist) / dist * 0.4;
            if (!a.pinned && a !== isDraggingNodeRef.current) {
              a.vx -= dx * force * 0.05;
              a.vy -= dy * force * 0.05;
            }
            if (!b.pinned && b !== isDraggingNodeRef.current) {
              b.vx += dx * force * 0.05;
              b.vy += dy * force * 0.05;
            }
          }
        }
      }

      // 2. 胡克弹簧引力
      links.forEach((l) => {
        const a = nodes.find((n) => n.id === l.source);
        const b = nodes.find((n) => n.id === l.target);
        if (!a || !b) return;
        const dx = b.x - a.x;
        const dy = b.y - a.y;
        const dist = Math.sqrt(dx * dx + dy * dy) || 1;
        const targetDist = 130;
        const force = (dist - targetDist) * 0.02;
        if (!a.pinned && a !== isDraggingNodeRef.current) {
          a.vx += (dx / dist) * force;
          a.vy += (dy / dist) * force;
        }
        if (!b.pinned && b !== isDraggingNodeRef.current) {
          b.vx -= (dx / dist) * force;
          b.vy -= (dy / dist) * force;
        }
      });

      // 3. 向心微引力与阻尼衰减
      nodes.forEach((n) => {
        if (n.pinned || n === isDraggingNodeRef.current) return;
        n.vx -= n.x * 0.002;
        n.vy -= n.y * 0.002;
        n.vx *= 0.85;
        n.vy *= 0.85;
        n.x += n.vx;
        n.y += n.vy;
      });
    };

    const render = () => {
      if (!running) return;
      stepSimulation();

      const rect = container.getBoundingClientRect();
      const w = rect.width;
      const h = rect.height;
      const isDark = theme === "dark";

      ctx.clearRect(0, 0, w, h);

      // 背景网格装饰微点
      ctx.save();
      ctx.fillStyle = isDark ? "rgba(255, 255, 255, 0.03)" : "rgba(0, 0, 0, 0.04)";
      for (let gx = 0; gx < w; gx += 40) {
        for (let gy = 0; gy < h; gy += 40) {
          ctx.beginPath();
          ctx.arc(gx, gy, 1, 0, Math.PI * 2);
          ctx.fill();
        }
      }
      ctx.restore();

      ctx.save();
      const view = viewPosRef.current;
      ctx.translate(w / 2 + view.x, h / 2 + view.y);
      ctx.scale(view.scale, view.scale);

      const nodes = nodesRef.current;
      const links = linksRef.current;

      // 绘制连线
      links.forEach((l) => {
        const a = nodes.find((n) => n.id === l.source);
        const b = nodes.find((n) => n.id === l.target);
        if (!a || !b) return;

        ctx.beginPath();
        ctx.moveTo(a.x, a.y);
        ctx.lineTo(b.x, b.y);

        if (l.style === "dashed") ctx.setLineDash([4, 4]);
        else if (l.style === "dotted") ctx.setLineDash([2, 3]);
        else ctx.setLineDash([]);

        ctx.strokeStyle = isDark ? "rgba(255, 255, 255, 0.2)" : "rgba(0, 0, 0, 0.15)";
        ctx.lineWidth = 1;
        ctx.stroke();
        ctx.setLineDash([]);

        // 连线文字
        const mx = (a.x + b.x) / 2;
        const my = (a.y + b.y) / 2;
        ctx.font = "10px sans-serif";
        ctx.fillStyle = isDark ? "#8c8e96" : "#7c776e";
        ctx.textAlign = "center";
        ctx.textBaseline = "middle";
        ctx.fillText(l.label, mx, my);
      });

      // 绘制节点
      nodes.forEach((n) => {
        ctx.save();
        ctx.beginPath();
        ctx.arc(n.x, n.y, n.radius, 0, Math.PI * 2);

        // 外发光/呼吸环
        if (n.type === "root") {
          ctx.shadowColor = isDark ? "rgba(255,255,255,0.4)" : "rgba(0,0,0,0.2)";
          ctx.shadowBlur = 12;
        }

        ctx.fillStyle = isDark ? "#212228" : "#ffffff";
        ctx.fill();
        ctx.lineWidth = n.type === "root" ? 3 : selectedNode?.id === n.id ? 2.5 : 1.5;
        ctx.strokeStyle = selectedNode?.id === n.id ? (isDark ? "#ffffff" : "#141414") : n.color;
        ctx.stroke();

        // 节点图标或文字
        ctx.font = n.type === "root" ? "bold 13px 'Noto Serif SC', serif" : "11px sans-serif";
        ctx.fillStyle = isDark ? "#ffffff" : "#141414";
        ctx.textAlign = "center";
        ctx.textBaseline = "middle";

        if (n.icon) {
          ctx.fillText(n.icon, n.x, n.y - 2);
        } else {
          ctx.fillText(n.label.slice(0, 5), n.x, n.y);
        }

        // 节点外悬浮标签
        if (n.type !== "root") {
          ctx.font = "10px sans-serif";
          ctx.fillStyle = isDark ? "#a6a8b1" : "#5e5b56";
          ctx.fillText(n.label, n.x, n.y + n.radius + 12);
        }

        ctx.restore();
      });

      ctx.restore();

      animFrameRef.current = requestAnimationFrame(render);
    };

    animFrameRef.current = requestAnimationFrame(render);

    return () => {
      running = false;
      if (animFrameRef.current) cancelAnimationFrame(animFrameRef.current);
      window.removeEventListener("resize", resize);
    };
  }, [mindGraphOpen, theme, selectedNode]);

  // 鼠标交互 (缩放、平移与拖拽)
  const handleMouseDown = (e: React.MouseEvent<HTMLDivElement>) => {
    const container = containerRef.current;
    if (!container) return;
    const rect = container.getBoundingClientRect();
    const cx = e.clientX - rect.left - rect.width / 2 - viewPosRef.current.x;
    const cy = e.clientY - rect.top - rect.height / 2 - viewPosRef.current.y;
    const worldX = cx / viewPosRef.current.scale;
    const worldY = cy / viewPosRef.current.scale;

    // 检测点击的节点
    const hit = nodesRef.current.find((n) => {
      const dx = n.x - worldX;
      const dy = n.y - worldY;
      return Math.sqrt(dx * dx + dy * dy) <= n.radius + 4;
    });

    if (hit) {
      isDraggingNodeRef.current = hit;
      setSelectedNode(hit);
    } else {
      isPanningRef.current = true;
      panStartRef.current = { x: e.clientX - viewPosRef.current.x, y: e.clientY - viewPosRef.current.y };
      setSelectedNode(null);
    }
  };

  const handleMouseMove = (e: React.MouseEvent<HTMLDivElement>) => {
    if (isDraggingNodeRef.current) {
      const container = containerRef.current;
      if (!container) return;
      const rect = container.getBoundingClientRect();
      const cx = e.clientX - rect.left - rect.width / 2 - viewPosRef.current.x;
      const cy = e.clientY - rect.top - rect.height / 2 - viewPosRef.current.y;
      isDraggingNodeRef.current.x = cx / viewPosRef.current.scale;
      isDraggingNodeRef.current.y = cy / viewPosRef.current.scale;
      isDraggingNodeRef.current.vx = 0;
      isDraggingNodeRef.current.vy = 0;
    } else if (isPanningRef.current) {
      viewPosRef.current.x = e.clientX - panStartRef.current.x;
      viewPosRef.current.y = e.clientY - panStartRef.current.y;
    }
  };

  const handleMouseUp = () => {
    isDraggingNodeRef.current = null;
    isPanningRef.current = false;
  };

  const handleWheel = (e: React.WheelEvent<HTMLDivElement>) => {
    e.preventDefault();
    const factor = e.deltaY < 0 ? 1.1 : 0.9;
    const newScale = Math.max(0.4, Math.min(2.5, viewPosRef.current.scale * factor));
    viewPosRef.current.scale = newScale;
  };

  if (!mindGraphOpen) return null;

  return (
    <Modal
      open={mindGraphOpen}
      onClose={() => setMindGraphOpen(false)}
      label="心智与实体星图"
      title="🕸️ 心智与实体引力星图 (Cognitive Galaxy)"
      description={`${char.name} · 实体拓扑与记忆微粒网络`}
      size="full"
      flush
      closeLabel="关闭心智星图"
      id="mind-graph-modal"
      dialogClassName="mind-graph-fullscreen"
      headerActions={
        <>
          {/* 分类过滤胶囊 */}
          <div className="mind-graph-filters">
            {FILTERS.map(({ key, label }) => (
              <button
                key={key}
                className={`memory-filter-btn ${activeFilter === key ? "active" : ""}`}
                onClick={() => setActiveFilter(key)}
              >
                {label}
              </button>
            ))}
          </div>

          <button
            className="btn-quiet"
            onClick={() => {
              viewPosRef.current = { x: 0, y: 0, scale: 1.0 };
            }}
          >
            ⌖ 视口居中
          </button>
        </>
      }
    >
      {view && <p className="mind-graph-limit-note">概览优先展示置顶和重要记忆，最多 100 条记忆、50 个实体和 50 项秘密；完整记录可在侧栏筛选查看。</p>}
      {/* 主画布容器 */}
      <div
        ref={containerRef}
        className="mind-graph-canvas-container"
        onMouseDown={handleMouseDown}
        onMouseMove={handleMouseMove}
        onMouseUp={handleMouseUp}
        onWheel={handleWheel}
        onTouchStart={(e) => {
          if (e.touches.length === 1) {
            const t = e.touches[0];
            handleMouseDown({ clientX: t.clientX, clientY: t.clientY } as any);
          }
        }}
        onTouchMove={(e) => {
          if (e.touches.length === 1) {
            const t = e.touches[0];
            handleMouseMove({ clientX: t.clientX, clientY: t.clientY } as any);
          }
        }}
        onTouchEnd={handleMouseUp}
      >
        <canvas ref={canvasRef} />

        {/* 交互提示气泡 */}
        <div className="mind-graph-hint">
          滚轮无级缩放 (0.4x ~ 2.5x) • 鼠标按住空白平移 • 拖拽节点弹性位移
        </div>

        {/* 右侧认知检查器抽屉 (Inspector Drawer) */}
        {selectedNode && (
          <div className="mind-graph-inspector-drawer">
            <div className="mind-graph-inspector-head">
              <span className="mind-graph-inspector-title">认知微粒检查器</span>
              <button className="probe-close-btn" onClick={() => setSelectedNode(null)}>
                ✕
              </button>
            </div>

            <div>
              <span
                className="mind-graph-node-badge"
                style={{ "--mg-node-color": selectedNode.color } as React.CSSProperties}
              >
                {selectedNode.type}
              </span>
              <h3 className="mind-graph-node-title">{selectedNode.label}</h3>
            </div>

            <div className="mind-graph-node-sub">{selectedNode.sub}</div>

            <div className="mind-graph-node-links">
              关联节点：{linksRef.current.filter((l) => l.source === selectedNode.id || l.target === selectedNode.id).length} 条连线
            </div>
          </div>
        )}
      </div>
    </Modal>
  );
};
