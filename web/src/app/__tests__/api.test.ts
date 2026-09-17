// 单测：SSE 事件块解析（后端 writeSSE 协议镜像，上一轮修复的回归面）。
// 协议格式（internal/adapters/http/server.go writeSSE）：
//   event: <type>\nid: <turnId>:<seq>\ndata: <json>\n\n
import { describe, expect, it, vi } from "vitest";
import { api, parseSSEBlock } from "../api";

describe("parseSSEBlock", () => {
  it("解析标准事件块", () => {
    const evt = parseSSEBlock('event: block.delta\nid: t1:5\ndata: {"delta":"她抬"}', "");
    expect(evt).not.toBeNull();
    expect(evt!.event).toBe("block.delta");
    expect(evt!.id).toBe("t1:5");
    expect(evt!.data).toEqual({ delta: "她抬" });
  });

  it("多行 data 按 SSE 规范以 \\n join（修复点：此前缺分隔符会粘连）", () => {
    const evt = parseSSEBlock("event: raw\ndata: line1\ndata: line2", "");
    expect(evt).not.toBeNull();
    expect(evt!.data).toBe("line1\nline2");
  });

  it("去掉 data: 后的单个前导空格", () => {
    const evt = parseSSEBlock('event: m\nid: t:1\ndata: {"a":1} ', "");
    expect(evt!.data).toEqual({ a: 1 });
  });

  it("心跳注释行返回 null（后端 15s : ping）", () => {
    expect(parseSSEBlock(": ping", "prev")).toBeNull();
  });

  it("无 data 行返回 null 但保留先前 id", () => {
    expect(parseSSEBlock("event: turn.started", "t9:3")).toBeNull();
  });

  it("非 JSON data 原样返回字符串", () => {
    const evt = parseSSEBlock("event: raw\ndata: plain text", "");
    expect(evt!.data).toBe("plain text");
  });

  it("id 缺失时沿用 prevId（SSE 规范的 last-event-id 语义）", () => {
    const evt = parseSSEBlock('event: block.appended\ndata: {}', "t7:12");
    expect(evt!.id).toBe("t7:12");
  });
});

describe("api.importCardFile 的错误语义", () => {
  it("服务端 4xx 原样抛出（带服务端 message），不被前端兜住", async () => {
    // 曾经前端有一个本地解析降级：4xx 之外的失败会静默改走本地。降级已删除，
    // 现在唯一的权威解析在服务端，错误必须如实交到用户手里。
    const body = JSON.stringify({ code: "CARD_INVALID", message: "角色定义无效" });
    vi.stubGlobal("fetch", vi.fn(async () => new Response(body, { status: 422 })));
    try {
      await expect(api.importCardFile(new File(["{}"], "bad.json", { type: "application/json" })))
        .rejects.toMatchObject({ status: 422, message: "角色定义无效" });
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("网络异常原样抛出，不再伪装成解析成功", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => { throw new TypeError("Failed to fetch"); }));
    try {
      await expect(api.importCardFile(new File(["{}"], "x.json")))
        .rejects.toThrow("Failed to fetch");
    } finally {
      vi.unstubAllGlobals();
    }
  });
});
