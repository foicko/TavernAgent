package application

import (
	"encoding/json"
	"testing"
)

const uiProfileCardJSON = `{
  "spec": "chara_card_v3",
  "spec_version": "3.0",
  "data": {
    "name": "Ceres Test",
    "description": "A knight.",
    "personality": "calm",
    "scenario": "Academy",
    "first_mes": "Hello <img cmd=\"smile\">",
    "post_history_instructions": "Append [CeresStatus:date=1214-01-01|marks=none] at the end.",
    "assets": [
      {"type": "icon", "uri": "data:image/png;base64,AAAA", "name": "main", "ext": "png"},
      {"type": "x-risu-asset", "uri": "embeded://assets/other/image/a.webp", "name": "smile", "ext": "webp"},
      {"type": "x-risu-asset", "uri": "embeded://assets/other/audio/b.mp3", "name": "bgm", "ext": "mp3"}
    ],
    "character_book": {
      "entries": [
        {"keys": ["knight"], "content": "The [VITALS] block is required.", "enabled": true}
      ]
    },
    "extensions": {
      "risuai": {
        "backgroundHTML": "<style>.x{}</style>",
        "defaultVariables": "{\"aff\":\"0\",\"lang\":\"KO\"}",
        "viewScreen": "emotion",
        "sdData": [["solo"]],
        "lowLevelAccess": false,
        "additionalText": ["extra"]
      },
      "depth_prompt": {"depth": 0, "prompt": "stay in character"}
    }
  }
}`

func TestDetectCardUIProfile(t *testing.T) {
	rep, err := ImportCardJSON(uiProfileCardJSON)
	if err != nil {
		t.Fatalf("ImportCardJSON: %v", err)
	}
	if rep.Card.UIProfile == nil {
		t.Fatal("UIProfile is nil")
	}
	caps := rep.Card.UIProfile.Capabilities
	if !caps.HasBackgroundHTML {
		t.Error("HasBackgroundHTML = false, want true")
	}
	if !caps.HasDefaultVariables {
		t.Error("HasDefaultVariables = false, want true")
	}
	if !caps.HasDepthPrompt {
		t.Error("HasDepthPrompt = false, want true")
	}
	if caps.ViewScreen != "emotion" {
		t.Errorf("ViewScreen = %q, want emotion", caps.ViewScreen)
	}
	if !caps.ImgCmd {
		t.Error("ImgCmd = false, want true")
	}
	if caps.ImageAssetCount != 2 {
		t.Errorf("ImageAssetCount = %d, want 2", caps.ImageAssetCount)
	}
	if caps.AudioAssetCount != 1 {
		t.Errorf("AudioAssetCount = %d, want 1", caps.AudioAssetCount)
	}
	if caps.LorebookEntries != 1 {
		t.Errorf("LorebookEntries = %d, want 1", caps.LorebookEntries)
	}
	want := map[string]bool{"CeresStatus": true, "VITALS": true}
	for _, f := range caps.MarkerFamilies {
		delete(want, f)
	}
	if len(want) != 0 {
		t.Errorf("missing marker families: %v (got %v)", want, caps.MarkerFamilies)
	}
	if rep.Card.UIProfile.VarDefaults["lang"] != "KO" {
		t.Errorf("VarDefaults[lang] = %q, want KO", rep.Card.UIProfile.VarDefaults["lang"])
	}
}

// TestUIProfileSurvivesRoundTrip 保证归一化卡 JSON 重新解析后能力不丢：
// 归一化会丢掉 extensions，若合并逻辑写错就会清空已冻结的能力。
func TestUIProfileSurvivesRoundTrip(t *testing.T) {
	rep, err := ImportCardJSON(uiProfileCardJSON)
	if err != nil {
		t.Fatalf("ImportCardJSON: %v", err)
	}
	normalized := rep.CardJSON()
	re, err := ImportCardJSON(normalized)
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if re.Card.UIProfile == nil {
		t.Fatal("UIProfile lost after round-trip")
	}
	if !re.Card.UIProfile.Capabilities.HasBackgroundHTML {
		t.Error("HasBackgroundHTML lost after round-trip")
	}
	if re.Card.UIProfile.VarDefaults["aff"] != "0" {
		t.Errorf("VarDefaults lost after round-trip: %v", re.Card.UIProfile.VarDefaults)
	}
}

