package application

// 精简测试后保留的共享夹具：安全/不可逆路径的用例依赖这里的最小脚手架。
import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"tavernagent/internal/adapters/providers/mock"
	"tavernagent/internal/adapters/sqlite"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

const testCard = `{
  "schemaVersion": 1,
  "cardId": "card_elena",
  "name": "Elena",
  "description": "酒馆老板娘，记得与玩家的约定。",
  "characters": [
    {"characterId": "npc_elena", "name": "Elena", "description": "酒馆老板娘", "participant": true}
  ],
  "items": [
    {"instanceId": "item_pocketwatch", "templateId": "tpl_watch", "name": "怀表", "ownerId": "player", "quantity": 1, "keepsake": true, "unique": true}
  ]
}`

func mockBlock(seq int, kind string, text string) string {
	t, _ := json.Marshal(text)
	return fmt.Sprintf(`{"v":1,"seq":%d,"type":"block","kind":"%s","text":%s}`, seq, kind, t)
}

func mockFinal(seq int, proposals, options string) string {
	return fmt.Sprintf(`{"v":1,"seq":%d,"type":"final","proposals":[%s],"options":[%s]}`, seq, proposals, options)
}

func happyScript() []mock.Item {
	return []mock.Item{
		mock.Frame(mockBlock(1, "dialogue", "你还记得我们的约定吗？")),
		mock.Frame(mockBlock(2, "narration", "雨水沿着窗框流下。")),
		mock.Frame(mockFinal(3,
			`{"proposalId":"p1","type":"relationship_delta","characterId":"npc_elena","field":"trust","delta":2,"evidenceBlockSeqs":[1]}`,
			`{"optionId":"o1","intent":"clever","text":"告诉她，我仍记得那个约定。"}`)),
	}
}

// affectionScript 返回一轮脚本：一段正文 + 一条对 npc_elena 的好感增量。
func affectionScript(delta int) []mock.Item {
	return []mock.Item{
		mock.Frame(mockBlock(1, "narration", "她的神情缓和了一些。")),
		mock.Frame(mockFinal(2,
			`{"proposalId":"p1","type":"relationship_delta","characterId":"npc_elena","field":"affection","delta":`+
				strconv.Itoa(delta)+`}`, "")),
	}
}

func newTestServices(t *testing.T, script []mock.Item) (*sqlite.Store, *TurnService, *SessionService, string, string, string) {
	t.Helper()
	st, err := sqlite.Open(t.TempDir(), ports.RealClock{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	prov := mock.New(script)
	bus := NewEventBus(st)
	compiler := ctxpkg.New(st, ctxpkg.DefaultOptions())
	turnSvc := NewTurnService(st, prov, compiler, bus)
	t.Cleanup(turnSvc.Close)
	sessSvc := NewSessionService(st)

	res, err := sessSvc.Setup(context.Background(), &SessionSetupRequest{
		Title: "测试", CharacterJSON: testCard, Player: Player{Name: "旅人"},
		OpeningText: "你推开酒馆的门，老板娘抬起了头。",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return st, turnSvc, sessSvc, res.Session.SessionID, res.Branch.BranchID, res.RootNode.NodeID
}

// asyncWaitBudget 是"等异步工作落到预期状态"的预算。
var asyncWaitBudget = func() time.Duration {
	if raw := os.Getenv("TAVERNAGENT_TEST_WAIT_SECONDS"); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return 60 * time.Second
}()

func waitTurn(t *testing.T, svc *TurnService, turnID string, want ...domain.TurnStatus) *domain.TurnRequest {
	t.Helper()
	deadline := time.Now().Add(asyncWaitBudget)
	for time.Now().Before(deadline) {
		tr, err := svc.Get(turnID)
		if err != nil {
			t.Fatalf("get turn: %v", err)
		}
		for _, w := range want {
			if tr.Status == w {
				return tr
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("turn %s did not reach %v (last %s %s)", turnID, want, trFromLast(t), lastStatus(t, svc, turnID))
	return nil
}

func lastStatus(t *testing.T, svc *TurnService, turnID string) domain.TurnStatus {
	tr, err := svc.Get(turnID)
	if err != nil {
		return domain.TurnStatus("err:" + err.Error())
	}
	return tr.Status
}

func trFromLast(t *testing.T) string { return "n/a" }

type cognitiveProviderFunc func(context.Context, ports.ChatRequest, ports.StreamSink) error

func (f cognitiveProviderFunc) Capabilities(context.Context) (ports.ProviderCapabilities, error) {
	return ports.ProviderCapabilities{}, nil
}

func (f cognitiveProviderFunc) Stream(ctx context.Context, r ports.ChatRequest, sink ports.StreamSink) error {
	return f(ctx, r, sink)
}

// blockingProvider 阻塞在 release/ctx 上，用于测试取消（T09 相关路径）。
type blockingProvider struct {
	release chan struct{}
}

func (b *blockingProvider) Capabilities(ctx context.Context) (ports.ProviderCapabilities, error) {
	return ports.ProviderCapabilities{ID: "blocking", Streaming: true}, nil
}

func (b *blockingProvider) Stream(ctx context.Context, req ports.ChatRequest, sink ports.StreamSink) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.release:
		return nil
	}
}

// acceptAndWait 以分支当前头为基准受理一轮并等待提交。
func acceptAndWait(t *testing.T, svc *TurnService, st *sqlite.Store, sessionID, branchID, key, text string) *domain.TurnRequest {
	t.Helper()
	return acceptAndWaitWith(t, svc, st, sessionID, branchID, key, domain.TurnInput{Kind: "text", Text: text})
}

func acceptAndWaitWith(t *testing.T, svc *TurnService, st *sqlite.Store, sessionID, branchID, key string, in domain.TurnInput) *domain.TurnRequest {
	t.Helper()
	br, err := st.GetBranch(branchID)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	tr, err := svc.Accept(context.Background(), sessionID, branchID, &TurnAcceptRequest{
		IdempotencyKey: key, ExpectedHeadID: br.HeadNodeID, ExpectedVersion: br.Version,
		Input: in,
	})
	if err != nil {
		t.Fatalf("accept %s: %v", key, err)
	}
	return waitTurn(t, svc, tr.TurnID, domain.TurnCommitted)
}
