package application

import (
	"errors"
	"strings"
	"time"
	"unicode"

	"tavernagent/internal/domain"
	"tavernagent/internal/ports"
)

// CardService 编排角色卡库：导入即入库、列表、取用与移除。
//
// 它只依赖窄接口 ports.CardStore，不碰会话与模板——卡库与故事在这里是
// 明确分离的两件事：删卡不删故事，建会话才把卡内容冻结进模板。
type CardService struct {
	store ports.CardStore
}

func NewCardService(store ports.Store) *CardService { return &CardService{store: store} }

// SaveReport 把一次导入的归一化卡片写入卡库，返回卡库条目。
//
// 这是「导入即入库」的落点：旧流程只解析不保存，玩家以为卡已收录，
// 实际只是内存里的一份预览。重复导入同一张卡按 card_id 覆盖，不产生副本。
func (s *CardService) SaveReport(rep *ImportReport) (*domain.CharacterCardEntry, error) {
	if rep == nil || rep.Card == nil {
		return nil, Err("CARD_INVALID", "角色卡内容为空", 422)
	}
	cardJSON := rep.CardJSON()
	if strings.TrimSpace(cardJSON) == "" {
		return nil, Err("CARD_INVALID", "角色卡内容为空", 422)
	}
	avatar := rep.Card.Avatar
	if avatar == "" && len(rep.Card.Characters) > 0 {
		avatar = rep.Card.Characters[0].Avatar
	}
	now := time.Now()
	entry := &domain.CharacterCardEntry{
		CardID:        rep.Card.CardID,
		Name:          rep.Card.Name,
		ShortName:     CardShortName(rep.Card.Name),
		Avatar:        avatar,
		Format:        rep.Format,
		Role:          CardRole(rep.Card.Description),
		CharacterJSON: cardJSON,
		ContentHash:   hashString(cardJSON),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.store.SaveCard(entry); err != nil {
		if errors.Is(err, ports.ErrCardLibraryFull) {
			return nil, Err("CARD_LIBRARY_FULL", "角色卡库已满（上限 256 张），请先移除不再使用的卡", 409)
		}
		return nil, Err("STORAGE_UNAVAILABLE", "保存角色卡失败: "+err.Error(), 503)
	}
	return entry, nil
}

// List 返回卡库摘要（CharacterJSON 为空，避免下发兆级内容）。
func (s *CardService) List() ([]*domain.CharacterCardEntry, error) {
	cards, err := s.store.ListCards()
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "读取角色卡库失败: "+err.Error(), 503)
	}
	return cards, nil
}

// Get 返回单张卡的完整内容。
func (s *CardService) Get(cardID string) (*domain.CharacterCardEntry, error) {
	entry, err := s.store.GetCard(cardID)
	if errors.Is(err, ports.ErrNotFound) {
		return nil, Err("CARD_NOT_FOUND", "角色卡不存在: "+cardID, 404)
	}
	if err != nil {
		return nil, Err("STORAGE_UNAVAILABLE", "读取角色卡失败: "+err.Error(), 503)
	}
	return entry, nil
}

// Delete 从卡库移除一张卡。已不存在按幂等成功处理——「已经没有了」与
// 「删成功」对调用方是同一个结果，重复点击删除不该报错。
func (s *CardService) Delete(cardID string) error {
	err := s.store.DeleteCard(cardID)
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return Err("STORAGE_UNAVAILABLE", "移除角色卡失败: "+err.Error(), 503)
	}
	return nil
}

// CardShortName 从全名截取展示用短名（与前端 cleanShortName 同规则）。
// 例："强约™ APP - 世界第一强约交友平台" → "强约™ APP"。
func CardShortName(fullName string) string {
	name := strings.TrimSpace(fullName)
	if name == "" {
		return "自定义"
	}
	for _, sep := range []string{" - ", "-", "—", "·", "–", "_"} {
		if i := strings.Index(name, sep); i > 0 {
			if head := strings.TrimSpace(name[:i]); head != "" {
				return head
			}
		}
	}
	return name
}

// CardRole 从描述里挑出可读的身份标语（列表行副标题）。
//
// 与前端 cleanShortRole 同一意图：跳过空行、纯标记行与属性/JSON 行，
// 避免把 "{\"Name\": (\"Gael\")}" 这种内容当身份显示。返回固定占位串
// 时说明整段描述没有可用的人话。
func CardRole(description string) string {
	for _, raw := range strings.Split(description, "\n") {
		line := strings.TrimSpace(raw)
		line = strings.TrimSpace(strings.TrimLeft(line, ">*#-–—+•\t "))
		if line == "" || startsWithMarkup(line) {
			continue
		}
		if !hasWordChars(line) {
			continue
		}
		if len([]rune(line)) > 60 {
			line = string([]rune(line)[:55]) + "…"
		}
		return line
	}
	return "导入的角色卡"
}

// startsWithMarkup 判定属性/JSON 行（首字符是括号或引号）。
func startsWithMarkup(line string) bool {
	switch line[0] {
	case '{', '[', '(', '"':
		return true
	}
	return false
}

// hasWordChars 判定一行是否含字母或数字（中文可读文本必然满足）。
func hasWordChars(line string) bool {
	for _, r := range line {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
