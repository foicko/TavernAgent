// Package config 提供基于文件的模型配置存储（契约 §12/§12.3）。
// 布局：data/config/settings.json（模型实例 + 槽位引用 + apiKeyRef）+ data/config/secrets.json（明文密钥）。
//
// 配置模型（v2）：模型实例是唯一携带连接信息的地方；三个槽位只保存 enabled + modelId。
// v1（槽位内联字段 + 档案）在启动时一次性迁移，迁移前把原文备份为 settings.json.v1.bak。
// API Key 只由后端读取；界面读取接口返回脱敏值。密钥文件写入时尽力设置最小权限。
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"tavernagent/internal/ports"
	"tavernagent/internal/util/atomicfile"
	"tavernagent/internal/util/id"
)

const (
	// envPrefix 是槽位级环境变量前缀（旧规则，保留）：TAVERNAGENT_KEY_<SLOT 大写>。
	envPrefix = "TAVERNAGENT_KEY_"
	// instanceEnvPrefix 是实例级环境变量前缀：TAVERNAGENT_KEY_MODEL_<实例 ID 规范化>。
	// 用独立前缀避免与槽位名相撞（实例 ID 以 PRIMARY 开头也不会误伤槽位规则）。
	instanceEnvPrefix = "TAVERNAGENT_KEY_MODEL_"
	// legacyBackupSuffix 是 v1 迁移备份后缀，只在首次迁移时写入一次。
	legacyBackupSuffix = ".v1.bak"

	maxInstances = 128
)

// FileStore 是文件型 ModelConfigStore。
type FileStore struct {
	mu           sync.Mutex
	settingsPath string
	secretsPath  string
}

// New 创建文件存储；目录不存在时创建（含 config 目录）。
func New(dataDir string) (*FileStore, error) {
	dir := filepath.Join(dataDir, "config")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %w", err)
	}
	return &FileStore{
		settingsPath: filepath.Join(dir, "settings.json"),
		secretsPath:  filepath.Join(dir, "secrets.json"),
	}, nil
}

// settingsFile 是磁盘上的非秘密设置形态（v2）。
// Providers/Profiles 是 v1 遗留块：只在读取与迁移时使用，写出时永远为空。
type settingsFile struct {
	Version int                      `json:"version,omitempty"`
	Models  map[string]*modelSetting `json:"models,omitempty"`
	Slots   map[string]*slotSetting  `json:"slots,omitempty"`

	Providers map[string]*legacySlotSetting    `json:"providers,omitempty"`
	Profiles  map[string]*legacyProfileSetting `json:"profiles,omitempty"`
}

type modelSetting struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Kind            string   `json:"kind"`
	BaseURL         string   `json:"baseUrl,omitempty"`
	Model           string   `json:"model,omitempty"`
	APIKeyRef       string   `json:"apiKeyRef,omitempty"` // 指向 secrets.json 的键
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxTokens       int      `json:"maxTokens,omitempty"`
	ContextWindow   int      `json:"contextWindow,omitempty"`
	ReasoningEffort string   `json:"reasoningEffort,omitempty"` // low | medium | high；空 = 默认
}

type slotSetting struct {
	Enabled bool   `json:"enabled"`
	ModelID string `json:"modelId,omitempty"` // 空 = 跟随主线
}

// legacySlotSetting 是 v1 的槽位形态（连接信息内联）。
type legacySlotSetting struct {
	Slot          string   `json:"slot"`
	Enabled       bool     `json:"enabled"`
	Kind          string   `json:"kind"`
	BaseURL       string   `json:"baseUrl,omitempty"`
	Model         string   `json:"model,omitempty"`
	APIKeyRef     string   `json:"apiKeyRef,omitempty"`
	ProfileID     string   `json:"profileId,omitempty"`
	Temperature   *float64 `json:"temperature,omitempty"`
	MaxTokens     int      `json:"maxTokens,omitempty"`
	ContextWindow int      `json:"contextWindow,omitempty"`
}

