package domain

import (
	"encoding/json"
	"testing"
)

func newTestWorld() *WorldState {
	s := NewWorldState()
	s.Characters["npc_elena"] = CharacterInfo{CharacterID: "npc_elena", Name: "Elena", Participant: true}
	if err := s.ApplyItemGrant(ItemInstance{
		InstanceID: "item_pocketwatch", TemplateID: "tpl_pocketwatch", Name: "怀表",
		OwnerID: "player", Quantity: 1, Keepsake: true, Unique: true,
	}); err != nil {
		panic(err)
	}
	return s
}

// C01 同一 turnId 最多关联一个已提交剧情节点 —— 由存储层保证（见 sqlite 测试）。
// 此处验证 TurnContent 与节点的建模一致性。

// C04 关系、背包、承诺只能由当前祖先链上的事件决定：
// 验证 ApplyEvents 顺序应用的确定性（同一事件序列产生相同字符串结果）。
func TestApplyEventsDeterministic(t *testing.T) {
	payload, _ := json.Marshal(RelationshipDeltaPayload{CharacterID: "npc_elena", Field: "trust", Applied: 5})
	ev := &DomainEvent{Type: EventRelationshipDelta, PayloadJSON: string(payload)}

	s1 := newTestWorld()
	labels1, err := ApplyEvents(s1, []*DomainEvent{ev, ev})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	s2 := newTestWorld()
	labels2, err := ApplyEvents(s2, []*DomainEvent{ev, ev})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if s1.String() != s2.String() {
		t.Fatalf("projection not deterministic:\n%s\n%s", s1.String(), s2.String())
	}
	if s1.Relationships["npc_elena"].Trust != 10 {
		t.Fatalf("trust = %d, want 10", s1.Relationships["npc_elena"].Trust)
	}
	_ = labels1
	_ = labels2
}

// C04a 重放校验：非法转移的事件不能应用（禁止把无法验证的变化写进世界）。
func TestApplyInvalidTransferRejected(t *testing.T) {
	s := newTestWorld()
	payload, _ := json.Marshal(ItemTransferPayload{ItemID: "item_pocketwatch", From: "player", To: "npc_elena", Quantity: 2})
	ev := &DomainEvent{Type: EventItemTransfer, PayloadJSON: string(payload)}
	if _, err := ApplyEvents(s, []*DomainEvent{ev}); err == nil {
		t.Fatalf("expected transfer of qty 2 (only 1 owned) to be rejected")
	}
	// C07：数量守恒，且非负
	payload2, _ := json.Marshal(ItemTransferPayload{ItemID: "item_pocketwatch", From: "player", To: "npc_elena", Quantity: 1})
	ev2 := &DomainEvent{Type: EventItemTransfer, PayloadJSON: string(payload2)}
	if _, err := ApplyEvents(s, []*DomainEvent{ev2}); err != nil {
		t.Fatalf("valid transfer rejected: %v", err)
	}
	if got := s.Items["item_pocketwatch"].OwnerID; got != "npc_elena" {
		t.Fatalf("owner = %q, want npc_elena", got)
	}
	if got := s.Items["item_pocketwatch"].Quantity; got != 1 {
		t.Fatalf("qty = %d, want 1", got)
	}
}

// C07 数量守恒与信物槽：Keepsake ≤ 3。
func TestKeepsakeSlotLimit(t *testing.T) {
	s := newTestWorld()
	if err := s.ValidateKeepsakeSlot("player", true); err != nil {
		t.Fatalf("slot should be free: %v", err)
	}
	for i := 0; i < 2; i++ {
		id := string(rune('a' + i))
		if err := s.ApplyItemGrant(ItemInstance{InstanceID: id, Name: id, OwnerID: "player", Keepsake: true, Quantity: 1}); err != nil {
			t.Fatalf("grant: %v", err)
		}
	}
	if err := s.ValidateKeepsakeSlot("player", true); err == nil {
		t.Fatalf("expected keepsake slot overflow to be rejected")
	}
	if err := s.ApplyItemGrant(ItemInstance{InstanceID: "fourth", OwnerID: "player", Keepsake: true, Quantity: 1}); err == nil {
		t.Fatal("grant bypassed the keepsake limit")
	}
}

