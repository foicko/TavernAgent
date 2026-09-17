package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"

	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/protocol"
)

// TurnService 编排回合生命周期（受理 → 准备 → 生成 → 校验 → 提交）。
type TurnService struct {
	store    ports.TurnDeps
	provider ports.ModelProvider // 静态供应商（兼容 M0 直连模式）
	manager  *ProviderManager    // 运行时换模（M1）：非 nil 时按模式解析
	compiler *ctxpkg.Compiler
	bus      *EventBus
	// rand 是随机源。注入而不是直接用全局随机：检定必须可确定性重放与测试，
	// 而且"重生成不重掷"这条契约要求需要一个能被替换的随机源来验证。
	rand     ports.Random
	checkMu  sync.Mutex
	acceptMu sync.Mutex
	// usage 是模型用量台账（观测数据）。nil 表示不采集（例如测试替身）。
	usage ports.UsageStore
	// metrics 是进程级运行计数器（观测数据）。nil 表示不计数。
	metrics *RuntimeMetrics
	// calibration 是用真实用量反推的 token 估算系数（见 calibration.go）。
	calibration tokenCalibration
	// ruleset 是当前生效的规则包。它同时是 **actionRef 的授权面**——
	// 未登记的动作一律不执行（契约 §4：actionRef 必须引用后端允许的动作）。
	ruleset domain.Ruleset

	compactor *CompactorService
	cognitive *CognitiveService

	// liveMu/live 记录在途回合的解析器，供刷新或断线重连的客户端取回
	// "正在写的这一段"（已完成的块走 durable 事件回放，不需要这里）。
	liveMu sync.Mutex
	live   map[string]*liveStream

	semMu       sync.Mutex
	sem         map[string]chan struct{} // branchID → 容量 1
	cancelMu    sync.Mutex
	cancels     map[string]*turnCancel
	workCtx     context.Context
	stopWork    context.CancelFunc
	lifecycleMu sync.Mutex
	closing     bool
	workers     sync.WaitGroup
	closeOnce   sync.Once
}

func NewTurnService(store ports.Store, provider ports.ModelProvider, compiler *ctxpkg.Compiler, bus *EventBus) *TurnService {
	workCtx, stopWork := context.WithCancel(context.Background())
	return &TurnService{
		store: store, provider: provider, compiler: compiler, bus: bus,
		// usage 单独持有而不并入 ports.TurnDeps：避免扩大既有窄接口，
		// 从而不破坏任何已有的测试替身。
		usage: store,
		rand:  ports.RealRandom{}, ruleset: domain.DefaultRuleset(),
		sem: map[string]chan struct{}{}, cancels: map[string]*turnCancel{},
		workCtx: workCtx, stopWork: stopWork,
	}
}

// SetRuleset 替换生效的规则包（规则升级入口；契约 §7 要求升级通过显式配置）。
func (s *TurnService) SetRuleset(rs domain.Ruleset) { s.ruleset = rs }

// SetRandom 替换随机源（测试与确定性重放用）。
func (s *TurnService) SetRandom(r ports.Random) { s.rand = r }

func (s *TurnService) Metrics() *RuntimeMetrics { return s.metrics }

// NewTurnServiceWithManager 使用提供商管理器（配置驱动、运行时换模）。
func NewTurnServiceWithManager(store ports.Store, manager *ProviderManager, compiler *ctxpkg.Compiler, bus *EventBus) *TurnService {
	s := NewTurnService(store, nil, compiler, bus)
	s.manager = manager
	if manager != nil {
		manager.SetUsageStore(store)
		s.compactor = NewCompactorService(store, manager, bus, ctxpkg.DefaultCompactionPolicy())
		s.cognitive = NewCognitiveService(store, func() (ports.ModelProvider, error) { return manager.BackgroundProvider("reflection") })
		s.cognitive.resolve = func() (ports.ModelProvider, ports.ProviderConfig, error) { return manager.Resolve("reflection", true) }
	}
	return s
}

// resolveProvider 在回合生成开始时解析供应商（新尝试才生效，不中断同一条流）。
func (s *TurnService) resolveProvider(mode string) (ports.ModelProvider, error) {
	if s.manager != nil {
		return s.manager.Provider(mode)
	}
	if s.provider == nil {
		return nil, Err("PROVIDER_NOT_CONFIGURED", "未配置模型供应商", 400)
	}
	return s.provider, nil
}

// Get 查询回合持久状态（重启恢复用）。
func (s *TurnService) Get(turnID string) (*domain.TurnRequest, error) {
	return s.store.GetTurn(turnID)
}

// Cancel 请求取消。可重复调用；若已提交则返回 committed 终态。
// 截断续讲态的"放弃"也走 Cancel：草稿作废并释放分支锁。
func (s *TurnService) Cancel(ctx context.Context, turnID string) (*domain.TurnRequest, error) {
	turn, changed, err := s.store.TransitionTurn(ports.TurnTransition{TurnID: turnID, Status: domain.TurnCancelled, FailureMessage: "已取消"})
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "取消失败: "+err.Error(), 503)
	}
	s.cancelMu.Lock()
	c := s.cancels[turnID]
	s.cancelMu.Unlock()
	if turn.Status == domain.TurnCancelled && c != nil {
		c.cancel()
	}
	if changed {
		s.metrics.addCancelled()
		s.bus.BroadcastOnly(&domain.OutboxEvent{AggregateID: turnID, Type: "turn.state.updated"})
	}
	return turn, nil
}

func newCommittedEvent(turnID string) *domain.OutboxEvent {
	return &domain.OutboxEvent{EventID: turnID + ":live", AggregateID: turnID, Type: "turn.committed", PayloadJSON: `{}`}
}

func frameSeq(frame any) int {
	switch f := frame.(type) {
	case protocol.BlockFrame:
		return f.Seq
	case protocol.FinalFrame:
		return f.Seq
	}
	return 0
}

func frameJSON(frame any) string {
	b, _ := json.Marshal(frame)
	return string(b)
}

// inputOf 解析持久化的回合输入载荷。
func inputOf(turn *domain.TurnRequest) domain.TurnInput {
	var in domain.TurnInput
	_ = json.Unmarshal([]byte(turn.InputJSON), &in)
	return in
}

func inputTextOf(turn *domain.TurnRequest) string {
	return inputOf(turn).Text
}

// rulesetOf 返回本次尝试采用的规则版本。
// 迁移前的旧回合没有记录，回退到当前常量（T20 之前的历史数据）。
func rulesetOf(turn *domain.TurnRequest) string {
	if turn != nil && turn.RulesetVersion != "" {
		return turn.RulesetVersion
	}
	return RulesetVersion
}

func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// contextCodeOf 把编译层错误的协议码透传到回合终态。
// 预算不足（CONTEXT_OVER_BUDGET）与上下文构造失败是不同的问题，
// 混用同一个码会让排查变成猜谜。
func contextCodeOf(err error) string {
	var ce *ctxpkg.Error
	if errors.As(err, &ce) && ce.Code != "" {
		return ce.Code
	}
	return "CONTEXT_FAILED"
}
