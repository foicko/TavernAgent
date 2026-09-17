// 模型供应商管理：配置加载、槽位解析与运行时换模。
// 契约：保存配置只影响新尝试，不在同一条生成流中切换模型或端点（§12.3）。
package application

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"tavernagent/internal/ports"
	"tavernagent/internal/protocol"
)

// ProviderManager 持有槽位配置与已构建客户端，按任务模式解析供应商。
// 配置来源是"模型实例 + 槽位引用"（见 ports.ModelConfigStore）：槽位在 Reload 时
// 解析成运行时用的 ProviderConfig，Resolve 的语义与 v1 完全一致。
type ProviderManager struct {
	store    ports.ModelConfigStore
	build    ports.ProviderBuilder
	fallback ports.ModelProvider // 无任何配置时的开发兜底（如 mock）

	mu      sync.RWMutex
	cfgs    map[string]ports.ProviderConfig // slot -> unmasked 配置
	clients map[string]ports.ModelProvider  // slot -> 已构建客户端
	usage   ports.UsageStore
	// usageFailureSink 在台账写入失败时被调用（组合根接到 RuntimeMetrics）。
	// 与 usage 同生命周期：读它必须持有 mu（Resolve 已在锁内，直接读字段）。
	usageFailureSink func()
}

// NewProviderManager 创建管理器。build 由组合根注入（避免应用层依赖具体适配器）。
func NewProviderManager(store ports.ModelConfigStore, build ports.ProviderBuilder, fallback ports.ModelProvider) *ProviderManager {
	return &ProviderManager{
		store: store, build: build, fallback: fallback,
		cfgs: map[string]ports.ProviderConfig{}, clients: map[string]ports.ModelProvider{},
	}
}

// Reload 从存储加载实例与槽位并投影成运行时配置（启动或配置变更后调用）。
func (m *ProviderManager) Reload() error {
	catalog, err := m.store.LoadCatalog(false)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfgs = map[string]ports.ProviderConfig{}
	m.clients = map[string]ports.ModelProvider{}
	for _, binding := range catalog.Slots {
		m.cfgs[binding.Slot] = ports.ProviderConfig{
			Slot: binding.Slot, Enabled: binding.Enabled, Kind: binding.Kind, BaseURL: binding.BaseURL,
			Model: binding.Model, APIKey: binding.APIKey, HasAPIKey: binding.HasAPIKey, ModelID: binding.ResolvedModelID,
			Temperature: binding.Temperature, MaxTokens: binding.MaxTokens, ContextWindow: binding.ContextWindow,
			ReasoningEffort: binding.ReasoningEffort,
		}
	}
	return nil
}

// Configs 返回槽位配置的脱敏视图（API Key 只回末四位）。
func (m *ProviderManager) Configs() []ports.ProviderConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ports.ProviderConfig, 0, len(m.cfgs))
	for _, c := range m.cfgs {
		c.HasAPIKey = c.APIKey != ""
		c.APIKey = ports.MaskAPIKey(c.APIKey)
		out = append(out, c)
	}
	return out
}

func (m *ProviderManager) ConfigSnapshot(slot string) ports.ProviderConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfgs[slot]
}

// Catalog 返回实例与槽位的视图（masked=true 时密钥脱敏）。
func (m *ProviderManager) Catalog(masked bool) (*ports.ModelCatalog, error) {
	return m.store.LoadCatalog(masked)
}

// SaveModel 新建或更新模型实例并刷新缓存客户端（影响后续新尝试）。
func (m *ProviderManager) SaveModel(instance ports.ModelInstance) (ports.ModelInstance, error) {
	if err := validateInstance(instance); err != nil {
		return ports.ModelInstance{}, err
	}
	saved, err := m.store.SaveModel(instance)
	if err != nil {
		return ports.ModelInstance{}, modelConfigError(err, "保存模型实例")
	}
	// 任何槽位都可能引用这个实例：整体作废客户端缓存最省心，也不会漏。
	m.invalidateClients("")
	if err := m.Reload(); err != nil {
		return ports.ModelInstance{}, Err("STORAGE_UNAVAILABLE", "重新加载模型配置失败: "+err.Error(), 503)
	}
	return saved, nil
}

// DeleteModel 删除未被引用的实例；仍被槽位引用时返回 409（绝不悄悄改槽位）。
func (m *ProviderManager) DeleteModel(modelID string) error {
	if strings.TrimSpace(modelID) == "" {
		return Err("BAD_REQUEST", "缺少模型实例 ID", 400)
	}
	if err := m.store.DeleteModel(modelID); err != nil {
		return modelConfigError(err, "删除模型实例")
	}
	m.invalidateClients("")
	return m.Reload()
}