// legacyProfileSetting 是 v1 的"模型档案"形态。
type legacyProfileSetting struct {
	ID     string             `json:"id"`
	Name   string             `json:"name"`
	Config *legacySlotSetting `json:"config"`
}

var _ ports.ModelConfigStore = (*FileStore)(nil)

// LoadCatalog 返回全部模型实例与三个槽位。masked=true 时密钥为脱敏值。
// 密钥优先级：secrets.json → 实例级环境变量 → 槽位级环境变量（旧规则仍生效）。
func (f *FileStore) LoadCatalog(masked bool) (*ports.ModelCatalog, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sf, err := f.readSettings()
	if err != nil {
		return nil, err
	}
	sec, err := f.readSecrets()
	if err != nil {
		return nil, err
	}
	return f.catalog(sf, sec, masked), nil
}

func (f *FileStore) catalog(sf *settingsFile, sec map[string]string, masked bool) *ports.ModelCatalog {
	instanceKey := func(m *modelSetting) string {
		key := sec[m.APIKeyRef]
		if env := os.Getenv(instanceEnvPrefix + envKeyForID(m.ID)); env != "" {
			key = env
		}
		return key
	}
	out := &ports.ModelCatalog{Models: make([]ports.ModelInstance, 0, len(sf.Models))}
	for _, m := range sf.Models {
		key := instanceKey(m)
		view := ports.ModelInstance{
			ID: m.ID, Name: m.Name, Kind: m.Kind, BaseURL: m.BaseURL, Model: m.Model,
			Temperature: m.Temperature, MaxTokens: m.MaxTokens, ContextWindow: m.ContextWindow,
			ReasoningEffort: readEffort(m.ReasoningEffort),
			HasAPIKey:       key != "",
		}
		if masked {
			view.APIKey = ports.MaskAPIKey(key)
		} else {
			view.APIKey = key
		}
		out.Models = append(out.Models, view)
	}
	sort.Slice(out.Models, func(i, j int) bool { return out.Models[i].ID < out.Models[j].ID })

	primaryID := sf.Slots[string(ports.SlotPrimary)].modelID()
	for _, slot := range ports.KnownSlots() {
		binding := sf.Slots[slot]
		view := ports.SlotBinding{Slot: slot}
		if binding != nil {
			view.Enabled = binding.Enabled
			view.ModelID = binding.ModelID
		}
		targetID := view.ModelID
		if targetID == "" {
			targetID = primaryID
		}
		if m := sf.Models[targetID]; m != nil {
			key := instanceKey(m)
			if env := os.Getenv(envPrefix + strings.ToUpper(slot)); env != "" {
				key = env
			}
			view.ResolvedModelID = m.ID
			view.Kind = m.Kind
			view.BaseURL = m.BaseURL
			view.Model = m.Model
			view.Temperature = m.Temperature
			view.MaxTokens = m.MaxTokens
			view.ContextWindow = m.ContextWindow
			view.ReasoningEffort = readEffort(m.ReasoningEffort)
			view.HasAPIKey = key != ""
			if masked {
				view.APIKey = ports.MaskAPIKey(key)
			} else {
				view.APIKey = key
			}
		}
		out.Slots = append(out.Slots, view)
	}
	return out
}

func (s *slotSetting) modelID() string {
	if s == nil {
		return ""
	}
	return s.ModelID
}

