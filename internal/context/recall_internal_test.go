package context

import (
	"strings"
	"testing"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/search"
)

// recency 的半衰期分档（契约 §8.1：低/中/高重要度 → 10/30/90 回合）。
func TestRecencyHalfLifeTiers(t *testing.T) {
	for _, tc := range []struct {
		importance int
		want       float64
	}{
		{1, 10}, {3, 10}, {4, 30}, {7, 30}, {8, 90}, {10, 90},
	} {
		if got := recencyHalfLife(tc.importance); got != tc.want {
			t.Fatalf("importance=%d 半衰期 = %v, want %v", tc.importance, got, tc.want)
		}
	}
}

// 查询文本必须把当前输入排在开场白之前：词元有上限，先到的先保留。
// 若顺序反了，长会话里被留下的全是开场白，检索退化成"按开场白找记忆"。
func TestMemoryQueryTextPutsInputFirst(t *testing.T) {
	longOpening := strings.Repeat("开场白的无关内容。", 40)
	got := memoryQueryText(longOpening, nil, "铜钥匙放在哪里")
	tokens := search.Tokens(got)
	if len(tokens) == 0 {
		t.Fatalf("空词元")
	}
	if tokens[0] != "铜钥" && tokens[0] != "钥匙" {
		t.Fatalf("首个词元应来自当前输入，得到 %q（前 5 个：%v）", tokens[0], tokens[:min(5, len(tokens))])
	}
}

// 实体识别要覆盖别名：用别称提问时也要定位到同一个角色（契约 §8.1）。
func TestMemoryQueryEntitiesMatchesAliases(t *testing.T) {
	state := domain.NewWorldState()
	state.Characters["npc_bell"] = domain.CharacterInfo{
		CharacterID: "npc_bell", Name: "看护人", Aliases: []string{"阿婆"},
	}
	if got := memoryQueryEntities(state, "阿婆今天在吗"); len(got) != 1 || got[0] != "npc_bell" {
		t.Fatalf("按别名未识别到实体：%v", got)
	}
	if got := memoryQueryEntities(state, "看护人在灯下坐着"); len(got) != 1 || got[0] != "npc_bell" {
		t.Fatalf("按正式名未识别到实体：%v", got)
	}
	if got := memoryQueryEntities(state, "我在路边买了水果"); len(got) != 0 {
		t.Fatalf("没出现称呼却识别出实体：%v", got)
	}
}

// 同等相关度下，重要度高的记忆排在前面（importance01 项生效）。
func TestSelectMemoriesPrefersHigherImportance(t *testing.T) {
	c := New(nil, DefaultOptions())
	low := &domain.MemoryRecord{MemoryID: "m_low", Content: "北方的冬天很长。", Importance: 1}
	high := &domain.MemoryRecord{MemoryID: "m_high", Content: "北方的冬天很长。", Importance: 10}
	cands := []*ports.MemoryCandidate{
		{Memory: low, Rank: -1, HasRank: true, SourceTurn: 5},
		{Memory: high, Rank: -1, HasRank: true, SourceTurn: 5},
	}
	got := c.selectMemories(cands, effectiveOf(cands), nil, "北方的冬天", 5)
	if len(got) != 2 || got[0].MemoryID != "m_high" {
		t.Fatalf("重要度高的记忆未排在前面：%v", idsOf(got))
	}
}

// 同等相关度下，最近被明确提及的记忆排在前面（recency01 项生效）。
func TestSelectMemoriesPrefersRecentMention(t *testing.T) {
	c := New(nil, DefaultOptions())
	old := &domain.MemoryRecord{MemoryID: "m_old", Content: "北方的冬天很长。", Importance: 5}
	fresh := &domain.MemoryRecord{MemoryID: "m_fresh", Content: "北方的冬天很长。", Importance: 5}
	cands := []*ports.MemoryCandidate{
		{Memory: old, Rank: -1, HasRank: true, SourceTurn: 1},
		// 只有 m_fresh 在第 30 轮被明确提及，m_old 停在来源回合。
		{Memory: fresh, Rank: -1, HasRank: true, SourceTurn: 1, LastMentionTurn: 30},
	}
	got := c.selectMemories(cands, effectiveOf(cands), nil, "北方的冬天", 30)
	if len(got) != 2 || got[0].MemoryID != "m_fresh" {
		t.Fatalf("最近被提及的记忆未排在前面：%v", idsOf(got))
	}
}

