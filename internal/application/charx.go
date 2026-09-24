package application

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// charx 是 RisuAI 的 ZIP 角色卡归档：根级 card.json 为 chara_card_v3 信封，
// assets/** 为立绘/图标资源（以 assets[].name 作为正文 <img cmd="..."> 的键），
// module.risum 为 RisuAI 模块（脚本，本应用不执行），x_meta/** 为逐资源元数据。
const (
	charxCardEntry = "card.json"
	// maxCharxCardBytes 是 card.json 解压后的上限（防止压缩炸弹）。
	maxCharxCardBytes = 8 << 20
	// maxCharxEntryBytes 是单个资源解压后的上限。
	maxCharxEntryBytes = 64 << 20
	// maxCharxTotalBytes 是全部资源解压后的总量上限。
	maxCharxTotalBytes = 2 << 30
)

// CharxAsset 是 charx 归档中待落地的一项资源。
type CharxAsset struct {
	Name    string // assets[].name：正文 <img cmd="..."> 的键
	ZipPath string // 归档内路径（已去除 embeded:// 前缀）
	Ref     string // 落地文件名（清洗后，存于卡 JSON 的 assets[].ref）
	Ext     string // 扩展名（不含点）
}

// ImportCardCharx 解析 .charx 归档并返回兼容报告。
//
// archivePath 是已落盘的临时文件路径：这里只读取 card.json（有界）并登记资源清单，
// 真正的资源落地由 CardService 在建卡成功后流式完成（避免解析阶段写盘产生孤儿资源）。
func ImportCardCharx(archivePath string) (*ImportReport, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("charx 归档无法打开: %w", err)
	}
	defer zr.Close()

	entry := findCharxCardEntry(&zr.Reader)
	if entry == nil {
		return nil, errors.New("charx 归档中未找到 card.json")
	}
	if entry.UncompressedSize64 > maxCharxCardBytes {
		return nil, fmt.Errorf("charx 的 card.json 解压后超过 %d MB 上限", maxCharxCardBytes>>20)
	}
	payload, err := readZipEntry(entry, maxCharxCardBytes)
	if err != nil {
		return nil, fmt.Errorf("读取 card.json 失败: %w", err)
	}

	rep, err := decodeCardJSON(string(payload), "charx")
	if err != nil {
		return nil, err
	}
	// 保留 spec_version（V3），但把展示格式标记为 charx，让卡库与预览能区分来源。
	rep.Format = "charx"
	rep.CharxArchivePath = archivePath
	rep.Supported = append(rep.Supported, "charx 归档（card.json 与内嵌资源）")

	rep.CharxAssets = collectCharxAssets(&zr.Reader, rep.Card)
	attachCharxAvatar(rep, &zr.Reader)
	rep.warnCharx(&zr.Reader)
	return rep, nil
}

// findCharxCardEntry 定位 card.json：优先根级，其次最浅的同名条目。
func findCharxCardEntry(zr *zip.Reader) *zip.File {
	var fallback *zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := strings.TrimPrefix(strings.ReplaceAll(f.Name, "\\", "/"), "./")
		if name == charxCardEntry {
			return f
		}
		if strings.EqualFold(path.Base(name), charxCardEntry) && fallback == nil {
			fallback = f
		}
	}
	return fallback
}

// collectCharxAssets 把卡片声明的资源与归档条目对齐，登记待落地清单并回填 Ref。
//
// 只有 embeded:// 形式的资源会落地为文件；data:/http 资源保持原样（不复制）。
func collectCharxAssets(zr *zip.Reader, card *CharacterCard) []CharxAsset {
	if card == nil || len(card.Assets) == 0 {
		return nil
	}
	byPath := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		byPath[strings.TrimPrefix(strings.ReplaceAll(f.Name, "\\", "/"), "./")] = f
	}
	used := map[string]bool{}
	var out []CharxAsset
	for i := range card.Assets {
		a := &card.Assets[i]
		zipPath, ok := charxEmbeddedPath(a.URI)
		if !ok {
			continue
		}
		if _, exists := byPath[zipPath]; !exists {
			continue
		}
		ext := strings.TrimPrefix(a.Ext, ".")
		if ext == "" {
			ext = strings.TrimPrefix(path.Ext(zipPath), ".")
		}
		ref := uniqueAssetFile(a.Name, zipPath, ext, used)
		a.Ref = ref
		a.Ext = ext
		out = append(out, CharxAsset{Name: a.Name, ZipPath: zipPath, Ref: ref, Ext: ext})
	}
	return out
}

// charxEmbeddedPath 把 embeded://assets/... 解析为归档内路径。
func charxEmbeddedPath(uri string) (string, bool) {
	const prefix = "embeded://"
	if !strings.HasPrefix(uri, prefix) {
		return "", false
	}
	p := strings.TrimPrefix(uri, prefix)
	p = strings.TrimPrefix(strings.ReplaceAll(p, "\\", "/"), "/")
	if p == "" || strings.Contains(p, "..") {
		return "", false
	}
	return p, true
}

