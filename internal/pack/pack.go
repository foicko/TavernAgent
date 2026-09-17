// Package pack 读写剧情包（.tavernpack）的格式编解码。
//
// 它不实现任何端口、不接触存储：应用层负责取数与写库，这里只做
// 「领域快照 ↔ ZIP」的转换与安全/完整性校验。
//
// 包是 ZIP，含 manifest.json 与若干数据文件（技术契约 §12.2）：
//
//	manifest.json     自描述头：格式版本、导出范围、根节点、文件清单与 SHA-256
//	templates.json    模板版本（角色/玩家/世界书/规则）
//	nodes.jsonl       剧情节点
//	events.jsonl      已提交领域事件
//	branches.json     分支（写入位置）
//	memories.jsonl    来源化记忆（含 copy-on-write 的覆盖关系）
//	bookmarks.json    书签
//	snapshots.jsonl   稀疏检查点（可选，可校验或重建）
//	assets/           资源目录（当前无立绘等资源，保留结构）
//
// 刻意**不含**草稿帧、进行中的回合、API Key、访问会话与日志（T28）。
//
// 记忆单独成文件而不是靠事件重建：用户的修订（纠正/置顶/隐藏）是
// copy-on-write 的覆盖记录，不产生领域事件，只能随包携带。
package pack

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"tavernagent/internal/domain"
)

// 读取上限：剧情包来自外部，必须假设它可能恶意或损坏。
const (
	MaxPackBytes         = 64 << 20
	MaxEntries           = 512
	MaxUncompressedTotal = 256 << 20
	MaxEntryBytes        = 64 << 20
)

// Format 是包的格式标识（manifest.format）。
const Format = "tavernpack"

// ErrTooLarge 表示包体超过硬上限。
var ErrTooLarge = errors.New("pack: pack too large")

// SecurityError 表示包触犯了路径或容量类限制（T22）。
// 与「格式损坏」区分开：前者是恶意或越权，后者只是内容不对。
type SecurityError struct{ Reason string }

func (e *SecurityError) Error() string { return "archive security: " + e.Reason }

func unsafe(reason string) error { return &SecurityError{Reason: reason} }

// 包内固定文件名。
const (
	fileManifest  = "manifest.json"
	fileTemplates = "templates.json"
	fileNodes     = "nodes.jsonl"
	fileEvents    = "events.jsonl"
	fileBranches  = "branches.json"
	fileMemories  = "memories.jsonl"
	fileReceipts  = "receipts.jsonl"
	fileBookmarks = "bookmarks.json"
	fileSnapshots = "snapshots.jsonl"
	dirAssets     = "assets/"
)

// Manifest 是剧情包的自描述头。
type Manifest struct {
	Format         string         `json:"format"`
	FormatVersion  int            `json:"formatVersion"`
	AppVersion     string         `json:"appVersion,omitempty"`
	ExportedAt     string         `json:"exportedAt"`
	RulesetVersion string         `json:"rulesetVersion,omitempty"`
	Scope          Scope          `json:"scope"`
	Session        SessionMeta    `json:"session"`
	Counts         map[string]int `json:"counts"`
	Files          []FileEntry    `json:"files"`
}

// Scope 描述导出范围。
type Scope struct {
	Full       bool   `json:"full"`
	BranchID   string `json:"branchId,omitempty"`
	BranchName string `json:"branchName,omitempty"`
}

// SessionMeta 是会话元数据（不含 ID：导入会分配新的）。
type SessionMeta struct {
	Title       string `json:"title"`
	RootNodeID  string `json:"rootNodeId"`
	CreatedAt   string `json:"createdAt,omitempty"`
	CharacterID string `json:"characterId,omitempty"`
}

// FileEntry 是清单里的一条文件记录。
type FileEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// ReadResult 是解包结果。
type ReadResult struct {
	Manifest *Manifest
	Bundle   *domain.SessionBundle
	Assets   map[string][]byte
}

