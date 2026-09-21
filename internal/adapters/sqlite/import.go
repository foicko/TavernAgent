package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"tavernagent/internal/domain"
	"tavernagent/internal/util/id"
)

// ---- 剧情包：导入写入（M3 · T21）----

// ImportSession 在一个事务内写入整包，并一致重映射全部 ID。
//
// 为什么必须重映射：包里的 ID 来自导出时的库，直接沿用会与既有数据撞主键；
// 更重要的是「一致」——节点被引用在会话根、分支头、事件、记忆来源、
// 快照以及 contentJson 内的 optionRef 里，任何一处漏改都会造成悬空引用。
//
// 模板按 contentHash 复用：模板是内容寻址的，同一角色的同一版本
// 重复导入不应产生第二份副本；复用时根节点的 templateVersionId 也要跟着改。
//
// 顺序受外键约束：sessions → template_versions → plot_nodes → branches
// → domain_events/memory_records/state_snapshots → bookmarks。
type importIDMaps struct {
	nodeMap    map[string]string
	branchMap  map[string]string
	memoryMap  map[string]string
	tplMap     map[string]string
	receiptMap map[string]string
	turnMap    map[string]string
	rollMap    map[string]string
	rootID     string
}

func buildImportIDMaps(bundle *domain.SessionBundle) (*importIDMaps, error) {
	nodeMap := make(map[string]string, len(bundle.Nodes))
	for _, n := range bundle.Nodes {
		nodeMap[n.NodeID] = id.New()
	}
	branchMap := make(map[string]string, len(bundle.Branches))
	for _, b := range bundle.Branches {
		branchMap[b.BranchID] = id.New()
	}
	memoryMap := make(map[string]string, len(bundle.Memories))
	for _, m := range bundle.Memories {
		memoryMap[m.MemoryID] = id.New()
	}
	tplMap := make(map[string]string, len(bundle.Templates))
	receiptMap, turnMap, rollMap := map[string]string{}, map[string]string{}, map[string]string{}
	for _, p := range bundle.Receipts {
		if p.Receipt == nil {
			return nil, fmt.Errorf("import: nil receipt")
		}
		r := p.Receipt
		receiptMap[r.ReceiptID] = id.New()
		if turnMap[r.TurnID] == "" {
			turnMap[r.TurnID] = id.New()
		}
		action := r.ActionID
		if r.RollID != domain.RollID(r.BaseHeadID, r.ActionID, r.RulesetVersion) {
			action += ":" + r.RollID
		}
		rollMap[r.RollID] = domain.RollID(nodeMap[r.BaseHeadID], action, r.RulesetVersion)
	}

	rootID, ok := nodeMap[bundle.RootNodeID]
	if !ok {
		return nil, fmt.Errorf("import: root node %q not present in bundle", bundle.RootNodeID)
	}
	return &importIDMaps{
		nodeMap:    nodeMap,
		branchMap:  branchMap,
		memoryMap:  memoryMap,
		tplMap:     tplMap,
		receiptMap: receiptMap,
		turnMap:    turnMap,
		rollMap:    rollMap,
		rootID:     rootID,
	}, nil
}

func resolveSessionMetadata(bundle *domain.SessionBundle, newSessionID string) (string, string) {
	characterID := bundle.Session.CharacterID
	if characterID == "" {
		for _, t := range bundle.Templates {
			if t.Kind == domain.TemplateCharacter {
				characterID = domain.CardIdentity(t.Content)
				break
			}
		}
	}
	if characterID == "" {
		characterID = "legacy_" + newSessionID
	}
	ruleset := bundle.Session.RulesetVersion
	if ruleset == "" {
		ruleset = bundle.RulesetVersion
	}
	return characterID, ruleset
}

func (s *Store) importTemplates(tx *sql.Tx, templates []*domain.TemplateVersion, tplMap map[string]string, now string) error {
	for _, t := range templates {
		var existing string
		err := tx.QueryRow(`SELECT template_version_id FROM template_versions WHERE kind=? AND content_hash=? LIMIT 1`,
			string(t.Kind), t.ContentHash).Scan(&existing)
		if err == nil && existing != "" {
			tplMap[t.TemplateVersionID] = existing
			continue
		}
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("import step [template lookup]: %w", err)
		}
		newID := id.New()
		tplMap[t.TemplateVersionID] = newID
		if _, err := tx.Exec(`INSERT INTO template_versions(template_version_id, kind, schema_version, content, content_hash, created_at) VALUES(?,?,?,?,?,?)`,
			newID, string(t.Kind), t.SchemaVersion, t.Content, t.ContentHash, now); err != nil {
			return fmt.Errorf("import step [template insert]: %w", err)
		}
		indexedTemplate := *t
		indexedTemplate.TemplateVersionID = newID
		if err := indexLorebookTx(tx, &indexedTemplate); err != nil {
			return fmt.Errorf("import step [lorebook index]: %w", err)
		}
	}
	return nil
}