// 置顶同样受预算约束：它优先入选，但不能挤掉全部名额的条数上限。
func TestSelectMemoriesPinnedStillBounded(t *testing.T) {
	opts := DefaultOptions()
	opts.MaxMemories = 2
	c := New(nil, opts)
	mk := func(id string) *ports.MemoryCandidate {
		return &ports.MemoryCandidate{
			Memory: &domain.MemoryRecord{MemoryID: id, Content: "北方的冬天很长。", Pinned: true},
			Rank:   -1, HasRank: true, SourceTurn: 1,
		}
	}
	cands := []*ports.MemoryCandidate{mk("p1"), mk("p2"), mk("p3"), mk("p4")}
	got := c.selectMemories(cands, effectiveOf(cands), nil, "北方的冬天", 1)
	if len(got) != 2 {
		t.Fatalf("置顶绕过了条数预算：%d 条（应为 2）", len(got))
	}
}

// 覆盖记录即使没有词法命中也要保留：否则 copy-on-write 在打分阶段解不出来，
// 被纠正的旧记录会被继续召回。
func TestSelectMemoriesKeepsCoverWithoutLexicalHit(t *testing.T) {
	c := New(nil, DefaultOptions())
	old := &domain.MemoryRecord{MemoryID: "m_old", Content: "她来自北方。", Importance: 5}
	cover := &domain.MemoryRecord{MemoryID: "m_cover", Content: "她其实来自南方。", Supersedes: "m_old", Importance: 5}
	cands := []*ports.MemoryCandidate{
		{Memory: old, Rank: -1, HasRank: true, SourceTurn: 1},
		{Memory: cover, SourceTurn: 2}, // 无词法命中，但它是覆盖记录
	}
	got := idsOf(c.selectMemories(cands, effectiveOf(cands), nil, "她来自北方", 2))
	if len(got) != 1 || got[0] != "m_cover" {
		t.Fatalf("覆盖记录未被保留或被纠正的旧记录仍在：%v", got)
	}
}

// 隐藏的对象即使置顶也不注入（契约 §8：隐藏是检索控制）。
func TestSelectMemoriesHiddenBeatsPinned(t *testing.T) {
	c := New(nil, DefaultOptions())
	hidden := &domain.MemoryRecord{MemoryID: "m_hid", Content: "她来自北方。", Hidden: true, Pinned: true}
	cands := []*ports.MemoryCandidate{{Memory: hidden, Rank: -1, HasRank: true, SourceTurn: 1}}
	if got := c.selectMemories(cands, effectiveOf(cands), nil, "她来自北方", 1); len(got) != 0 {
		t.Fatalf("隐藏且置顶的记忆被注入了：%v", idsOf(got))
	}
}

func effectiveOf(cands []*ports.MemoryCandidate) map[string]bool {
	records := make([]*domain.MemoryRecord, 0, len(cands))
	for _, c := range cands {
		records = append(records, c.Memory)
	}
	out := map[string]bool{}
	for _, m := range domain.ApplyMemoryOverlays(records) {
		out[m.MemoryID] = true
	}
	return out
}

func idsOf(ms []*domain.MemoryRecord) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.MemoryID)
	}
	return out
}

func TestTemporalQueryDetection(t *testing.T) {
	c := New(nil, DefaultOptions())
	temporalCases := []string{
		"林晚最近一次提到铜钥匙，是放在哪里？",
		"最新的一条线索是什么？",
		"刚才说的那件事",
		"上次提到过的箱子",
		"最后放在哪里了",
		"上一次见面是什么时候",
		"前不久发生的事情",
		"之前提到的那把剑",
		"刚买的东西放在哪了",
	}
	for _, tc := range temporalCases {
		if !c.isTemporal(tc) {
			t.Errorf("expected temporal detection for %q, got false", tc)
		}
	}
	nonTemporalCases := []string{
		"北方的冬天很冷",
		"今天天气不错，我去买了些水果。",
		"艾拉的陶罗盘放在哪里？",
		"黑猫的银怀表在哪里",
	}
	for _, tc := range nonTemporalCases {
		if c.isTemporal(tc) {
			t.Errorf("expected non-temporal for %q, got true", tc)
		}
	}
}

