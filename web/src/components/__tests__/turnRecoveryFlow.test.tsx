// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ComposerShelf } from "../ComposerShelf";
import { NarrativeStream } from "../NarrativeStream";
import { useStory } from "../../stores/storyStore";
import { useUi } from "../../stores/uiStore";
import { useSettings } from "../../stores/settingsStore";
import { deferred, recoveryView } from "../../stores/__tests__/turnRecoveryFixtures";

// Only fetch is substituted: the REST client, SSE parser, store and React
// controls all run together, without a server, user data or model requests.
let turnSequence = 0;
function recoveryTransport() {
  const turnId = `recovery_${++turnSequence}`;
  let saved = false;
  let turnEvents: ReadableStreamDefaultController<Uint8Array>;
  const encoder = new TextEncoder();
  const transport = {
    turnId,
    failedViewReads: 0,
    memoryResponse: null as Promise<Response> | null,
    posts: [] as Record<string, unknown>[],
    statusReads: 0,
    closedStreams: [] as string[],
    commit() {
      saved = true;
      // Older servers emitted live terminal events with sequence zero. The
      // parser correctly ignores this ID, but a heartbeat keeps SSE alive.
      turnEvents.enqueue(encoder.encode(`id: ${turnId}:0\nevent: turn.committed\ndata: {}\n\n: ping\n\n`));
    },
    thinking(text: string) {
      turnEvents.enqueue(encoder.encode(`event: turn.thinking\ndata: ${JSON.stringify({ thinking: text })}\n\n`));
    },
    heartbeat() { turnEvents.enqueue(encoder.encode(": ping\n\n")); },
    fetch: vi.fn<typeof fetch>(async (input, init): Promise<Response> => {
      const path = new URL(String(input), "http://localhost").pathname;
      if (path.endsWith("/events")) {
        return new Response(new ReadableStream<Uint8Array>({
          start(controller) {
            if (path === `/api/v1/turns/${turnId}/events`) {
              turnEvents = controller;
              const block = { frameSeq: 1, frame: { kind: "narration", text: "向导收好了地图。" } };
              controller.enqueue(encoder.encode(`id: ${turnId}:1\nevent: block.appended\ndata: ${JSON.stringify(block)}\n\n`));
            } else {
              controller.enqueue(encoder.encode(": ping\n\n"));
            }
          },
          cancel() { transport.closedStreams.push(path); },
        }), { headers: { "Content-Type": "text/event-stream" } });
      }
      if (path === "/api/v1/sessions/A/branches/branch_A/turns" && init?.method === "POST") {
        transport.posts.push(JSON.parse(String(init.body)) as Record<string, unknown>);
        return Response.json({ turnId, status: "preparing" });
      }
      if (path === `/api/v1/turns/${turnId}`) {
        transport.statusReads++;
        return Response.json({ turnId, status: saved ? "committed" : "generating" });
      }
      if (path === "/api/v1/sessions/A") {
        if (saved && transport.failedViewReads > 0) {
          transport.failedViewReads--;
          return Response.json({ message: "temporary view failure" }, { status: 503 });
        }
        return Response.json(recoveryView("A", saved));
      }
      if (path === "/api/v1/sessions/A/memories") {
        return saved && transport.memoryResponse ? transport.memoryResponse : Response.json({ memories: [] });
      }
      throw new Error(`Unexpected test request: ${init?.method ?? "GET"} ${path}`);
    }),
  };
  return transport;
}

