package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/util/id"
)

// 摘要维护的受阻退避（借鉴 Reasonix 的 storm breaker）：
// 同一个错误连续 N 次说明"再试一次"不会变好（典型是模型持续写错摘要结构），
// 继续每回合重付一次完整生成没有任何收益。进入冷却后等分支头前进或冷却到期。
const (
	summaryStormThreshold = 3
	summaryStormCooldown  = 5 * time.Minute
)

// summaryFailure 记录某个分支最近的摘要失败特征，用于触发受阻退避。
type summaryFailure struct {
	signature string
	count     int
	head      string
	until     time.Time
}

// CompactorService creates independent summaries of immutable source ranges.
type CompactorService struct {
	store         ports.Store
	pm            *ProviderManager
	bus           *EventBus
	policy        ctxpkg.CompactionPolicy
	queue         *coalescingQueue
	pressureMu    sync.Mutex
	pressureHeads map[string]string
	pressureTails map[string]int
	blockedMu     sync.Mutex
	blocked       map[string]summaryFailure
	metrics       *RuntimeMetrics
	now           func() time.Time
}

func NewCompactorService(store ports.Store, pm *ProviderManager, bus *EventBus, policy ctxpkg.CompactionPolicy) *CompactorService {
	return &CompactorService{
		store: store, pm: pm, bus: bus, policy: policy,
		// 摘要与记忆抽取的取消语义不同：记忆"最新状态胜出"，在途结果被取代就该取消；
		// 摘要的产物按不可变区间键控，晚到一样有效，取消只是白付一次生成费用。
		queue:         newCoalescingQueue(1, 250*time.Millisecond, false),
		pressureHeads: map[string]string{}, pressureTails: map[string]int{},
		blocked: map[string]summaryFailure{}, now: time.Now,
	}
}

// SetMetrics 注入运行读数（可观测：预算淘汰、摘要归一/受阻都要能被看到）。
func (c *CompactorService) SetMetrics(metrics *RuntimeMetrics) {
	if c == nil {
		return
	}
	c.metrics = metrics
	c.queue.metrics = metrics
}

// normalizeSummaryErrorSignature 把错误文本压成"同类失败"的签名。
//
// 必须把数字抹平：像 `summary not smaller than its source range: summary=528
// source=517` 这类错误每次的数字都不同，逐字比较会让同一个结构性问题永远累计
// 不到阈值，退避就形同虚设（模型每次都在同一个地方失败，却一直在重付）。
func normalizeSummaryErrorSignature(message string) string {
	var b strings.Builder
	inDigits := false
	for _, r := range message {
		if r >= '0' && r <= '9' {
			if !inDigits {
				b.WriteByte('#')
				inDigits = true
			}
			continue
		}
		inDigits = false
		b.WriteRune(r)
	}
	signature := b.String()
	if len(signature) > 200 {
		signature = signature[:200]
	}
	return signature
}

// summaryBlocked 判断该分支当前是否处于受阻冷却期。
// 分支头前进说明区间变了，允许立刻重试（旧失败不再代表这次会失败）。
func (c *CompactorService) summaryBlocked(branchID, headNodeID string) bool {
	c.blockedMu.Lock()
	defer c.blockedMu.Unlock()
	state, ok := c.blocked[branchID]
	if !ok || state.until.IsZero() || !c.now().Before(state.until) {
		return false
	}
	if state.head != headNodeID {
		delete(c.blocked, branchID)
		return false
	}
	return true
}

// recordSummaryFailure 累计同一错误的连续次数，达到阈值进入冷却。
// 返回是否"刚刚进入冷却"（只在这时打一条 WARN，避免刷屏）。
func (c *CompactorService) recordSummaryFailure(branchID, headNodeID string, err error) bool {
	if err == nil {
		return false
	}
	signature := normalizeSummaryErrorSignature(err.Error())
	c.blockedMu.Lock()
	defer c.blockedMu.Unlock()
	state := c.blocked[branchID]
	if state.signature == signature && state.head == headNodeID {
		state.count++
	} else {
		state = summaryFailure{signature: signature, count: 1, head: headNodeID}
	}
	entering := state.count >= summaryStormThreshold && state.until.IsZero()
	if state.count >= summaryStormThreshold {
		state.until = c.now().Add(summaryStormCooldown)
	}
	c.blocked[branchID] = state
	if c.metrics != nil && state.count >= summaryStormThreshold {
		c.metrics.addSummaryBlocked()
	}
	return entering
}