// 时序查询下，同话题同实体的记忆必须优先召回最新发生/提及的回合（T2.1）。
// 此单测具有齿效应：若在时序提问下老记忆排在第 1 位则必然报错。
func TestTemporalRecallPrefersLatestOverOld(t *testing.T) {
	c := New(nil, DefaultOptions())
	state := domain.NewWorldState()
	state.Characters["char_1"] = domain.CharacterInfo{CharacterID: "char_1", Name: "艾拉"}

	oldMem := &domain.MemoryRecord{
		MemoryID:   "m_old",
		Kind:       domain.MemoryObserved,
		Content:    "艾拉把铜钥匙放在偏厅的矮柜上。",
		EntityIDs:  []string{"char_1"},
		Importance: 5,
	}
	newMem := &domain.MemoryRecord{
		MemoryID:   "m_new",
		Kind:       domain.MemoryObserved,
		Content:    "艾拉后来又说起铜钥匙，这次是放在门后的钉子上。",
		EntityIDs:  []string{"char_1"},
		Importance: 5,
	}
	// 构造多条候选模拟真实 FTS5 排名分布：旧记忆因 rowid 较小在 BM25 tie-breaker 中排第一，
	// 若无时序意图加成，旧记忆仅凭微弱的词法分即可压制晚 1 回合的新记忆。
	cands := []*ports.MemoryCandidate{
		{Memory: oldMem, Rank: -0.030, HasRank: true, SourceTurn: 79},
		{Memory: newMem, Rank: -0.029, HasRank: true, SourceTurn: 80},
		{Memory: &domain.MemoryRecord{MemoryID: "m_other", Content: "无关背景记忆", Importance: 1}, Rank: -0.010, HasRank: true, SourceTurn: 1},
	}
	got := c.selectMemories(cands, effectiveOf(cands), state, "艾拉最近一次提到铜钥匙，是放在哪里？", 150, "艾拉最近一次提到铜钥匙，是放在哪里？")
	if len(got) == 0 || got[0].MemoryID != "m_new" {
		t.Fatalf("时序查询未优先选出最新记忆，首位条目：%v (want m_new)", idsOf(got))
	}
}

// 无词法和实体命中的重要记忆（importance >= MinFallbackImportance）能够有限作为保底入池，且不超过配额（T2.2）。
func TestUnrankedFallbackBounded(t *testing.T) {
	opts := DefaultOptions()
	opts.MaxUnrankedFallback = 2
	opts.MinFallbackImportance = 7
	c := New(nil, opts)

	// 构造 4 条无词法无实体命中的记忆：三条重要度 >= 7，一条 < 7
	mHigh1 := &domain.MemoryRecord{MemoryID: "m_high1", Content: "王国核心机密：地下藏有上古符文巨构。", Importance: 9}
	mHigh2 := &domain.MemoryRecord{MemoryID: "m_high2", Content: "王国第二机密：深渊封印即将在冬至破裂。", Importance: 8}
	mHigh3 := &domain.MemoryRecord{MemoryID: "m_high3", Content: "王国第三机密：国王早已被影子替身取代。", Importance: 7}
	mLow := &domain.MemoryRecord{MemoryID: "m_low", Content: "昨天晚上吃的烤面包略微烤焦了。", Importance: 3}

	cands := []*ports.MemoryCandidate{
		{Memory: mHigh1, HasRank: false, SourceTurn: 1},
		{Memory: mHigh2, HasRank: false, SourceTurn: 2},
		{Memory: mHigh3, HasRank: false, SourceTurn: 3},
		{Memory: mLow, HasRank: false, SourceTurn: 4},
	}
	got := c.selectMemories(cands, effectiveOf(cands), nil, "今天天气不错，去集市买苹果。", 5)
	if len(got) > 2 {
		t.Fatalf("保底候选超出了 MaxUnrankedFallback 配额: got %d, want <= 2", len(got))
	}
	for _, m := range got {
		if m.Importance < 7 {
			t.Fatalf("低重要度记忆不应作为保底入池: %s (imp=%d)", m.MemoryID, m.Importance)
		}
	}
	if len(got) != 2 {
		t.Fatalf("高重要度保底候选应达到配额 2，实际为: %d (%v)", len(got), idsOf(got))
	}
}

