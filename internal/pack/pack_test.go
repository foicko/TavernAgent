package pack

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"tavernagent/internal/domain"
)

// ---- F6：剧情包编解码与安全（T21 / T22）----

// sampleBundle 返回一个结构完整的最小会话快照。
func sampleBundle() *domain.SessionBundle {
	root := &domain.PlotNode{
		NodeID: "root_1", SessionID: "sess_1", Kind: domain.NodeKindRoot,
		Depth: 0, SchemaVersion: 1,
		ContentJSON: `{"cardRef":"tpl_1","templates":{"character":{"templateVersionId":"tpl_1"}},"openingText":"开场。"}`,
	}
	turn := &domain.PlotNode{
		NodeID: "node_1", SessionID: "sess_1", ParentID: "root_1", Kind: domain.NodeKindTurn,
		Depth: 1, TurnNumber: 1, SchemaVersion: 1,
		ContentJSON: `{"inputKind":"text","inputText":"你好。","blocks":[{"kind":"narration","text":"她点头。"}],"options":[]}`,
	}
	return &domain.SessionBundle{
		FormatVersion:  domain.PackFormatVersion,
		RulesetVersion: "ruleset.simplified.v1",
		Session:        &domain.Session{SessionID: "sess_1", RootNodeID: "root_1", Title: "测试会话", CreatedAt: time.Unix(1700000000, 0).UTC()},
		RootNodeID:     "root_1",
		Templates: []*domain.TemplateVersion{
			{TemplateVersionID: "tpl_1", Kind: domain.TemplateCharacter, SchemaVersion: 2, Content: `{"name":"Elena"}`, ContentHash: "h1"},
		},
		Nodes:     []*domain.PlotNode{root, turn},
		Events:    []*domain.DomainEvent{{EventID: "ev_1", NodeID: "node_1", EventIndex: 0, Type: domain.EventMemoryAdd, PayloadJSON: `{}`, RulesetVersion: "ruleset.simplified.v1"}},
		Branches:  []*domain.Branch{{BranchID: "branch_1", SessionID: "sess_1", Name: "main", HeadNodeID: "node_1", Version: 1}},
		Memories:  []*domain.MemoryRecord{{MemoryID: "mem_1", SourceNodeID: "node_1", Kind: domain.MemoryObserved, Content: "她点头。"}},
		Bookmarks: []domain.Bookmark{},
		Snapshots: []*domain.StateSnapshot{{NodeID: "node_1", SnapshotVersion: 1, RulesetVersion: "ruleset.simplified.v1", StateJSON: `{}`, StateHash: "sh"}},
	}
}

func writeSample(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if _, err := Write(&buf, sampleBundle(), "test", time.Unix(1700000000, 0)); err != nil {
		t.Fatalf("write: %v", err)
	}
	return buf.Bytes()
}

