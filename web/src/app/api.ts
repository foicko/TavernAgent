// API 客户端：REST + fetch 流式 SSE（技术契约 §11）。
import type {
	DirectorAction, DirectorDraft, DirectorPlan, DirectorRequest, DirectorState, DirectorView,
  AcceptResult,
  AssistantEditResult,
  BranchView,
  CardLibrarySummary,
  CardPreview,
  DeriveResult,
  ImportResult,
  MemoryView,
  MemoryUsage,
  ModelCatalog,
  ModelInstance,
  PlotNode,
  ProbeResult,
  ProviderConfig,
  SessionSummary,
  SessionView,
  SSEEvent,
  TextBlock,
  TurnView,
  WorldState,
  GraphView,
  LorebookPage,
  TTSVoice,
} from "./types";

const BASE = ""; // Vite 代理 /api → 后端

const AUTH_STORAGE_KEY = "tavernagent_auth_token";

export function getStoredAuthToken(): string {
  try {
    return localStorage.getItem(AUTH_STORAGE_KEY) || "";
  } catch {
    return "";
  }
}

export function setStoredAuthToken(token: string) {
  try {
    if (token) {
      localStorage.setItem(AUTH_STORAGE_KEY, token);
    } else {
      localStorage.removeItem(AUTH_STORAGE_KEY);
    }
  } catch {
    // ignore
  }
}

type AuthRequiredListener = () => void;
const authListeners: Set<AuthRequiredListener> = new Set();

export function onAuthRequired(fn: AuthRequiredListener): () => void {
  authListeners.add(fn);
  return () => authListeners.delete(fn);
}

function notifyAuthRequired() {
  authListeners.forEach((fn) => {
    try {
      fn();
    } catch {
      // ignore
    }
  });
}

// The same authentication and deadline also apply to uploads and downloads.
async function binaryRequest(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers);
  const token = getStoredAuthToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  try {
    const res = await fetch(BASE + path, { ...init, headers, signal: AbortSignal.timeout(30_000) });
    if (!res.ok) {
      if (res.status === 401) notifyAuthRequired();
      throw new ApiError(res.status, await res.text());
    }
    return res;
  } catch (error) {
    if (error instanceof DOMException && ["TimeoutError", "AbortError"].includes(error.name)) {
      throw new ApiError(0, "请求超时，请检查网络或稍后重试");
    }
    throw error;
  }
}

async function http<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {};
  if (body) {
    headers["Content-Type"] = "application/json";
  }
  const token = getStoredAuthToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  let res: Response;
  try {
    res = await fetch(BASE + path, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
      signal: AbortSignal.timeout(30_000),
    });
  } catch (e) {
    // AbortSignal.timeout 在超时时抛 TimeoutError（Safari 旧版为 AbortError）
    if (e instanceof DOMException && (e.name === "TimeoutError" || e.name === "AbortError")) {
      throw new ApiError(0, "请求超时，请检查网络或稍后重试");
    }
    throw e;
  }
  const text = await res.text();
  if (!res.ok) {
    if (res.status === 401) {
      try {
        const errJson = JSON.parse(text);
        if (errJson.code === "AUTH_REQUIRED") {
          notifyAuthRequired();
        }
      } catch {
        // ignore
      }
    }
    throw new ApiError(res.status, text);
  }
  return text ? (JSON.parse(text) as T) : (undefined as T);
}

export class ApiError extends Error {
  constructor(
    public status: number,
    public body: string,
  ) {
    let message = body;
    try {
      const payload = JSON.parse(body);
      if (typeof payload?.message === "string") message = payload.message;
    } catch { /* Plain-text errors retain their original message. */ }
    super(message);
  }
}