// SaveSlot 保存槽位指派（空 modelID 表示跟随主线）并刷新该槽位客户端。
func (m *ProviderManager) SaveSlot(slot string, enabled bool, modelID string) error {
	if !validSlot(slot) {
		return Err("BAD_SLOT", "未知模型槽位: "+slot, 422)
	}
	if err := m.store.SaveSlot(slot, enabled, strings.TrimSpace(modelID)); err != nil {
		return modelConfigError(err, "保存槽位配置")
	}
	m.invalidateClients(slot)
	return m.Reload()
}

func (m *ProviderManager) invalidateClients(slot string) {
	m.mu.Lock()
	if slot == "" {
		m.clients = map[string]ports.ModelProvider{}
	} else {
		delete(m.clients, slot)
	}
	m.mu.Unlock()
}

// modelConfigError 把存储层的错误映射成 API 错误身份。
func modelConfigError(err error, action string) error {
	var inUse *ports.ModelInUseError
	switch {
	case errors.As(err, &inUse):
		return Err("MODEL_IN_USE", "该模型实例正在被 "+strings.Join(inUse.Slots, "、")+" 使用，请先在这些槽位里换一个模型", 409)
	case errors.Is(err, ports.ErrModelNotFound):
		return Err("MODEL_NOT_FOUND", "模型实例不存在", 404)
	default:
		return Err("STORAGE_UNAVAILABLE", action+"失败: "+err.Error(), 503)
	}
}

// validateInstance 校验实例的必填字段与地址/预算约束（与存储层同规则，返回带码错误）。
func validateInstance(instance ports.ModelInstance) error {
	if strings.TrimSpace(instance.Name) == "" {
		return Err("PROVIDER_BAD_CONFIG", "实例名称不能为空", 422)
	}
	if strings.TrimSpace(instance.Model) == "" {
		return Err("MISSING_MODEL", "模型名不能为空", 422)
	}
	if instance.BaseURL != "" {
		u, err := url.Parse(instance.BaseURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
			return Err("PROVIDER_BAD_CONFIG", "模型地址必须是有效的 HTTP(S) 地址，凭据请填写在密钥栏", 422)
		}
	}
	if instance.MaxTokens < 0 || instance.ContextWindow < 0 || instance.ContextWindow > 2_000_000 || (instance.ContextWindow > 0 && instance.ContextWindow-instance.MaxTokens < 2048) {
		return Err("PROVIDER_BAD_CONFIG", "上下文窗口与输出预算不合法，至少需保留 2048 输入 token", 422)
	}
	if _, ok := ports.NormalizeEffort(instance.ReasoningEffort); !ok {
		return Err("PROVIDER_BAD_CONFIG", "思考强度只能是 low / medium / high（留空为默认）", 422)
	}
	return nil
}

func validSlot(slot string) bool {
	for _, known := range ports.KnownSlots() {
		if slot == known {
			return true
		}
	}
	return false
}

// Provider 按任务模式解析供应商：assist/reflection 缺省复用 primary；
// primary 未配置时回退开发兜底。每个回合开始时调用一次（运行时换模边界）。
func (m *ProviderManager) Provider(mode string) (ports.ModelProvider, error) {
	p, _, err := m.Resolve(mode, false)
	return p, err
}

// Resolve snapshots the provider and its budget together. Configuration changes
// apply to the next attempt, never between compilation and streaming.
func (m *ProviderManager) Resolve(mode string, background bool) (ports.ModelProvider, ports.ProviderConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	slot := slotForMode(mode)
	cfg, exists := m.cfgs[slot]
	if background && exists && !cfg.Enabled {
		return nil, cfg, nil
	}
	if !cfg.Enabled || cfg.Kind == "" {
		slot = string(ports.SlotPrimary)
		cfg = m.cfgs[slot]
	}
	if !cfg.Enabled || cfg.Kind == "" {
		if background {
			return nil, cfg, nil
		}
		if m.fallback != nil {
			if m.usage == nil {
				return m.fallback, cfg, nil
			}
			return &observedProvider{ModelProvider: m.fallback, config: ports.ProviderConfig{Slot: slot, Model: "mock", Kind: "mock"}, store: m.usage, onWriteFailure: m.usageFailureSink}, cfg, nil
		}
		return nil, cfg, Err("PROVIDER_NOT_CONFIGURED", "未配置任何模型供应商，请在设置中完成 primary 配置", 400)
	}
	if p := m.clients[slot]; p != nil {
		return p, cfg, nil
	}
	p, err := m.buildForSlot(cfg, m.usage, m.usageFailureSink)
	if err == nil {
		m.clients[slot] = p
	}
	return p, cfg, err
}