// T3.2: ValidFromTurn 参与 recency01 计算（max(SourceTurn, LastMentionTurn, ValidFromTurn)）
func TestSelectMemoriesIncorporatesValidFromTurnInRecency(t *testing.T) {
	c := New(nil, DefaultOptions())
	mOld := &domain.MemoryRecord{MemoryID: "m_old", Content: "北方的冬天很长。", Importance: 5, ValidFromTurn: 0}
	mNewValid := &domain.MemoryRecord{MemoryID: "m_new_valid", Content: "北方的冬天很长。", Importance: 5, ValidFromTurn: 29}
	cands := []*ports.MemoryCandidate{
		{Memory: mOld, Rank: -1, HasRank: true, SourceTurn: 1},
		{Memory: mNewValid, Rank: -1, HasRank: true, SourceTurn: 1},
	}
	got := c.selectMemories(cands, effectiveOf(cands), nil, "北方的冬天", 30)
	if len(got) != 2 || got[0].MemoryID != "m_new_valid" {
		t.Fatalf("带有更新 ValidFromTurn 的记忆应优先召回：%v", idsOf(got))
	}
}

// T3.2: 超过 ValidUntilTurn 的记忆被排除
func TestSelectMemoriesExcludesExpiredValidUntilTurn(t *testing.T) {
	c := New(nil, DefaultOptions())
	mActive := &domain.MemoryRecord{MemoryID: "m_active", Content: "北方的冬天很长。", Importance: 5, ValidUntilTurn: 0}
	mExpired := &domain.MemoryRecord{MemoryID: "m_expired", Content: "北方的冬天很冷。", Importance: 5, ValidUntilTurn: 20}
	cands := []*ports.MemoryCandidate{
		{Memory: mActive, Rank: -1, HasRank: true, SourceTurn: 1},
		{Memory: mExpired, Rank: -1, HasRank: true, SourceTurn: 1},
	}
	got := c.selectMemories(cands, effectiveOf(cands), nil, "北方的冬天", 25)
	if len(got) != 1 || got[0].MemoryID != "m_active" {
		t.Fatalf("已过期的记忆未被排除：%v", idsOf(got))
	}
}

// T3.1: 候选池中同 SubjectKey 仅保留最新一条
func TestSelectMemoriesDeduplicatesSubjectKey(t *testing.T) {
	c := New(nil, DefaultOptions())
	mOlder := &domain.MemoryRecord{MemoryID: "m_subj_old", Content: "克拉拉情绪低落。", SubjectKey: "clara.mood", Importance: 5, CreatedTurn: 5, ValidFromTurn: 5}
	mNewer := &domain.MemoryRecord{MemoryID: "m_subj_new", Content: "克拉拉情绪转好。", SubjectKey: "clara.mood", Importance: 5, CreatedTurn: 25, ValidFromTurn: 25}
	cands := []*ports.MemoryCandidate{
		{Memory: mOlder, Rank: -1, HasRank: true, SourceTurn: 5},
		{Memory: mNewer, Rank: -1, HasRank: true, SourceTurn: 25},
	}
	got := c.selectMemories(cands, effectiveOf(cands), nil, "克拉拉情绪", 30)
	if len(got) != 1 || got[0].MemoryID != "m_subj_new" {
		t.Fatalf("同 SubjectKey 仅保留最新生效的一条: %v", idsOf(got))
	}
}

// T3.3: renderMemorySection 包含推测与降级标注
func TestRenderMemorySectionTrustAnnotations(t *testing.T) {
	memories := []*domain.MemoryRecord{
		{
			MemoryID: "m1",
			Kind:     domain.MemoryInferred,
			Content:  "他似乎在隐瞒什么",
			Evidence: &domain.MemoryEvidence{Confidence: "medium"},
		},
		{
			MemoryID: "m2",
			Kind:     domain.MemoryObserved,
			Content:  "她收下了银币",
			Evidence: &domain.MemoryEvidence{Confidence: "medium", AutoDowngraded: true},
		},
	}
	rendered := renderMemorySection(memories)
	if !strings.Contains(rendered, "推测，尚未被叙事确证") {
		t.Fatalf("未包含推测标注: %s", rendered)
	}
	if !strings.Contains(rendered, "引文不足降级") {
		t.Fatalf("未包含降级标注: %s", rendered)
	}
}
