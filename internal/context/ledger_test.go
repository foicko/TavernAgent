package context

import (
	"strings"
	"testing"

	"tavernagent/internal/domain"
)

// ---- M4h 确定性硬状态下界账本 单测 ----

func TestRenderDomainLedgerEmpty(t *testing.T) {
	if s := RenderDomainLedger(domain.NewWorldState(), nil, "玩家", nil); s != "" {
		t.Fatalf("空状态应返回空串，得到 %q", s)
	}
	if s := RenderDomainLedger(nil, nil, "玩家", nil); s != "" {
		t.Fatalf("nil 状态应返回空串，得到 %q", s)
	}
}

func TestRenderLedgerItems(t *testing.T) {
	state := domain.NewWorldState()
	state.Items["a"] = domain.ItemInstance{
		InstanceID: "a", Name: "粗制解毒草药", OwnerID: "player", Quantity: 2,
	}
	state.Items["b"] = domain.ItemInstance{
		InstanceID: "b", Name: "银质怀表", OwnerID: "player", Quantity: 1, Keepsake: true,
	}
	state.Items["c"] = domain.ItemInstance{
		InstanceID: "c", Name: "遗落在废墟的断剑", Quantity: 1,
	}
	s := RenderDomainLedger(state, nil, "玩家", nil)
	if !strings.Contains(s, "🛍") && !strings.Contains(s, "🎒") {
		t.Fatalf("应含物品账本标记: %s", s)
	}
	// 信物：无法丢弃
	if !strings.Contains(s, "银质怀表") || !strings.Contains(s, "无法丢弃") {
		t.Fatalf("信物应标无法丢弃: %s", s)
	}
	// 多数量 ×N
	if !strings.Contains(s, "粗制解毒草药") || !strings.Contains(s, "2") {
		t.Fatalf("多数量物品应标数量: %s", s)
	}
	// 无主物品归属地点
	if !strings.Contains(s, "地点/场景") {
		t.Fatalf("无主物品应标归属地点: %s", s)
	}
	// 平铺物品段已删除（不在 dynamicContextPrompt 里，但账本自身也不含"当前持有物品："）
	if strings.Contains(s, "当前持有物品：") {
		t.Fatalf("账本不应含旧平铺标题")
	}
}

func TestRenderLedgerPromises(t *testing.T) {
	state := domain.NewWorldState()
	state.Characters["elena"] = domain.CharacterInfo{CharacterID: "elena", Name: "艾莲娜", Participant: true}
	state.Promises["p1"] = domain.Promise{
		PromiseID:      "p1",
		ParticipantIDs: []string{"player", "elena"},
		Content:        "日落前在老钟楼汇合",
		State:          domain.PromiseActive,
	}
	state.Promises["p2"] = domain.Promise{
		PromiseID:      "p2",
		ParticipantIDs: []string{"player", "elena"},
		Content:        "永不背叛",
		State:          domain.PromiseBroken,
	}
	state.Promises["p3"] = domain.Promise{
		PromiseID:      "p3",
		ParticipantIDs: []string{"player", "elena"},
		Content:        "尚未承诺",
		State:          domain.PromiseProposed,
	}
	s := RenderDomainLedger(state, nil, "玩家", nil)
	if !strings.Contains(s, "艾莲娜对玩家立誓") {
		t.Fatalf("应解析出角色名: %s", s)
	}
	if !strings.Contains(s, "[生效中 (active)]") {
		t.Fatalf("active 誓言应有生效标记: %s", s)
	}
	if !strings.Contains(s, "[已破碎 (broken)]") {
		t.Fatalf("broken 誓言应标已破碎: %s", s)
	}
	if strings.Contains(s, "尚未承诺") {
		t.Fatalf("proposed 誓言不应入账本: %s", s)
	}
}