// Write 把会话快照编码为剧情包，并返回写入清单。
// now 由调用方注入以便测试确定性；为空时取当前时间。
func Write(w io.Writer, bundle *domain.SessionBundle, appVersion string, now time.Time) (*Manifest, error) {
	if bundle == nil || bundle.Session == nil {
		return nil, fmt.Errorf("pack: empty bundle")
	}
	if now.IsZero() {
		now = time.Now()
	}

	payloads := map[string][]byte{}
	// putJSON 写入 JSON 文档；putJSONL 写入逐行 JSON。
	// 两者必须分开：把 []byte 交给 json.Marshal 会得到 base64 字符串，
	// 读回时第一行就不是对象了。
	putJSON := func(name string, v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		payloads[name] = b
		return nil
	}
	if err := putJSON(fileTemplates, map[string]any{"templates": bundle.Templates}); err != nil {
		return nil, err
	}
	if err := putJSONL(payloads, fileNodes, bundle.Nodes); err != nil {
		return nil, err
	}
	if err := putJSONL(payloads, fileEvents, bundle.Events); err != nil {
		return nil, err
	}
	if err := putJSON(fileBranches, map[string]any{"branches": bundle.Branches}); err != nil {
		return nil, err
	}
	if err := putJSONL(payloads, fileMemories, bundle.Memories); err != nil {
		return nil, err
	}
	if err := putJSON(fileBookmarks, map[string]any{"bookmarks": bundle.Bookmarks}); err != nil {
		return nil, err
	}
	if err := putJSONL(payloads, fileSnapshots, bundle.Snapshots); err != nil {
		return nil, err
	}
	if err := putJSONL(payloads, fileReceipts, bundle.Receipts); err != nil {
		return nil, err
	}

	entries := make([]FileEntry, 0, len(payloads))
	for _, name := range []string{fileTemplates, fileNodes, fileEvents, fileBranches, fileMemories, fileBookmarks, fileSnapshots, fileReceipts} {
		b := payloads[name]
		entries = append(entries, FileEntry{Path: name, Size: int64(len(b)), SHA256: hashBytes(b)})
	}

	manifest := &Manifest{
		Format:         Format,
		FormatVersion:  domain.PackVersionForEvents(bundle.Events),
		AppVersion:     appVersion,
		ExportedAt:     now.UTC().Format(time.RFC3339),
		RulesetVersion: bundle.RulesetVersion,
		Scope: Scope{
			Full: bundle.Scope.Full, BranchID: bundle.Scope.BranchID, BranchName: bundle.Scope.BranchName,
		},
		Session: SessionMeta{
			Title:       bundle.Session.Title,
			RootNodeID:  bundle.RootNodeID,
			CreatedAt:   bundle.Session.CreatedAt.UTC().Format(time.RFC3339),
			CharacterID: bundle.Session.CharacterID,
		},
		Counts: map[string]int{
			"templates": len(bundle.Templates),
			"nodes":     len(bundle.Nodes),
			"events":    len(bundle.Events),
			"branches":  len(bundle.Branches),
			"memories":  len(bundle.Memories),
			"bookmarks": len(bundle.Bookmarks),
			"snapshots": len(bundle.Snapshots),
			"receipts":  len(bundle.Receipts),
		},
		Files: entries,
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}

	zw := zip.NewWriter(w)
	// 先写清单，便于流式读取时尽早判断格式版本。
	if err := writeZipEntry(zw, fileManifest, manifestBytes); err != nil {
		return nil, err
	}
	for _, e := range entries {
		if err := writeZipEntry(zw, e.Path, payloads[e.Path]); err != nil {
			return nil, err
		}
	}
	// 资源目录：当前无内容，保留目录结构以便后续扩展。
	if _, err := zw.Create(dirAssets); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return manifest, nil
}