// uniqueAssetFile 由资源名与归档路径派生一个安全且唯一的落地文件名。
func uniqueAssetFile(name, zipPath, ext string, used map[string]bool) string {
	base := sanitizeAssetName(name)
	if base == "" {
		base = sanitizeAssetName(strings.TrimSuffix(path.Base(zipPath), path.Ext(zipPath)))
	}
	if base == "" {
		base = "asset"
	}
	if ext != "" && !strings.HasSuffix(strings.ToLower(base), "."+strings.ToLower(ext)) {
		base += "." + ext
	}
	candidate := base
	for i := 1; used[strings.ToLower(candidate)]; i++ {
		candidate = fmt.Sprintf("%s_%d", base, i)
	}
	used[strings.ToLower(candidate)] = true
	return candidate
}

// sanitizeAssetName 只保留字母、数字、点、下划线与连字符，避免路径穿越。
func sanitizeAssetName(raw string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(raw) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), ".")
}

// attachCharxAvatar 用归档中的图标资源生成有界头像缩略图。
// 依次尝试卡片声明的 icon 资源，再退回 assets/icon/** 下的任意图片。
func attachCharxAvatar(rep *ImportReport, zr *zip.Reader) {
	if rep == nil || rep.Card == nil {
		return
	}
	byPath := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		byPath[strings.TrimPrefix(strings.ReplaceAll(f.Name, "\\", "/"), "./")] = f
	}

	var candidates []string
	for _, a := range rep.Card.Assets {
		if a.Type == "icon" {
			if p, ok := charxEmbeddedPath(a.URI); ok {
				candidates = append(candidates, p)
			}
		}
	}
	for _, a := range rep.Card.Assets {
		if p, ok := charxEmbeddedPath(a.URI); ok {
			candidates = append(candidates, p)
		}
	}
	// 兜底：归档里 assets/icon/ 下的图片。
	var iconFiles []string
	for name := range byPath {
		if strings.HasPrefix(name, "assets/icon/") {
			iconFiles = append(iconFiles, name)
		}
	}
	sort.Strings(iconFiles)
	candidates = append(candidates, iconFiles...)

	for _, p := range candidates {
		f, ok := byPath[p]
		if !ok {
			continue
		}
		data, err := readZipEntry(f, maxCharxEntryBytes)
		if err != nil {
			continue
		}
		if avatar := buildAvatarDataURL(data); avatar != "" {
			rep.Card.Avatar = avatar
			if len(rep.Card.Characters) == 1 {
				rep.Card.Characters[0].Avatar = avatar
			}
			return
		}
	}
	rep.Warnings = append(rep.Warnings, "charx 图标资源无法解码，未生成头像缩略图（其余字段已正常导入）")
}

// warnCharx 补充 charx 专属的兼容提示。
func (r *ImportReport) warnCharx(zr *zip.Reader) {
	total := 0
	hasModule := false
	hasMeta := false
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := strings.TrimPrefix(strings.ReplaceAll(f.Name, "\\", "/"), "./")
		switch {
		case name == "module.risum":
			hasModule = true
		case strings.HasPrefix(name, "x_meta/"):
			hasMeta = true
		case name == charxCardEntry:
		default:
			total++
		}
	}
	if len(r.CharxAssets) > 0 {
		r.Supported = append(r.Supported, fmt.Sprintf("内嵌资源 %d 项（<img cmd> 可渲染）", len(r.CharxAssets)))
	}
	if hasModule {
		r.Warnings = append(r.Warnings, "charx 内含 module.risum（RisuAI 模块/脚本）：出于安全考虑不执行，仅作为普通资源保留")
	}
	if hasMeta {
		r.Warnings = append(r.Warnings, "charx 内含 x_meta 逐资源元数据：不参与导入，已忽略")
	}
	if extra := total - len(r.CharxAssets); extra > 0 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("charx 归档含 %d 个资源文件，其中 %d 个未被卡片 assets 引用，已忽略", total, extra))
	}
	if hasImgCmd(r.Card) {
		r.Warnings = append(r.Warnings, "开场或描述包含 RisuAI 图像指令 <img cmd=\"...\">：按纯文本保留，前端在资源存在时渲染为卡内图片")
	}
	if r.Card != nil && hasRisuExtension(r.RawJSON) {
		r.Warnings = append(r.Warnings, "卡片 extensions 含 risuai/depth_prompt 扩展：保留于原卡，不参与上下文注入")
	}
}

// hasImgCmd 检测开场/描述里是否含 RisuAI 图像指令。
func hasImgCmd(card *CharacterCard) bool {
	if card == nil {
		return false
	}
	if strings.Contains(card.Description, "<img") {
		return true
	}
	for _, v := range card.OpeningVariants {
		if strings.Contains(v.Text, "<img") {
			return true
		}
	}
	return false
}

// hasRisuExtension 粗略检测原始 JSON 是否带 RisuAI 扩展键。
func hasRisuExtension(raw string) bool {
	return strings.Contains(raw, "\"risuai\"") || strings.Contains(raw, "\"depth_prompt\"")
}

// readZipEntry 读取一个归档条目，最多 limit 字节（防止压缩炸弹）。
func readZipEntry(f *zip.File, limit int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, limit))
}