func TestExtractThemeTokensWhitelist(t *testing.T) {
	html := `<style>
	  :root{--fm-bg:#1b1c20; --fm-accent:#c98a5a; --fm-ink:#e9e6df;
	        --evil:url(javascript:alert(1)); --bad-font:expression(x);
	        --ui-font:'Noto Serif KR', serif; --size:12px;}
	</style>`
	tokens := extractThemeTokens(html)
	if tokens["bg"] != "#1b1c20" || tokens["accent"] != "#c98a5a" || tokens["ink"] != "#e9e6df" {
		t.Fatalf("tokens = %+v", tokens)
	}
	if tokens["font"] != "'Noto Serif KR', serif" {
		t.Fatalf("font = %q", tokens["font"])
	}
	// 非白名单键与危险值不得进入。
	for _, k := range []string{"size", "evil", "bad-font", "bad_font"} {
		if _, ok := tokens[k]; ok {
			t.Errorf("unexpected token %q", k)
		}
	}
}

func TestExtractThemeTokensRejectsUnsafe(t *testing.T) {
	if got := extractThemeTokens("--accent:url(evil)"); got != nil {
		t.Errorf("unsafe value accepted: %+v", got)
	}
	if got := extractThemeTokens("--font:javascript:x"); got != nil {
		t.Errorf("unsafe font accepted: %+v", got)
	}
}

func TestParseDefaultVariablesRejectsNonObject(t *testing.T) {
	if got := parseDefaultVariables("not json"); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	if got := parseDefaultVariables(""); got != nil {
		t.Errorf("expected nil for empty, got %v", got)
	}
}

func TestDetectMarkerFamiliesIgnoresLowercaseTags(t *testing.T) {
	got := detectMarkerFamilies("see [lowercase] and [STATUS:ok] and [1BAD:x]")
	want := []string{"STATUS"}
	if len(got) != len(want) || (len(got) == 1 && got[0] != want[0]) {
		t.Errorf("detectMarkerFamilies = %v, want %v", got, want)
	}
}

func TestDetectMarkerFamiliesExcludesStandardMarkers(t *testing.T) {
	// 标准标记（HUD/SCENE/VAR/IMG）不是「卡作者自定义标记」，不应进别名提示。
	got := detectMarkerFamilies("[HUD:hp=1] [SCENE:x] [VAR:k=v] [IMG:pic] [CeresStatus:date=x]")
	want := []string{"CeresStatus"}
	if len(got) != len(want) || (len(got) == 1 && got[0] != want[0]) {
		t.Errorf("detectMarkerFamilies = %v, want %v", got, want)
	}
}

func TestParseTogglesScalarOnly(t *testing.T) {
	got := parseToggles(json.RawMessage(`{"nudity":"off","gore":false,"level":3,"obj":{"x":1},"arr":[1]}`))
	if len(got) != 3 || got["nudity"] != "off" || got["gore"] != "false" || got["level"] != "3" {
		t.Errorf("parseToggles = %+v", got)
	}
	if parseToggles(json.RawMessage(`["not","object"]`)) != nil {
		t.Error("array input must be rejected")
	}
}

func TestParseSdDataFlattens(t *testing.T) {
	nested := parseSdData(json.RawMessage(`[["solo","1girl"],["masterpiece"]]`))
	if len(nested) != 3 || nested[0] != "solo" || nested[2] != "masterpiece" {
		t.Errorf("nested sdData = %v", nested)
	}
	flat := parseSdData(json.RawMessage(`["a","b",""]`))
	if len(flat) != 2 {
		t.Errorf("flat sdData = %v", flat)
	}
	if parseSdData(json.RawMessage(`{}`)) != nil {
		t.Error("object sdData must be rejected")
	}
}

func TestCapabilitiesMarshalStable(t *testing.T) {
	caps := CardCapabilities{HasModule: true, MarkerFamilies: []string{"A", "B"}}
	b, err := json.Marshal(caps)
	if err != nil {
		t.Fatal(err)
	}
	var back CardCapabilities
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !back.HasModule || len(back.MarkerFamilies) != 2 {
		t.Errorf("round-trip mismatch: %+v", back)
	}
}
