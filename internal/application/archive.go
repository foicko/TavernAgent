package application

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"tavernagent/internal/domain"
	"tavernagent/internal/pack"
	"tavernagent/internal/ports"
)

// ArchiveService 负责剧情包的导出与导入（M3 · T21/T22）。
//
// 职责划分：本服务负责取数与校验（业务规则），pack 包负责格式编解码与
// 安全/完整性校验（外部输入不可信），存储层负责在单个事务内写入与 ID 重映射。
type ArchiveService struct {
	store      ports.ArchiveStore
	appVersion string
}

// NewArchiveService 创建剧情包服务。
func NewArchiveService(store ports.Store, appVersion string) *ArchiveService {
	return &ArchiveService{store: store, appVersion: appVersion}
}

// ExportResult 是导出结果。
type ExportResult struct {
	Data     []byte
	Manifest *pack.Manifest
}

// Export 导出会话；branchID 为空表示导出整个会话。
func (s *ArchiveService) Export(ctx context.Context, sessionID, branchID string) (*ExportResult, error) {
	bundle, err := s.store.ExportSession(sessionID, branchID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, Err("NOT_FOUND", "会话或分支不存在", 404)
		}
		return nil, Err("STORAGE_UNAVAILABLE", "读取会话失败: "+err.Error(), 503)
	}
	var buf bytes.Buffer
	manifest, err := pack.Write(&buf, bundle, s.appVersion, time.Now())
	if err != nil {
		return nil, Err("EXPORT_FAILED", "生成剧情包失败: "+err.Error(), 500)
	}
	return &ExportResult{Data: buf.Bytes(), Manifest: manifest}, nil
}

// ImportResult 是导入结果。
type ImportResult struct {
	Session  *domain.Session
	Manifest *pack.Manifest
	Assets   int
}

// Import 校验并写入剧情包。
//
// 校验顺序：包级安全与完整性（pack）→ 结构与兼容性（本地）→ 单事务写入。
// 任何一步不通过都不会写入已有数据（T22：拒绝写入而不是留下半截状态）。
func (s *ArchiveService) Import(ctx context.Context, data []byte) (*ImportResult, error) {
	rr, err := pack.Read(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		var sec *pack.SecurityError
		switch {
		case errors.Is(err, pack.ErrTooLarge):
			return nil, Err("PACK_TOO_LARGE", "剧情包超过大小上限", 413)
		case errors.As(err, &sec):
			return nil, Err("PACK_UNSAFE", "剧情包被拒绝: "+sec.Reason, 400)
		default:
			return nil, Err("INVALID_PACK", "剧情包无效: "+err.Error(), 400)
		}
	}
	if err := validateBundle(rr.Bundle); err != nil {
		return nil, err
	}
	sess, err := s.store.ImportSession(rr.Bundle)
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "写入失败: "+err.Error(), 503)
	}
	return &ImportResult{Session: sess, Manifest: rr.Manifest, Assets: len(rr.Assets)}, nil
}