export const api = {
  getDirector: (sessionId: string, branchId: string, viewNodeId?: string) =>
    http<DirectorView>("GET", `/api/v1/sessions/${sessionId}/branches/${branchId}/director${viewNodeId ? `?viewNodeId=${encodeURIComponent(viewNodeId)}` : ""}`),
  saveDirectorDraft: (sessionId: string, branchId: string, body: { expectedCharacterId: string; expectedDraftVersion: number; baseRevisionId: string; plan: DirectorPlan; viewNodeId?: string }) =>
    http<DirectorDraft>("PUT", `/api/v1/sessions/${sessionId}/branches/${branchId}/director/draft`, body),
  directorCommand: (sessionId: string, branchId: string, body: { expectedCharacterId: string; expectedHeadId: string; expectedVersion: number; idempotencyKey: string; action: DirectorAction; beatId?: string; draftVersion?: number; replace?: boolean }) =>
    http<{ nodeId: string; version: number; state: DirectorState }>("POST", `/api/v1/sessions/${sessionId}/branches/${branchId}/director/commands`, body),
  directorMessage: (sessionId: string, branchId: string, body: { expectedCharacterId: string; expectedHeadId: string; expectedDraftVersion: number; idempotencyKey: string; text: string }) =>
    http<{ requestId: string; status: DirectorRequest["status"]; request: DirectorRequest; statusUrl: string; eventsUrl: string }>("POST", `/api/v1/sessions/${sessionId}/branches/${branchId}/director/messages`, body),
  getDirectorRequest: (requestId: string) => http<DirectorRequest>("GET", `/api/v1/director-requests/${requestId}`),
  cancelDirectorRequest: (requestId: string, expectedCharacterId: string) =>
    http<DirectorRequest>("POST", `/api/v1/director-requests/${requestId}/cancel`, { expectedCharacterId }),
  authStatus: () =>
    http<{ authenticated: boolean; required: boolean }>("GET", "/api/v1/auth/status"),
  pair: (pin: string) =>
    http<{ ok: boolean; token: string }>("POST", "/api/v1/auth/pair", { pin }),
  // 桌面壳桥（浏览器部署下 desktop=false / theme 返回 404）：
  // 窗口跑在 loopback 源上，拿不到 Wails 注入的 window.runtime，
  // 原生窗口配色只能由前端通过同源接口回传。
  desktopInfo: () => http<{ desktop: boolean }>("GET", "/api/v1/desktop"),
  setDesktopTheme: (mode: "light" | "dark") =>
    http<void>("POST", "/api/v1/desktop/theme", { mode }),
  listSessions: () => http<{ sessions: SessionSummary[] }>("GET", "/api/v1/sessions"),
  // 永久删除一个故事及其全部数据。不可撤销，调用方必须先与用户确认；
  // 仍有进行中的回合时服务端返回 409 SESSION_BUSY。
  deleteSession: (id: string) => http<{ ok: boolean }>("DELETE", `/api/v1/sessions/${id}`),
  // branchId 决定写入位置，viewNodeId 决定查看位置；二者分离（技术契约 §7），
  // 浏览历史不推进任何分支指针。
  // limit/before 用于窗口化读取：服务端默认只返回最近若干轮，
  // 更早的历史用 before 游标向上翻页（万级节点的会话不能一次全给前端）。
  getSession: (
    id: string,
    opts?: { branchId?: string; viewNodeId?: string; limit?: number; before?: string },
  ) => {
    const q = new URLSearchParams();
    if (opts?.branchId) q.set("branchId", opts.branchId);
    if (opts?.viewNodeId) q.set("viewNodeId", opts.viewNodeId);
    if (opts?.limit) q.set("limit", String(opts.limit));
    if (opts?.before) q.set("before", opts.before);
    const qs = q.toString();
    return http<SessionView>("GET", `/api/v1/sessions/${id}${qs ? `?${qs}` : ""}`);
  },
  // 分支图谱：只取目标节点周围的一圈（M4e），万级会话不能整树下发。
  getGraph: (sessionId: string, opts?: { nodeId?: string; up?: number; down?: number }) => {
    const q = new URLSearchParams();
    if (opts?.nodeId) q.set("nodeId", opts.nodeId);
    if (opts?.up != null) q.set("up", String(opts.up));
    if (opts?.down != null) q.set("down", String(opts.down));
    const qs = q.toString();
    return http<GraphView>("GET", `/api/v1/sessions/${sessionId}/graph${qs ? `?${qs}` : ""}`);
  },

  searchLorebook: (sessionId: string, text: string, offset = 0) =>
    http<LorebookPage>("GET", `/api/v1/sessions/${sessionId}/lorebook?${new URLSearchParams({ q: text, offset: String(offset), limit: "40" })}`),

  getNode: (nodeId: string) =>
    http<{ node: PlotNode; state: WorldState | null; candidates: PlotNode[] }>(
      "GET",
      `/api/v1/nodes/${nodeId}`,
    ),
  createSession: (req: {
    idempotencyKey?: string;
    title?: string;
    cardId?: string;
    characterJson?: string;
    playerName: string;
    playerRole?: string;
    playerBackpack?: unknown[];
    openingVariantId?: string;
    openingText?: string;
  }) =>
    http<{
      sessionId: string;
      branchId: string;
      rootNodeId: string;
      branchVersion: number;
      openingText: string;
      state: WorldStateDto;
    }>("POST", "/api/v1/sessions", req),
  acceptTurn: (
    sessionId: string,
    branchId: string,
    req: {
      idempotencyKey: string;
      expectedHeadId: string;
      expectedVersion: number;
      expectedCharacterId: string;
      input: { kind: string; text: string; note?: string; options?: string; optionRef?: unknown };
    },
  ) =>
    http<AcceptResult>(
      "POST",
      `/api/v1/sessions/${sessionId}/branches/${branchId}/turns`,
      req,
    ),
  getTurn: (turnId: string) => http<TurnView>("GET", `/api/v1/turns/${turnId}`),
  cancelTurn: (turnId: string, expectedCharacterId?: string) =>
    http<{ turnId: string; status: string }>("POST", `/api/v1/turns/${turnId}/cancel`, { expectedCharacterId }),
  continueTurn: (turnId: string, expectedCharacterId?: string) =>
    http<{ turnId: string; status: string; eventsUrl: string }>(
      "POST",
      `/api/v1/turns/${turnId}/continue`,
      { expectedCharacterId },
    ),
  // ---- M1-5：模型配置 ----
  getProviderConfigs: () =>
    http<{ providers: ProviderConfig[] }>("GET", "/api/v1/config/provider"),
  listModels: () => http<ModelCatalog>("GET", "/api/v1/config/models"),
  saveModel: (instance: ModelInstance) =>
    http<ModelInstance>(instance.id ? "PUT" : "POST",
      instance.id ? `/api/v1/config/models/${encodeURIComponent(instance.id)}` : "/api/v1/config/models", instance),
  deleteModel: (id: string) => http<{ ok: boolean }>("DELETE", `/api/v1/config/models/${encodeURIComponent(id)}`),
  assignSlot: (slot: string, enabled: boolean, modelId: string) =>
    http<{ ok: boolean; providers: ProviderConfig[] }>("PUT", "/api/v1/config/provider", { slot, enabled, modelId }),
  probeProvider: (slot: string, format = false) =>
    http<ProbeResult>("POST", "/api/v1/config/provider/probe", { slot, format }),
  probeModel: (modelId: string, format = false) =>
    http<ProbeResult>("POST", `/api/v1/config/models/${encodeURIComponent(modelId)}/probe`, { format }),
  // ---- M2/M3：分支派生（分叉 / 重生成 / 编辑）----
  // fork 只建分支不生成内容；regenerations 与 edits(input) 都会新建候选分支，
  // edits(blocks) 是同步提交的纯叙事候选（零状态变化）。
  forkBranch: (sessionId: string, req: { fromNodeId: string; name?: string; expectedCharacterId: string }) =>
    http<{ branch: BranchView }>("POST", `/api/v1/sessions/${sessionId}/branches`, req),
  regenerateTurn: (
    sessionId: string,
    branchId: string,
    req: { nodeId: string; idempotencyKey?: string; name?: string; expectedCharacterId: string; recheck?: boolean; note?: string; options?: string },
  ) =>
    http<DeriveResult>(
      "POST",
      `/api/v1/sessions/${sessionId}/branches/${branchId}/regenerations`,
      req,
    ),
  editTurn: (
    sessionId: string,
    branchId: string,
    req: {
      nodeId: string;
      expectedCharacterId: string;
      input?: { kind: string; text: string };
      blocks?: TextBlock[];
      idempotencyKey?: string;
    },
  ) =>
    http<DeriveResult | AssistantEditResult>(
      "POST",
      `/api/v1/sessions/${sessionId}/branches/${branchId}/edits`,
      req,
    ),
  // ---- M3：剧情包（导出/导入）----
  // 剧情包是二进制，不复用 JSON 客户端：导出要拿 Blob，导入要发原始字节。
  exportSessionPack: async (sessionId: string, branchId?: string): Promise<Blob> => {
    const qs = branchId ? `?branchId=${encodeURIComponent(branchId)}` : "";
    const res = await binaryRequest(`/api/v1/sessions/${sessionId}/export${qs}`);
    if (!res.ok) {
      throw new ApiError(res.status, await res.text());
    }
    return res.blob();
  },
  importSessionPack: async (data: ArrayBuffer): Promise<ImportResult> => {
    const res = await binaryRequest("/api/v1/sessions/import", {
      method: "POST",
      headers: { "Content-Type": "application/zip" },
      body: data,
    });
    const text = await res.text();
    if (!res.ok) {
      throw new ApiError(res.status, text);
    }
    return JSON.parse(text) as ImportResult;
  },
  // 服务端卡库：导入即入库。返回的 CardPreview 带 cardId，可直接用它建会话。
  importCardFile: async (file: File): Promise<CardPreview> => {
    const form = new FormData();
    form.append("file", file);
    const res = await binaryRequest("/api/v1/cards", {
      method: "POST",
      body: form,
    });
    const text = await res.text();
    if (!res.ok) {
      throw new ApiError(res.status, text);
    }
    return JSON.parse(text) as CardPreview;
  },
  // 卡库摘要列表：只含元数据，不回传完整 characterJson。
  listCards: () => http<{ cards: CardLibrarySummary[] }>("GET", "/api/v1/cards"),
  // 取单张卡的完整预览（服务端重新解析存储的归一化 JSON，与导入同形状）。
  getCard: (cardId: string) => http<CardPreview>("GET", `/api/v1/cards/${encodeURIComponent(cardId)}`),
  deleteCard: (cardId: string) => http<{ ok: boolean }>("DELETE", `/api/v1/cards/${encodeURIComponent(cardId)}`),
  // ---- M3：记忆管理 ----
  // 带 branchId 时后端会按该分支解析覆盖关系并标记 effective。
  listMemories: (sessionId: string, branchId?: string, nodeId?: string, query?: { search?: string; kind?: string; cursor?: string; limit?: number }) =>
    http<{ memories: MemoryView[]; usage?: MemoryUsage; nodeId?: string; nextCursor?: string; total?: number; counts?: Record<string, number> }>(
      "GET",
      `/api/v1/sessions/${sessionId}/memories?${new URLSearchParams({ ...(branchId ? { branchId } : {}), ...(nodeId ? { nodeId } : {}), ...(query?.search ? { search: query.search } : {}), ...(query?.kind ? { kind: query.kind } : {}), ...(query?.cursor ? { cursor: query.cursor } : {}), limit: String(query?.limit ?? 40) })}`,
    ),
  // 修订是 copy-on-write：后端新建一条覆盖记录，不改动原记录（因此不影响其它分支）。
  patchMemory: (
    sessionId: string,
    branchId: string,
    memoryId: string,
    patch: { content?: string; pinned?: boolean; hidden?: boolean; expectedCharacterId: string; expectedHeadId: string; expectedVersion: number; idempotencyKey?: string },
  ) =>
    http<MemoryView>(
      "PATCH",
      `/api/v1/sessions/${sessionId}/branches/${branchId}/memories/${memoryId}`,
      patch,
    ),
  organizeMemories: (sessionId: string, branchId: string, expectedCharacterId: string) =>
    http<{ merged?: unknown[] }>("POST", `/api/v1/sessions/${sessionId}/branches/${branchId}/memories/organize`, { expectedCharacterId }),
};

