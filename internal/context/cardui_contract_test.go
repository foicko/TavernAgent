package context

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// 卡 JSON 里的引号是转义过的（`<img cmd=\"x\">`）：引用键必须从**解码后**的文本里取，
// 否则会连反斜杠一起取出来，与资源名对不上而丢失"引用优先"。
func TestAttachCardImageKeys(t *testing.T) {
	cardJSON := `{"name":"晴","description":"她笑了。<img cmd=\"school.shy.1\"> 还有 [IMG:bikini.calm.1]",
		"assets":[{"name":"school.shy.1","ext":"avif"},{"name":"bikini.calm.1","ext":"avif"},
		          {"name":"school.smug.1","ext":"avif"},{"name":"bgm","ext":"mp3"}]}`
	var card struct {
		Assets []cardAssetJSON `json:"assets"`
	}
	if err := json.Unmarshal([]byte(cardJSON), &card); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	sc := sessionContext{CardUI: &cardUIInfo{}}
	attachCardImageKeys(&sc, cardJSON, card.Assets)
	want := []string{"school.shy.1", "bikini.calm.1", "school.smug.1"}
	if !reflect.DeepEqual(sc.CardUI.ImageKeys, want) {
		t.Fatalf("ImageKeys = %v, want %v（引用优先 + 其余按名排序 + 只收图片）", sc.CardUI.ImageKeys, want)
	}
	// 渲染进契约块
	if block := cardUIContractBlock(sc); !strings.Contains(block, "school.shy.1") {
		t.Fatalf("契约未列出资源键:\n%s", block)
	}
}

func TestAttachCardImageKeysNoopCases(t *testing.T) {
	assets := []cardAssetJSON{{Name: "a.png", Ext: "png"}}
	// 无卡片档案：不建档案（保持原生卡提示词字节稳定）
	sc := sessionContext{}
	attachCardImageKeys(&sc, `{}`, assets)
	if sc.CardUI != nil {
		t.Fatalf("无档案时不应创建 cardUI: %+v", sc.CardUI)
	}
	// 已有键：不动（冻结档案优先）
	sc2 := sessionContext{CardUI: &cardUIInfo{ImageKeys: []string{"frozen.png"}}}
	attachCardImageKeys(&sc2, `{}`, assets)
	if !reflect.DeepEqual(sc2.CardUI.ImageKeys, []string{"frozen.png"}) {
		t.Fatalf("已有键不应被覆盖: %v", sc2.CardUI.ImageKeys)
	}
	// 无资源：不动
	sc3 := sessionContext{CardUI: &cardUIInfo{}}
	attachCardImageKeys(&sc3, `{}`, nil)
	if len(sc3.CardUI.ImageKeys) != 0 {
		t.Fatalf("无资源时不应写入键: %v", sc3.CardUI.ImageKeys)
	}
}

func TestCardStringValuesWalksNestedStrings(t *testing.T) {
	raw := `{"a":"x","b":["y",{"c":"z"}],"d":3}`
	got := cardStringValues(raw)
	for _, want := range []string{"x", "y", "z"} {
		if !strings.Contains(got, want) {
			t.Fatalf("cardStringValues 缺少 %q: %q", want, got)
		}
	}
	if cardStringValues("{not json") != "" {
		t.Fatal("非法 JSON 应返回空串")
	}
}

// 契约块必须列出可用图片资源键，并明确"只能使用这些、逐字复制"；
// 无卡片档案时不注入（保持原生卡提示词字节稳定）。
func TestCardUIContractBlockListsImageKeys(t *testing.T) {
	sc := sessionContext{CardUI: &cardUIInfo{}}
	sc.CardUI.ImageKeys = []string{"school.annoyed.1", "school.shy.1"}
	sc.CardUI.Capabilities.MarkerFamilies = []string{"CeresStatus"}

	got := cardUIContractBlock(sc)
	for _, want := range []string{
		"school.annoyed.1", "school.shy.1",
		"只能使用下列键", "逐字复制",
		"CeresStatus",
		"不得自造或改写键名",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("契约缺少 %q:\n%s", want, got)
		}
	}
}

func TestCardUIContractBlockAbsentWithoutProfile(t *testing.T) {
	if got := cardUIContractBlock(sessionContext{}); got != "" {
		t.Fatalf("无卡片档案时不应注入契约，得到 %q", got)
	}
	// 有档案但无可用键：只注入静态契约，不出现键清单小节。
	sc := sessionContext{CardUI: &cardUIInfo{}}
	got := cardUIContractBlock(sc)
	if strings.Contains(got, "只能使用下列键") {
		t.Fatalf("无键时不应出现键清单：\n%s", got)
	}
}
