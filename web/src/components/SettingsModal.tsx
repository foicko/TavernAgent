// SettingsModal：模型设置宿主。只负责加载目录与套上弹窗外壳。
import { useEffect } from "react";
import { useSettings } from "../stores/settingsStore";
import { Modal } from "../ui/Modal";
import { ModelSettingsPage } from "./ModelSettingsPage";

export function SettingsModal({ onClose }: { onClose: () => void }) {
  const load = useSettings((s) => s.load);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <Modal
      id="settings-modal"
      label="模型设置"
      title="模型设置"
      description="先选好每个角色用哪个模型；地址与密钥只需要填一次。"
      size="lg"
      onClose={onClose}
    >
      <ModelSettingsPage />
    </Modal>
  );
}