type WorldStateDto = {
  relationships?: Record<string, { affection: number; trust: number; alertness: number }>;
  items?: Record<string, { instanceId: string; name: string; quantity: number; keepsake?: boolean }>;
  characters?: Record<string, { characterId: string; name: string }>;
};

export type TurnEventType =
  | "turn.started"
  | "turn.thinking"
  | "block.delta"
  | "block.appended"
  | "turn.truncated"
  | "turn.awaiting_continuation"
  | "turn.awaiting_approval"
  | "turn.committed"
  | "turn.cancelled"
  | "turn.failed"
  | "turn.conflicted";

export interface TurnStreamHandlers {
  lastEventId?: string;
  onEvent: (ev: SSEEvent) => void;
  // 重连耗尽或不可恢复错误（如回合不存在）时回调；store 层负责终态兜底。
  onFinalError: (err: Error) => void;
  // 可选：每次重连成功时回调（用于 UI 提示"连接已恢复"）。
  onReconnect?: (attempt: number) => void;
}

// SSE 重连参数：退避 1s/2s/4s/8s/16s，上限 5 次，防止重连风暴。
const SSE_BACKOFF_MS = [1_000, 2_000, 4_000, 8_000, 16_000];
// 心跳看门狗：后端每 15s 发一次 `: ping`，3 周期无任何字节视为半开连接。
const SSE_STALL_MS = 45_000;