// SaveModel 新建或更新一个模型实例，返回脱敏视图。空密钥或回传掩码都表示"保留原密钥"。
func (f *FileStore) SaveModel(instance ports.ModelInstance) (ports.ModelInstance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	instance.Name = strings.TrimSpace(instance.Name)
	instance.BaseURL = strings.TrimSpace(instance.BaseURL)
	instance.Model = strings.TrimSpace(instance.Model)
	instance.Kind = normalizeKind(instance.Kind)
	if err := validateInstance(instance); err != nil {
		return ports.ModelInstance{}, err
	}
	sf, err := f.readSettings()
	if err != nil {
		return ports.ModelInstance{}, err
	}
	sec, err := f.readSecrets()
	if err != nil {
		return ports.ModelInstance{}, err
	}
	if instance.ID == "" {
		instance.ID = "m_" + id.New()
	}
	if len(sf.Models) >= maxInstances && sf.Models[instance.ID] == nil {
		return ports.ModelInstance{}, fmt.Errorf("最多保存 %d 个模型实例", maxInstances)
	}
	previous := sf.Models[instance.ID]
	ref := ""
	if previous != nil {
		ref = previous.APIKeyRef
	}
	ref, err = f.keyReference(ref, instance.APIKey, sec)
	if err != nil {
		return ports.ModelInstance{}, err
	}
	stored := &modelSetting{
		ID: instance.ID, Name: instance.Name, Kind: instance.Kind, BaseURL: instance.BaseURL, Model: instance.Model,
		APIKeyRef: ref, Temperature: instance.Temperature, MaxTokens: instance.MaxTokens, ContextWindow: instance.ContextWindow,
		ReasoningEffort: readEffort(instance.ReasoningEffort),
	}
	sf.Models[instance.ID] = stored
	if err := f.commitFiles(sf, sec); err != nil {
		return ports.ModelInstance{}, err
	}
	return modelView(f.catalog(sf, sec, true), instance.ID), nil
}

// DeleteModel 删除实例。仍被槽位引用时返回 *ports.ModelInUseError，绝不悄悄改槽位。
func (f *FileStore) DeleteModel(modelID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	sf, err := f.readSettings()
	if err != nil {
		return err
	}
	if sf.Models[modelID] == nil {
		return fmt.Errorf("%w: %s", ports.ErrModelNotFound, modelID)
	}
	inUse := make([]string, 0, len(sf.Slots))
	for slot, binding := range sf.Slots {
		if binding != nil && binding.ModelID == modelID {
			inUse = append(inUse, slot)
		}
	}
	if len(inUse) > 0 {
		sort.Strings(inUse)
		return &ports.ModelInUseError{Slots: inUse}
	}
	delete(sf.Models, modelID)
	return f.writeAtomic(f.settingsPath, sf)
}

// SaveSlot 保存槽位指派。空 modelID 表示跟随主线；引用不存在的实例会被拒绝。
func (f *FileStore) SaveSlot(slot string, enabled bool, modelID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !validSlot(slot) {
		return fmt.Errorf("未知槽位: %s", slot)
	}
	sf, err := f.readSettings()
	if err != nil {
		return err
	}
	if modelID != "" && sf.Models[modelID] == nil {
		return fmt.Errorf("%w: %s", ports.ErrModelNotFound, modelID)
	}
	if sf.Slots == nil {
		sf.Slots = map[string]*slotSetting{}
	}
	sf.Slots[slot] = &slotSetting{Enabled: enabled, ModelID: modelID}
	return f.writeAtomic(f.settingsPath, sf)
}

// MigrateLegacy 把 v1 配置（槽位内联字段 + 档案）迁移为 v2（实例 + 槽位引用）。
//
// 迁移是幂等的：只有检测到 v1 形态才动作；迁移前把原文备份为 settings.json.v1.bak
// （已存在则不覆盖），迁移后磁盘上只保留 v2，旧块被清理。
// 返回是否发生了迁移。
func (f *FileStore) MigrateLegacy() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, sf, err := f.readSettingsRaw()
	if err != nil {
		return false, err
	}
	if !needsMigration(sf) {
		return false, nil
	}
	secrets, err := f.readSecrets()
	if err != nil {
		return false, err
	}
	migrated := migrateLegacySettings(sf, secrets)
	if len(raw) > 0 {
		if _, statErr := os.Stat(f.settingsPath + legacyBackupSuffix); errors.Is(statErr, os.ErrNotExist) {
			if err := atomicfile.Write(f.settingsPath+legacyBackupSuffix, raw, 0o600); err != nil {
				return false, fmt.Errorf("写入迁移备份失败: %w", err)
			}
		}
	}
	if err := f.writeAtomic(f.settingsPath, migrated); err != nil {
		return false, err
	}
	return true, nil
}

