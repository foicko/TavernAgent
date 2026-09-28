import { describe, expect, it } from "vitest";
import { createStoryRuntime, storyRuntime } from "../storyRuntime";

describe("storyRuntime", () => {
  it("reset 清在途句柄与思考表，不动各序号，且不丢待建会话的幂等键（G38）", () => {
    const rt = createStoryRuntime();
    rt.sendSeq = 3;
    rt.navigationEpoch = 5;
    rt.reloadSeq = 2;
    rt.memorySeq = 7;
    rt.sessionListSeq = 1;
    rt.branchOpSeq = 4;
    let stopped = false;
    rt.sessionStop = () => { stopped = true; };
    rt.pendingMemoryPatch = { payload: "p", key: "k" };
    rt.thinkingStartedAt.set("t1", 1);
    rt.pendingSessionSetup = { epoch: 5, payload: "p", key: "k" };

    rt.reset();

    expect(stopped).toBe(true);
    expect(rt.sessionStop).toBe(null);
    expect(rt.pendingMemoryPatch).toBe(null);
    expect(rt.pendingSubmission).toBe(null);
    expect(rt.thinkingStartedAt.size).toBe(0);
    // 序号不受 reset 影响（由调用方按需自增）
    expect([rt.sendSeq, rt.navigationEpoch, rt.reloadSeq, rt.memorySeq, rt.sessionListSeq, rt.branchOpSeq])
      .toEqual([3, 5, 2, 7, 1, 4]);
    // 幂等键必须跨导航存活，否则会重复建故事
    expect(rt.pendingSessionSetup?.key).toBe("k");
  });

  it("createStoryRuntime 每次返回独立实例（便于测试隔离，不再靠间接重置）", () => {
    const a = createStoryRuntime();
    const b = createStoryRuntime();
    a.thinkingStartedAt.set("x", 1);
    a.branchOpSeq = 9;
    expect(b.thinkingStartedAt.size).toBe(0);
    expect(b.branchOpSeq).toBe(0);
    expect(storyRuntime).not.toBe(a);
  });
});