// subscribeTurnEvents 使用 fetch + ReadableStream 解析 SSE，断线后带
// Last-Event-ID 指数退避重连（后端 Poll 重放 + Subscribe 实时，见 server.go turnEvents）。
// 事件 id 格式为 `turnId:sequence`（outbox 序号单调递增），按 maxSeq 去重，
// 重放事件与实时流共用同一过滤，保证幂等。
export function subscribeTurnEvents(turnId: string, handlers: TurnStreamHandlers, signal?: AbortSignal): () => void {
  return subscribeEvents(turnId, `/api/v1/turns/${turnId}/events`, handlers, signal);
}

export function subscribeSessionEvents(sessionId: string, handlers: TurnStreamHandlers, signal?: AbortSignal): () => void {
  return subscribeEvents(sessionId, `/api/v1/sessions/${sessionId}/events`, handlers, signal);
}

export function subscribeDirectorEvents(requestId: string, handlers: TurnStreamHandlers, signal?: AbortSignal): () => void {
  return subscribeEvents(requestId, `/api/v1/director-requests/${requestId}/events`, handlers, signal);
}

function subscribeEvents(aggregateId: string, path: string, handlers: TurnStreamHandlers, signal?: AbortSignal): () => void {
  const lifetime = new AbortController();
  const stop = () => lifetime.abort();
  if (signal?.aborted) stop();
  signal?.addEventListener("abort", stop, { once: true });
  const terminal = new Set(["turn.committed", "turn.cancelled", "turn.failed", "turn.conflicted", "director.completed", "director.failed", "director.cancelled", "director.interrupted"]);
  void (async () => {
    let lastEventId = handlers.lastEventId?.startsWith(aggregateId + ":") ? handlers.lastEventId : "";
    let maxSeq = Number(lastEventId.split(":").at(-1)) || 0;
    let retries = 0;
    try {
      while (!lifetime.signal.aborted) {
        const connection = new AbortController();
        const abortConnection = () => connection.abort();
        lifetime.signal.addEventListener("abort", abortConnection, { once: true });
        let reader: ReadableStreamDefaultReader<Uint8Array> | undefined;
        let timer: ReturnType<typeof setTimeout> | undefined;
        try {
          const token = getStoredAuthToken();
          const headers: Record<string, string> = { Accept: "text/event-stream" };
          if (token) headers.Authorization = `Bearer ${token}`;
          if (lastEventId) headers["Last-Event-ID"] = lastEventId;
          timer = setTimeout(abortConnection, SSE_STALL_MS);
          const res = await fetch(BASE + path, { signal: connection.signal, headers });
          clearTimeout(timer);
          if (!res.ok || !res.body) {
            if (res.status >= 400 && res.status < 500 && res.status !== 429) {
              if (res.status === 401) notifyAuthRequired();
              handlers.onFinalError(new ApiError(res.status, "SSE connect failed"));
              return;
            }
            throw new Error("SSE temporarily unavailable");
          }
          if (retries > 0) handlers.onReconnect?.(retries);
          reader = res.body.getReader();
          const decoder = new TextDecoder();
          let buf = "";
          for (;;) {
            const chunk = await readWithStall(reader, connection.signal, SSE_STALL_MS);
            if (chunk.done) break;
            buf += decoder.decode(chunk.value, { stream: true });
            if (buf.length > 1024 * 1024) throw new Error("SSE event too large");
            let sep: RegExpExecArray | null;
            while ((sep = /\r?\n\r?\n/.exec(buf))) {
              const raw = buf.slice(0, sep.index);
              buf = buf.slice(sep.index + sep[0].length);
              const evt = parseSSEBlock(raw, lastEventId);
              if (!evt) continue;
              // Unnumbered deltas/thinking inherit last-event-id per SSE, but
              // must not be treated as duplicate durable business events.
              if (/^id:/m.test(raw)) {
                if (!evt.id.startsWith(aggregateId + ":")) continue;
                const seq = Number(evt.id.slice(aggregateId.length + 1));
                if (!Number.isSafeInteger(seq) || seq <= maxSeq) continue;
                maxSeq = seq;
                lastEventId = evt.id;
              }
              if (lifetime.signal.aborted) return;
              handlers.onEvent(evt);
              if (terminal.has(evt.event)) return;
            }
          }
        } catch {
          if (lifetime.signal.aborted) return;
        } finally {
          clearTimeout(timer);
          connection.abort();
          if (reader) { void reader.cancel().catch(() => undefined); reader.releaseLock(); }
          lifetime.signal.removeEventListener("abort", abortConnection);
        }
        if (lifetime.signal.aborted) return;
        if (retries >= SSE_BACKOFF_MS.length) {
          handlers.onFinalError(new Error("SSE reconnect exhausted"));
          return;
        }
        if (!(await sleepCancellable(SSE_BACKOFF_MS[retries++], lifetime.signal))) return;
      }
    } finally {
      signal?.removeEventListener("abort", stop);
    }
  })().catch(() => undefined);
  return stop;
}