func readPack(t *testing.T, data []byte) *ReadResult {
	t.Helper()
	res, err := Read(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return res
}

// 正常往返：写出的包能被读回，且关键内容一致。
func TestWriteReadRoundTrip(t *testing.T) {
	data := writeSample(t)
	res := readPack(t, data)

	if res.Manifest.Format != Format || res.Manifest.FormatVersion != domain.PackFormatVersion {
		t.Fatalf("清单格式异常: %+v", res.Manifest)
	}
	if res.Bundle.RootNodeID != "root_1" {
		t.Fatalf("根节点 = %q", res.Bundle.RootNodeID)
	}
	if len(res.Bundle.Nodes) != 2 || len(res.Bundle.Events) != 1 || len(res.Bundle.Branches) != 1 {
		t.Fatalf("内容数量不符: nodes=%d events=%d branches=%d",
			len(res.Bundle.Nodes), len(res.Bundle.Events), len(res.Bundle.Branches))
	}
	if len(res.Bundle.Memories) != 1 || res.Bundle.Memories[0].Content != "她点头。" {
		t.Fatalf("记忆未被携带: %+v", res.Bundle.Memories)
	}
	if len(res.Bundle.Templates) != 1 {
		t.Fatalf("模板未被携带")
	}
	if res.Manifest.Counts["nodes"] != 2 {
		t.Fatalf("清单统计不符: %+v", res.Manifest.Counts)
	}
}

// rewriteZip 解开 ZIP、按需改写条目后重新打包。
// 用于构造恶意或损坏的包——这类输入不可能由 Write 生成。
func rewriteZip(t *testing.T, data []byte, mutate func(entries map[string][]byte)) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	entries := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open entry: %v", err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		entries[f.Name] = b
	}
	mutate(entries)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, b := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create entry: %v", err)
		}
		if _, err := w.Write(b); err != nil {
			t.Fatalf("write entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// 目录穿越必须被拒绝（T22）。
func TestRejectsPathTraversal(t *testing.T) {
	data := rewriteZip(t, writeSample(t), func(entries map[string][]byte) {
		entries["../evil.json"] = []byte(`{"x":1}`)
	})
	_, err := Read(bytes.NewReader(data), int64(len(data)))
	var sec *SecurityError
	if !errors.As(err, &sec) {
		t.Fatalf("目录穿越应被安全拒绝，实际: %v", err)
	}
	if !strings.Contains(sec.Reason, "traversal") {
		t.Fatalf("原因应指明穿越: %s", sec.Reason)
	}
}

// 绝对路径必须被拒绝。
func TestRejectsAbsolutePath(t *testing.T) {
	for _, name := range []string{"/etc/passwd", `C:\Windows\system.ini`, `\\server\share\x`} {
		data := rewriteZip(t, writeSample(t), func(entries map[string][]byte) {
			entries[name] = []byte("x")
		})
		_, err := Read(bytes.NewReader(data), int64(len(data)))
		var sec *SecurityError
		if !errors.As(err, &sec) {
			t.Fatalf("绝对路径 %q 应被拒绝，实际: %v", name, err)
		}
	}
}

// 重复归档路径必须被拒绝。
func TestRejectsDuplicateEntry(t *testing.T) {
	base := writeSample(t)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// 同名条目写两次（不经过 map，才能造出重复）。
	zr, _ := zip.NewReader(bytes.NewReader(base), int64(len(base)))
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		for i := 0; i < 2; i++ {
			w, _ := zw.Create(f.Name)
			w.Write(b)
		}
	}
	zw.Close()

	_, err := Read(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	var sec *SecurityError
	if !errors.As(err, &sec) {
		t.Fatalf("重复条目应被拒绝，实际: %v", err)
	}
}

// 符号链接条目必须被拒绝：它可以把解压引到包外。
func TestRejectsSymlinkEntry(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	hdr := &zip.FileHeader{Name: "link", Method: zip.Store}
	hdr.SetMode(0o120777) // 符号链接
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		t.Fatalf("create header: %v", err)
	}
	w.Write([]byte("../../etc/passwd"))
	zw.Close()

	_, err = Read(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err == nil {
		t.Fatalf("符号链接条目应被拒绝")
	}
}

// 摘要不匹配说明包被篡改或损坏，必须拒绝而不是"尽力读取"。
func TestRejectsChecksumMismatch(t *testing.T) {
	data := rewriteZip(t, writeSample(t), func(entries map[string][]byte) {
		// 等长替换：先过大小检查，才能验证摘要这一层。
		b := bytes.Replace(entries["nodes.jsonl"], []byte("root_1"), []byte("r00t_1"), 1)
		entries["nodes.jsonl"] = b
	})
	_, err := Read(bytes.NewReader(data), int64(len(data)))
	if err == nil {
		t.Fatalf("摘要不匹配应被拒绝")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("错误应指明摘要不匹配: %v", err)
	}
}

// 大小不匹配同样拒绝。
func TestRejectsSizeMismatch(t *testing.T) {
	data := rewriteZip(t, writeSample(t), func(entries map[string][]byte) {
		// 改长度但不改内容语义 → 与清单记录的 size 不符。
		entries["bookmarks.json"] = append(entries["bookmarks.json"], []byte(" ")...)
	})
	_, err := Read(bytes.NewReader(data), int64(len(data)))
	if err == nil {
		t.Fatalf("大小不匹配应被拒绝")
	}
}

// 不支持的格式版本：要么拒绝，要么迁移；这里选择明确拒绝。
func TestRejectsUnsupportedFormatVersion(t *testing.T) {
	data := rewriteZip(t, writeSample(t), func(entries map[string][]byte) {
		var m Manifest
		json.Unmarshal(entries["manifest.json"], &m)
		m.FormatVersion = 99
		b, _ := json.Marshal(m)
		entries["manifest.json"] = b
	})
	_, err := Read(bytes.NewReader(data), int64(len(data)))
	if err == nil || !strings.Contains(err.Error(), "formatVersion") {
		t.Fatalf("不支持的格式版本应被拒绝，实际: %v", err)
	}
}

// 缺少清单 → 不是剧情包。
func TestRejectsMissingManifest(t *testing.T) {
	data := rewriteZip(t, writeSample(t), func(entries map[string][]byte) {
		delete(entries, "manifest.json")
	})
	_, err := Read(bytes.NewReader(data), int64(len(data)))
	if err == nil || !strings.Contains(err.Error(), "missing manifest") {
		t.Fatalf("缺清单应被拒绝，实际: %v", err)
	}
}

// 非 ZIP 内容 → 明确报错而不是 panic。
func TestRejectsNonZip(t *testing.T) {
	body := []byte("this is not a zip file")
	if _, err := Read(bytes.NewReader(body), int64(len(body))); err == nil {
		t.Fatalf("非 ZIP 应被拒绝")
	}
}

// 超过硬上限的包体直接拒绝（在解压之前）。
func TestRejectsTooLarge(t *testing.T) {
	data := writeSample(t)
	_, err := Read(bytes.NewReader(data), MaxPackBytes+1)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("超大包应返回 ErrTooLarge，实际: %v", err)
	}
}

// 清单声明的文件在包里缺失 → 拒绝。
func TestRejectsMissingListedFile(t *testing.T) {
	data := rewriteZip(t, writeSample(t), func(entries map[string][]byte) {
		delete(entries, "events.jsonl")
	})
	_, err := Read(bytes.NewReader(data), int64(len(data)))
	if err == nil || !strings.Contains(err.Error(), "missing file") {
		t.Fatalf("缺文件应被拒绝，实际: %v", err)
	}
}

// JSONL 行内容不合法 → 拒绝（不静默跳过坏行）。
func TestRejectsMalformedJSONL(t *testing.T) {
	// 手改内容会让摘要不匹配，所以连同清单一起改。
	data := rewriteZip(t, writeSample(t), func(entries map[string][]byte) {
		bad := []byte("{not json}\n")
		entries["nodes.jsonl"] = bad
		var m Manifest
		json.Unmarshal(entries["manifest.json"], &m)
		for i := range m.Files {
			if m.Files[i].Path == "nodes.jsonl" {
				m.Files[i].Size = int64(len(bad))
				m.Files[i].SHA256 = hashBytes(bad)
			}
		}
		mb, _ := json.Marshal(m)
		entries["manifest.json"] = mb
	})
	_, err := Read(bytes.NewReader(data), int64(len(data)))
	if err == nil {
		t.Fatalf("坏 JSONL 应被拒绝")
	}
}

// 空切片编码为空文件，读回得到空切片而不是 nil（避免调用方判空方式不一致）。
func TestEmptyListsStayEmpty(t *testing.T) {
	b := sampleBundle()
	b.Memories = nil
	b.Bookmarks = nil
	var buf bytes.Buffer
	if _, err := Write(&buf, b, "test", time.Unix(1700000000, 0)); err != nil {
		t.Fatalf("write: %v", err)
	}
	res := readPack(t, buf.Bytes())
	if res.Bundle.Memories == nil || len(res.Bundle.Memories) != 0 {
		t.Fatalf("空记忆应读回空切片，实际 %#v", res.Bundle.Memories)
	}
}
