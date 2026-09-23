// FirstRunBanner：首次使用引导。未配置任何模型时，在舞台顶部提示去设置里添加连接。
// 用户可关闭，选择持久化在 localStorage，避免反复打扰。
import { useEffect, useState } from "react";
import { useSettings } from "../stores/settingsStore";
import { Banner } from "../ui/Banner";
import { Button } from "../ui/Button";

const DISMISS_KEY = "tavernagent.onboarding.model.dismissed";

export function FirstRunBanner() {
  const loaded = useSettings((s) => s.loaded);
  const models = useSettings((s) => s.models);
  const openSettings = useSettings((s) => s.openSettings);
  const load = useSettings((s) => s.load);
  const [dismissed, setDismissed] = useState(() => {
    try {
      return window.localStorage.getItem(DISMISS_KEY) === "1";
    } catch {
      return false;
    }
  });

  useEffect(() => {
    if (!loaded) void load();
  }, [loaded, load]);

  if (!loaded || dismissed || models.length > 0) return null;

  const dismiss = () => {
    setDismissed(true);
    try {
      window.localStorage.setItem(DISMISS_KEY, "1");
    } catch {
      // 隐私模式下写不进去也不影响本次隐藏
    }
  };

  return (
    <div style={{ margin: "0 auto 12px", maxWidth: "840px" }}>
      <Banner tone="info" onDismiss={dismiss}>
        还没有配置任何模型。先在设置里添加一条连接（地址 + 密钥），才能开始真实推演。
        <Button size="sm" onClick={openSettings}>配置模型</Button>
      </Banner>
    </div>
  );
}