func (s *Store) importPlotNodes(tx *sql.Tx, nodes []*domain.PlotNode, maps *importIDMaps, newSessionID, now string) error {
	for _, n := range nodes {
		parent := ""
		if n.ParentID != "" {
			parent = maps.nodeMap[n.ParentID]
		}
		content, err := remapNodeContent(n.Kind, n.ContentJSON, maps.nodeMap, maps.tplMap)
		if err != nil {
			return fmt.Errorf("import step [node remap %s]: %w", n.NodeID, err)
		}
		content, err = remapExtraContent(content, maps.nodeMap, maps.memoryMap, maps.turnMap, maps.receiptMap, maps.rollMap)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO plot_nodes(node_id, session_id, parent_id, kind, depth, turn_number, schema_version, content_json, created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			maps.nodeMap[n.NodeID], newSessionID, parent, string(n.Kind), n.Depth, n.TurnNumber, n.SchemaVersion, content, now); err != nil {
			return fmt.Errorf("import step [node insert]: %w", err)
		}
	}
	return nil
}

func (s *Store) importBranches(tx *sql.Tx, branches []*domain.Branch, maps *importIDMaps, newSessionID, now string) error {
	for _, b := range branches {
		head, ok := maps.nodeMap[b.HeadNodeID]
		if !ok {
			return fmt.Errorf("import: branch %q head %q not present in bundle", b.BranchID, b.HeadNodeID)
		}
		// 导入的分支不带活动回合：草稿与进行中的回合不属于包内容。
		if _, err := tx.Exec(`INSERT INTO branches(branch_id, session_id, name, head_node_id, version, active_turn_id, created_at) VALUES(?,?,?,?,?,?,?)`,
			maps.branchMap[b.BranchID], newSessionID, b.Name, head, b.Version, "", now); err != nil {
			return fmt.Errorf("import step [branch]: %w", err)
		}
	}
	return nil
}

func (s *Store) importDomainEvents(tx *sql.Tx, events []*domain.DomainEvent, maps *importIDMaps) error {
	for _, ev := range events {
		nodeID, ok := maps.nodeMap[ev.NodeID]
		if !ok {
			return fmt.Errorf("import: event %q references unknown node %q", ev.EventID, ev.NodeID)
		}
		payload, err := remapEventPayload(ev.Type, ev.PayloadJSON, maps.nodeMap, maps.memoryMap, maps.receiptMap)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO domain_events(event_id, node_id, event_index, type, payload_json, ruleset_version) VALUES(?,?,?,?,?,?)`,
			id.New(), nodeID, ev.EventIndex, string(ev.Type), payload, ev.RulesetVersion); err != nil {
			return fmt.Errorf("import step [event]: %w", err)
		}
	}
	return nil
}

func (s *Store) importMemoryRecords(tx *sql.Tx, bundle *domain.SessionBundle, maps *importIDMaps) error {
	importTerms := map[string][]string{}
	for _, sn := range bundle.Snapshots {
		for id, t := range entityTermsMap(sn.StateJSON) {
			if _, ok := importTerms[id]; !ok {
				importTerms[id] = t
			}
		}
	}
	for _, m := range bundle.Memories {
		rec, err := remapMemory(m, maps.nodeMap, maps.memoryMap)
		if err != nil {
			return err
		}
		if err := s.insertMemoryTx(tx, rec, importTerms); err != nil {
			return fmt.Errorf("import step [memory]: %w", err)
		}
	}
	return nil
}

func (s *Store) importSnapshots(tx *sql.Tx, snapshots []*domain.StateSnapshot, maps *importIDMaps) error {
	for _, sn := range snapshots {
		nodeID, ok := maps.nodeMap[sn.NodeID]
		if !ok {
			continue
		}
		stateJSON, stateHash, err := remapSnapshot(sn.StateJSON, maps.nodeMap, maps.receiptMap)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR REPLACE INTO state_snapshots(node_id, snapshot_version, ruleset_version, state_json, state_hash) VALUES(?,?,?,?,?)`,
			nodeID, sn.SnapshotVersion, sn.RulesetVersion, stateJSON, stateHash); err != nil {
			return fmt.Errorf("import step [snapshot]: %w", err)
		}
	}
	return nil
}

