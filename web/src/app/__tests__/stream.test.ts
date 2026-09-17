import { afterEach, describe, expect, it, vi } from "vitest";
import { subscribeTurnEvents } from "../api";

const stops: (() => void)[] = [];
afterEach(() => { stops.splice(0).forEach(stop => stop()); vi.unstubAllGlobals(); vi.useRealTimers(); });
function stream(parts: string[]) {
  return new Response(new ReadableStream({ start(controller) { for (const part of parts) controller.enqueue(new TextEncoder().encode(part)); controller.close(); } }), { headers: { "Content-Type": "text/event-stream" } });
}
describe("recoverable SSE transport", () => {
  it("accepts chunked CRLF, deduplicates durable events, and preserves unnumbered deltas", async () => {
    const fetchMock = vi.fn().mockResolvedValue(stream([
      'id: t:1\r\nevent: block.appended\r\ndata: {}\r', '\n\r\n',
      'event: turn.thinking\ndata: {"thinking":"想"}\n\n',
      'event: block.delta\ndata: {"delta":"文"}\n\n',
      'id: t:1\nevent: block.appended\ndata: {}\n\n',
      'id: t:2\nevent: turn.committed\ndata: {}\n\n',
    ])); vi.stubGlobal("fetch", fetchMock);
    const onEvent = vi.fn(), onFinalError = vi.fn();
    stops.push(subscribeTurnEvents("t", { onEvent, onFinalError }));
    await vi.waitFor(() => expect(onEvent).toHaveBeenCalledTimes(4));
    expect(onEvent.mock.calls.map(c => c[0].event)).toEqual(["block.appended", "turn.thinking", "block.delta", "turn.committed"]);
    expect(onFinalError).not.toHaveBeenCalled(); expect(fetchMock).toHaveBeenCalledOnce();
  });

  it("reconnects with the durable cursor and stop cancels a pending retry", async () => {
    vi.useFakeTimers();
    const fetchMock = vi.fn().mockResolvedValueOnce(stream(['id: t:7\nevent: block.appended\ndata: {}\n\n']))
      .mockResolvedValue(new Response("busy", { status: 503 }));
    vi.stubGlobal("fetch", fetchMock);
    const stop = subscribeTurnEvents("t", { onEvent: vi.fn(), onFinalError: vi.fn() }); stops.push(stop);
    await vi.advanceTimersByTimeAsync(1000);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[1][1].headers["Last-Event-ID"]).toBe("t:7");
    stop(); await vi.advanceTimersByTimeAsync(60000);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("an already-aborted subscription performs no network requests", async () => {
    const fetchMock = vi.fn(); vi.stubGlobal("fetch", fetchMock);
    const ctrl = new AbortController(); ctrl.abort();
    stops.push(subscribeTurnEvents("t", { onEvent: vi.fn(), onFinalError: vi.fn() }, ctrl.signal));
    await Promise.resolve(); expect(fetchMock).not.toHaveBeenCalled();
  });
});