// needsMigration 只在磁盘上确实存在 v1 块时才为真。
// v2 形态（models/slots）即使缺少 version 字段也不能当成 v1——否则每次读取都会
// 把已经写好的实例"迁移"没。
func needsMigration(sf *settingsFile) bool {
	if sf.Version >= 2 {
		return false
	}
	return len(sf.Providers) > 0 || len(sf.Profiles) > 0
}

// migrateLegacySettings 合并 v1 的档案与槽位内联配置：同一 (kind, baseUrl, model)
// 且**密钥值相同（或一方没有密钥）**只保留一个实例，避免三个槽位各建一份；
// 密钥值不同则保留成两个实例——迁移绝不悄悄换掉一份可用凭据。
func migrateLegacySettings(sf *settingsFile, secrets map[string]string) *settingsFile {
	out := &settingsFile{Version: 2, Models: map[string]*modelSetting{}, Slots: map[string]*slotSetting{}}
	for id, m := range sf.Models {
		out.Models[id] = m
	}
	for slot, binding := range sf.Slots {
		out.Slots[slot] = binding
	}
	keyValue := func(ref string) string { return secrets[ref] }
	byKey := map[string]*modelSetting{}
	byTriple := map[string][]*modelSetting{}
	index := func(m *modelSetting) {
		byKey[instanceKey(m.Kind, m.BaseURL, m.Model, keyValue(m.APIKeyRef))] = m
		triple := instanceTriple(m.Kind, m.BaseURL, m.Model)
		byTriple[triple] = append(byTriple[triple], m)
	}
	for _, m := range out.Models {
		index(m)
	}
	// matchInstance 找出可安全合并的实例：同一连接三要素，且密钥相同、或有一方没有密钥。
	matchInstance := func(kind, baseURL, model, ref string) *modelSetting {
		value := keyValue(ref)
		for _, candidate := range byTriple[instanceTriple(kind, baseURL, model)] {
			candidateValue := keyValue(candidate.APIKeyRef)
			if candidateValue == value || candidateValue == "" || value == "" {
				return candidate
			}
		}
		return nil
	}
	profileInstance := map[string]string{}

	for profileID, profile := range sf.Profiles {
		if profile == nil || profile.Config == nil {
			continue
		}
		c := profile.Config
		key := instanceKey(c.Kind, c.BaseURL, c.Model, keyValue(c.APIKeyRef))
		if existing := byKey[key]; existing != nil {
			profileInstance[profileID] = existing.ID
			continue
		}
		m := &modelSetting{
			ID: profileInstanceID(profileID), Name: strings.TrimSpace(profile.Name), Kind: normalizeKind(c.Kind),
			BaseURL: c.BaseURL, Model: c.Model, APIKeyRef: c.APIKeyRef,
			Temperature: c.Temperature, MaxTokens: c.MaxTokens, ContextWindow: c.ContextWindow,
		}
		if m.Name == "" {
			m.Name = fallbackInstanceName(m.Model)
		}
		byKey[key] = m
		index(m)
		out.Models[m.ID] = m
		profileInstance[profileID] = m.ID
	}

	for _, slot := range ports.KnownSlots() {
		ps := sf.Providers[slot]
		if ps == nil {
			out.Slots[slot] = &slotSetting{}
			continue
		}
		if ps.ProfileID != "" && profileInstance[ps.ProfileID] != "" {
			out.Slots[slot] = &slotSetting{Enabled: ps.Enabled, ModelID: profileInstance[ps.ProfileID]}
			continue
		}
		if ps.BaseURL == "" && ps.Model == "" {
			out.Slots[slot] = &slotSetting{Enabled: ps.Enabled}
			continue
		}
		matched := matchInstance(ps.Kind, ps.BaseURL, ps.Model, ps.APIKeyRef)
		if matched != nil && matched.APIKeyRef == "" && ps.APIKeyRef != "" {
			// 合并到已有实例时把槽位自己的凭据移过去，不丢密钥。
			matched.APIKeyRef = ps.APIKeyRef
			index(matched)
		}
		if matched == nil {
			matched = &modelSetting{
				ID: "m_" + id.New(), Name: fallbackInstanceName(ps.Model), Kind: normalizeKind(ps.Kind),
				BaseURL: ps.BaseURL, Model: ps.Model, APIKeyRef: ps.APIKeyRef,
				Temperature: ps.Temperature, MaxTokens: ps.MaxTokens, ContextWindow: ps.ContextWindow,
			}
			index(matched)
			out.Models[matched.ID] = matched
		}
		out.Slots[slot] = &slotSetting{Enabled: ps.Enabled, ModelID: matched.ID}
	}
	return out
}

