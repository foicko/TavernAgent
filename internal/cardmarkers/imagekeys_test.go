package cardmarkers

import (
	"fmt"
	"reflect"
	"testing"
)

// 卡正文引用过的键排前（模型最可能需要），其余按名称排序，只收图片类资源，去重。
func TestCollectImageKeys(t *testing.T) {
	assets := []ImageAsset{
		{Name: "school.annoyed.1", Ext: "avif"},
		{Name: "school.shy.1", Ext: "avif"},
		{Name: "bikini.calm.1", Ext: "avif"},
		{Name: "bgm_main", Ext: "mp3"}, // 音频不收
		{Ref: "only_ref.png", Ext: "png"},
		{Name: "school.annoyed.1", Ext: "avif"}, // 重复
		{Name: "ghost.key", Ext: "txt"},         // 非图片不收
	}
	text := `她笑了。<img cmd="school.shy.1"> 还有 [IMG:bikini.calm.1] 以及不存在的 [IMG:school.smug.1]`

	got := CollectImageKeys(assets, text, 0)
	want := []string{"school.shy.1", "bikini.calm.1", "only_ref.png", "school.annoyed.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CollectImageKeys = %v, want %v（引用优先 + 其余按名排序 + 去重 + 只收图片）", got, want)
	}
}

func TestCollectImageKeysNoReferenceAndEmpty(t *testing.T) {
	if got := CollectImageKeys([]ImageAsset{{Name: "b.png", Ext: "png"}, {Name: "a.webp", Ext: "webp"}}, "无标记", 0); !reflect.DeepEqual(got, []string{"a.webp", "b.png"}) {
		t.Fatalf("按名称排序 = %v", got)
	}
	if got := CollectImageKeys([]ImageAsset{{Name: "x", Ext: "mp3"}}, "", 0); got != nil {
		t.Fatalf("无图片资源应返回 nil，得到 %v", got)
	}
	if got := CollectImageKeys(nil, "", 0); got != nil {
		t.Fatalf("空资源应返回 nil，得到 %v", got)
	}
}

// 超过上限时截断（提示词必须有界），且不重复。
func TestCollectImageKeysBounded(t *testing.T) {
	var assets []ImageAsset
	for i := 0; i < ImageKeyLimit+25; i++ {
		assets = append(assets, ImageAsset{Name: fmt.Sprintf("k%03d", i), Ext: "png"})
	}
	got := CollectImageKeys(assets, "", 0)
	if len(got) != ImageKeyLimit {
		t.Fatalf("len = %d, want %d", len(got), ImageKeyLimit)
	}
	seen := map[string]bool{}
	for _, k := range got {
		if seen[k] {
			t.Fatalf("重复键 %q", k)
		}
		seen[k] = true
	}
	// 显式 limit 生效
	if got := CollectImageKeys(assets, "", 3); len(got) != 3 {
		t.Fatalf("limit=3 时 len = %d", len(got))
	}
}
