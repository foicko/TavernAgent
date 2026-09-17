package sqlite

import "testing"

// decodeIDList 必须容错：坏数据按空值继续、不 panic，合法数据正常解码。
func TestDecodeIDListToleratesCorruptData(t *testing.T) {
	var dst []string
	decodeIDList("不是 JSON", "ownerIds", &dst)
	if dst != nil {
		t.Fatalf("损坏输入应保持零值，得到 %v", dst)
	}
	decodeIDList("", "ownerIds", &dst)
	if dst != nil {
		t.Fatalf("空输入应为 no-op，得到 %v", dst)
	}
	decodeIDList(`["alice","bob"]`, "entities", &dst)
	if len(dst) != 2 || dst[0] != "alice" || dst[1] != "bob" {
		t.Fatalf("合法输入解码错误: %v", dst)
	}
}
