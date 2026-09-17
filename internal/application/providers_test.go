package application

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"tavernagent/internal/ports"
)

// fakeConfigStore 是内存配置存储（测试替身）：模型实例 + 槽位引用。
type fakeConfigStore struct {
	mu      sync.Mutex
	models  map[string]ports.ModelInstance
	slots   map[string]ports.SlotBinding
	nextSeq int
}

func newFakeConfigStore() *fakeConfigStore {
	return &fakeConfigStore{models: map[string]ports.ModelInstance{}, slots: map[string]ports.SlotBinding{}}
}

// fakeStoreWithSlot 构造"某槽位已绑定某实例"的存储：旧测试写法（直接给槽位配置）的最短替身。
// 实例必须有模型名（真实存储的必填校验），旧写法没写就补一个占位名。
func fakeStoreWithSlot(slot string, cfg ports.ProviderConfig) *fakeConfigStore {
	if cfg.Model == "" {
		cfg.Model = "test-model"
	}
	store := newFakeConfigStore()
	instance, err := store.SaveModel(ports.ModelInstance{
		Name: slot, Kind: cfg.Kind, BaseURL: cfg.BaseURL, Model: cfg.Model, APIKey: cfg.APIKey,
		Temperature: cfg.Temperature, MaxTokens: cfg.MaxTokens, ContextWindow: cfg.ContextWindow,
	})
	if err != nil {
		panic(err)
	}
	if err := store.SaveSlot(slot, cfg.Enabled, instance.ID); err != nil {
		panic(err)
	}
	return store
}

func (f *fakeConfigStore) LoadCatalog(masked bool) (*ports.ModelCatalog, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cat := &ports.ModelCatalog{}
	for _, m := range f.models {
		view := m
		if masked {
			view.APIKey = ports.MaskAPIKey(view.APIKey)
		}
		view.HasAPIKey = m.APIKey != ""
		cat.Models = append(cat.Models, view)
	}
	primary := f.slots[string(ports.SlotPrimary)]
	for _, slot := range ports.KnownSlots() {
		binding := f.slots[slot]
		view := ports.SlotBinding{Slot: slot, Enabled: binding.Enabled, ModelID: binding.ModelID}
		target := binding.ModelID
		if target == "" {
			target = primary.ModelID
		}
		if m, ok := f.models[target]; ok {
			view.ResolvedModelID = m.ID
			view.Kind, view.BaseURL, view.Model = m.Kind, m.BaseURL, m.Model
			view.Temperature, view.MaxTokens, view.ContextWindow = m.Temperature, m.MaxTokens, m.ContextWindow
			view.ReasoningEffort = m.ReasoningEffort
			view.HasAPIKey = m.APIKey != ""
			if masked {
				view.APIKey = ports.MaskAPIKey(m.APIKey)
			} else {
				view.APIKey = m.APIKey
			}
		}
		cat.Slots = append(cat.Slots, view)
	}
	return cat, nil
}

func (f *fakeConfigStore) SaveModel(instance ports.ModelInstance) (ports.ModelInstance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if instance.ID == "" {
		f.nextSeq++
		instance.ID = "m_test_" + string(rune('a'+f.nextSeq-1))
	}
	instance.HasAPIKey = instance.APIKey != ""
	stored := instance
	masked := instance
	masked.APIKey = ports.MaskAPIKey(instance.APIKey)
	f.models[stored.ID] = stored
	return masked, nil
}

func (f *fakeConfigStore) DeleteModel(modelID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.models[modelID]; !ok {
		return ports.ErrModelNotFound
	}
	inUse := []string{}
	for slot, binding := range f.slots {
		if binding.ModelID == modelID {
			inUse = append(inUse, slot)
		}
	}
	if len(inUse) > 0 {
		return &ports.ModelInUseError{Slots: inUse}
	}
	delete(f.models, modelID)
	return nil
}

func (f *fakeConfigStore) SaveSlot(slot string, enabled bool, modelID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if modelID != "" {
		if _, ok := f.models[modelID]; !ok {
			return ports.ErrModelNotFound
		}
	}
	f.slots[slot] = ports.SlotBinding{Slot: slot, Enabled: enabled, ModelID: modelID}
	return nil
}

// countingProvider 记录构建次数与槽位，验证客户端缓存与失效。
type countingProvider struct {
	slot   string
	stream bool
}

