// startupSession: 刷新/重开后回到上次打开的故事，而不是列表第一条。
// 这里只放"记住 id + 选择策略"两件事，纯函数便于单测；写入失败（隐私模式/配额）
// 一律忽略——记忆失败不能让打开故事失败。
const KEY = "tavernagent.lastSessionId";

export function rememberSession(sessionId: string | null) {
  try {
    if (sessionId) localStorage.setItem(KEY, sessionId);
    else localStorage.removeItem(KEY);
  } catch {
    // localStorage 不可用时不记忆，也不报错
  }
}

export function lastSessionId(): string | null {
  try {
    return localStorage.getItem(KEY) || null;
  } catch {
    return null;
  }
}

// chooseStartupSession 的优先级：上次打开的（且仍在列表里）→ 角色匹配 → 列表第一条。
// remembered 指向已删除的会话时自动降级，不让用户卡在空白页。
export function chooseStartupSession<T extends { sessionId: string }>(
  sessions: T[],
  remembered: string | null,
  matchesCharacter: (session: T) => boolean,
): T | null {
  if (sessions.length === 0) return null;
  const last = remembered ? sessions.find((s) => s.sessionId === remembered) : undefined;
  if (last) return last;
  return sessions.find(matchesCharacter) ?? sessions[0];
}