func profileInstanceID(profileID string) string {
	if profileID == "" {
		return "m_" + id.New()
	}
	return profileID
}

// instanceTriple 是连接三要素；instanceKey 再加上密钥值，构成"同一个实例"的判据。
func instanceTriple(kind, baseURL, model string) string {
	return strings.Join([]string{normalizeKind(kind), baseURL, model}, "\x1f")
}

// instanceKey = 连接三要素 + 密钥值：密钥值不同就不合并，迁移不得悄悄换掉可用凭据。
func instanceKey(kind, baseURL, model, keyValue string) string {
	return instanceTriple(kind, baseURL, model) + "\x1f" + keyValue
}

func fallbackInstanceName(model string) string {
	if strings.TrimSpace(model) == "" {
		return "未命名模型"
	}
	return strings.TrimSpace(model)
}

func normalizeKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case "", "openai-compatible", "openai-chat":
		return "openai-chat"
	case "openai-responses":
		return "openai-responses"
	case "anthropic-messages":
		return "anthropic-messages"
	default:
		return strings.TrimSpace(kind)
	}
}

// readEffort 归一化思考强度。磁盘上手写的未知值按"默认"处理，
// 不因为这个可选字段让整个配置读不出来。
func readEffort(raw string) string {
	effort, ok := ports.NormalizeEffort(raw)
	if !ok {
		return ""
	}
	return effort
}