func (c *countingProvider) Capabilities(ctx context.Context) (ports.ProviderCapabilities, error) {
	return ports.ProviderCapabilities{ID: c.slot, Streaming: c.stream}, nil
}

func (c *countingProvider) Stream(ctx context.Context, req ports.ChatRequest, sink ports.StreamSink) error {
	return nil
}

func TestProviderManagerSlotResolution(t *testing.T) {
	store := fakeStoreWithSlot("primary", ports.ProviderConfig{Slot: "primary", Enabled: true, Kind: "mock"})
	var builds int
	mgr := NewProviderManager(store, func(cfg ports.ProviderConfig) (ports.ModelProvider, error) {
		builds++
		return &countingProvider{slot: cfg.Slot}, nil
	}, nil)
	if err := mgr.Reload(); err != nil {
		t.Fatal(err)
	}

	// 未配置槽位 → 回落 primary。
	p, err := mgr.Provider("structured")
	if err != nil {
		t.Fatal(err)
	}
	if cp, ok := p.(*countingProvider); !ok || cp.slot != "primary" {
		t.Fatalf("provider = %#v", p)
	}
	// assist 任务同回 primary。
	p2, err := mgr.Provider("assist")
	if err != nil {
		t.Fatal(err)
	}
	if cp, ok := p2.(*countingProvider); !ok || cp.slot != "primary" {
		t.Fatalf("assist→primary 失败: %#v", p2)
	}
	// 同一个已缓存客户端复用（builds 不再增长）。
	p3, _ := mgr.Provider("structured")
	if p3 != p {
		t.Fatalf("客户端未缓存复用")
	}
	if builds != 1 {
		t.Fatalf("builds = %d, want 1", builds)
	}
}

func TestProviderManagerFallback(t *testing.T) {
	store := newFakeConfigStore()
	mockP := &countingProvider{slot: "fallback"}
	mgr := NewProviderManager(store, nil, mockP)
	if err := mgr.Reload(); err != nil {
		t.Fatal(err)
	}
	p, err := mgr.Provider("structured")
	if err != nil {
		t.Fatal(err)
	}
	if p != mockP {
		t.Fatalf("应回退到开发兜底")
	}
}

func TestProviderManagerSaveInvalidates(t *testing.T) {
	store := fakeStoreWithSlot("primary", ports.ProviderConfig{Slot: "primary", Enabled: true, Kind: "mock"})
	var builds int
	mgr := NewProviderManager(store, func(cfg ports.ProviderConfig) (ports.ModelProvider, error) {
		builds++
		return &countingProvider{slot: cfg.Slot}, nil
	}, nil)
	_ = mgr.Reload()
	_, _ = mgr.Provider("structured")
	if builds != 1 {
		t.Fatalf("builds = %d", builds)
	}
	// 改实例 → 客户端失效重建。
	catalog, _ := store.LoadCatalog(false)
	instance := catalog.Models[0]
	instance.Kind = "openai-compatible"
	if _, err := mgr.SaveModel(instance); err != nil {
		t.Fatal(err)
	}
	_, _ = mgr.Provider("structured")
	if builds < 2 {
		t.Fatalf("SaveModel 后客户端未重建: builds = %d", builds)
	}
	// 配置视图已脱敏。
	for _, c := range mgr.Configs() {
		if c.APIKey != "" && c.APIKey == "sk-"+c.APIKey[3:] {
			t.Fatalf("未脱敏: %q", c.APIKey)
		}
	}
}