// Read 解包并校验剧情包。校验顺序是"先廉价后昂贵"：
// 包大小 → 条目安全 → 清单格式 → 逐文件大小与摘要 → 反序列化。
func Read(r io.ReaderAt, size int64) (*ReadResult, error) {
	if size > MaxPackBytes {
		return nil, ErrTooLarge
	}
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("pack: not a zip: %w", err)
	}
	if len(zr.File) > MaxEntries {
		return nil, unsafe(fmt.Sprintf("too many entries (%d)", len(zr.File)))
	}

	assets := map[string][]byte{}
	found := map[string]*zip.File{}
	var total int64
	seen := map[string]bool{}
	for _, f := range zr.File {
		p, err := safePath(f.Name)
		if err != nil {
			return nil, err
		}
		if seen[p] {
			return nil, unsafe(fmt.Sprintf("duplicate entry %q", p))
		}
		seen[p] = true
		if f.Mode()&0100000 != 0 { // 符号链接位
			return nil, unsafe(fmt.Sprintf("symlink entry not allowed: %q", p))
		}
		if int64(f.UncompressedSize64) > MaxEntryBytes {
			return nil, unsafe(fmt.Sprintf("entry too large: %q", p))
		}
		total += int64(f.UncompressedSize64)
		if total > MaxUncompressedTotal {
			return nil, unsafe("uncompressed size exceeds limit")
		}
		if strings.HasSuffix(p, "/") {
			continue
		}
		if strings.HasPrefix(p, dirAssets) {
			b, err := readZipEntry(f)
			if err != nil {
				return nil, err
			}
			assets[strings.TrimPrefix(p, dirAssets)] = b
			continue
		}
		found[p] = f
	}

	mf, ok := found[fileManifest]
	if !ok {
		return nil, fmt.Errorf("pack: missing %s", fileManifest)
	}
	mfBytes, err := readZipEntry(mf)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(mfBytes, &manifest); err != nil {
		return nil, fmt.Errorf("pack: manifest invalid: %w", err)
	}
	if manifest.Format != Format {
		return nil, fmt.Errorf("pack: not a %s pack (format=%q)", Format, manifest.Format)
	}
	if !domain.SupportedPackVersion(manifest.FormatVersion) {
		return nil, fmt.Errorf("pack: unsupported pack formatVersion %d", manifest.FormatVersion)
	}

	// 逐文件校验摘要，确保清单描述的内容与包内实际内容一致。
	payload := map[string][]byte{}
	for _, e := range manifest.Files {
		p, err := safePath(e.Path)
		if err != nil {
			return nil, err
		}
		f, ok := found[p]
		if !ok {
			return nil, fmt.Errorf("pack: manifest lists missing file %q", p)
		}
		b, err := readZipEntry(f)
		if err != nil {
			return nil, err
		}
		if int64(len(b)) != e.Size {
			return nil, fmt.Errorf("pack: size mismatch for %q", p)
		}
		if hashBytes(b) != e.SHA256 {
			return nil, fmt.Errorf("pack: checksum mismatch for %q", p)
		}
		payload[p] = b
	}

	bundle := &domain.SessionBundle{
		FormatVersion:  manifest.FormatVersion,
		RulesetVersion: manifest.RulesetVersion,
		AppVersion:     manifest.AppVersion,
		ExportedAt:     manifest.ExportedAt,
		Scope: domain.ExportScope{
			Full: manifest.Scope.Full, BranchID: manifest.Scope.BranchID, BranchName: manifest.Scope.BranchName,
		},
		RootNodeID: manifest.Session.RootNodeID,
		Session: &domain.Session{
			Title:          manifest.Session.Title,
			CharacterID:    manifest.Session.CharacterID,
			RulesetVersion: manifest.RulesetVersion,
		},
	}
	if t, err := time.Parse(time.RFC3339, manifest.Session.CreatedAt); err == nil {
		bundle.Session.CreatedAt = t
	}

	if b, ok := payload[fileTemplates]; ok {
		var w struct {
			Templates []*domain.TemplateVersion `json:"templates"`
		}
		if err := json.Unmarshal(b, &w); err != nil {
			return nil, fmt.Errorf("pack: templates invalid: %w", err)
		}
		bundle.Templates = w.Templates
	}
	if b, ok := payload[fileNodes]; ok {
		nodes, err := unmarshalJSONL[domain.PlotNode](b)
		if err != nil {
			return nil, fmt.Errorf("pack: nodes invalid: %w", err)
		}
		bundle.Nodes = pointers(nodes)
	}
	if b, ok := payload[fileEvents]; ok {
		events, err := unmarshalJSONL[domain.DomainEvent](b)
		if err != nil {
			return nil, fmt.Errorf("pack: events invalid: %w", err)
		}
		bundle.Events = pointers(events)
	}
	if b, ok := payload[fileBranches]; ok {
		var w struct {
			Branches []*domain.Branch `json:"branches"`
		}
		if err := json.Unmarshal(b, &w); err != nil {
			return nil, fmt.Errorf("pack: branches invalid: %w", err)
		}
		bundle.Branches = w.Branches
	}
	if b, ok := payload[fileMemories]; ok {
		memories, err := unmarshalJSONL[domain.MemoryRecord](b)
		if err != nil {
			return nil, fmt.Errorf("pack: memories invalid: %w", err)
		}
		bundle.Memories = pointers(memories)
	}
	if b, ok := payload[fileBookmarks]; ok {
		var w struct {
			Bookmarks []domain.Bookmark `json:"bookmarks"`
		}
		if err := json.Unmarshal(b, &w); err != nil {
			return nil, fmt.Errorf("pack: bookmarks invalid: %w", err)
		}
		bundle.Bookmarks = w.Bookmarks
	}
	if b, ok := payload[fileSnapshots]; ok {
		snapshots, err := unmarshalJSONL[domain.StateSnapshot](b)
		if err != nil {
			return nil, fmt.Errorf("pack: snapshots invalid: %w", err)
		}
		bundle.Snapshots = pointers(snapshots)
	}
	if b, ok := payload[fileReceipts]; ok {
		receipts, err := unmarshalJSONL[domain.PortableReceipt](b)
		if err != nil {
			return nil, fmt.Errorf("pack: receipts invalid: %w", err)
		}
		bundle.Receipts = receipts
	}

	return &ReadResult{Manifest: &manifest, Bundle: bundle, Assets: assets}, nil
}

