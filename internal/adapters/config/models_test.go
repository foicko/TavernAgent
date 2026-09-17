package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tavernagent/internal/ports"
)

// v1 形态：槽位内联连接信息 + 一份"档案"。迁移后应变成实例 + 槽位引用。
func writeLegacySettings(t *testing.T, dir string, providers, profiles string) string {
	t.Helper()
	path := filepath.Join(dir, "config")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	settings := `{"providers":` + providers + `,"profiles":` + profiles + `}`
	if err := os.WriteFile(filepath.Join(path, "settings.json"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "secrets.json"), []byte(`{"primary":"sk-slot-primary","key_p1":"sk-profile-one"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func openStore(t *testing.T, dir string) *FileStore {
	t.Helper()
	store, err := New(dir)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return store
}

func slotByName(t *testing.T, cat *ports.ModelCatalog, slot string) ports.SlotBinding {
	t.Helper()
	for _, binding := range cat.Slots {
		if binding.Slot == slot {
			return binding
		}
	}
	t.Fatalf("槽位 %s 不在 catalog 里: %+v", slot, cat.Slots)
	return ports.SlotBinding{}
}

func modelByName(t *testing.T, cat *ports.ModelCatalog, name string) ports.ModelInstance {
	t.Helper()
	for _, m := range cat.Models {
		if m.Name == name {
			return m
		}
	}
	t.Fatalf("实例 %s 不在 catalog 里: %+v", name, cat.Models)
	return ports.ModelInstance{}
}

const legacyProviders = `{
  "primary": {"slot":"primary","enabled":true,"kind":"openai-chat","baseUrl":"https://api.deepseek.com/v1","model":"deepseek-chat","apiKeyRef":"key_p1","temperature":0.8,"maxTokens":4096,"contextWindow":131072},
  "assist": {"slot":"assist","enabled":true,"kind":"openai-chat","baseUrl":"https://api.deepseek.com/v1","model":"deepseek-chat","profileId":"p1","contextWindow":131072},
  "reflection": {"slot":"reflection","enabled":false,"kind":"openai-chat"}
}`

const legacyProfiles = `{
  "p1": {"id":"p1","name":"DeepSeek 官方","config":{"kind":"openai-chat","baseUrl":"https://api.deepseek.com/v1","model":"deepseek-chat","apiKeyRef":"key_p1","temperature":0.8,"maxTokens":4096,"contextWindow":131072}}
}`

func TestLegacyMigrationBuildsInstancesAndBindings(t *testing.T) {
	dir := writeLegacySettings(t, t.TempDir(), legacyProviders, legacyProfiles)
	store := openStore(t, dir)

	migrated, err := store.MigrateLegacy()
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !migrated {
		t.Fatal("v1 配置应当被迁移")
	}

	cat, err := store.LoadCatalog(false)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// primary 与 profile 指向同一个 (kind, baseUrl, model)：合并成一个实例，密钥取 profile 的。
	if len(cat.Models) != 1 {
		t.Fatalf("实例应当去重为 1 个，实际 %d 个: %+v", len(cat.Models), cat.Models)
	}
	deepseek := cat.Models[0]
	if deepseek.Name != "DeepSeek 官方" || deepseek.Model != "deepseek-chat" {
		t.Fatalf("实例字段 = %+v", deepseek)
	}
	if deepseek.APIKey != "sk-profile-one" {
		t.Fatalf("合并时应保留可用密钥，实际 %q", deepseek.APIKey)
	}
	primary := slotByName(t, cat, "primary")
	if !primary.Enabled || primary.ModelID != deepseek.ID || primary.ResolvedModelID != deepseek.ID {
		t.Fatalf("主线绑定 = %+v", primary)
	}
	assist := slotByName(t, cat, "assist")
	if assist.ModelID != deepseek.ID || assist.ResolvedModelID != deepseek.ID {
		t.Fatalf("辅助应当绑定到同一实例: %+v", assist)
	}
	reflection := slotByName(t, cat, "reflection")
	if reflection.Enabled || reflection.ModelID != "" {
		t.Fatalf("关闭的槽位不应带上引用: %+v", reflection)
	}
}

func TestLegacyMigrationBacksUpOnceAndDropsLegacyBlocks(t *testing.T) {
	dir := writeLegacySettings(t, t.TempDir(), legacyProviders, legacyProfiles)
	settingsPath := filepath.Join(dir, "config", "settings.json")
	before, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	store := openStore(t, dir)
	if _, err := store.MigrateLegacy(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	backup, err := os.ReadFile(settingsPath + legacyBackupSuffix)
	if err != nil {
		t.Fatalf("缺少迁移备份: %v", err)
	}
	if string(backup) != string(before) {
		t.Fatal("备份内容必须与迁移前原文一致")
	}
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"providers"`) || strings.Contains(string(raw), `"profiles"`) {
		t.Fatalf("迁移后不应保留旧块: %s", raw)
	}
	var shape map[string]any
	if err := json.Unmarshal(raw, &shape); err != nil {
		t.Fatalf("迁移后的文件必须是合法 JSON: %v", err)
	}
	if shape["version"] != float64(2) || shape["models"] == nil || shape["slots"] == nil {
		t.Fatalf("迁移后应是 v2 形态: %s", raw)
	}
	// 再次迁移是幂等的：备份不被 v2 覆盖。
	again, err := store.MigrateLegacy()
	if err != nil || again {
		t.Fatalf("第二次迁移应当无操作: migrated=%v err=%v", again, err)
	}
	backup2, _ := os.ReadFile(settingsPath + legacyBackupSuffix)
	if string(backup2) != string(before) {
		t.Fatal("第二次迁移覆盖了备份")
	}
}

func TestLegacySlotWithoutProfileKeepsItsOwnCredential(t *testing.T) {
	providers := `{"primary":{"slot":"primary","enabled":true,"kind":"anthropic-messages","baseUrl":"https://api.anthropic.com","model":"claude-sonnet-4-5","apiKeyRef":"primary","contextWindow":200000}}`
	dir := writeLegacySettings(t, t.TempDir(), providers, `{}`)
	store := openStore(t, dir)
	if _, err := store.MigrateLegacy(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cat, err := store.LoadCatalog(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Models) != 1 {
		t.Fatalf("应当由槽位生成 1 个实例: %+v", cat.Models)
	}
	if cat.Models[0].APIKey != "sk-slot-primary" {
		t.Fatalf("槽位密钥必须迁到实例上，实际 %q", cat.Models[0].APIKey)
	}
	if cat.Models[0].Name != "claude-sonnet-4-5" {
		t.Fatalf("无档案时以模型名作实例名，实际 %q", cat.Models[0].Name)
	}
	if slotByName(t, cat, "assist").ModelID != "" {
		t.Fatal("未配置的槽位不应绑定实例")
	}
}

func TestSaveModelAndSlotRoundTrip(t *testing.T) {
	store := openStore(t, t.TempDir())
	created, err := store.SaveModel(ports.ModelInstance{
		Name: "本地 Ollama", Kind: "openai-chat", BaseURL: "http://127.0.0.1:11434/v1",
		Model: "qwen2.5:14b", APIKey: "sk-local-one", ContextWindow: 32768, MaxTokens: 2048,
		ReasoningEffort: " HIGH ", // 顺带验证归一化：去空白 + 转小写
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if created.ID == "" || !created.HasAPIKey || created.APIKey == "sk-local-one" {
		t.Fatalf("返回值必须是带 ID 的脱敏视图: %+v", created)
	}
	if !strings.Contains(created.APIKey, "…") {
		t.Fatalf("密钥必须脱敏，实际 %q", created.APIKey)
	}
	if err := store.SaveSlot("primary", true, created.ID); err != nil {
		t.Fatalf("save slot: %v", err)
	}

	cat, err := store.LoadCatalog(false)
	if err != nil {
		t.Fatal(err)
	}
	primary := slotByName(t, cat, "primary")
	if primary.ModelID != created.ID || primary.Model != "qwen2.5:14b" || primary.BaseURL != "http://127.0.0.1:11434/v1" {
		t.Fatalf("槽位未解析到实例字段: %+v", primary)
	}
	if primary.APIKey != "sk-local-one" {
		t.Fatalf("未脱敏读取应当拿到明文密钥用于构建客户端，实际 %q", primary.APIKey)
	}
	// 思考强度随实例走：实例视图与槽位解析视图都要带上（归一化后的小写值）。
	if cat.Models[0].ReasoningEffort != "high" {
		t.Fatalf("实例视图应回传思考强度 high，实际 %q", cat.Models[0].ReasoningEffort)
	}
	if primary.ReasoningEffort != "high" {
		t.Fatalf("槽位解析视图应带上思考强度，实际 %q", primary.ReasoningEffort)
	}

	// 跟随主线：辅助清空引用后解析到主线实例。
	if err := store.SaveSlot("assist", true, ""); err != nil {
		t.Fatal(err)
	}
	cat, _ = store.LoadCatalog(false)
	assist := slotByName(t, cat, "assist")
	if assist.ModelID != "" || assist.ResolvedModelID != created.ID || assist.Model != "qwen2.5:14b" {
		t.Fatalf("辅助应跟随主线: %+v", assist)
	}

	// 掩码读取：实例与槽位都只给脱敏值。
	masked, err := store.LoadCatalog(true)
	if err != nil {
		t.Fatal(err)
	}
	if masked.Models[0].APIKey == "sk-local-one" || slotByName(t, masked, "primary").APIKey == "sk-local-one" {
		t.Fatal("masked=true 时不得回传明文密钥")
	}
}

func TestSaveModelKeepsSecretWhenKeyOmittedOrMasked(t *testing.T) {
	store := openStore(t, t.TempDir())
	created, err := store.SaveModel(ports.ModelInstance{Name: "A", Kind: "openai-chat", BaseURL: "https://a.example/v1", Model: "m1", APIKey: "sk-first-secret"})
	if err != nil {
		t.Fatal(err)
	}
	// 留空 = 保留
	if _, err := store.SaveModel(ports.ModelInstance{ID: created.ID, Name: "A 改名", Kind: "openai-chat", BaseURL: "https://a.example/v1", Model: "m1"}); err != nil {
		t.Fatal(err)
	}
	cat, _ := store.LoadCatalog(false)
	if modelByName(t, cat, "A 改名").APIKey != "sk-first-secret" {
		t.Fatal("留空保存必须保留原密钥")
	}
	// 回传掩码 = 保留
	if _, err := store.SaveModel(ports.ModelInstance{ID: created.ID, Name: "A 改名", Kind: "openai-chat", BaseURL: "https://a.example/v1", Model: "m1", APIKey: "sk-f…cret"}); err != nil {
		t.Fatalf("回传掩码应当被当作保留：%v", err)
	}
	if modelByName(t, cat, "A 改名").APIKey != "sk-first-secret" {
		t.Fatal("掩码回传不得改密钥")
	}
	// 真给一个新密钥 = 替换
	if _, err := store.SaveModel(ports.ModelInstance{ID: created.ID, Name: "A 改名", Kind: "openai-chat", BaseURL: "https://a.example/v1", Model: "m1", APIKey: "sk-second-secret"}); err != nil {
		t.Fatal(err)
	}
	cat, _ = store.LoadCatalog(false)
	if modelByName(t, cat, "A 改名").APIKey != "sk-second-secret" {
		t.Fatal("新密钥必须生效")
	}
	// 纯掩码（无对应旧值）不能当密钥
	if _, err := store.SaveModel(ports.ModelInstance{Name: "B", Kind: "openai-chat", BaseURL: "https://b.example/v1", Model: "m2", APIKey: "****"}); err == nil {
		t.Fatal("纯掩码不能作为新密钥保存")
	}
}

func TestDeleteModelRefusesWhileReferenced(t *testing.T) {
	store := openStore(t, t.TempDir())
	a, err := store.SaveModel(ports.ModelInstance{Name: "A", Kind: "openai-chat", BaseURL: "https://a.example/v1", Model: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.SaveModel(ports.ModelInstance{Name: "B", Kind: "openai-chat", BaseURL: "https://b.example/v1", Model: "m2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSlot("primary", true, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSlot("reflection", true, a.ID); err != nil {
		t.Fatal(err)
	}

	err = store.DeleteModel(a.ID)
	var inUse *ports.ModelInUseError
	if !errors.As(err, &inUse) {
		t.Fatalf("被引用时必须返回 ModelInUseError，实际 %v", err)
	}
	if len(inUse.Slots) != 2 {
		t.Fatalf("应当报出两个引用槽位: %+v", inUse.Slots)
	}
	if err := store.DeleteModel(b.ID); err != nil {
		t.Fatalf("未被引用的实例应当可以删除: %v", err)
	}
	cat, _ := store.LoadCatalog(false)
	if len(cat.Models) != 1 {
		t.Fatalf("删除后应剩 1 个实例: %+v", cat.Models)
	}
}

func TestSaveSlotRejectsUnknownInstance(t *testing.T) {
	store := openStore(t, t.TempDir())
	if err := store.SaveSlot("primary", true, "m_missing"); !errors.Is(err, ports.ErrModelNotFound) {
		t.Fatalf("引用不存在的实例必须报错，实际 %v", err)
	}
	if err := store.SaveSlot("nope", true, ""); err == nil {
		t.Fatal("非法槽位必须报错")
	}
}

func TestInstanceEnvironmentOverrideWins(t *testing.T) {
	store := openStore(t, t.TempDir())
	created, err := store.SaveModel(ports.ModelInstance{Name: "A", Kind: "openai-chat", BaseURL: "https://a.example/v1", Model: "m1", APIKey: "sk-file"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSlot("primary", true, created.ID); err != nil {
		t.Fatal(err)
	}
	t.Setenv(instanceEnvPrefix+envKeyForID(created.ID), "sk-from-env")
	cat, err := store.LoadCatalog(false)
	if err != nil {
		t.Fatal(err)
	}
	if modelByName(t, cat, "A").APIKey != "sk-from-env" {
		t.Fatalf("实例级环境变量应优先，实际 %q", modelByName(t, cat, "A").APIKey)
	}
	if slotByName(t, cat, "primary").APIKey != "sk-from-env" {
		t.Fatal("槽位解析也必须用覆盖后的密钥")
	}
	// 旧规则：TAVERNAGENT_KEY_<SLOT> 仍然生效（文档保留）
	t.Setenv("TAVERNAGENT_KEY_ASSIST", "sk-slot-env")
	cat, _ = store.LoadCatalog(false)
	if slotByName(t, cat, "assist").APIKey != "sk-slot-env" {
		t.Fatal("槽位级环境变量必须继续生效")
	}
}

// 同一个模型、多个槽位各用自己名字的密钥引用（但密钥值相同）→ 必须合并成一个实例。
// 真实配置就是这样：primary/assist/reflection 的 ref 名不同，值指向同一把密钥。
func TestMigrateMergesSlotsSharingTheSameKeyValue(t *testing.T) {
	providers := `{"primary":{"slot":"primary","enabled":true,"kind":"openai-chat","baseUrl":"https://api.deepseek.com","model":"deepseek-chat","apiKeyRef":"primary","contextWindow":65536},
	              "assist":{"slot":"assist","enabled":true,"kind":"openai-chat","baseUrl":"https://api.deepseek.com","model":"deepseek-chat","apiKeyRef":"assist","contextWindow":65536},
	              "reflection":{"slot":"reflection","enabled":true,"kind":"openai-chat","baseUrl":"https://api.deepseek.com","model":"deepseek-chat","apiKeyRef":"reflection","contextWindow":65536}}`
	dir := writeLegacySettings(t, t.TempDir(), providers, `{}`)
	// 三把 ref 指向同一份密钥值。
	secrets := `{"primary":"sk-same","assist":"sk-same","reflection":"sk-same"}`
	if err := os.WriteFile(filepath.Join(dir, "config", "secrets.json"), []byte(secrets), 0o600); err != nil {
		t.Fatal(err)
	}
	store := openStore(t, dir)
	if _, err := store.MigrateLegacy(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cat, err := store.LoadCatalog(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Models) != 1 {
		t.Fatalf("密钥值相同的槽位应当合并成一个实例，实际 %d 个: %+v", len(cat.Models), cat.Models)
	}
	for _, slot := range []string{"primary", "assist", "reflection"} {
		binding := slotByName(t, cat, slot)
		if binding.APIKey != "sk-same" || binding.ResolvedModelID != cat.Models[0].ID {
			t.Fatalf("%s 未指向唯一实例或丢了密钥: %+v", slot, binding)
		}
	}
}

// 手改配置文件写了未知档位时按"默认"处理：可选字段不该让整份配置读不出来，
// 未知值也不会被写回磁盘。
func TestSaveModelDropsUnknownReasoningEffort(t *testing.T) {
	dir := t.TempDir()
	store := openStore(t, dir)
	created, err := store.SaveModel(ports.ModelInstance{
		Name: "A", Kind: "openai-chat", BaseURL: "https://a.example/v1", Model: "m1",
		ReasoningEffort: "super-high",
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if created.ReasoningEffort != "" {
		t.Fatalf("未知档位应按默认处理，实际 %q", created.ReasoningEffort)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "super-high") {
		t.Fatalf("未知档位不应落盘: %s", raw)
	}
}