// A3：格式探测必须给出真实结论，不能是硬编码的通过。
func TestCheckFrameFormat(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		ok   bool
	}{
		{
			name: "合法帧序列",
			raw: `{"v":1,"seq":1,"type":"block","kind":"narration","speakerId":null,"text":"就绪"}
{"v":1,"seq":2,"type":"final","proposals":[],"options":[]}
`,
			ok: true,
		},
		{
			name: "带 markdown 代码围栏的合法帧",
			raw: "```json\n" +
				`{"v":1,"seq":1,"type":"block","kind":"narration","speakerId":null,"text":"就绪"}` + "\n" +
				`{"v":1,"seq":2,"type":"final","proposals":[],"options":[]}` + "\n" +
				"```\n",
			ok: true,
		},
		{
			name: "带前导对话文本的合法帧",
			raw: "好的，以下是为您生成的协议测试数据：\n" +
				`{"v":1,"seq":1,"type":"block","kind":"narration","speakerId":null,"text":"就绪"}` + "\n" +
				`{"v":1,"seq":2,"type":"final","proposals":[],"options":[]}` + "\n",
			ok: true,
		},
		{
			name: "纯文本（走兼容模式）",
			raw:  "这是普通散文，不是帧。\n",
			ok:   false,
		},
		{
			name: "缺少 final",
			raw:  `{"v":1,"seq":1,"type":"block","kind":"narration","speakerId":null,"text":"就绪"}` + "\n",
			ok:   false,
		},
		{
			name: "非法帧尝试",
			raw:  `{"v":1,"seq":1,"type":"block","kind":"theater","text":"x"}` + "\n",
			ok:   false,
		},
		{
			name: "空响应",
			raw:  "   ",
			ok:   false,
		},
	}
	for _, c := range cases {
		ok, msg := checkFrameFormat(c.raw)
		if ok != c.ok {
			t.Fatalf("%s: ok = %v (%s), want %v", c.name, ok, msg, c.ok)
		}
		if !ok && msg == "" {
			t.Fatalf("%s: 失败时必须给出原因", c.name)
		}
	}
}

func TestProbeFormatFlags(t *testing.T) {
	store := fakeStoreWithSlot("primary", ports.ProviderConfig{Slot: "primary", Enabled: true, Kind: "openai-chat", BaseURL: "http://mock", Model: "m1"})
	mgr := NewProviderManager(store, func(cfg ports.ProviderConfig) (ports.ModelProvider, error) {
		return &countingProvider{slot: cfg.Slot}, nil
	}, nil)
	_ = mgr.Reload()

	// 仅连通性测试：formatTested 应为 false，formatOk 应为 nil
	res, err := mgr.Probe(context.Background(), "primary", false)
	if err != nil {
		t.Fatal(err)
	}
	if res.FormatTested {
		t.Errorf("expected formatTested = false, got true")
	}
	if res.FormatOK != nil {
		t.Errorf("expected formatOk = nil, got %v", *res.FormatOK)
	}
}