func (s *Store) importReceipts(tx *sql.Tx, receipts []domain.PortableReceipt, maps *importIDMaps, firstBranchID, newSessionID, now string) error {
	createdTurns := map[string]bool{}
	for _, p := range receipts {
		r := p.Receipt
		turnID := maps.turnMap[r.TurnID]
		if !createdTurns[turnID] {
			if _, err := tx.Exec(`INSERT INTO turn_requests(turn_id,session_id,branch_id,idempotency_key,payload_hash,expected_head_id,expected_version,status,mode,ruleset_version,input_json,result_node_id,created_at,updated_at) VALUES(?,?,?,?,?,?,0,'committed','structured',?,'{}',?,?,?)`, turnID, newSessionID, maps.branchMap[firstBranchID], "import_"+turnID, "imported", maps.nodeMap[r.BaseHeadID], r.RulesetVersion, maps.nodeMap[p.NodeID], now, now); err != nil {
				return err
			}
			createdTurns[turnID] = true
		}
		cr, err := r.CheckResultOf()
		if err != nil {
			return err
		}
		cr.RollID = maps.rollMap[r.RollID]
		for i, e := range cr.Effects {
			payload, err := remapEventPayload(e.Type, string(e.Payload), maps.nodeMap, maps.memoryMap, maps.receiptMap)
			if err != nil {
				return err
			}
			cr.Effects[i].Payload = json.RawMessage(payload)
		}
		result, err := json.Marshal(cr)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO action_receipts(`+receiptCols+`) VALUES(?,?,?,?,?,?,?,?)`, maps.receiptMap[r.ReceiptID], turnID, r.ActionID, cr.RollID, maps.nodeMap[r.BaseHeadID], r.RulesetVersion, string(result), string(domain.ReceiptCommitted)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) importBookmarks(tx *sql.Tx, bookmarks []domain.Bookmark, maps *importIDMaps, newSessionID string) error {
	for _, b := range bookmarks {
		nodeID, ok := maps.nodeMap[b.NodeID]
		if !ok {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO bookmarks(bookmark_id, session_id, node_id, title) VALUES(?,?,?,?)`,
			id.New(), newSessionID, nodeID, b.Title); err != nil {
			return fmt.Errorf("import step [bookmark]: %w", err)
		}
	}
	return nil
}

func (s *Store) importDirectorProjections(tx *sql.Tx, events []*domain.DomainEvent, maps *importIDMaps) error {
	var directorNodes []*domain.PlotNode
	seenDirector := map[string]bool{}
	for _, ev := range events {
		if ev.Type != domain.EventDirectorChange || seenDirector[ev.NodeID] {
			continue
		}
		seenDirector[ev.NodeID] = true
		node, err := scanNode(tx.QueryRow(`SELECT `+nodeCols+` FROM plot_nodes WHERE node_id=?`, maps.nodeMap[ev.NodeID]))
		if err != nil {
			return err
		}
		directorNodes = append(directorNodes, node)
	}
	if err := rebuildDirectorProjectionsTx(tx, directorNodes); err != nil {
		return fmt.Errorf("import director projection: %w", err)
	}
	return nil
}

