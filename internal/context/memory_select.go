package context

import (
	"math"
	"sort"
	"strings"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
	"tavernagent/internal/search"
)

// ---- 记忆召回（来源化认知，技术契约 §8.1）----

// 评分权重就是契约 §8.1 给出的初始公式，不在代码里自行调参：
//
//	score = 0.45*textRank01 + 0.25*entityOverlap01 + 0.20*importance01 + 0.10*recency01
//
// 之所以把权重写成常量而不做成配置项：调参应当由语料评测驱动（Recall@5），
// 而不是让部署方凭手感改。要改就改契约与本文件的常量，并重跑评测。
const (
	weightMemoryTextRank   = 0.45
	weightMemoryEntity     = 0.25
	weightMemoryImportance = 0.20
	weightMemoryRecency    = 0.10
)

// recencyHalfLife 返回 recency 项的半衰期（回合数）：低/中/高重要度 → 10/30/90。
func recencyHalfLife(importance int) float64 {
	switch {
	case importance <= 3:
		return 10
	case importance <= 7:
		return 30
	default:
		return 90
	}
}

// memoryQueryText 是记忆检索用的查询文本，顺序即相关度优先级。
//
// 与 buildHaystack 的区别只在**顺序**：词元有上限（search.MaxQueryTerms），
// 先出现的先保留。因此当前输入与最近回合必须排在开场白之前——否则长会话里
// 被留下来的全是开场白，检索就退化成"按开场白找记忆"。
func memoryQueryText(opening string, recent []*domain.PlotNode, inputText string) string {
	var sb strings.Builder
	sb.WriteString(inputText)
	for i := len(recent) - 1; i >= 0; i-- {
		n := recent[i]
		if n.Kind != domain.NodeKindTurn {
			continue
		}
		tc, err := parseTurnContent(n.ContentJSON)
		if err != nil {
			continue
		}
		sb.WriteString("\n")
		sb.WriteString(tc.InputText)
		sb.WriteString("\n")
		sb.WriteString(renderBlocks(tc.Blocks))
	}
	sb.WriteString("\n")
	sb.WriteString(opening)
	return sb.String()
}

