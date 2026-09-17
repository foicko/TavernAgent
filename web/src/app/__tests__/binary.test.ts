import { afterEach, describe, expect, it, vi } from "vitest";
import { api, onAuthRequired } from "../api";

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

describe("authenticated archive transfer", () => {
  it("uploads and downloads use the stored token and a bounded deadline", async () => {
    vi.stubGlobal("localStorage", { getItem: () => "test-only-token" });
    const timeout = vi.spyOn(AbortSignal, "timeout");
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response("pack"))
      .mockResolvedValueOnce(new Response(JSON.stringify({ sessionId: "imported" })));
    vi.stubGlobal("fetch", fetchMock);
    expect(await (await api.exportSessionPack("session")).text()).toBe("pack");
    expect((await api.importSessionPack(new ArrayBuffer(2))).sessionId).toBe("imported");
    for (const [, request] of fetchMock.mock.calls) {
      expect(request.headers.get("Authorization")).toBe("Bearer test-only-token");
      expect(request.signal).toBeInstanceOf(AbortSignal);
    }
    expect(timeout).toHaveBeenNthCalledWith(1, 30000);
    expect(timeout).toHaveBeenNthCalledWith(2, 30000);
  });

  it("unauthorized archives trigger pairing and do not return a success blob", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("AUTH_REQUIRED", { status: 401 })));
    const auth = vi.fn(), stop = onAuthRequired(auth);
    try { await expect(api.exportSessionPack("session")).rejects.toThrow(); expect(auth).toHaveBeenCalledOnce(); }
    finally { stop(); }
  });

  it("deadline failures have a recoverable message", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new DOMException("timeout", "TimeoutError")));
    await expect(api.importSessionPack(new ArrayBuffer(0))).rejects.toThrow("请求超时");
  });
});
