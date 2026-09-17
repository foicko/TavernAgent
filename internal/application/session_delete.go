package application

import (
	"context"
	"errors"
	"strings"

	"tavernagent/internal/ports"
)

// DeleteSession 永久删除一个会话及其全部故事数据。
//
// 这是本服务里唯一不可撤销的操作，因此刻意保守：
//   - 会话不存在 → 404（重复删除得到同样的回答，不会假装成功）；
//   - 仍有进行中的回合 → 409，让调用方先取消或等它结束，避免删除与在途写入竞争；
//   - 其余存储错误 → 503（沿用既有错误映射）。
//
// 共享模板（template_versions）不随会话删除：它们按内容寻址、可能被其它会话复用。
func (s *SessionService) DeleteSession(ctx context.Context, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return Err("BAD_REQUEST", "缺少会话 ID", 400)
	}
	err := s.store.DeleteSession(id)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ports.ErrNotFound):
		return Err("NOT_FOUND", "会话不存在", 404)
	case errors.Is(err, ports.ErrSessionBusy):
		return Err("SESSION_BUSY", "该故事还有正在进行的回合，请先取消或等它结束后再删除", 409)
	default:
		return Err("STORAGE_UNAVAILABLE", "删除故事失败: "+err.Error(), 503)
	}
}