// validateBundle 做导入前的结构与兼容性校验（技术契约 §12.2 + T22）。
//
// 检查项：规则版本、单根、父引用存在、无环且全可达、分支头存在、
// 事件的节点归属与序号唯一、记忆来源与覆盖链接有效、根引用的模板齐全。
// 这些都是"写入前必须成立"的不变量——写进去再发现问题就晚了。
func validateBundle(b *domain.SessionBundle) error {
	if b == nil || b.Session == nil {
		return Err("INVALID_PACK", "剧情包缺少会话信息", 400)
	}
	if !domain.SupportedPackVersion(b.FormatVersion) {
		return Err("INVALID_PACK", fmt.Sprintf("不支持的包格式版本 %d", b.FormatVersion), 400)
	}

	// 节点：唯一 ID、恰好一个根。
	if len(b.Nodes) == 0 {
		return Err("INVALID_PACK", "剧情包没有任何节点", 400)
	}
	byID := make(map[string]*domain.PlotNode, len(b.Nodes))
	roots := make([]string, 0, 1)
	for _, n := range b.Nodes {
		if n == nil || n.NodeID == "" {
			return Err("INVALID_PACK", "存在空节点 ID", 400)
		}
		if _, dup := byID[n.NodeID]; dup {
			return Err("INVALID_PACK", "节点 ID 重复: "+n.NodeID, 400)
		}
		byID[n.NodeID] = n
		if n.ParentID == "" {
			roots = append(roots, n.NodeID)
		}
	}
	if len(roots) != 1 {
		return Err("INVALID_PACK", fmt.Sprintf("剧情包必须恰好有一个根节点，实际 %d 个", len(roots)), 400)
	}
	if b.RootNodeID != roots[0] {
		return Err("INVALID_PACK", "清单记录的根节点与节点树不一致", 400)
	}

	// 父引用必须存在，且从根出发全可达（不可达即意味着成环或多出一棵子树）。
	children := make(map[string][]string, len(b.Nodes))
	for _, n := range b.Nodes {
		if n.ParentID == "" {
			continue
		}
		if _, ok := byID[n.ParentID]; !ok {
			return Err("INVALID_PACK", fmt.Sprintf("节点 %s 的父节点 %s 不在包内", n.NodeID, n.ParentID), 400)
		}
		children[n.ParentID] = append(children[n.ParentID], n.NodeID)
	}
	visited := make(map[string]bool, len(b.Nodes))
	var walk func(id string) error
	walk = func(id string) error {
		if visited[id] {
			return Err("INVALID_PACK", "节点树存在环: "+id, 400)
		}
		visited[id] = true
		for _, c := range children[id] {
			if err := walk(c); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(roots[0]); err != nil {
		return err
	}
	if len(visited) != len(b.Nodes) {
		return Err("INVALID_PACK", "存在无法从根节点到达的节点（环或多根）", 400)
	}

	// 分支：至少一条，且头节点存在。
	if len(b.Branches) == 0 {
		return Err("INVALID_PACK", "剧情包没有任何分支", 400)
	}
	branchIDs := map[string]bool{}
	for _, br := range b.Branches {
		if br == nil || br.BranchID == "" || branchIDs[br.BranchID] {
			return Err("INVALID_PACK", "分支为空或重复", 400)
		}
		branchIDs[br.BranchID] = true
		if _, ok := byID[br.HeadNodeID]; !ok {
			return Err("INVALID_PACK", fmt.Sprintf("分支 %s 的头节点不存在", br.BranchID), 400)
		}
	}

	// 事件：归属节点存在，且同一节点内序号唯一。
	seenEvent := make(map[string]bool, len(b.Events))
	for _, ev := range b.Events {
		if ev == nil {
			return Err("INVALID_PACK", "事件为空", 400)
		}
		if _, ok := byID[ev.NodeID]; !ok {
			return Err("INVALID_PACK", fmt.Sprintf("事件指向不存在的节点 %s", ev.NodeID), 400)
		}
		key := fmt.Sprintf("%s#%d", ev.NodeID, ev.EventIndex)
		if seenEvent[key] {
			return Err("INVALID_PACK", "事件序号重复: "+key, 400)
		}
		seenEvent[key] = true
	}

	// 记忆：来源节点存在，覆盖链接指向包内记忆。
	memIDs := make(map[string]bool, len(b.Memories))
	for _, m := range b.Memories {
		if m == nil || m.MemoryID == "" {
			return Err("INVALID_PACK", "存在空记忆 ID", 400)
		}
		if memIDs[m.MemoryID] {
			return Err("INVALID_PACK", "记忆 ID 重复: "+m.MemoryID, 400)
		}
		memIDs[m.MemoryID] = true
	}
	for _, m := range b.Memories {
		if _, ok := byID[m.SourceNodeID]; !ok {
			return Err("INVALID_PACK", fmt.Sprintf("记忆 %s 的来源节点不存在", m.MemoryID), 400)
		}
		if m.Supersedes != "" && !memIDs[m.Supersedes] {
			return Err("INVALID_PACK", fmt.Sprintf("记忆 %s 的覆盖目标 %s 不在包内", m.MemoryID, m.Supersedes), 400)
		}
	}

	// 模板：根节点引用到的模板必须齐全，否则导入后会出现悬空引用。
	tplIDs := make(map[string]bool, len(b.Templates))
	for _, t := range b.Templates {
		if t == nil || t.TemplateVersionID == "" || tplIDs[t.TemplateVersionID] {
			return Err("INVALID_PACK", "模板为空或重复", 400)
		}
		tplIDs[t.TemplateVersionID] = true
	}
	root := byID[roots[0]]
	for _, ref := range pack.TemplateRefsOfRoot(root.ContentJSON) {
		if !tplIDs[ref] {
			return Err("INVALID_PACK", fmt.Sprintf("根节点引用的模板 %s 不在包内", ref), 400)
		}
	}
	if err := validateBundleRules(b, root); err != nil {
		return err
	}
	if err := validateBundleDirector(b, byID); err != nil {
		return err
	}
	return validateBundleReferences(b, byID)
}