// 实例被槽位引用时删除必须被拒绝，并告诉用户是哪些槽位在用。
func TestDeleteModelInUseIsRefused(t *testing.T) {
	store := newFakeConfigStore()
	instance, err := store.SaveModel(ports.ModelInstance{Name: "A", Kind: "openai-chat", BaseURL: "http://x", Model: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	mgr := NewProviderManager(store, func(ports.ProviderConfig) (ports.ModelProvider, error) {
		return &countingProvider{slot: "p"}, nil
	}, nil)
	if err := mgr.SaveSlot("primary", true, instance.ID); err != nil {
		t.Fatalf("save slot: %v", err)
	}
	err = mgr.DeleteModel(instance.ID)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "MODEL_IN_USE" || apiErr.StatusCode != 409 {
		t.Fatalf("应当返回 409 MODEL_IN_USE，实际 %v", err)
	}
	if !strings.Contains(apiErr.Message, "primary") {
		t.Fatalf("提示里应指出是哪个槽位在用：%s", apiErr.Message)
	}
	if err := mgr.SaveSlot("primary", false, ""); err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteModel(instance.ID); err != nil {
		t.Fatalf("解除引用后应当可以删除: %v", err)
	}
}

func TestSaveSlotAndProbeModelRejectMissingInstance(t *testing.T) {
	store := newFakeConfigStore()
	mgr := NewProviderManager(store, func(ports.ProviderConfig) (ports.ModelProvider, error) {
		return &countingProvider{slot: "p"}, nil
	}, nil)
	var apiErr *APIError
	if err := mgr.SaveSlot("primary", true, "m_missing"); !errors.As(err, &apiErr) || apiErr.Code != "MODEL_NOT_FOUND" {
		t.Fatalf("引用不存在的实例应当 404，实际 %v", err)
	}
	if err := mgr.SaveSlot("nope", true, ""); !errors.As(err, &apiErr) || apiErr.Code != "BAD_SLOT" {
		t.Fatalf("未知槽位应当 422 BAD_SLOT，实际 %v", err)
	}
	if _, err := mgr.ProbeModel(context.Background(), "m_missing", false); !errors.As(err, &apiErr) || apiErr.Code != "MODEL_NOT_FOUND" {
		t.Fatalf("探测不存在的实例应当 404，实际 %v", err)
	}
}

func TestProbeModelUsesTheInstanceWithoutTouchingSlots(t *testing.T) {
	store := newFakeConfigStore()
	instance, err := store.SaveModel(ports.ModelInstance{Name: "A", Kind: "openai-chat", BaseURL: "http://mock", Model: "m1", APIKey: "sk-x"})
	if err != nil {
		t.Fatal(err)
	}
	var built ports.ProviderConfig
	mgr := NewProviderManager(store, func(cfg ports.ProviderConfig) (ports.ModelProvider, error) {
		built = cfg
		return &countingProvider{slot: cfg.Slot}, nil
	}, nil)
	res, err := mgr.ProbeModel(context.Background(), instance.ID, false)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if !res.OK || res.Model != "m1" || res.Kind != "openai-chat" {
		t.Fatalf("探测结果 = %+v", res)
	}
	if built.Model != "m1" || built.APIKey != "sk-x" {
		t.Fatalf("探测必须用实例的连接信息: %+v", built)
	}
	// 未指派任何槽位也不影响探测（设置页里测的就是当前编辑的实例）。
	catalog, _ := store.LoadCatalog(false)
	if catalog.Slots[0].ModelID != "" {
		t.Fatalf("探测不应改动槽位: %+v", catalog.Slots[0])
	}
}

// 保存后运行时配置必须立即反映新实例（cfgs 陈旧会让生成拿到旧模型或空配置）。
func TestSaveSlotRefreshesRuntimeConfig(t *testing.T) {
	store := newFakeConfigStore()
	mgr := NewProviderManager(store, func(cfg ports.ProviderConfig) (ports.ModelProvider, error) {
		return &countingProvider{slot: cfg.Slot}, nil
	}, nil)
	instance, err := store.SaveModel(ports.ModelInstance{Name: "A", Kind: "openai-chat", BaseURL: "http://x", Model: "m1", ContextWindow: 65536, MaxTokens: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.SaveSlot("primary", true, instance.ID); err != nil {
		t.Fatal(err)
	}
	snapshot := mgr.ConfigSnapshot("primary")
	if !snapshot.Enabled || snapshot.Model != "m1" || snapshot.ContextWindow != 65536 || snapshot.ModelID != instance.ID {
		t.Fatalf("运行时配置未刷新: %+v", snapshot)
	}
	if len(mgr.Configs()) != 3 {
		t.Fatalf("应当始终有三个槽位视图: %+v", mgr.Configs())
	}
	// 改实例同样立即生效。
	instance.ContextWindow = 131072
	if _, err := mgr.SaveModel(instance); err != nil {
		t.Fatal(err)
	}
	if got := mgr.ConfigSnapshot("primary").ContextWindow; got != 131072 {
		t.Fatalf("改实例后运行时配置未刷新: %d", got)
	}
}

// 思考强度只接受 low / medium / high（留空 = 默认）：非法值不许保存，
// 合法值要一路进到运行时配置（保存即生效的边界）。
func TestSaveModelValidatesReasoningEffort(t *testing.T) {
	store := newFakeConfigStore()
	mgr := NewProviderManager(store, func(ports.ProviderConfig) (ports.ModelProvider, error) {
		return &countingProvider{slot: "p"}, nil
	}, nil)
	var apiErr *APIError
	_, err := mgr.SaveModel(ports.ModelInstance{
		Name: "A", Kind: "openai-chat", BaseURL: "http://x", Model: "m1", ReasoningEffort: "maximum",
	})
	if !errors.As(err, &apiErr) || apiErr.Code != "PROVIDER_BAD_CONFIG" || apiErr.StatusCode != 422 {
		t.Fatalf("非法思考强度应当 422 PROVIDER_BAD_CONFIG，实际 %v", err)
	}

	saved, err := mgr.SaveModel(ports.ModelInstance{
		Name: "A", Kind: "openai-chat", BaseURL: "http://x", Model: "m1", ReasoningEffort: "medium",
	})
	if err != nil {
		t.Fatalf("保存合法档位失败: %v", err)
	}
	if err := mgr.SaveSlot("primary", true, saved.ID); err != nil {
		t.Fatal(err)
	}
	if cfg := mgr.ConfigSnapshot("primary"); cfg.ReasoningEffort != "medium" {
		t.Fatalf("运行时配置未带上思考强度: %+v", cfg)
	}
}