let transport: ReturnType<typeof recoveryTransport>;
beforeEach(async () => {
  await useStory.getState().startNewSessionForCurrentChar();
  vi.useFakeTimers();
  localStorage.clear();
  useStory.setState(useStory.getInitialState(), true);
  useUi.setState({ ...useUi.getInitialState(), viewMode: "studio", notifyQuiet: vi.fn() }, true);
  useSettings.setState({ autoContinue: false, optionMode: "fill-edit" });
  transport = recoveryTransport();
  vi.stubGlobal("fetch", transport.fetch);
  await useStory.getState().openSession("A");
});
afterEach(async () => {
  cleanup();
  await useStory.getState().startNewSessionForCurrentChar();
  await vi.advanceTimersByTimeAsync(0);
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

async function sendFromComposer() {
  render(<><NarrativeStream /><ComposerShelf /></>);
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "沿着码头前行。" } });
  await act(async () => { fireEvent.click(screen.getByRole("button", { name: "推进剧情" })); });
  expect(screen.getByText("沿着码头前行。")).toBeTruthy();
  expect(screen.getByText("✦ 玩家行动 · 第一人称")).toBeTruthy();
  expect(screen.getByText("向导收好了地图。")).toBeTruthy();
  expect(screen.getByText("生成中", { exact: true })).toBeTruthy();
  expect((screen.getByRole("textbox") as HTMLTextAreaElement).disabled).toBe(true);
  expect(screen.queryByText("查看灯塔", { exact: true })).toBeNull();
}
function expectOptionsReady() {
  expect(screen.queryByText("生成中", { exact: true })).toBeNull();
  expect(screen.queryByText("同步中", { exact: true })).toBeNull();
  for (const option of ["查看灯塔", "向导打招呼", "记录航线"]) expect(screen.getByText(option, { exact: true })).toBeTruthy();
  expect((screen.getByRole("textbox") as HTMLTextAreaElement).disabled).toBe(false);
  expect(transport.posts).toHaveLength(1);
  expect(useStory.getState().view?.sessionId).toBe("A");
  expect(useStory.getState().view?.branch.headNodeId).toBe("turn_A");
}

describe("reply recovery through the real SSE parser and rendered controls", () => {
  it("immediately displays player bubble upon send and preserves thinking through commit", async () => {
    await sendFromComposer();
    await act(async () => {
      transport.thinking("正在深度分析码头地势与潜伏危机...");
    });
    expect(screen.getByText("正在深度分析码头地势与潜伏危机...")).toBeTruthy();
    await act(async () => {
      transport.commit();
      await vi.advanceTimersByTimeAsync(5000);
    });
    expectOptionsReady();
    // 提交后，思考过程依然保留在历史回合卡片中，点击可自如展开
    expect(screen.getByText("思考过程 (Reasoning)")).toBeTruthy();
    fireEvent.click(screen.getByText("思考过程 (Reasoning)"));
    expect(screen.getByText("正在深度分析码头地势与潜伏危机...")).toBeTruthy();
  });

  it("reveals usable choices without navigation when a legacy commit is ignored and heartbeats continue", async () => {
    await sendFromComposer();
    await act(async () => { transport.commit(); });
    await act(async () => { await vi.advanceTimersByTimeAsync(4999); transport.heartbeat(); });
    expect(transport.statusReads).toBe(0);
    expect(screen.queryByText("查看灯塔", { exact: true })).toBeNull();
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    expectOptionsReady();
    expect(transport.closedStreams).toContain(`/api/v1/turns/${transport.turnId}/events`);
    fireEvent.click(screen.getByText("查看灯塔", { exact: true }));
    expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("查看灯塔");
    expect((screen.getByRole("button", { name: "推进剧情" }) as HTMLButtonElement).disabled).toBe(false);
    await act(async () => { await vi.advanceTimersByTimeAsync(60000); });
    expect(transport.statusReads).toBe(1);
    expect(transport.posts).toHaveLength(1);
  });

  it("recovers on window focus while the auxiliary memory request is still pending", async () => {
    await sendFromComposer();
    const memory = deferred<Response>();
    transport.memoryResponse = memory.promise;
    await act(async () => {
      transport.commit();
      window.dispatchEvent(new Event("focus"));
      await vi.advanceTimersByTimeAsync(0);
    });
    expectOptionsReady();
    expect(useStory.getState().memoryLoading).toBe(true);
    await act(async () => { memory.resolve(Response.json({ memories: [] })); });
    expect(useStory.getState().memoryLoading).toBe(false);
    expect(transport.statusReads).toBe(1);
  });

  it("keeps the completed draft visible and the composer locked until a failed view read recovers", async () => {
    await sendFromComposer();
    transport.failedViewReads = 1;
    await act(async () => { transport.commit(); await vi.advanceTimersByTimeAsync(5000); });
    expect(screen.getByText("向导收好了地图。")).toBeTruthy();
    expect(screen.getByText("（回复已完成，正在同步选项…）")).toBeTruthy();
    expect(screen.queryByText("生成中", { exact: true })).toBeNull();
    expect((screen.getByRole("textbox") as HTMLTextAreaElement).disabled).toBe(true);
    expect(screen.queryByText("查看灯塔", { exact: true })).toBeNull();
    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
    expectOptionsReady();
    expect(useStory.getState().error).toBeNull();
  });
});
