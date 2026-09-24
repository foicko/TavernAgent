// SceneBar：常驻场景标签，展示最近一次 [SCENE] 标记（服务端已归一化进 UIHints）。
//
// 只做展示，不改世界状态：权威场景仍由导演/规则驱动（见 docs/CARD_UI_MARKERS.md）。
import { useStory } from "../stores/storyStore";
import { latestSceneTitle } from "../lib/cardScene";
import { Icon } from "../ui/Icon";
import "./SceneBar.css";

export function SceneBar() {
  const title = useStory((s) => latestSceneTitle(s.messages));
  if (!title) return null;
  return (
    <div className="scene-bar">
      <Icon name="compass" size={13} />
      <span className="scene-bar-label">场景</span>
      <span className="scene-bar-title">{title}</span>
    </div>
  );
}