function readWithStall(reader: ReadableStreamDefaultReader<Uint8Array>, signal: AbortSignal, ms: number): Promise<ReadableStreamReadResult<Uint8Array>> {
  return new Promise((resolve, reject) => {
    const cleanup = () => { clearTimeout(timer); signal.removeEventListener("abort", onAbort); };
    const onAbort = () => { cleanup(); reject(new Error("SSE aborted")); };
    const timer = setTimeout(() => { cleanup(); reject(new Error("SSE stalled")); }, ms);
    if (signal.aborted) { onAbort(); return; }
    signal.addEventListener("abort", onAbort, { once: true });
    reader.read().then(chunk => { cleanup(); resolve(chunk); }, err => { cleanup(); reject(err); });
  });
}

function sleepCancellable(ms: number, signal: AbortSignal): Promise<boolean> {
  return new Promise(resolve => {
    if (signal.aborted) { resolve(false); return; }
    const done = (ok: boolean) => { clearTimeout(timer); signal.removeEventListener("abort", onAbort); resolve(ok); };
    const onAbort = () => done(false);
    const timer = setTimeout(() => done(true), ms);
    signal.addEventListener("abort", onAbort, { once: true });
  });
}

// parseSSEBlock 解析一个 SSE 事件块（导出供单测覆盖协议边界）。
export function parseSSEBlock(raw: string, prevId: string): SSEEvent | null {
  let event = "message";
  let id = prevId;
  const dataLines: string[] = [];
  for (const line of raw.split(/\r?\n/)) {
    if (line.startsWith(":")) continue; // 注释行（心跳 ping）
    if (line.startsWith("event:")) event = line.slice(6).trim();
    else if (line.startsWith("id:")) id = line.slice(3).trim();
    else if (line.startsWith("data:")) {
      // SSE 规范：去掉 data: 后的单个前导空格，多行 data 以 \n join。
      let v = line.slice(5);
      if (v.startsWith(" ")) v = v.slice(1);
      dataLines.push(v);
    }
  }
  if (dataLines.length === 0) return null;
  const data = dataLines.join("\n");
  let parsed: unknown = data;
  try {
    parsed = JSON.parse(data);
  } catch {
    parsed = data;
  }
  return { id, event, data: parsed };
}

