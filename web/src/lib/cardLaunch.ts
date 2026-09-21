// 从卡库打开一张角色卡：有对应存档就直接进故事，没有就用这张卡预填开局弹窗。
//
// 左栏卡库与命令面板共用这一份判定——同一个动作从两个入口进来必须是同一个结果，
// 否则会出现"从左栏点能进故事、从命令面板点却弹导入框"这种说不清的行为差异。
import { useStory } from "../stores/storyStore";
import { useUi } from "../stores/uiStore";

export interface LaunchableCard {
  cardId: string;
  characterId: string;
}

export function openCard(card: LaunchableCard): void {
  const { sessions, openSession } = useStory.getState();
  // characterId 与 cardId 都可能是会话里存的那个值（历史版本用过后一种），两边都认。
  const matched = sessions.find(
    (session) => session.characterId === card.characterId || session.characterId === card.cardId,
  );
  if (matched) {
    void openSession(matched.sessionId);
    return;
  }
  // 卡已经在服务端：直接预填开局弹窗，不必重新上传文件。
  useUi.getState().openCharImportWithCard(card.cardId);
}
