// 前后端载荷契约测试 (T5.3)
//
// 目的：
// 校验 web/src/app/types.ts 声明的前端接口与后端 Go HTTP/SSE 序列化产物 (contract_fixtures.json) 的强一致性。
// 任何字段大小写拼写、camelCase 命名、枚举值定义不一致均会在此报错，阻止前后端静默漂移。

import { describe, expect, it } from "vitest";
import fixtures from "./contract_fixtures.json";
import type { TurnEventType } from "../api";
import type {
  AcceptResult,
  CheckResult,
  DeriveResult,
  ItemInstance,
  MemoryUsage,
  MemoryView,
  Option,
  PromiseItem,
  RelationValue,
  SessionView,
  TextBlock,
  TurnContent,
} from "../types";

describe("前后端载荷契约测试 (Contract Verification)", () => {
  it("协议版本号对齐", () => {
    expect(fixtures.version).toBe(1);
  });

  it("SessionView 核心字段及结构对齐", () => {
    const sv = fixtures.sessionView as unknown as SessionView;
    expect(sv.sessionId).toBe("sess_contract_1");
    expect(sv.characterId).toBe("char_1");
    expect(sv.title).toBe("风暴前夕");
    expect(sv.rootNodeId).toBe("node_root");
    expect(sv.branch.branchId).toBe("branch_main");
    expect(sv.branch.headNodeId).toBe("node_turn_1");
    expect(sv.branch.version).toBe(1);

    expect(sv.headNode.nodeId).toBe("node_turn_1");
    expect(sv.headNode.turnNumber).toBe(1);
    expect(sv.headNode.kind).toBe("turn");

    expect(sv.secrets).toBeDefined();
    expect(sv.secrets![0].secretId).toBe("sec_1");
    expect(sv.secrets![0].revealed).toBe(true);

    expect(sv.outline).toBeDefined();
    expect(sv.outline!.milestones[0].milestoneId).toBe("m_1");
    expect(sv.outline!.goals[0].goalId).toBe("g_1");
    expect(sv.outline!.scene).toBe("古堡大门");

    expect(sv.director).toBeDefined();
    expect(sv.director!.status).toBe("active");
    expect(sv.director!.currentBeatId).toBe("beat_1");

    expect(sv.actions).toBeDefined();
    expect(sv.actions![0].actionId).toBe("act_search");
    expect(sv.actions![0].dc).toBe(12);

    expect(sv.activeSummary).toBeDefined();
    expect(sv.activeSummary!.summaryId).toBe("sum_1");
  });

  it("WorldState 投影状态字段与命名对齐", () => {
    const sv = fixtures.sessionView as unknown as SessionView;
    const state = sv.state as any;

    expect(state.relationships).toBeDefined();
    const rel: RelationValue = state.relationships["npc_1"];
    expect(rel.affection).toBe(10);
    expect(rel.trust).toBe(15);
    expect(rel.alertness).toBe(0);

    expect(state.items).toBeDefined();
    const item: ItemInstance = state.items["item_1"];
    expect(item.instanceId).toBe("item_1");
    expect(item.keepsake).toBe(true);

    expect(state.promises).toBeDefined();
    const prom: PromiseItem = state.promises["prom_1"];
    expect(prom.promiseId).toBe("prom_1");
    expect(["proposed", "active", "fulfilled", "broken", "cancelled"]).toContain(prom.state);
  });

  it("TurnContent 正文块与枚举对齐", () => {
    const tc = fixtures.turnContent as unknown as TurnContent;
    expect(tc.inputKind).toBe("dialogue");
    expect(tc.inputText).toBe("请问前面有什么危险？");

    expect(tc.blocks.length).toBe(3);
    const validKinds: TextBlock["kind"][] = ["narration", "dialogue", "inner_monologue"];
    for (const b of tc.blocks) {
      expect(validKinds).toContain(b.kind);
    }
    expect(tc.blocks[1].speakerId).toBe("char_1");

    expect(tc.options.length).toBe(2);
    const validIntents: Option["intent"][] = ["aggressive", "clever", "emotional", "chaotic"];
    for (const opt of tc.options) {
      expect(validIntents).toContain(opt.intent);
    }

    expect(tc.checks).toBeDefined();
    const chk = tc.checks![0] as CheckResult;
    expect(chk.rollId).toBe("roll_1");
    expect(chk.actionId).toBe("act_search");
    expect(chk.attributeModifier).toBe(2);
    expect(chk.natural).toBe(15);
    expect(chk.total).toBe(17);
    expect(chk.outcome).toBe("success");

    expect(tc.mood).toBeDefined();
    expect(tc.mood!.characterId).toBe("char_1");
    expect(tc.mood!.moodCode).toBe("vigilant");
  });

  it("MemoryView 结构与时间轴字段对齐", () => {
    const mem = fixtures.memoryView as unknown as MemoryView;
    expect(mem.memoryId).toBe("mem_contract_1");
    expect(mem.sourceNodeId).toBe("node_turn_1");
    expect(["observed", "reported", "inferred", "secret"]).toContain(mem.kind);
    expect(mem.subjectKey).toBe("char_1.badge");
    expect(mem.createdTurn).toBe(1);
    expect(mem.validFromTurn).toBe(1);
    expect(mem.effective).toBe(true);

    const usage = fixtures.memoryUsage as unknown as MemoryUsage;
    expect(usage.used).toBe(1);
    expect(usage.limit).toBe(200);
    expect(["Normal", "Notice", "Degraded", "Critical"]).toContain(usage.tier);
  });

  it("AcceptResult 与 DeriveResult 契约字段对齐", () => {
    const acc = fixtures.acceptResult as unknown as AcceptResult;
    expect(acc.turnId).toBe("turn_new_1");
    expect(acc.status).toBe("processing");
    expect(acc.statusUrl).toBe("/api/v1/turns/turn_new_1");
    expect(acc.eventsUrl).toBe("/api/v1/turns/turn_new_1/events");

    const der = fixtures.deriveResult as unknown as DeriveResult;
    expect(der.branchId).toBe("branch_sub_1");
    expect(der.turnId).toBe("turn_new_2");
    expect(der.branch.branchId).toBe("branch_sub_1");
  });

  it("API Error 契约对齐", () => {
    const err = fixtures.apiError as { code: string; message: string; retryable: boolean; turnId?: string };
    expect(err.code).toBe("INVALID_ARGUMENT");
    expect(err.message).toBe("参数校验失败");
    expect(err.retryable).toBe(false);
    expect(err.turnId).toBe("turn_err_1");
  });

  it("SSE 实时流事件命名与载荷形态对齐", () => {
    const sse = fixtures.sseEvents;
    const recognizedTurnEvents: TurnEventType[] = [
      "turn.started",
      "turn.thinking",
      "block.delta",
      "block.appended",
      "turn.truncated",
      "turn.awaiting_continuation",
      "turn.awaiting_approval",
      "turn.committed",
      "turn.cancelled",
      "turn.failed",
      "turn.conflicted",
    ];

    for (const evName of Object.keys(sse)) {
      expect(recognizedTurnEvents).toContain(evName as TurnEventType);
    }

    const delta = sse["block.delta"] as { attemptId: string; seq: number; kind: string; delta: string };
    expect(delta.attemptId).toBe("att_1");
    expect(delta.seq).toBe(1);
    expect(delta.kind).toBe("narration");
    expect(delta.delta).toBe("森林深处的阴影中，");

    const appended = sse["block.appended"] as { frameSeq: number; frame: { v: number; kind: string; text: string } };
    expect(appended.frameSeq).toBe(1);
    expect(appended.frame.v).toBe(1);
    expect(appended.frame.kind).toBe("narration");

    const failed = sse["turn.failed"] as { code: string; message: string };
    expect(failed.code).toBe("LLM_GATEWAY_TIMEOUT");
  });
});