func (c *CompactorService) recordSummarySuccess(branchID string) {
	c.blockedMu.Lock()
	delete(c.blocked, branchID)
	c.blockedMu.Unlock()
}

func (c *CompactorService) Close() {
	if c != nil {
		c.queue.Close()
	}
}

func (c *CompactorService) TriggerAsync(sessionID, branchID, headNodeID string) {
	c.trigger(sessionID, branchID, headNodeID, false)
}

func (c *CompactorService) TriggerPressure(sessionID, branchID, headNodeID string, protectedTurns ...int) {
	if c != nil && len(protectedTurns) > 0 {
		c.pressureMu.Lock()
		c.pressureTails[branchID] = protectedTurns[0]
		c.pressureMu.Unlock()
	}
	c.trigger(sessionID, branchID, headNodeID, true)
}

func (c *CompactorService) trigger(sessionID, branchID, headNodeID string, pressure bool) {
	if c == nil || c.pm == nil {
		return
	}
	c.pressureMu.Lock()
	pressure = pressure || c.pressureHeads[branchID] != ""
	if pressure {
		c.pressureHeads[branchID] = headNodeID
	}
	c.pressureMu.Unlock()
	c.queue.Submit(branchID, func(ctx context.Context) {
		art, err := c.runCompaction(ctx, sessionID, branchID, headNodeID, pressure)
		if err != nil && ctx.Err() == nil {
			slog.Warn("summary maintenance failed", "branchId", branchID, "error", err)
		}
		if art != nil {
			c.pressureMu.Lock()
			if c.pressureHeads[branchID] == headNodeID {
				delete(c.pressureHeads, branchID)
				delete(c.pressureTails, branchID)
			}
			c.pressureMu.Unlock()
		}
	})
}

func (c *CompactorService) RunOnce(ctx context.Context, sessionID, branchID, headNodeID string) (*domain.SummaryArtifact, error) {
	return c.runCompaction(ctx, sessionID, branchID, headNodeID, false)
}

// runCompaction 是摘要维护的入口：先看受阻冷却，再执行，并按结果记账。
//
// blocked 的状态在成功、分支头前进或冷却到期时清除；这样"同一错误连续失败"
// 不会变成每回合重付一次完整生成，但换了区间（头前进）就立刻恢复尝试。
func (c *CompactorService) runCompaction(ctx context.Context, sessionID, branchID, headNodeID string, pressure bool) (*domain.SummaryArtifact, error) {
	if c.summaryBlocked(branchID, headNodeID) {
		slog.Debug("summary maintenance blocked", "branchId", branchID, "head", headNodeID)
		return nil, nil
	}
	art, err := c.runCompactionOnce(ctx, sessionID, branchID, headNodeID, pressure)
	if err != nil && ctx.Err() == nil {
		if c.recordSummaryFailure(branchID, headNodeID, err) {
			slog.Warn("summary maintenance blocked after repeated failure",
				"branchId", branchID, "cooldown", summaryStormCooldown.String(), "error", err)
		}
	} else if err == nil {
		c.recordSummarySuccess(branchID)
	}
	return art, err
}