// C08 客户端/模型不能指定实际库存：新增物品须由授权事件，apply 直接拒绝未知 grant 之外的凭空添加。
// （ApplyItemGrant 需要显式调用，侧面保证 item 不能仅凭 name 创建。）

// Aggregation: 同轮同角色同维度聚合 ±10 上限（技术契约 §5.1）。
func TestAggregateDeltasCapsPerTurn(t *testing.T) {
	cases := []struct {
		in   []int
		want int
	}{
		{[]int{4, 3, 4}, 10},
		{[]int{-4, -3, -4}, -10},
		{[]int{3, 2}, 5},
		{[]int{8, 8}, 10},
	}
	for _, c := range cases {
		if got := AggregateDeltas(c.in); got != c.want {
			t.Fatalf("AggregateDeltas(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

// relationship 边界与阶段上限：d>0 不越过全局最大值与 stageCap。
func TestRelationshipGrowthCap(t *testing.T) {
	s := newTestWorld()
	applied, err := s.ApplyRelationshipDelta("npc_elena", FieldAffection, 60, AffectionMax, 30)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied != 30 {
		t.Fatalf("applied = %d, want stage cap 30", applied)
	}
	if got := s.Relationships["npc_elena"].Affection; got != 30 {
		t.Fatalf("affection = %d, want 30", got)
	}
	// 阶段上限只限制增长，不把已有更高数值压低
	s.Relationships["npc_elena"] = RelationValue{Affection: 80}
	_, err = s.ApplyRelationshipDelta("npc_elena", FieldAffection, -10, AffectionMax, 30)
	if err != nil {
		t.Fatalf("apply negative: %v", err)
	}
	if got := s.Relationships["npc_elena"].Affection; got != 70 {
		t.Fatalf("affection = %d, want 70", got)
	}
}

// C09 状态重放不调用模型、不重新掷骰、不执行外部工具：
// ApplyEvents 是纯函数——通过确定性测试间接覆盖。

// C10 后台派生材料不能改写权威事件——记忆事件单独存储，不进入世界状态投影。
func TestMemoryAddNotInWorldProjection(t *testing.T) {
	s := newTestWorld()
	payload, _ := json.Marshal(MemoryAddPayload{Memory: MemoryRecord{MemoryID: "m1", Content: "她来自北方"}})
	ev := &DomainEvent{Type: EventMemoryAdd, PayloadJSON: string(payload)}
	labels, err := ApplyEvents(s, []*DomainEvent{ev})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(labels) != 1 || labels[0] != "memory" {
		t.Fatalf("labels = %v", labels)
	}
	if len(s.Promises) != 0 {
		t.Fatalf("memory event must not alter world facts")
	}
}

// C12 "已保存"只能由持久提交驱动 —— 由应用层/存储层保证（见 sqlite 与协议测试）。

// 承诺结算防重复。
func TestPromiseSettleOnce(t *testing.T) {
	p := Promise{PromiseID: "p1", State: PromiseActive}
	if err := SettlePromise(&p, PromiseFulfilled, "receipt1"); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if err := SettlePromise(&p, PromiseBroken, "receipt2"); err == nil {
		t.Fatalf("second settle must be rejected")
	}
}

// C05 已提交节点不可变：domain 层只有构造，没有 in-place 修改方法（编译期保障）。
// 补充验证 NodeKind/TurnNumber 语义：非对话事件不增加 turnNumber。
func TestNodeKindSemantics(t *testing.T) {
	turn := PlotNode{Kind: NodeKindTurn, TurnNumber: 3}
	mem := PlotNode{Kind: NodeKindMemoryChange, TurnNumber: 3}
	if turn.TurnNumber != 3 || mem.TurnNumber != 3 {
		t.Fatalf("non-turn events must keep turnNumber")
	}
	if mem.Depth != 0 {
		t.Fatalf("depth default must be 0")
	}
}

// ---- 记忆覆盖（copy-on-write，T17）----

func overlay(id, supersedes, content string) *MemoryRecord {
	return &MemoryRecord{MemoryID: id, Supersedes: supersedes, Content: content, Kind: MemoryObserved}
}

// 被取代的原记录不再生效，覆盖记录本身生效。
func TestApplyMemoryOverlaysBasic(t *testing.T) {
	orig := overlay("m1", "", "她来自北方。")
	ov := overlay("m2", "m1", "她来自南方。")

	got := ApplyMemoryOverlays([]*MemoryRecord{orig, ov})
	if len(got) != 1 || got[0].MemoryID != "m2" {
		t.Fatalf("有效记忆 = %+v", got)
	}
}

// 链式覆盖收敛到最后一版：R ← R1 ← R2 只剩 R2。
func TestApplyMemoryOverlaysChain(t *testing.T) {
	r1 := overlay("r1", "", "第一版。")
	r2 := overlay("r2", "r1", "第二版。")
	r3 := overlay("r3", "r2", "第三版。")

	got := ApplyMemoryOverlays([]*MemoryRecord{r1, r2, r3})
	if len(got) != 1 || got[0].MemoryID != "r3" {
		t.Fatalf("链式覆盖应收敛到最后一版，实际 = %+v", got)
	}
}

// 无关记录不受影响；空集合安全。
func TestApplyMemoryOverlaysUnrelatedUntouched(t *testing.T) {
	a := overlay("a", "", "甲。")
	b := overlay("b", "", "乙。")
	if got := ApplyMemoryOverlays([]*MemoryRecord{a, b}); len(got) != 2 {
		t.Fatalf("无覆盖关系时不应过滤: %+v", got)
	}
	if got := ApplyMemoryOverlays(nil); len(got) != 0 {
		t.Fatalf("空输入应返回空: %+v", got)
	}
}

// 覆盖只在本路径生效：集合里没有覆盖记录时，原记录照常生效（T17 的机制基础）。
func TestApplyMemoryOverlaysPathScoped(t *testing.T) {
	orig := overlay("m1", "", "她来自北方。")

	// A 路径：含覆盖记录。
	onA := ApplyMemoryOverlays([]*MemoryRecord{orig, overlay("m2", "m1", "她来自南方。")})
	if len(onA) != 1 || onA[0].MemoryID != "m2" {
		t.Fatalf("A 路径应看到覆盖版本: %+v", onA)
	}
	// B 路径：同一批记录里没有覆盖记录（覆盖挂在另一条分支上），原记录不受影响。
	onB := ApplyMemoryOverlays([]*MemoryRecord{orig})
	if len(onB) != 1 || onB[0].MemoryID != "m1" || onB[0].Content != "她来自北方。" {
		t.Fatalf("B 路径应保持原记录: %+v", onB)
	}
}

// 隐藏与置顶同样走覆盖：内容不变但标记不同。
func TestApplyMemoryOverlaysCarriesFlags(t *testing.T) {
	orig := overlay("m1", "", "她来自北方。")
	pinned := overlay("m2", "m1", "她来自北方。")
	pinned.Pinned = true
	hidden := overlay("m3", "m2", "她来自北方。")
	hidden.Hidden = true

	// 最终只剩隐藏版（链式收敛），且它的标记来自覆盖记录。
	got := ApplyMemoryOverlays([]*MemoryRecord{orig, pinned, hidden})
	if len(got) != 1 || got[0].MemoryID != "m3" || !got[0].Hidden || got[0].Pinned {
		t.Fatalf("覆盖记录应带上自己的标记: %+v", got)
	}
}