// safePath 校验归档条目路径：拒绝绝对路径、上级穿越与平台前缀。
// 这是"不信任外部输入"的第一道闸；解压前就用纯字符串判断，不依赖解压器行为。
func safePath(name string) (string, error) {
	if name == "" {
		return "", unsafe("empty entry name")
	}
	// Windows 盘符或 UNC 前缀。
	if strings.Contains(name, ":") {
		return "", unsafe(fmt.Sprintf("illegal entry path %q", name))
	}
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) {
		return "", unsafe(fmt.Sprintf("absolute entry path %q", name))
	}
	clean := path.Clean(strings.ReplaceAll(name, `\`, "/"))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", unsafe(fmt.Sprintf("path traversal in %q", name))
	}
	if clean == "." {
		return "", unsafe(fmt.Sprintf("illegal entry path %q", name))
	}
	return clean, nil
}

func writeZipEntry(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(rc, MaxEntryBytes+1)); err != nil {
		return nil, err
	}
	if int64(buf.Len()) > MaxEntryBytes {
		return nil, unsafe(fmt.Sprintf("entry %q exceeds size limit", f.Name))
	}
	return buf.Bytes(), nil
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// TemplateRefsOfRoot 从根节点内容里提取模板版本引用（去重并排序）。
//
// 这是**持久化格式**的知识（知道 contentJson 里有 cardRef 与
// templates.*.templateVersionId），因此属于 pack 而不是 domain。
// 模板表是全局的（没有 session_id），导出与导入校验只能靠根快照的引用
// 反查"哪些模板属于本会话"。排序是为了让导出结果稳定可比。
func TemplateRefsOfRoot(contentJSON string) []string {
	var rc struct {
		CardRef   string `json:"cardRef"`
		Templates map[string]struct {
			TemplateVersionID string `json:"templateVersionId"`
		} `json:"templates"`
	}
	if err := json.Unmarshal([]byte(contentJSON), &rc); err != nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(rc.Templates)+1)
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	add(rc.CardRef)
	for _, t := range rc.Templates {
		add(t.TemplateVersionID)
	}
	sort.Strings(out)
	return out
}

// putJSONL 把切片编码为逐行 JSON 并登记进待写入集合。
// （泛型函数而非闭包：闭包无法带类型参数，泛型函数可以。）
func putJSONL[T any](payloads map[string][]byte, name string, items []T) error {
	b, err := jsonl(items)
	if err != nil {
		return err
	}
	payloads[name] = b
	return nil
}

// jsonl 把切片编码为逐行 JSON（每行一个对象；空切片得到空文件）。
// 编码失败直接报错而不是跳过：静默丢节点或事件会让包"看起来正常"却缺内容。
func jsonl[T any](items []T) ([]byte, error) {
	var buf bytes.Buffer
	for i := range items {
		b, err := json.Marshal(items[i])
		if err != nil {
			return nil, fmt.Errorf("pack: encode line %d: %w", i+1, err)
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// unmarshalJSONL 解析逐行 JSON；空行跳过。
func unmarshalJSONL[T any](data []byte) ([]T, error) {
	out := []T{}
	for i, line := range bytes.Split(data, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var v T
		if err := json.Unmarshal(line, &v); err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// pointers 把值切片转成指针切片（包的读入侧统一用值切片解析）。
func pointers[T any](items []T) []*T {
	out := make([]*T, 0, len(items))
	for i := range items {
		out = append(out, &items[i])
	}
	return out
}