// memoryQueryEntities 找出检索文本里出现的实体 ID。
//
// 这些 ID 的词元会被**优先**加入词法查询：记忆正文经常不写名字
// （"她习惯在清晨磨刀"），只靠正文双字无法把"关于某人"的记忆捞出来，
// 而它们恰恰是最需要被召回的关系性认知。放在最前面是因为词元有上限。
func memoryQueryEntities(state *domain.WorldState, haystack string) []string {
	if state == nil {
		return nil
	}
	low := strings.ToLower(haystack)
	out := make([]string, 0, 4)
	for id, ch := range state.Characters {
		if id == "" {
			continue
		}
		hit := strings.Contains(low, strings.ToLower(id))
		for _, term := range ch.SearchTerms() {
			if hit {
				break
			}
			hit = strings.Contains(low, strings.ToLower(term))
		}
		if hit {
			out = append(out, id)
		}
	}
	for id, it := range state.Items {
		for _, term := range append([]string{it.Name}, it.Aliases...) {
			if term != "" && strings.Contains(low, strings.ToLower(term)) {
				out = append(out, id)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// memoryEntityTerms 把实体 ID 切成词元。逐个 ID 单独切分，
// 避免多个 ID 拼在一起时产生跨 ID 的假词元。
func memoryEntityTerms(ids []string) []string {
	var out []string
	for _, id := range ids {
		out = append(out, search.Tokenize(id)...)
	}
	return out
}

// recall 挑出与当前上下文相关的若干条记忆。
//
// 只读：不写库、不改状态、不因被检索而提升权重或刷新提及时间（T18）。
func (c *Compiler) recall(nodeID, haystack, queryText string, state *domain.WorldState, currentTurn int, inputTexts ...string) ([]*domain.MemoryRecord, error) {
	cands, err := c.memoryCandidates(nodeID, haystack, queryText, state)
	if err != nil {
		return nil, err
	}
	if len(cands) == 0 {
		return nil, nil
	}
	effective := effectiveMemoryIDs(cands)
	// 提及回合随候选一起返回（同一条查询、同一个祖先集合），不再单独查一次：
	// 那会为同一份可见性判断多走一遍 O(链深) 的递归遍历。
	target := queryText
	if len(inputTexts) > 0 && strings.TrimSpace(inputTexts[0]) != "" {
		target = inputTexts[0]
	}
	return c.selectMemories(cands, effective, state, haystack, currentTurn, target), nil
}

// memoryIDsOf 取注入清单（顺序无关，只用于提交时复核提及）。
func memoryIDsOf(memories []*domain.MemoryRecord) []string {
	if len(memories) == 0 {
		return nil
	}
	out := make([]string, 0, len(memories))
	for _, m := range memories {
		out = append(out, m.MemoryID)
	}
	return out
}

// memoryRefDisplayRunes 是单条展示引用的文本上限。
//
// 引用会被抄进节点内容，而节点内容会整包导出：不设上限时，一条异常长的
// 记忆会产生“每个回合都背一份”的体积放大。截断是展示层的取舍，并会显式补省略号，
// 读者不会把它误当成完整原文（完整原文在记忆面板里，带证据链）。
const memoryRefDisplayRunes = 240

// memoryRefsOf 把注入的记忆转成随节点落库的展示快照。
func memoryRefsOf(memories []*domain.MemoryRecord) []domain.MemoryRef {
	if len(memories) == 0 {
		return nil
	}
	out := make([]domain.MemoryRef, 0, len(memories))
	for _, m := range memories {
		if m == nil {
			continue
		}
		out = append(out, domain.MemoryRef{MemoryID: m.MemoryID, Text: truncateRunes(m.Content, memoryRefDisplayRunes), Kind: string(m.Kind)})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}

// effectiveMemoryIDs 解析路径上的 copy-on-write 覆盖关系（纠正/置顶/隐藏），
// 返回在当前路径上仍然生效的记录 ID 集合。
func effectiveMemoryIDs(cands []*ports.MemoryCandidate) map[string]bool {
	records := make([]*domain.MemoryRecord, 0, len(cands))
	for _, cand := range cands {
		records = append(records, cand.Memory)
	}
	out := make(map[string]bool, len(records))
	for _, m := range domain.ApplyMemoryOverlays(records) {
		out[m.MemoryID] = true
	}
	return out
}

// memoryCandidates 取得候选池。
//
// 主路径把检索下推到 FTS5 索引：候选池 = 词法前 N ∪ 置顶 ∪ 覆盖记录 ∪ 重要性保底（隐藏记录不进候选池，由管理视图单独处理），
// 全部已在 SQL 侧按路径可见性过滤（先在可见集合内排名，契约 §8.1）。
// 早期实现在 Go 侧对整条路径的记忆逐条算双字重合，代价与记忆总数同阶
// （万级记忆实测 134ms）。
//
// 兜底路径用于 FTS5 不可用的构建：此时没有排名可下推，只能全取回在 Go 侧算
// 重合度当名次——仍是纯词法，不做语义猜测。
func (c *Compiler) memoryCandidates(nodeID, haystack, queryText string, state *domain.WorldState) ([]*ports.MemoryCandidate, error) {
	if c.store.LexicalIndexAvailable() {
		// Expand only the current input, not incidental descriptions in history.
		currentInput := strings.SplitN(queryText, "\n", 2)[0]
		entities := memoryQueryEntities(state, currentInput)
		if len(entities) == 0 {
			entities = memoryQueryEntities(state, queryText)
		}
		focus := search.Tokens(currentInput + " " + strings.Join(search.ConceptTerms(currentInput), " "))
		terms := append(memoryEntityTerms(entities), focus...)
		terms = append(terms, search.Tokens(queryText)...)
		var owners, secrets []string
		if state != nil {
			owners = []string{}
			secrets = state.UnlockedSecrets
			for id, ch := range state.Characters {
				if ch.Participant || id == "player" {
					owners = append(owners, id)
				}
			}
		}
		fallbackLimit := 0
		if c.options.AllowUnrankedFallback {
			fallbackLimit = 20
		}
		return c.store.RecallMemories(nodeID, ports.MemoryRecallQuery{
			MatchExpr:     search.MatchExprOf(terms),
			RawText:       queryText,
			OwnerIDs:      owners,
			SecretIDs:     secrets,
			EntityIDs:     entities,
			FallbackLimit: fallbackLimit,
		})
	}
	cands, err := c.store.MemoriesOnPath(nodeID)
	if err != nil {
		return nil, err
	}
	hay := bigrams(strings.ToLower(haystack))
	for _, cand := range cands {
		n := bigramOverlap(cand.Memory.Content, hay)
		// 门限与旧实现一致（默认 2）：重合太少不算命中。
		if n >= c.options.MinMemoryBigramOverlap {
			// 借用同一套归一化：重合越多越相关，取负以复用"越小越相关"。
			cand.Rank = -float64(n)
			cand.HasRank = true
		}
	}
	return cands, nil
}

// memoryWeights 是四路打分权重（时序模式下重排，见 newMemoryScorer）。
type memoryWeights struct {
	textRank   float64
	entity     float64
	importance float64
	recency    float64
}

// scoredMemory 是一条候选记忆的打分结果。
type scoredMemory struct {
	memory     *domain.MemoryRecord
	score      float64
	textRank01 float64
	lastTurn   int
	hasRank    bool
}

// memoryScorer 是一次选择过程的共享状态：归一化依据、查询实体与保底计数
// 都在候选之间共享，因此必须随循环携带而不是每条候选重算。
type memoryScorer struct {
	c              *Compiler
	state          *domain.WorldState
	lowHay         string
	queryEntities  map[string]bool
	focused        map[string]float64
	w              memoryWeights
	minRank        float64
	maxRank        float64
	ranked         bool
	currentTurn    int
	unrankedCount  int
	maxUnranked    int
	minFallbackImp int
}

// selectMemories 按契约 §8.1 的公式打分、排序并应用预算。
// 支持通过 queryTexts 传入当前输入以识别时序检索意图（T2.1）。
func (c *Compiler) selectMemories(cands []*ports.MemoryCandidate, effective map[string]bool, state *domain.WorldState, haystack string, currentTurn int, queryTexts ...string) []*domain.MemoryRecord {
	checkText := haystack
	if len(queryTexts) > 0 && strings.TrimSpace(queryTexts[0]) != "" {
		checkText = queryTexts[0]
	}
	sc := c.newMemoryScorer(cands, effective, state, haystack, checkText, currentTurn)
	hits := make([]scoredMemory, 0, len(cands))
	for _, cand := range cands {
		if hit, ok := sc.score(cand, effective); ok {
			hits = append(hits, hit)
		}
	}
	if c.isTemporal(checkText) {
		applyTemporalBoost(hits, sc.queryEntities)
	}
	sortScoredMemories(hits)
	return c.budgetMemories(hits)
}

// newMemoryScorer 计算权重、名次归一化区间、查询实体与当前问题相关性。
func (c *Compiler) newMemoryScorer(cands []*ports.MemoryCandidate, effective map[string]bool, state *domain.WorldState, haystack, checkText string, currentTurn int) *memoryScorer {
	w := memoryWeights{textRank: weightMemoryTextRank, entity: weightMemoryEntity, importance: weightMemoryImportance, recency: weightMemoryRecency}
	if c.isTemporal(checkText) {
		w = memoryWeights{textRank: 0.30, entity: 0.20, importance: 0.20, recency: 0.30}
	}
	// 词法名次归一化到 0~1。契约明确要求：BM25 数值越小越相关，但**未经
	// 归一化不能与 1~10 的重要度直接相加**（两者量纲不同）。
	minRank, maxRank, ranked := 0.0, 0.0, false
	for _, cand := range cands {
		if !effective[cand.Memory.MemoryID] || !cand.HasRank {
			continue
		}
		if !ranked {
			minRank, maxRank, ranked = cand.Rank, cand.Rank, true
			continue
		}
		if cand.Rank < minRank {
			minRank = cand.Rank
		}
		if cand.Rank > maxRank {
			maxRank = cand.Rank
		}
	}
	queryEntities := map[string]bool{}
	for _, id := range memoryQueryEntities(state, checkText) {
		queryEntities[id] = true
	}
	if len(queryEntities) == 0 {
		for _, id := range memoryQueryEntities(state, haystack) {
			queryEntities[id] = true
		}
	}
	maxUnranked := c.options.MaxUnrankedFallback
	if maxUnranked <= 0 {
		maxUnranked = 2
	}
	minFallbackImp := c.options.MinFallbackImportance
	if minFallbackImp <= 0 {
		minFallbackImp = 7
	}
	return &memoryScorer{
		c: c, state: state, lowHay: strings.ToLower(haystack),
		queryEntities: queryEntities, focused: currentMemoryRelevance(cands, checkText, state),
		w: w, minRank: minRank, maxRank: maxRank, ranked: ranked, currentTurn: currentTurn,
		maxUnranked: maxUnranked, minFallbackImp: minFallbackImp,
	}
}

// score 为单条候选打分；ok=false 表示它被可见性/有效期/相关性门限挡掉。
func (sc *memoryScorer) score(cand *ports.MemoryCandidate, effective map[string]bool) (scoredMemory, bool) {
	m := cand.Memory
	if !domain.MemoryVisibleTo(m, sc.state) {
		return scoredMemory{}, false
	}
	if !effective[m.MemoryID] || m.Hidden {
		// 隐藏是「在本路径上不再注入」：覆盖记录带 hidden，原记录已被取代，
		// 因此这里只需跳过带 hidden 的生效记录（置顶也不能绕过，T17）。
		return scoredMemory{}, false
	}
	if m.ValidUntilTurn > 0 && sc.currentTurn >= m.ValidUntilTurn {
		return scoredMemory{}, false // 已失效（T3.2）
	}
	entityHit := memoryEntityHit(m, sc.state, sc.lowHay)
	// 相关性门限：没有词法命中也没有实体命中的记忆，除置顶与覆盖记录外，
	// 允许在 AllowUnrankedFallback 开启且达到最低重要度门槛时有限度作为保底候选入池（T2.2）。
	isUnranked := false
	if !cand.HasRank && !entityHit && !m.Pinned && m.Supersedes == "" {
		if !sc.c.options.AllowUnrankedFallback || sc.unrankedCount >= sc.maxUnranked || m.Importance < sc.minFallbackImp {
			return scoredMemory{}, false
		}
		isUnranked = true
		sc.unrankedCount++
	}

	var textRank01 float64
	if cand.HasRank && sc.ranked {
		textRank01 = 1
		// Reciprocal-rank fusion produces small magnitudes. An absolute
		// 0.005 threshold collapsed distinct relevant candidates to ties.
		if span := sc.maxRank - sc.minRank; span > 1e-12 {
			textRank01 = (sc.maxRank - cand.Rank) / span
		}
	}
	if len(sc.focused) > 0 {
		// The current question's distinctive content outranks incidental
		// history matches. Entity, importance and time still break ties.
		textRank01 = 0.25*textRank01 + 0.75*sc.focused[m.MemoryID]
	}

	entity01 := 0.0
	if n := len(m.EntityIDs); n > 0 {
		hit := 0
		for _, id := range m.EntityIDs {
			if sc.queryEntities[id] {
				hit++
			}
		}
		entity01 = float64(hit) / float64(n)
	}

	importance := domain.ClampMemoryImportance(m.Importance)
	importance01 := float64(importance-1) / 9

	// lastMeaningfulMentionTurn：路径上最近一次明确提及的回合。
	// 没有任何提及记录时用来源回合——产生这条记忆的那次提交本身
	// 就在正文里写了它，那是一次有依据的提及（不是凭空的默认值）。
	// 结合 T3.2：由"路径最近提及"扩展为 max(cand.SourceTurn, cand.LastMentionTurn, m.ValidFromTurn)
	last := cand.SourceTurn
	if cand.LastMentionTurn > last {
		last = cand.LastMentionTurn
	}
	if m.ValidFromTurn > last {
		last = m.ValidFromTurn
	}
	delta := float64(sc.currentTurn - last)
	if delta < 0 {
		delta = 0
	}
	recency01 := math.Pow(2, -delta/recencyHalfLife(importance))

	return scoredMemory{
		memory: m,
		score: sc.w.textRank*textRank01 +
			sc.w.entity*entity01 +
			sc.w.importance*importance01 +
			sc.w.recency*recency01,
		textRank01: textRank01,
		lastTurn:   last,
		hasRank:    cand.HasRank && !isUnranked,
	}, true
}

// applyTemporalBoost 是时序模式的后处理（T2.1）：识别与查询主体最相关的候选子集
// （属于查询实体且词法排名处于顶层），锁定其中最新发生/提及的回合并给予时序加成，
// 纠正 FTS5 词法同分时的早项偏置。
func applyTemporalBoost(hits []scoredMemory, queryEntities map[string]bool) {
	if len(hits) == 0 {
		return
	}
	matchesEntity := func(m *domain.MemoryRecord) bool {
		if len(queryEntities) == 0 {
			return true
		}
		for _, id := range m.EntityIDs {
			if queryEntities[id] {
				return true
			}
		}
		return false
	}

	maxTextRank := 0.0
	for _, h := range hits {
		if !h.hasRank || !matchesEntity(h.memory) {
			continue
		}
		if h.textRank01 > maxTextRank {
			maxTextRank = h.textRank01
		}
	}
	if maxTextRank <= 0 {
		return
	}
	threshold := maxTextRank - 0.15
	maxTurn := -1
	for _, h := range hits {
		if !h.hasRank || h.textRank01 < threshold || !matchesEntity(h.memory) {
			continue
		}
		if h.lastTurn > maxTurn {
			maxTurn = h.lastTurn
		}
	}
	if maxTurn < 0 {
		return
	}
	for i := range hits {
		h := &hits[i]
		if !h.hasRank || h.textRank01 < threshold || !matchesEntity(h.memory) {
			continue
		}
		if h.lastTurn == maxTurn {
			h.score += 0.25
		}
	}
}

// sortScoredMemories 排序稳定：得分降序；同分时优先取最近提及（lastTurn DESC），
// 再按 ID 升序兜底（T18/T2.1）。
func sortScoredMemories(hits []scoredMemory) {
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		if hits[i].lastTurn != hits[j].lastTurn {
			return hits[i].lastTurn > hits[j].lastTurn
		}
		return hits[i].memory.MemoryID < hits[j].memory.MemoryID
	})
}

// budgetMemories 应用条数与字符预算：置顶同样受约束，不允许无限挤占上下文。
func (c *Compiler) budgetMemories(hits []scoredMemory) []*domain.MemoryRecord {
	var out []*domain.MemoryRecord
	usedRunes := 0
	seenSubject := map[string]bool{}
	for _, h := range hits {
		if h.memory.SubjectKey != "" {
			if seenSubject[h.memory.SubjectKey] {
				continue // 同一 SubjectKey 仅保留得分最高/最新的一条（T3.1）
			}
			seenSubject[h.memory.SubjectKey] = true
		}
		n := len([]rune(h.memory.Content))
		if len(out) >= c.options.MaxMemories || usedRunes+n > c.options.MaxMemoryRunes {
			continue // 超预算跳过该条，后续更短的记忆仍有机会入选
		}
		out = append(out, h.memory)
		usedRunes += n
	}
	return out
}

// isTemporal 判断输入文本是否包含时序指示词（T2.1）。
func (c *Compiler) isTemporal(text string) bool {
	keywords := c.options.TemporalKeywords
	if len(keywords) == 0 {
		keywords = defaultTemporalKeywords()
	}
	low := strings.ToLower(text)
	for _, kw := range keywords {
		if kw != "" && strings.Contains(low, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// defaultTemporalKeywords 返回默认时序意图词列表（T2.1）。
func defaultTemporalKeywords() []string {
	return []string{
		"最近", "最新", "刚才", "上次", "最后", "上一次", "前不久", "之前", "刚", "最近一次",
	}
}

// memoryEntityHit 判断记忆声明的实体是否出现在当前上下文中。
// 优先用角色显示名匹配：实体 ID 形如 npc_xxx，一般不会出现在正文里；
// 同时兼容直接写出实体 ID 的情况（英文名/自定义 ID）。
func memoryEntityHit(m *domain.MemoryRecord, state *domain.WorldState, lowHaystack string) bool {
	for _, id := range m.EntityIDs {
		if id == "" {
			continue
		}
		if strings.Contains(lowHaystack, strings.ToLower(id)) {
			return true
		}
		if state == nil {
			continue
		}
		if ch, ok := state.Characters[id]; ok && strings.TrimSpace(ch.Name) != "" &&
			strings.Contains(lowHaystack, strings.ToLower(ch.Name)) {
			return true
		}
	}
	return false
}

// bigramOverlap 返回 text 的相邻双字与给定集合的重合个数。
// 中文没有空格分词，双字重合是不依赖外部分词器的确定性近似；
// 路线图把中文检索列为待验证项，FTS5 分词留作可替换后端。
func bigramOverlap(text string, set map[string]bool) int {
	n := 0
	for b := range bigrams(strings.ToLower(text)) {
		if set[b] {
			n++
		}
	}
	return n
}

// bigrams 返回文本的相邻双字集合（不足 2 字返回空集）。
func bigrams(s string) map[string]bool {
	r := []rune(s)
	out := make(map[string]bool, len(r))
	for i := 0; i+1 < len(r); i++ {
		out[string(r[i:i+2])] = true
	}
	return out
}
