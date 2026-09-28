// SSE 传输层：fetch + ReadableStream 解析、指数退避重连、游标去重与事件错误边界。
//
// 从 `app/api.ts` 拆出（B10-R5）：REST/二进制与流式传输各自成模块后，SSE 的协议边界
// 可以独立单测，`api.ts` 也不再混合两种关注点。对外仍然从 `app/api` 导出（re-export），
// 既有 `vi.mock("../app/api")` 与调用点无需改动。
import type { SSEEvent } from "./types";
import { ApiError, getStoredAuthToken, notifyAuthRequired } from "./api";

const BASE = ""; // Vite 代理 /api → 后端

export type TurnEventType =
  | "turn.started"
  | "turn.thinking"
  | "block.delta"
  | "block.appended"
  | "turn.truncated"
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
            // 连接确实在出数据才算恢复：退避计数归零，否则长连接一生只能掉线 5 次
            retries = 0;
            buf += decoder.decode(chunk.value, { stream: true });
            if (buf.length > 1024 * 1024) {
              // 单个事件超过上限：无法安全解析出 id 以跳过，直接上报并终止，
              // 避免"从同一游标重连 → 再撞同一事件"的死循环。
              handlers.onFinalError(new Error("SSE event too large"));
              return;
            }
            let sep: RegExpExecArray | null;
            while ((sep = /\r?\n\r?\n/.exec(buf))) {
              const raw = buf.slice(0, sep.index);
              buf = buf.slice(sep.index + sep[0].length);
              const evt = parseSSEBlock(raw, lastEventId);
              if (!evt) continue;
              // Unnumbered deltas/thinking inherit last-event-id per SSE, but
              // must not be treated as duplicate durable business events.
              let durableSeq = 0;
              if (/^id:/m.test(raw)) {
                if (!evt.id.startsWith(aggregateId + ":")) continue;
                durableSeq = Number(evt.id.slice(aggregateId.length + 1));
                if (!Number.isSafeInteger(durableSeq) || durableSeq <= maxSeq) continue;
              }
              if (lifetime.signal.aborted) return;
              try {
                handlers.onEvent(evt);
              } catch (handlerError) {
                // 事件处理异常不能与"连接断开"混为一谈：游标尚未推进，事件不会被跳过；
                // 但重放同一事件只会再次抛错，故显式上报并终止，避免静默丢事件 + 重连风暴。
                handlers.onFinalError(handlerError instanceof Error ? handlerError : new Error(String(handlerError)));
                return;
              }
              if (durableSeq > 0) {
                maxSeq = durableSeq;
                lastEventId = evt.id;
              }
              if (terminal.has(evt.event)) return;
            }
          }
        } catch {
          if (lifetime.signal.aborted) return;
        } finally {
          clearTimeout(timer);
          connection.abort();
          if (reader) {
            void reader.cancel().catch(() => undefined);
            // Some engines throw if a read() is still in flight; never let that
            // abort the reconnect loop.
            try { reader.releaseLock(); } catch { /* ignore */ }
          }
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