// ImportSession 在一个事务内写入整包，并一致重映射全部 ID。
//
// 为什么必须重映射：包里的 ID 来自导出时的库，直接沿用会与既有数据撞主键；
// 更重要的是「一致」——节点被引用在会话根、分支头、事件、记忆来源、
// 快照以及 contentJson 内的 optionRef 里，任何一处漏改都会造成悬空引用。
//
// 模板按 contentHash 复用：模板是内容寻址的，同一角色的同一版本
// 重复导入不应产生第二份副本；复用时根节点的 templateVersionId 也要跟着改。
//
// 顺序受外键约束：sessions → template_versions → plot_nodes → branches
// → domain_events/memory_records/state_snapshots → bookmarks。
func (s *Store) ImportSession(bundle *domain.SessionBundle) (*domain.Session, error) {
	if bundle == nil || bundle.Session == nil {
		return nil, fmt.Errorf("import: empty bundle")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 1. 先建纯内存 ID 映射（插入过程需要它改写引用）。
	maps, err := buildImportIDMaps(bundle)
	if err != nil {
		return nil, err
	}

	// 2. 会话（root_node_id 无外键，但值要正确）。
	newSessionID := id.New()
	characterID, ruleset := resolveSessionMetadata(bundle, newSessionID)
	now := s.now()

	if _, err := tx.Exec(`INSERT INTO sessions(session_id, root_node_id, title, created_at, ruleset_version, character_id, updated_at) VALUES(?,?,?,?,?,?,?)`,
		newSessionID, maps.rootID, bundle.Session.Title, now, ruleset, characterID, now); err != nil {
		return nil, fmt.Errorf("import step [session]: %w", err)
	}

	// 3. 模板：按 (kind, contentHash) 复用，否则新建。
	if err := s.importTemplates(tx, bundle.Templates, maps.tplMap, now); err != nil {
		return nil, err
	}

	// 4. 节点：contentJson 内的跨节点与模板引用一并改写。
	if err := s.importPlotNodes(tx, bundle.Nodes, maps, newSessionID, now); err != nil {
		return nil, err
	}

	// 5. 分支。
	if err := s.importBranches(tx, bundle.Branches, maps, newSessionID, now); err != nil {
		return nil, err
	}

	// 6. 事件。
	if err := s.importDomainEvents(tx, bundle.Events, maps); err != nil {
		return nil, err
	}

	// 7. 记忆：source_node_id 与 supersedes 都是引用，必须一起改写。
	if err := s.importMemoryRecords(tx, bundle, maps); err != nil {
		return nil, err
	}

	// 8. 稀疏检查点（可选内容；缺失时由 StateAt 用根快照 + 事件重放重建）。
	if err := s.importSnapshots(tx, bundle.Snapshots, maps); err != nil {
		return nil, err
	}

	// 9. 收据与检定流水。
	firstBranchID := ""
	if len(bundle.Branches) > 0 {
		firstBranchID = bundle.Branches[0].BranchID
	}
	if err := s.importReceipts(tx, bundle.Receipts, maps, firstBranchID, newSessionID, now); err != nil {
		return nil, err
	}

	// 10. 书签。
	if err := s.importBookmarks(tx, bundle.Bookmarks, maps, newSessionID); err != nil {
		return nil, err
	}

	// 11. 导演阶段。
	if err := s.importDirectorProjections(tx, bundle.Events, maps); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("import step [commit]: %w", err)
	}
	return &domain.Session{
		SessionID: newSessionID, RootNodeID: maps.rootID,
		Title: bundle.Session.Title, CreatedAt: bundle.Session.CreatedAt,
		CharacterID: characterID, RulesetVersion: ruleset, UpdatedAt: s.clock.Now(),
	}, nil
}

// remapNodeContent 改写节点内容里的跨引用。
//
// 两类节点各有引用：
//   - 根节点：cardRef 与 templates.*.templateVersionId 指向模板版本
//   - 回合节点：optionRef.nodeId 指向产生该选项的节点
//
// 只改这两处已知路径，不做"全字段字符串替换"——那会误伤正文里
// 恰好长得像 ID 的文本。
func remapNodeContent(kind domain.NodeKind, contentJSON string, nodeMap, tplMap map[string]string) (string, error) {
	if contentJSON == "" {
		return contentJSON, nil
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(contentJSON), &doc); err != nil {
		return "", err
	}

	switch kind {
	case domain.NodeKindRoot:
		if ref, ok := doc["cardRef"].(string); ok {
			if mapped, ok := tplMap[ref]; ok {
				doc["cardRef"] = mapped
			}
		}
		if tpls, ok := doc["templates"].(map[string]any); ok {
			for _, v := range tpls {
				m, ok := v.(map[string]any)
				if !ok {
					continue
				}
				if cur, ok := m["templateVersionId"].(string); ok {
					if mapped, ok := tplMap[cur]; ok {
						m["templateVersionId"] = mapped
					}
				}
			}
		}
	default:
		if ref, ok := doc["optionRef"].(map[string]any); ok {
			if cur, ok := ref["nodeId"].(string); ok && cur != "" {
				if mapped, ok := nodeMap[cur]; ok {
					ref["nodeId"] = mapped
				}
			}
		}
	}

	out, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
