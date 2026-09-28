// Package cardassets 落地角色卡内嵌资源（charx 归档中的立绘/图标等）。
//
// 资源按 cardID 归档在 <root>/<cardID>/<file>：卡 JSON 里只保存引用名，
// 二进制本体留在文件系统。所有路径都经过清洗，绝不接受 .. 或分隔符，
// 因此调用方传入的 cardID / file 即使来自外部卡包也无法越出资源根目录。
package cardassets

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Store 是基于文件系统的卡资源存储。
type Store struct {
	root string
}

// New 创建存储并确保根目录存在。
func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("cardassets: empty root")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("cardassets: create root: %w", err)
	}
	return &Store{root: root}, nil
}

// Root 返回资源根目录（诊断用）。
func (s *Store) Root() string { return s.root }

// SaveAsset 写入单个资源。先写临时文件再原子改名，避免半截文件被读到。
func (s *Store) SaveAsset(cardID, file string, r io.Reader) error {
	dir, err := s.cardDir(cardID)
	if err != nil {
		return err
	}
	name, err := safeSegment(file)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cardassets: create card dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("cardassets: create temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("cardassets: write: %w", err)
	}
	// 落盘后再改名：崩溃不会留下"已改名但内容未刷盘"的半截文件。
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("cardassets: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("cardassets: close: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(dir, name)); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("cardassets: rename: %w", err)
	}
	return nil
}

// OpenAsset 读取单个资源，返回内容流与 MIME 类型。
func (s *Store) OpenAsset(cardID, file string) (io.ReadCloser, string, error) {
	dir, err := s.cardDir(cardID)
	if err != nil {
		return nil, "", err
	}
	name, err := safeSegment(file)
	if err != nil {
		return nil, "", err
	}
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", os.ErrNotExist
		}
		return nil, "", err
	}
	return f, sniffMime(f, name), nil
}

// sniffMime 按内容识别 MIME：charx 资源经常名实不符（扩展名 .png 实际是 WebP），
// 只信扩展名会把 WebP 标成 image/png。嗅探失败时回退扩展名，绝不因此拒绝提供资源。
func sniffMime(f *os.File, name string) string {
	var head [512]byte
	n, _ := io.ReadFull(f, head[:])
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return mimeForExt(filepath.Ext(name))
	}
	if n > 0 {
		// 只在内容确实是图片时才覆盖扩展名推断，避免把非图片内容误报成 text/plain。
		if mime := http.DetectContentType(head[:n]); strings.HasPrefix(mime, "image/") {
			return mime
		}
	}
	return mimeForExt(filepath.Ext(name))
}

// ListAssets 返回某张卡已落地的资源文件名（已清洗，可直接用于 URL）。无资源时返回空切片。
func (s *Store) ListAssets(cardID string) ([]string, error) {
	dir, err := s.cardDir(cardID)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out, nil
}

// DeleteCardAssets 移除某张卡的全部资源；目录不存在时视为成功（幂等）。
func (s *Store) DeleteCardAssets(cardID string) error {
	dir, err := s.cardDir(cardID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cardassets: delete: %w", err)
	}
	return nil
}

// cardDir 返回清洗后的卡资源目录，并校验它确实位于根目录之下。
func (s *Store) cardDir(cardID string) (string, error) {
	seg, err := safeSegment(cardID)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(s.root, seg)
	rel, err := filepath.Rel(s.root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cardassets: unsafe card id")
	}
	return dir, nil
}

// safeSegment 把外部标识清洗成单一安全的路径段。
// 只保留字母、数字、点、下划线与连字符；拒绝空、"."、".." 与前导点。
func safeSegment(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("cardassets: empty segment")
	}
	var b strings.Builder
	for _, r := range trimmed {
		switch {
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-'):
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), ".")
	if out == "" || out == "." || out == ".." {
		return "", fmt.Errorf("cardassets: unsafe segment %q", raw)
	}
	// Windows 保留设备名（含带扩展名形式）不能作为文件名，否则在 Windows 上创建失败或行为怪异。
	base := out
	if i := strings.IndexByte(out, '.'); i >= 0 {
		base = out[:i]
	}
	if reservedWindowsNames[strings.ToUpper(base)] {
		return "", fmt.Errorf("cardassets: reserved segment %q", raw)
	}
	return out, nil
}

// reservedWindowsNames 是 Windows 保留的设备名（大小写不敏感）。
var reservedWindowsNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// mimeForExt 按扩展名推断 MIME（卡资源常见类型）。
func mimeForExt(ext string) string {
	ext = strings.ToLower(ext)
	// 同名资源会带数字后缀（iconx.png_1）；去掉后按真实扩展名判断。
	if i := strings.LastIndexByte(ext, '_'); i > 0 {
		if _, err := strconv.Atoi(ext[i+1:]); err == nil {
			ext = ext[:i]
		}
	}
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".avif":
		return "image/avif"
	case ".gif":
		return "image/gif"
	case ".json":
		return "application/json"
	case ".risum", ".bin":
		return "application/octet-stream"
	default:
		return "application/octet-stream"
	}
}