// BackgroundProvider honors an explicitly disabled slot. An absent slot may
// reuse the primary model; a development fallback never starts paid/background
// work implicitly and is handled by deterministic maintenance only.
func (m *ProviderManager) BackgroundProvider(mode string) (ports.ModelProvider, error) {
	p, _, err := m.Resolve(mode, true)
	return p, err
}

// ProbeResult 是连通性/格式测试结果（M1-5 设置模态接线）。
type ProbeResult struct {
	Slot         string `json:"slot"`
	OK           bool   `json:"ok"`
	Kind         string `json:"kind"`
	Model        string `json:"model"`
	ConnectMsg   string `json:"connectMsg,omitempty"`
	FormatTested bool   `json:"formatTested"`
	FormatOK     *bool  `json:"formatOk,omitempty"`
	FormatMsg    string `json:"formatMsg,omitempty"`
	LatencyMS    int64  `json:"latencyMs"`
}

// Probe 按槽位执行连通性（+可选格式）测试，不改变已保存配置。
func (m *ProviderManager) Probe(ctx context.Context, slot string, format bool) (*ProbeResult, error) {
	m.mu.RLock()
	cfg, ok := m.cfgs[slot]
	usage := m.usage
	sink := m.usageFailureSink
	m.mu.RUnlock()
	res := &ProbeResult{Slot: slot, FormatTested: format}
	if !ok || !cfg.Enabled {
		res.ConnectMsg = "该槽位未启用，请先在设置中指派一个模型实例"
		return res, nil
	}
	m.probe(ctx, cfg, usage, sink, res, format)
	return res, nil
}

// ProbeModel 按实例探测（与槽位无关）：设置页里测的是"当前正在编辑的这个实例"。
func (m *ProviderManager) ProbeModel(ctx context.Context, modelID string, format bool) (*ProbeResult, error) {
	catalog, err := m.store.LoadCatalog(false)
	if err != nil {
		return nil, modelConfigError(err, "读取模型实例")
	}
	var instance *ports.ModelInstance
	for i := range catalog.Models {
		if catalog.Models[i].ID == modelID {
			instance = &catalog.Models[i]
			break
		}
	}
	if instance == nil {
		return nil, Err("MODEL_NOT_FOUND", "模型实例不存在", 404)
	}
	m.mu.RLock()
	usage := m.usage
	sink := m.usageFailureSink
	m.mu.RUnlock()
	res := &ProbeResult{Slot: "probe", Kind: instance.Kind, Model: instance.Model, FormatTested: format}
	m.probe(ctx, ports.ProviderConfig{
		Slot: "probe", Enabled: true, Kind: instance.Kind, BaseURL: instance.BaseURL, Model: instance.Model,
		APIKey: instance.APIKey, ModelID: instance.ID, Temperature: instance.Temperature,
		MaxTokens: instance.MaxTokens, ContextWindow: instance.ContextWindow,
		ReasoningEffort: instance.ReasoningEffort,
	}, usage, sink, res, format)
	return res, nil
}

// probe 是探测的共享实现：不落库、不改配置、不进客户端缓存。
func (m *ProviderManager) probe(ctx context.Context, cfg ports.ProviderConfig, usage ports.UsageStore, sink func(), res *ProbeResult, format bool) {
	if cfg.BaseURL == "" || cfg.Model == "" {
		res.ConnectMsg = "缺少 Base URL 或模型名，无法连通性测试"
		return
	}
	p, err := m.buildForSlot(cfg, usage, sink)
	if err != nil {
		res.ConnectMsg = err.Error()
		res.Kind = cfg.Kind
		res.Model = cfg.Model
		return
	}
	start := time.Now()
	prompt := "连通性测试：请只回复 OK。"
	var systemPrompt string
	if format {
		systemPrompt = frameProbeSystem
		prompt = "请执行协议格式输出测试：严格只按系统提示要求输出两行 JSON，不要附带任何前导文字或解释。"
	}
	msgs := make([]ports.ChatMessage, 0, 2)
	if systemPrompt != "" {
		msgs = append(msgs, ports.ChatMessage{Role: "system", Content: systemPrompt})
	}
	msgs = append(msgs, ports.ChatMessage{Role: "user", Content: prompt})

	var sb strings.Builder
	connectErr := p.Stream(ctx, ports.ChatRequest{
		Task: "probe", Model: "probe", MaxTokens: 256, Messages: msgs,
	}, ports.OnChunkSink(func(chunk []byte) error {
		if sb.Len() < 64<<10 { // 探测只用于校验，避免异常供应商刷屏
			sb.Write(chunk)
		}
		return nil
	}))
	res.LatencyMS = time.Since(start).Milliseconds()
	res.Kind = cfg.Kind
	res.Model = cfg.Model

	if connectErr != nil {
		res.ConnectMsg = connectErr.Error()
		res.OK = false
		return
	}
	res.OK = true
	if format {
		ok, msg := checkFrameFormat(sb.String())
		res.FormatOK = &ok
		res.FormatMsg = msg
	}
}

