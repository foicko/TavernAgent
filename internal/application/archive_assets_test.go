package application

import (
	"bytes"
	"context"
	"testing"
	"time"

	"tavernagent/internal/adapters/cardassets"
	"tavernagent/internal/pack"
)

// 导出时携带卡资源，导入后资源应落到新会话角色名下。
func TestArchiveService_RoundTripsCardAssets(t *testing.T) {
	st, _, _, sid, _, _ := newTestServices(t, happyScript())
	bundle, err := st.ExportSession(sid, "")
	if err != nil {
		t.Fatal(err)
	}
	assets := map[string][]byte{
		"smile.webp": []byte("RIFF....WEBPVP8 "),
		"icon.png":   []byte("png-bytes"),
	}
	var buf bytes.Buffer
	if _, err := pack.WriteWithAssets(&buf, bundle, assets, "test", time.Now()); err != nil {
		t.Fatal(err)
	}

	store, err := cardassets.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewArchiveServiceWithAssets(st, store, "test")
	res, err := svc.Import(context.Background(), buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if res.Assets != 2 {
		t.Fatalf("import assets = %d，期望 2", res.Assets)
	}
	if res.Session.CharacterID == "" {
		t.Skip("测试会话没有角色 ID，无法断言资源归属")
	}
	names, err := store.ListAssets(res.Session.CharacterID)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatalf("落地资源 = %v，期望 2", names)
	}
	rc, _, err := store.OpenAsset(res.Session.CharacterID, "smile.webp")
	if err != nil {
		t.Fatal(err)
	}
	rc.Close()
}
