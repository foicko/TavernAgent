package application

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"tavernagent/internal/adapters/providers/mock"
	"tavernagent/internal/adapters/sqlite"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

func TestCompactorService_RunOnceAndCompile(t *testing.T) {
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	bus := NewEventBus(st)
	sessionSvc := NewSessionService(st)

	// 1. 开局创建会话
	setup, err := sessionSvc.Setup(context.Background(), &SessionSetupRequest{
		Title:         "演义测试",
		CharacterJSON: testCard,
		Player:        Player{Name: "玩家"},
		OpeningText:   "你推开酒馆的门，老板娘抬起了头。",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	turnSvc := NewTurnService(st, nil, ctxpkg.New(st, ctxpkg.DefaultOptions()), bus)

	// 2. 连续提交 15 个回合，每个回合包含对白与心声
	headID := setup.Branch.HeadNodeID
	for i := 1; i <= 15; i++ {
		node, _, err := turnSvc.SubmitBlocks(context.Background(), SubmitBlocksRequest{
			SessionID: setup.Session.SessionID,
			BranchID:  setup.Branch.BranchID,
			HeadID:    headID,
			Version:   int64(i - 1),
			Input:     domain.TurnInput{Text: fmt.Sprintf("这是第 %d 轮输入", i)},
			Blocks: []domain.TextBlock{
				{Kind: "inner_monologue", Text: fmt.Sprintf("心声_%d", i)},
				{Kind: "dialogue", Text: fmt.Sprintf("对白_%d", i)},
			},
			Mode: "structured",
		})
		if err != nil {
			t.Fatalf("submit turn %d: %v", i, err)
		}
		headID = node.NodeID
	}

	// 3. 配置 Mock Provider 返回标准演义交接 XML
	xmlSummary := `<story_checkpoint>
<narrative_arc>
玩家与艾莲娜在酒馆达成了初步约定。
</narrative_arc>
<character_dynamics>
<mindset character="Elena">对玩家心存戒备但逐渐信任</mindset>
</character_dynamics>
<open_loops>
- [承诺] 明早把怀表归还
</open_loops>
</story_checkpoint>`

	mockProv := mock.New([]mock.Item{
		{Line: xmlSummary},
	})

	cfgStore := fakeStoreWithSlot("reflection", ports.ProviderConfig{Slot: "reflection", Enabled: true, Kind: "mock"})
	mgr := NewProviderManager(cfgStore, func(cfg ports.ProviderConfig) (ports.ModelProvider, error) {
		return mockProv, nil
	}, mockProv)
	if err := mgr.Reload(); err != nil {
		t.Fatal(err)
	}

	policy := ctxpkg.CompactionPolicy{
		TailWindowTurns:     6,
		MinUncompactedTurns: 8,
		PruneThoughtDepth:   6,
	}

	compactor := NewCompactorService(st, mgr, bus, policy)

	// 4. 执行一次压缩
	art, err := compactor.RunOnce(context.Background(), setup.Session.SessionID, setup.Branch.BranchID, headID)
	if err != nil {
		t.Fatalf("runCompaction error: %v", err)
	}
	if art == nil {
		t.Fatalf("expected summary artifact to be created, got nil")
	}

	// 5. 校验 SQLite 落盘
	sums, err := st.SummariesOnPath(headID)
	if err != nil {
		t.Fatalf("summaries on path: %v", err)
	}
	if len(sums) == 0 {
		t.Fatalf("expected summary to be saved in sqlite, got none")
	}
	if !strings.Contains(sums[0].Text, "初步约定") {
		t.Errorf("summary text does not contain expected content: %s", sums[0].Text)
	}

	// 6. 使用编译器编译下一轮上下文，验证：
	// a. 摘要被成功注入 System Prompt
	// b. 超出尾部窗口的历史心声被修剪（Thought Pruning）
	// c. 尾部窗口内的历史心声仍然保留
	compiler := ctxpkg.New(st, ctxpkg.CompilerOptions{
		CompactionPolicy: policy,
	})

	baseSnap, err := st.StateAt(headID)
	if err != nil {
		t.Fatalf("stateAt: %v", err)
	}
	baseState, err := domain.UnmarshalWorld(baseSnap.StateJSON)
	if err != nil {
		t.Fatalf("unmarshal world: %v", err)
	}

	req, err := compiler.Compile(context.Background(), setup.Session.SessionID, headID, "继续。", ctxpkg.TurnDirectives{}, baseState, nil)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	sysPrompt := req.Messages[0].Content
	if !strings.Contains(sysPrompt, "初步约定") {
		t.Errorf("system prompt did not contain injected summary:\n%s", sysPrompt)
	}

	// 验证心声修剪：
	// 历史总共 15 轮，TailWindowTurns 为 6 轮。
	// 故第 1~9 轮的心声（"心声_1" 到 "心声_9"）应当被修剪剔除；
	// 第 10~15 轮的心声（"心声_10" 到 "心声_15"）应当保留！
	allHistoryText := ""
	for _, m := range req.Messages[1:] {
		allHistoryText += m.Content + "\n"
	}

	// 较早的历史心声应该被修剪
	if strings.Contains(allHistoryText, "（心声_1）") {
		t.Errorf("turn 1 inner monologue should have been pruned out from history")
	}
	if strings.Contains(allHistoryText, "（心声_5）") {
		t.Errorf("turn 5 inner monologue should have been pruned out from history")
	}

	// 尾部窗口内的对白与心声应该完整保留
	if !strings.Contains(allHistoryText, "（心声_15）") {
		t.Errorf("tail window turn 15 inner monologue should be preserved")
	}
	if !strings.Contains(allHistoryText, "对白_1") {
		t.Errorf("historical dialogue should not be dropped, only thoughts pruned")
	}
}