// frameProbeSystem 是格式探测专用的提示词：要求供应商输出最小合法帧序列。
// 与 context.FrameProtocolInstruction 同源（此处内联避免应用层反向依赖编译包）。
const frameProbeSystem = `你是输出协议测试器。只输出以下两行 JSON，不要任何解释或代码围栏：
{"v":1,"seq":1,"type":"block","kind":"narration","speakerId":null,"text":"就绪"}
{"v":1,"seq":2,"type":"final","proposals":[],"options":[]}`

// checkFrameFormat 用一次性解析器做只读校验，判断供应商是否稳定输出帧协议。
// 不落库、不改配置；兼容模式降级视为格式不通过（说明该模型未遵守结构化协议）。
func checkFrameFormat(raw string) (bool, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false, "供应商未返回内容"
	}
	// 先清洗外部 markdown 围栏与空行
	lines := strings.Split(trimmed, "\n")
	var cleanedLines []string
	inCodeBlock := false
	for _, l := range lines {
		tl := strings.TrimSpace(l)
		if strings.HasPrefix(tl, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if tl == "" {
			continue
		}
		cleanedLines = append(cleanedLines, tl)
	}

	if len(cleanedLines) == 0 {
		return false, "供应商未返回有效行"
	}

	// 某些模型习惯输出前置客套说明（如 "好的，这是测试帧："），
	// 找到第一个以 "{" 开头的行作为有效帧序列起点
	startIdx := -1
	for i, line := range cleanedLines {
		if strings.HasPrefix(line, "{") {
			startIdx = i
			break
		}
	}

	if startIdx < 0 {
		return false, "模型返回纯文本，未遵守帧协议（运行时会走兼容模式）"
	}

	cleanedPayload := strings.Join(cleanedLines[startIdx:], "\n") + "\n"

	p := protocol.NewStreamParser()
	if err := p.Feed([]byte(cleanedPayload)); err != nil {
		return false, "帧格式不合法: " + clip(firstLine(err.Error()), 120)
	}
	if p.Mode() == protocol.ModeNarrative {
		return false, "模型返回纯文本，未遵守帧协议（运行时会走兼容模式）"
	}
	if _, err := p.Finish(); err != nil {
		if errors.Is(err, protocol.ErrMissingFinal) {
			if te := p.TailError(); te != nil {
				return false, "final 帧非法: " + clip(te.Error(), 150)
			}
			return false, "已输出正文但缺少 final 帧"
		}
		return false, "帧格式不合法: " + clip(err.Error(), 120)
	}
	if startIdx > 0 {
		return true, "帧协议校验通过（已过滤前置说明文本）"
	}
	return true, "帧协议校验通过"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// buildForSlot 按配置构建一次性客户端（不写入客户端缓存）。
func (m *ProviderManager) buildForSlot(cfg ports.ProviderConfig, usage ports.UsageStore, sink func()) (ports.ModelProvider, error) {
	if m.build == nil {
		return nil, Err("PROVIDER_NOT_CONFIGURED", "供应商构建器未注入", 400)
	}
	p, err := m.build(cfg)
	if err != nil {
		return nil, Err("PROVIDER_BAD_CONFIG", "供应商构建失败: "+err.Error(), 400)
	}
	if usage == nil {
		return p, nil
	}
	return &observedProvider{ModelProvider: p, config: cfg, store: usage, onWriteFailure: sink}, nil
}

// slotForMode 任务模式 → 槽位（assist/reflection 显式任务；其余走 primary）。
func slotForMode(mode string) string {
	switch mode {
	case "assist":
		return string(ports.SlotAssist)
	case "reflection":
		return string(ports.SlotReflection)
	default:
		return string(ports.SlotPrimary)
	}
}