export async function getTTSVoices(): Promise<TTSVoice[]> {
  try {
    const data = await http<{ voices: TTSVoice[] }>("GET", "/api/v1/tts/voices");
    return data.voices || [];
  } catch {
    return [];
  }
}

export interface TTSSynthesizeParams {
  text: string;
  instruction?: string;
  voice?: string;
  engine?: string;
  baseUrl?: string;
  apiKey?: string;
  model?: string;
  speed?: number;
}

export async function synthesizeTTS(
  voiceOrParams: string | TTSSynthesizeParams,
  fallbackText?: string
): Promise<Blob> {
  const body: TTSSynthesizeParams =
    typeof voiceOrParams === "string"
      ? { voice: voiceOrParams, text: fallbackText || "" }
      : voiceOrParams;

  const res = await binaryRequest("/api/v1/tts/synthesize", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return res.blob();
}

export interface GenerateImageParams {
  prompt: string;
  negativePrompt?: string;
  engine?: string;
  baseUrl?: string;
  apiKey?: string;
  model?: string;
  size?: string;
  quality?: string;
  style?: string;
}

export interface GeneratedImageResult {
  id: string;
  url: string;
  prompt: string;
  mimeType: string;
  createdAt: string;
}

export async function generateImage(params: GenerateImageParams): Promise<GeneratedImageResult> {
  return http<GeneratedImageResult>("POST", "/api/v1/images/generate", params);
}

export interface EmbeddingProbeRequest {
  baseUrl: string;
  apiKey?: string;
  model: string;
  sampleText?: string;
}

export interface EmbeddingProbeResponse {
  ok: boolean;
  model: string;
  dimension: number;
  latencyMs: number;
  preview?: number[];
  error?: string;
}

export async function probeEmbedding(req: EmbeddingProbeRequest): Promise<EmbeddingProbeResponse> {
  return http<EmbeddingProbeResponse>("POST", "/api/v1/embeddings/probe", req);
}

export interface EmbeddingComputeRequest {
  texts: string[];
  baseUrl?: string;
  apiKey?: string;
  model?: string;
  dimension?: number;
}

export interface EmbeddingComputeResponse {
  model: string;
  dimension: number;
  embeddings: number[][];
}

export async function computeEmbeddings(req: EmbeddingComputeRequest): Promise<EmbeddingComputeResponse> {
  return http<EmbeddingComputeResponse>("POST", "/api/v1/embeddings", req);
}
