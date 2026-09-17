package http_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tavernagent/internal/application"
	"tavernagent/internal/domain"
	"tavernagent/internal/protocol"
)

// TestExportAndVerifyPayloadContracts (T5.3 前后端载荷契约测试)
//
// 目的：
// 锁定 Go 后端 HTTP 与 SSE 载荷结构，输出到 web/src/app/__tests__/contract_fixtures.json，
// 并由前端单元测试 web/src/app/__tests__/contract.test.ts 读取校验。
// 防止前后端类型在缺乏 protobuf 的情况下产生静默漂移（字段名大写/驼峰不符、枚举值漂移、字段缺失）。
func TestExportAndVerifyPayloadContracts(t *testing.T) {
	fixedTime := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)

	// 1. SessionView 载荷
	sampleState := map[string]any{
		"relationships": map[string]any{
			"npc_1": map[string]any{"affection": 10, "trust": 15, "alertness": 0},
		},
		"items": map[string]any{
			"item_1": map[string]any{
				"instanceId": "item_1",
				"name":       "旧怀表",
				"quantity":   1,
				"keepsake":   true,
			},
		},
		"promises": map[string]any{
			"prom_1": map[string]any{
				"promiseId":    "prom_1",
				"content":      "遵守约定",
				"sourceNodeId": "node_root",
				"state":        "active",
			},
		},
		"characters": map[string]any{
			"char_1": map[string]any{
				"characterId": "char_1",
				"name":        "艾莲娜",
			},
		},
		"moods": map[string]any{
			"char_1": map[string]any{
				"moodCode": "calm",
				"text":     "心如止水",
			},
		},
		"scene": map[string]any{
			"sceneId": "scene_gates",
			"title":   "古堡大门",
		},
	}
	stateJSON, err := json.Marshal(sampleState)
	if err != nil {
		t.Fatal(err)
	}

	sessionView := application.SessionView{
		SessionID:   "sess_contract_1",
		CharacterID: "char_1",
		Title:       "风暴前夕",
		RootNodeID:  "node_root",
		Branch: application.BranchView{
			BranchID:   "branch_main",
			Name:       "主分支",
			HeadNodeID: "node_turn_1",
			Version:    1,
		},
		ViewNodeID: "node_turn_1",
		HeadNode: &domain.PlotNode{
			NodeID:      "node_turn_1",
			SessionID:   "sess_contract_1",
			ParentID:    "node_root",
			Kind:        domain.NodeKindTurn,
			Depth:       1,
			TurnNumber:  1,
			ContentJSON: `{"inputKind":"dialogue","inputText":"你好"}`,
			CreatedAt:   fixedTime,
		},
		Nodes: []*domain.PlotNode{
			{
				NodeID:      "node_root",
				SessionID:   "sess_contract_1",
				Kind:        domain.NodeKindRoot,
				Depth:       0,
				TurnNumber:  0,
				ContentJSON: `{}`,
				CreatedAt:   fixedTime.Add(-10 * time.Minute),
			},
		},
		State: json.RawMessage(stateJSON),
		Secrets: []application.SecretView{
			{
				SecretID: "sec_1",
				Title:    "失落遗迹",
				Order:    1,
				Content:  "神殿藏有远古符文石。",
				Revealed: true,
			},
		},
		Outline: &application.OutlineView{
			Milestones: []application.OutlineEntry{
				{MilestoneID: "m_1", Description: "穿越黑森林"},
			},
			Goals: []application.GoalEntry{
				{GoalID: "g_1", CharacterID: "char_1", Text: "找到失落的石板"},
			},
			Scene: "古堡大门",
		},
		ActiveSummary: &domain.SummaryArtifact{
			SummaryID:  "sum_1",
			FromNodeID: "node_root",
			ToNodeID:   "node_turn_1",
			SourceHash: "hash_abcdef",
			Text:       "主角踏入了大门。",
		},
		Actions: []application.ActionView{
			{ActionID: "act_search", Label: "搜寻线索", Attribute: "perception", DC: 12},
		},
		Director: &domain.DirectorSummary{
			Title:            "第一幕：启程",
			RevisionID:       "rev_1",
			Status:           "active",
			CurrentBeatID:    "beat_1",
			CurrentBeatTitle: "初遇",
			Completed:        0,
			Total:            3,
		},
		HasMore: false,
	}

	// 2. TurnContent 载荷
	turnContent := domain.TurnContent{
		InputKind: "dialogue",
		InputText: "请问前面有什么危险？",
		Checks: []domain.CheckResult{
			{
				RollID:          "roll_1",
				ActionID:        "act_search",
				Attribute:       "perception",
				AttributeMod:    2,
				Natural:         15,
				Total:           17,
				DC:              12,
				Outcome:         domain.OutcomeSuccess,
				RulesetVer:      "v1",
				PermanentEffect: "感知敏锐",
			},
		},
		Blocks: []domain.TextBlock{
			{
				Kind: "narration",
				Text: "远方传来低沉的咆哮。",
			},
			{
				Kind:      "dialogue",
				SpeakerID: "char_1",
				Text:      "小心，前面有魔兽出没。",
			},
			{
				Kind: "inner_monologue",
				Text: "（它比预想的还要敏捷……）",
			},
		},
		Options: []domain.Option{
			{
				OptionID: "opt_1",
				Intent:   "clever",
				Text:     "攀上树枝观察周围地形",
			},
			{
				OptionID: "opt_2",
				Intent:   "aggressive",
				Text:     "拔剑戒备准备正面突击",
			},
		},
		Mood: &domain.Mood{
			CharacterID: "char_1",
			MoodCode:    "vigilant",
			Text:        "警惕环视四周",
		},
	}

	// 3. MemoryView 载荷
	memoryView := application.MemoryView{
		MemoryRecord: &domain.MemoryRecord{
			MemoryID:      "mem_contract_1",
			SourceNodeID:  "node_turn_1",
			Kind:          "observed",
			OwnerIDs:      []string{"char_1"},
			Content:       "艾莲娜佩戴着一枚带有雄鹰印记的银制徽章。",
			EntityIDs:     []string{"char_1"},
			Confidence:    0.95,
			Pinned:        false,
			SubjectKey:    "char_1.badge",
			CreatedTurn:   1,
			ValidFromTurn: 1,
		},
		Effective: true,
		Protected: false,
	}

	memoryUsage := domain.MemoryUsage{
		Used:        1,
		Protected:   0,
		Limit:       200,
		Reclaimable: 0,
		Headroom:    199,
		Tier:        "Normal",
	}

	// 4. AcceptResult & DeriveResult
	acceptResult := map[string]any{
		"turnId":    "turn_new_1",
		"status":    "processing",
		"statusUrl": "/api/v1/turns/turn_new_1",
		"eventsUrl": "/api/v1/turns/turn_new_1/events",
	}

	deriveResult := map[string]any{
		"branch": application.BranchView{
			BranchID:   "branch_sub_1",
			Name:       "新分支",
			HeadNodeID: "node_root",
			Version:    1,
		},
		"branchId":  "branch_sub_1",
		"turnId":    "turn_new_2",
		"status":    "processing",
		"statusUrl": "/api/v1/turns/turn_new_2",
		"eventsUrl": "/api/v1/turns/turn_new_2/events",
	}

	// 5. API Error 载荷 (与 writeError 对齐)
	apiError := map[string]any{
		"code":      "INVALID_ARGUMENT",
		"message":   "参数校验失败",
		"retryable": false,
		"turnId":    "turn_err_1",
	}

	// 6. SSE 实时事件载荷
	sseEvents := map[string]any{
		"turn.thinking": map[string]any{
			"attemptId": "att_1",
			"thinking":  "正在构思剧情转折...",
		},
		"block.delta": map[string]any{
			"attemptId": "att_1",
			"seq":       1,
			"kind":      "narration",
			"speakerId": nil,
			"delta":     "森林深处的阴影中，",
		},
		"block.appended": map[string]any{
			"frameSeq": 1,
			"frame": protocol.BlockFrame{
				V:    1,
				Seq:  1,
				Type: protocol.FrameBlock,
				Kind: protocol.BlockNarration,
				Text: "森林深处的阴影中，隐约有红色的眼睛亮起。",
			},
		},
		"turn.started":   map[string]any{},
		"turn.committed": map[string]any{},
		"turn.cancelled": map[string]any{},
		"turn.failed": map[string]any{
			"code":    "LLM_GATEWAY_TIMEOUT",
			"message": "模型供应商响应超时",
		},
		"turn.conflicted": map[string]any{},
	}

	fixtures := map[string]any{
		"version":      protocol.Version,
		"sessionView":  sessionView,
		"turnContent":  turnContent,
		"memoryView":   memoryView,
		"memoryUsage":  memoryUsage,
		"acceptResult": acceptResult,
		"deriveResult": deriveResult,
		"apiError":     apiError,
		"sseEvents":    sseEvents,
	}

	// 验证可正常序列化
	data, err := json.MarshalIndent(fixtures, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixtures failed: %v", err)
	}

	// 输出到 web/src/app/__tests__/contract_fixtures.json 供前端测试加载
	targetPath := filepath.Join("..", "..", "..", "web", "src", "app", "__tests__", "contract_fixtures.json")
	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
		t.Fatalf("write fixture file %s failed: %v", targetPath, err)
	}
}
