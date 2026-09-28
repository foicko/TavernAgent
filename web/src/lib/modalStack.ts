// 统一模态栈（唯一实现）：App 里所有浮层（Modal / 命令面板 / 导演工作区 / 灯箱）
// 都注册到同一栈。
//
// 不变式：
//   • 栈非空时应用根 `#root` 被 `inert`（指针与辅助技术都到不了背景）；栈空后恢复。
//   • 只有**栈顶**可交互：更下层图层被 `inert`（叠层时下层不再可点/可读）。
//   • 栈非空期间锁定 body 滚动（只保存/恢复一次）。
//   • Esc 统一由 `closeTopLayer()` 关闭栈顶，保证"只关最上面那一个"。
//
// 放在 `lib` 而非 `ui`：`ui/` 只允许依赖 `react` 与 `../lib`（见 web/scripts/check.mjs），
// 因此栈与焦点边界必须是 lib 层能力，`ui/Modal` 再复用它。

export interface ModalLayer {
  /** 该层进出栈顶时调用：`inert=true` 表示它不再是栈顶，应停止响应指针/辅助技术。 */
  setInert: (inert: boolean) => void;
  /** 请求关闭（Esc / 遮罩）。阻塞式弹窗不提供 → `closeTopLayer()` 返回 false。 */
  onClose?: () => void;
}

interface LayerEntry extends ModalLayer {
  id: number;
}

let nextLayerId = 1;
const layers: LayerEntry[] = [];
const listeners = new Set<() => void>();
let savedBodyOverflow = "";

function rootElement(): HTMLElement | null {
  return typeof document === "undefined" ? null : document.getElementById("root");
}

/** 让只有栈顶可交互：其余图层与（栈非空时的）应用根都置为 inert。 */
function syncInertness(): void {
  layers.forEach((layer, index) => {
    layer.setInert(index !== layers.length - 1);
  });
  const root = rootElement();
  if (!root) return;
  if (layers.length > 0) {
    root.setAttribute("inert", "");
    root.setAttribute("aria-hidden", "true");
  } else {
    root.removeAttribute("inert");
    root.removeAttribute("aria-hidden");
  }
}

function notify(): void {
  listeners.forEach((listener) => listener());
}

/** 注册一个图层，返回其 id（卸载时用 `popLayer(id)` 移除）。 */
export function pushLayer(layer: ModalLayer): number {
  const id = nextLayerId++;
  if (layers.length === 0 && typeof document !== "undefined") {
    savedBodyOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
  }
  layers.push({ id, ...layer });
  syncInertness();
  notify();
  return id;
}

export function popLayer(id: number): void {
  const index = layers.findIndex((entry) => entry.id === id);
  if (index < 0) return;
  layers.splice(index, 1);
  if (layers.length === 0 && typeof document !== "undefined") {
    document.body.style.overflow = savedBodyOverflow;
  }
  syncInertness();
  notify();
}

/** 关闭栈顶图层；栈为空或栈顶不可关闭时返回 false（交由上层决定怎么处理）。 */
export function closeTopLayer(): boolean {
  const top = layers[layers.length - 1];
  if (!top || !top.onClose) return false;
  top.onClose();
  return true;
}

export function isLayerOpen(): boolean {
  return layers.length > 0;
}

export function layerCount(): number {
  return layers.length;
}

export function subscribeLayers(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