func (c *CompactorService) runCompactionOnce(ctx context.Context, sessionID, branchID, headNodeID string, pressure bool) (*domain.SummaryArtifact, error) {
	branch, err := c.store.GetBranch(branchID)
	if err != nil {
		return nil, err
	}
	if branch.SessionID != sessionID {
		return nil, errors.New("summary session mismatch")
	}
	if ok, err := c.store.IsAncestor(headNodeID, branch.HeadNodeID); err != nil {
		return nil, err
	} else if !ok {
		return nil, errors.New("summary head is outside branch")
	}
	prov, cfg, err := c.pm.Resolve("reflection", true)
	if err != nil || prov == nil {
		return nil, err
	}
	ancestors, err := c.store.AncestorChain(headNodeID, true)
	if err != nil {
		return nil, err
	}
	sums, err := c.store.SummariesOnPath(headNodeID)
	if err != nil {
		return nil, err
	}
	policy := c.policy
	snap, err := c.store.StateAt(headNodeID)
	if err != nil {
		return nil, err
	}
	state, err := domain.UnmarshalWorld(snap.StateJSON)
	if err != nil {
		return nil, err
	}
	primaryCfg := c.pm.ConfigSnapshot("primary")
	if primaryCfg.ContextWindow <= 0 {
		primaryCfg = cfg
	}
	opts := ctxpkg.DefaultOptions()
	opts.CompactionPolicy = policy
	compiler := ctxpkg.New(c.store, opts).WithBudget(primaryCfg.ContextWindow, primaryCfg.MaxTokens)
	policy, err = compiler.CompactionPolicyAt(ctx, sessionID, headNodeID, ancestors, state)
	if err != nil {
		return nil, err
	}
	if pressure {
		policy.MinUncompactedTurns = 1
		c.pressureMu.Lock()
		if tail := c.pressureTails[branchID]; tail > 0 && c.pressureHeads[branchID] == headNodeID {
			policy.TailWindowTurns = tail
		}
		c.pressureMu.Unlock()
	}
	decision := ctxpkg.DecideCompaction(ancestors, sums, policy)
	if !decision.ShouldCompact {
		return nil, nil
	}

	window, output := cfg.ContextWindow, cfg.MaxTokens
	if window <= 0 {
		window = 8192
	}
	if output <= 0 || output > 2048 {
		output = 2048
	}
	budget := window - output - max(512, window/20)
	// Include all nodes in the exact interval, including memory corrections.
	// Previous summaries belong to different source ranges and are not inputs.
	var sources []*domain.PlotNode
	var req ports.ChatRequest
	fromDepth := decision.NodesToFold[0].Depth
	toDepth := decision.NodesToFold[len(decision.NodesToFold)-1].Depth
	for _, n := range ancestors {
		if n.Depth < fromDepth || n.Depth > toDepth {
			continue
		}
		trial := append(append([]*domain.PlotNode{}, sources...), n)
		next := ctxpkg.BuildCompactionChatRequest("", "", "", nil, trial, state)
		if ctxpkg.MessageTokens(next.Messages) > budget {
			break
		}
		sources, req = trial, next
	}
	if len(sources) == 0 {
		return nil, Err("CONTEXT_OVER_BUDGET", "单个历史回合超出摘要模型输入预算，请调大 reflection 上下文窗口", 422)
	}
	// Do not pay for a summary that a later correction would immediately
	// invalidate. Wait until that correction can be part of the same range.
	records, err := pathMemories(c.store, headNodeID)
	if err != nil {
		return nil, err
	}
	depths := map[string]int{}
	for _, n := range ancestors {
		depths[n.NodeID] = n.Depth
	}
	byID := map[string]*domain.MemoryRecord{}
	for _, m := range records {
		byID[m.MemoryID] = m
	}
	endDepth := sources[len(sources)-1].Depth
	for _, revision := range records {
		original := byID[revision.Supersedes]
		if original != nil && depths[revision.SourceNodeID] > endDepth && depths[original.SourceNodeID] >= sources[0].Depth && depths[original.SourceNodeID] <= endDepth && (revision.Hidden || revision.Content != original.Content) {
			return nil, nil
		}
	}
	req.MaxTokens = output
	req.InputBudget = budget
	req.EstimatedInputTokens = ctxpkg.MessageTokens(req.Messages)
	req.Task = "summary"
	raw, err := summarizeOnce(ctx, prov, req)
	if err != nil {
		return nil, err
	}
	clean, normalized, err := ctxpkg.ParseAndNormalizeSummary(raw)
	if err != nil {
		// 一次修复重试：把校验器的具体错误回喂给模型（Reasonix 的做法—
		// 结构化错误回喂而不是自动改写），仍失败才按受阻处理。
		repaired, retryErr := repairSummary(ctx, prov, req, raw, err, budget)
		if retryErr != nil {
			return nil, retryErr
		}
		clean, normalized = repaired, true
	}
	if normalized && c.metrics != nil {
		c.metrics.addSummaryNormalized()
	}
	// 采纳侧要求"严格变小"（借鉴 Reasonix acceptCheckpointCandidate）：
	// 摘要若不比它替代的原文更省，压下来也没有意义，还会占着预算挤掉其他材料。
	if ctxpkg.EstimateTokens(clean) >= ctxpkg.EstimateTokens(renderCompactionSources(sources)) {
		return nil, fmt.Errorf("summary not smaller than its source range: summary=%d source=%d",
			ctxpkg.EstimateTokens(clean), ctxpkg.EstimateTokens(renderCompactionSources(sources)))
	}
	cfg.APIKey = ""
	configJSON, _ := json.Marshal(cfg)
	art := &domain.SummaryArtifact{
		SummaryID: id.New(), FromNodeID: sources[0].NodeID, ToNodeID: sources[len(sources)-1].NodeID,
		SourceHash: domain.SummarySourceHash(sources), Text: clean, SummaryVersion: 1,
		ModelConfigVersion: hashString(string(configJSON)),
	}
	if err := c.store.SaveSummary(art); err != nil {
		return nil, err
	}
	if c.bus != nil {
		_, err = c.bus.Publish(sessionID, "session.updated", map[string]any{
			"reason": "summary.created", "summaryId": art.SummaryID, "sessionId": sessionID,
			"branchId": branchID, "fromNodeId": art.FromNodeID, "toNodeId": art.ToNodeID,
		})
	}
	return art, err
}

