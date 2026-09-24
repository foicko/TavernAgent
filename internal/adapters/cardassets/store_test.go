package cardassets

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStore_SaveOpenRoundTrip(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAsset("card_1", "anis_smile.webp", strings.NewReader("payload")); err != nil {
		t.Fatal(err)
	}
	rc, mime, err := s.OpenAsset("card_1", "anis_smile.webp")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != "payload" {
		t.Fatalf("内容 = %q", data)
	}
	if mime != "image/webp" {
		t.Fatalf("mime = %q", mime)
	}
	names, err := s.ListAssets("card_1")
	if err != nil || len(names) != 1 || names[0] != "anis_smile.webp" {
		t.Fatalf("list = %v err=%v", names, err)
	}
}

func TestStore_SanitizesUnsafeSegments(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	// 空 / 点段应被拒绝。
	for _, bad := range []string{"", "..", "."} {
		if err := s.SaveAsset("card_1", bad, strings.NewReader("x")); err == nil {
			t.Fatalf("资源名 %q 应被拒绝", bad)
		}
	}
	// 含穿越意图的名字会被清洗为安全文件名，且绝不越出根目录。
	if err := s.SaveAsset("card_1", "../../evil.webp", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "evil.webp")); err == nil {
		t.Fatal("资源被写到了根目录之外")
	}
	names, err := s.ListAssets("card_1")
	if err != nil || len(names) != 1 {
		t.Fatalf("list = %v err=%v", names, err)
	}
	if strings.ContainsAny(names[0], "/\\") || strings.HasPrefix(names[0], ".") {
		t.Fatalf("清洗后的文件名仍不安全: %q", names[0])
	}
}

func TestStore_DeleteIsIdempotent(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAsset("card_1", "a.webp", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCardAssets("card_1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.OpenAsset("card_1", "a.webp"); !os.IsNotExist(err) {
		t.Fatalf("删除后应不存在，err=%v", err)
	}
	if err := s.DeleteCardAssets("card_1"); err != nil {
		t.Fatalf("重复删除应幂等: %v", err)
	}
}
