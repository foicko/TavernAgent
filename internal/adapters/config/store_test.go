package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tavernagent/internal/ports"
)

func TestInstanceKeysStayOutOfSettingsAndSurviveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	f, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	temp := 0.7
	created, err := f.SaveModel(ports.ModelInstance{
		Name: "DeepSeek 官方", Kind: "openai-compatible", BaseURL: "https://api.example.com",
		Model: "deepseek-chat", APIKey: "sk-abcdef123456", Temperature: &temp, MaxTokens: 2048, ContextWindow: 131072,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SaveSlot("primary", true, created.ID); err != nil {
		t.Fatal(err)
	}

	// 直接读取 settings.json：不得包含明文密钥，也不得残留 v1 块。
	settingBytes, err := os.ReadFile(filepath.Join(dir, "config", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(settingBytes), "sk-abcdef123456") {
		t.Fatal("settings.json 不得包含明文 API Key")
	}
	if strings.Contains(string(settingBytes), `"providers"`) || strings.Contains(string(settingBytes), `"profiles"`) {
		t.Fatalf("新写入的配置不应包含 v1 块: %s", settingBytes)
	}

	masked, err := f.LoadCatalog(true)
	if err != nil {
		t.Fatal(err)
	}
	model := modelByName(t, masked, "DeepSeek 官方")
	if model.Kind != "openai-chat" || model.Model != "deepseek-chat" || model.Temperature == nil || *model.Temperature != 0.7 {
		t.Fatalf("实例字段 = %+v", model)
	}
	if model.APIKey != ports.MaskAPIKey("sk-abcdef123456") || !model.HasAPIKey {
		t.Fatalf("密钥未按契约脱敏: %+v", model)
	}
	primary := slotByName(t, masked, "primary")
	if !primary.Enabled || primary.Model != "deepseek-chat" || primary.APIKey != ports.MaskAPIKey("sk-abcdef123456") {
		t.Fatalf("槽位视图 = %+v", primary)
	}

	// 界面原样回传脱敏值 → 保留旧密钥。
	if _, err := f.SaveModel(ports.ModelInstance{ID: created.ID, Name: "DeepSeek 官方", Kind: "openai-chat", BaseURL: "https://api.example.com", Model: "deepseek-chat", APIKey: model.APIKey}); err != nil {
		t.Fatal(err)
	}
	raw, _ := f.LoadCatalog(false)
	if got := modelByName(t, raw, "DeepSeek 官方").APIKey; got != "sk-abcdef123456" {
		t.Fatalf("密钥未保留: %q", got)
	}

	// 新密钥覆盖。
	if _, err := f.SaveModel(ports.ModelInstance{ID: created.ID, Name: "DeepSeek 官方", Kind: "openai-chat", BaseURL: "https://api.example.com", Model: "deepseek-chat", APIKey: "sk-new-key-9999"}); err != nil {
		t.Fatal(err)
	}
	raw2, _ := f.LoadCatalog(false)
	if got := modelByName(t, raw2, "DeepSeek 官方").APIKey; got != "sk-new-key-9999" {
		t.Fatalf("密钥未更新: %q", got)
	}
}

func TestSlotEnvOverride(t *testing.T) {
	f, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := f.SaveModel(ports.ModelInstance{Name: "A", Kind: "openai-chat", BaseURL: "https://a.example/v1", Model: "m", APIKey: "file-key"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SaveSlot("primary", true, created.ID); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TAVERNAGENT_KEY_PRIMARY", "env-key")
	raw, err := f.LoadCatalog(false)
	if err != nil {
		t.Fatal(err)
	}
	if got := slotByName(t, raw, "primary").APIKey; got != "env-key" {
		t.Fatalf("环境变量未覆盖: %q", got)
	}
	masked, _ := f.LoadCatalog(true)
	if got := slotByName(t, masked, "primary").APIKey; got != ports.MaskAPIKey("env-key") {
		t.Fatalf("masked = %q", got)
	}
}

func TestMissingConfigDefaults(t *testing.T) {
	f, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cat, err := f.LoadCatalog(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Models) != 0 {
		t.Fatalf("全新目录不应有实例: %+v", cat.Models)
	}
	if len(cat.Slots) != 3 {
		t.Fatalf("slots = %d", len(cat.Slots))
	}
	for _, binding := range cat.Slots {
		if binding.Enabled {
			t.Fatalf("默认应未启用: %+v", binding)
		}
	}
}

func TestCorruptSettingsNeverOverwritten(t *testing.T) {
	dir := t.TempDir()
	f, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config", "settings.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.LoadCatalog(true); err == nil {
		t.Fatal("损坏的配置必须报错")
	}
	if _, err := f.SaveModel(ports.ModelInstance{Name: "A", Kind: "openai-chat", Model: "m"}); err == nil {
		t.Fatal("损坏的配置下保存必须失败，绝不覆盖")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "{not json" {
		t.Fatalf("损坏文件被改写了: %s", raw)
	}
}

func TestMigrateKeepsDistinctCredentialsApart(t *testing.T) {
	// 同一 (kind, baseUrl, model) 但密钥不同：必须保留成两个实例，
	// 迁移不得悄悄把一份可用凭据换成另一份。
	providers := `{"primary":{"slot":"primary","enabled":true,"kind":"openai-chat","baseUrl":"https://api.deepseek.com/v1","model":"deepseek-chat","apiKeyRef":"primary"},
	               "assist":{"slot":"assist","enabled":true,"kind":"openai-chat","baseUrl":"https://api.deepseek.com/v1","model":"deepseek-chat","apiKeyRef":"key_p1"}}`
	profiles := `{"p1":{"id":"p1","name":"档案里的密钥","config":{"kind":"openai-chat","baseUrl":"https://api.deepseek.com/v1","model":"deepseek-chat","apiKeyRef":"key_p1"}}}`
	dir := writeLegacySettings(t, t.TempDir(), providers, profiles)
	store := openStore(t, dir)
	if _, err := store.MigrateLegacy(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cat, err := store.LoadCatalog(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Models) != 2 {
		t.Fatalf("密钥不同的槽位不应合并: %+v", cat.Models)
	}
	assist := slotByName(t, cat, "assist")
	if assist.APIKey != "sk-profile-one" {
		t.Fatalf("辅助应继续用档案密钥: %+v", assist)
	}
	primary := slotByName(t, cat, "primary")
	if primary.APIKey != "sk-slot-primary" {
		t.Fatalf("主线应继续用槽位自己的密钥: %+v", primary)
	}
	if primary.ResolvedModelID == assist.ResolvedModelID {
		t.Fatal("两个槽位不应指向同一个实例")
	}
}