func TestRenderLedgerReceiptsSelection(t *testing.T) {
	crit := domain.CheckResult{ActionID: "crit", Attribute: "感知", DC: 20, Natural: 20, Total: 22, Outcome: domain.OutcomeCriticalSuccess}
	fail15 := domain.CheckResult{ActionID: "f15", Attribute: "敏捷", DC: 15, Natural: 8, Total: 10, AttributeMod: 2, Outcome: domain.OutcomeFailure}
	failLow := domain.CheckResult{ActionID: "low", Attribute: "力量", DC: 8, Natural: 5, Total: 6, Outcome: domain.OutcomeFailure}
	critFail := domain.CheckResult{ActionID: "cf", Attribute: "智力", DC: 16, Natural: 1, Total: 0, Outcome: domain.OutcomeCriticalFailure}

	// DC15 failure + critical + DC8 failure：前两者入选，后者排除
	ledger := renderLedgerReceipts([]LedgerCheck{
		{TurnNumber: 5, Result: fail15},
		{TurnNumber: 6, Result: crit},
		{TurnNumber: 2, Result: failLow},
	})
	if !strings.Contains(ledger, "敏捷") || !strings.Contains(ledger, "Turn#5") {
		t.Fatalf("应含 DC15 failure: %s", ledger)
	}
	if !strings.Contains(ledger, "Turn#6") {
		t.Fatalf("应含 critical: %s", ledger)
	}
	if strings.Contains(ledger, "力量") {
		t.Fatalf("DC8 failure 不应入选: %s", ledger)
	}

	// >10 条按轮次降序截断
	checks := make([]LedgerCheck, 0, 12)
	for i := 1; i <= 12; i++ {
		checks = append(checks, LedgerCheck{TurnNumber: i, Result: critFail})
	}
	filtered := filterLedgerChecks(checks)
	if len(filtered) != 10 {
		t.Fatalf("应截断到 10 条，得到 %d", len(filtered))
	}
	// 降序：首条是最近回合
	if filtered[0].TurnNumber != 12 {
		t.Fatalf("应按轮次降序，首条回合 = %d", filtered[0].TurnNumber)
	}
}

func TestRenderLedgerMilestones(t *testing.T) {
	state := domain.NewWorldState()
	state.Milestones["bell_tower_riot"] = "钟楼暴乱已爆发"
	state.Milestones["bridge_open"] = "北门大桥已通行"
	s := RenderDomainLedger(state, nil, "玩家", nil)
	if !strings.Contains(s, "钟楼暴乱已爆发") || !strings.Contains(s, "bell_tower_riot") {
		t.Fatalf("应渲染里程碑: %s", s)
	}
	if !strings.Contains(s, "北门大桥已通行") {
		t.Fatalf("应渲染多个里程碑: %s", s)
	}
}

func TestRenderLedgerAuthoritativeTag(t *testing.T) {
	state := domain.NewWorldState()
	state.Items["x"] = domain.ItemInstance{InstanceID: "x", Name: "火把", OwnerID: "player", Quantity: 1}
	s := RenderDomainLedger(state, nil, "玩家", nil)
	if !strings.HasPrefix(s, `<domain_state_ledger authoritative="true">`) {
		t.Fatalf("缺少 authoritative 包裹: %s", s)
	}
}

func TestRenderLedgerRelationships(t *testing.T) {
	state := domain.NewWorldState()
	state.Characters["npc_elena"] = domain.CharacterInfo{CharacterID: "npc_elena", Name: "艾莲娜"}
	state.Relationships["npc_elena"] = domain.RelationValue{Affection: 25, Trust: 20, Alertness: 0}

	relations := []LedgerRelationDelta{
		{TurnNumber: 2, CharacterID: "npc_elena", Field: "affection", Delta: 10, Applied: 10},
		{TurnNumber: 5, CharacterID: "npc_elena", Field: "trust", Delta: 15, Applied: 15},
		{TurnNumber: 7, CharacterID: "npc_elena", Field: "alertness", Delta: -10, Applied: -10},
	}

	s := RenderDomainLedger(state, nil, "玩家", relations)
	if !strings.Contains(s, "角色关系轨迹") {
		t.Fatalf("应含关系轨迹段: %s", s)
	}
	if !strings.Contains(s, "艾莲娜 (npc_elena)") {
		t.Fatalf("应解析角色名: %s", s)
	}
	if !strings.Contains(s, "Turn#2 [好感 +10]") || !strings.Contains(s, "Turn#5 [信任 +15]") || !strings.Contains(s, "Turn#7 [戒备 -10]") {
		t.Fatalf("应包含具体转折点与增量: %s", s)
	}
	if !strings.Contains(s, "当前: 好感 25, 信任 20, 戒备 0") {
		t.Fatalf("应包含当前关系状态: %s", s)
	}
}