func validateInstance(instance ports.ModelInstance) error {
	if instance.Name == "" || utf8.RuneCountInString(instance.Name) > 128 || len(instance.ID) > 128 {
		return errors.New("实例名称无效（1-128 字）")
	}
	if instance.Model == "" {
		return errors.New("模型名不能为空")
	}
	if err := validateEndpoint(instance.BaseURL, instance.MaxTokens, instance.ContextWindow); err != nil {
		return err
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

// envKeyForID 把实例 ID 规范成环境变量片段：大写，非字母数字转下划线。
func envKeyForID(modelID string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(modelID) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func (f *FileStore) readSettings() (*settingsFile, error) {
	_, sf, err := f.readSettingsRaw()
	if err != nil {
		return nil, err
	}
	if needsMigration(sf) {
		// 迁移需要密钥值来判重（同一模型的不同 ref 名可能指向同一份密钥）；
		// 密钥文件读不出来时不迁移，交由调用方报错。
		secrets, err := f.readSecrets()
		if err != nil {
			return nil, err
		}
		sf = migrateLegacySettings(sf, secrets)
	}
	sf.Version = 2
	return sf, nil
}

// readSettingsRaw 返回磁盘原文与解析结果（不做迁移）。
func (f *FileStore) readSettingsRaw() ([]byte, *settingsFile, error) {
	sf := &settingsFile{Models: map[string]*modelSetting{}, Slots: map[string]*slotSetting{}, Providers: map[string]*legacySlotSetting{}, Profiles: map[string]*legacyProfileSetting{}}
	b, err := os.ReadFile(f.settingsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, sf, nil
		}
		return nil, nil, fmt.Errorf("读取模型设置失败: %w", err)
	}
	if strings.TrimSpace(string(b)) == "null" {
		return nil, nil, errors.New("模型设置必须是 JSON 对象")
	}
	if err := json.Unmarshal(b, sf); err != nil {
		return nil, nil, fmt.Errorf("模型设置 JSON 损坏: %w", err)
	}
	if sf.Models == nil {
		sf.Models = map[string]*modelSetting{}
	}
	if sf.Slots == nil {
		sf.Slots = map[string]*slotSetting{}
	}
	if sf.Providers == nil {
		sf.Providers = map[string]*legacySlotSetting{}
	}
	if sf.Profiles == nil {
		sf.Profiles = map[string]*legacyProfileSetting{}
	}
	for id, m := range sf.Models {
		if m == nil {
			return nil, nil, fmt.Errorf("模型实例损坏: %s", id)
		}
		if m.ID == "" {
			m.ID = id
		}
	}
	for _, p := range sf.Profiles {
		if p == nil || p.Config == nil {
			return nil, nil, errors.New("模型档案损坏")
		}
	}
	return b, sf, nil
}

func (f *FileStore) readSecrets() (map[string]string, error) {
	m := map[string]string{}
	b, err := os.ReadFile(f.secretsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return m, nil
		}
		return nil, fmt.Errorf("读取模型凭据失败: %w", err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("模型凭据 JSON 损坏: %w", err)
	}
	if m == nil {
		return nil, errors.New("模型凭据必须是 JSON 对象")
	}
	return m, nil
}

// writeAtomic 以临时文件 + 原子改名写入设置文件。
func (f *FileStore) writeAtomic(path string, v any) error {
	return atomicfile.WriteJSON(path, v, 0o600)
}

// keyReference 决定密钥引用：留空或回传旧掩码 = 保留；纯掩码 = 拒绝；其余 = 新密钥。
func (f *FileStore) keyReference(oldRef, key string, secrets map[string]string) (string, error) {
	if key == "" || key == ports.MaskAPIKey(secrets[oldRef]) {
		return oldRef, nil
	}
	if key == "****" || strings.Contains(key, "…") {
		return "", errors.New("脱敏凭据不能作为新密钥，请重新输入")
	}
	ref := "key_" + id.New()
	secrets[ref] = key
	return ref, nil
}

// Credentials are immutable referenced entries. Persist new entries first,
// then atomically switch settings. A failed second write leaves old settings
// pointing at their old credentials, never at a partially updated key.
func (f *FileStore) commitFiles(settings *settingsFile, secrets map[string]string) error {
	if err := atomicfile.WriteJSON(f.secretsPath, secrets, 0o600); err != nil {
		return err
	}
	return f.writeAtomic(f.settingsPath, settings)
}

// modelView 从 catalog 里取一个实例的脱敏视图（SaveModel 的返回值）。
func modelView(c *ports.ModelCatalog, modelID string) ports.ModelInstance {
	for _, m := range c.Models {
		if m.ID == modelID {
			return m
		}
	}
	return ports.ModelInstance{ID: modelID}
}

// validateEndpoint 校验地址与预算约束（与运行时的校验保持一致，存储层也不接受坏值）。
func validateEndpoint(baseURL string, maxTokens, contextWindow int) error {
	if baseURL != "" {
		lower := strings.ToLower(baseURL)
		if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
			return errors.New("接口地址必须是 http(s) 地址，凭据请填在密钥栏")
		}
		if at := strings.Index(baseURL, "@"); at >= 0 && at < strings.IndexAny(baseURL, "/?") {
			return errors.New("接口地址不能包含凭据，请把密钥填在密钥栏")
		}
	}
	if maxTokens < 0 || contextWindow < 0 || contextWindow > 2_000_000 || (contextWindow > 0 && contextWindow-maxTokens < 2048) {
		return errors.New("上下文窗口与输出预算不合法，至少需保留 2048 输入 token")
	}
	return nil
}