// summarizeOnce 执行一次摘要流式调用，返回原始输出（≤32 KiB）。
func summarizeOnce(ctx context.Context, prov ports.ModelProvider, req ports.ChatRequest) (string, error) {
	var buf bytes.Buffer
	if err := prov.Stream(ctx, req, ports.OnChunkSink(func(chunk []byte) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if buf.Len()+len(chunk) > 32*1024 {
			return errors.New("summary response too large")
		}
		_, err := buf.Write(chunk)
		return err
	})); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// repairSummary 在结构校验失败后带着具体错误重试一次。
//
// 借鉴 Reasonix 的"未执行错误 + 逐条 violation 回喂"：不自动改写模型输出，
// 而是把"哪里不合法"讲清楚让它自己重写。只给一次机会——同一个错误连续出现
// 说明这次生成的方向不对，继续重试属于白付费（由受阻退避接手）。
func repairSummary(ctx context.Context, prov ports.ModelProvider, req ports.ChatRequest, firstOutput string, parseErr error, budget int) (string, error) {
	// 超预算就不追加（摘要输入本身已经吃满窗口时，追加会把请求顶爆）。
	extra := ctxpkg.EstimateTokens(firstOutput) + 200
	if req.EstimatedInputTokens > 0 && req.EstimatedInputTokens+extra > budget {
		return "", parseErr
	}
	retry := req
	retry.Messages = append(append([]ports.ChatMessage{}, req.Messages...),
		ports.ChatMessage{Role: "assistant", Content: firstOutput},
		ports.ChatMessage{Role: "user", Content: "上面的摘要不符合格式要求：" + parseErr.Error() +
			"。请只输出修正后的完整 <story_checkpoint> XML（章节只用 narrative_arc、character_dynamics、open_loops、milestones，" +
			"character_dynamics 下只用 mindset、hidden_tension），不要解释、不要 Markdown 围栏。"})
	retry.EstimatedInputTokens += extra
	raw, err := summarizeOnce(ctx, prov, retry)
	if err != nil {
		return "", parseErr
	}
	clean, _, err := ctxpkg.ParseAndNormalizeSummary(raw)
	if err != nil {
		return "", fmt.Errorf("摘要修复重试仍不合法：%v（首次错误：%v）", err, parseErr)
	}
	return clean, nil
}

// renderCompactionSources 把被摘要替代的原文渲染成可比较的文本，
// 用于"摘要必须严格小于来源"的采纳判定。
func renderCompactionSources(sources []*domain.PlotNode) string {
	var b strings.Builder
	for _, n := range sources {
		if n == nil {
			continue
		}
		b.WriteString(n.ContentJSON)
	}
	return b.String()
}
