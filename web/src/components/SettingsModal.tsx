// SettingsModal：系统设置宿主。支持「模型与连接」、「语音设置 (TTS)」、「图像生成 (生图)」、「记忆向量 (Embedding)」四标签切换。
import { useEffect, useState } from "react";
import { useSettings } from "../stores/settingsStore";
import { Modal } from "../ui/Modal";
import { Tabs } from "../ui/Tabs";
import { ModelSettingsPage } from "./ModelSettingsPage";
import { TTSSettingsPage } from "./TTSSettingsPage";
import { ImageSettingsPage } from "./ImageSettingsPage";
import { EmbeddingSettingsPage } from "./EmbeddingSettingsPage";

export type SettingsTab = "models" | "tts" | "images" | "embedding";

const TAB_ITEMS: Array<{ value: SettingsTab; label: string }> = [
  { value: "models", label: "模型与连接" },
  { value: "tts", label: "语音设置 (TTS)" },
  { value: "images", label: "图像生成" },
  { value: "embedding", label: "记忆向量 (Embedding)" },
];

export function SettingsModal({
  onClose,
  initialTab = "models",
}: {
  onClose: () => void;
  initialTab?: SettingsTab;
}) {
  const load = useSettings((s) => s.load);
  const [activeTab, setActiveTab] = useState<SettingsTab>(initialTab);

  useEffect(() => {
    void load();
  }, [load]);

  const description =
    activeTab === "models"
      ? "先选好每个角色用哪个模型；地址与密钥只需要填一次。"
      : activeTab === "tts"
        ? "配置语音合成引擎、音色、本地/在线端点与角色对白朗读偏好。"
        : activeTab === "images"
          ? "配置生图引擎（SiliconFlow / DALL-E / 本地 SD WebUI）、画幅尺寸与画面艺术风格。"
          : "配置向量嵌入模型（默认 bge-small-zh-v1.5 / Ollama）、连通性测试与混合检索权重。";

  return (
    <Modal
      id="settings-modal"
      label="模型设置"
      title={
        <div className="settings-modal-header">
          <span className="settings-modal-title">设置</span>
          <Tabs label="设置分类" size="sm" value={activeTab} onChange={setActiveTab} items={TAB_ITEMS} className="settings-modal-tabs" />
        </div>
      }
      description={description}
      size="lg"
      onClose={onClose}
    >
      {activeTab === "models" ? (
        <ModelSettingsPage />
      ) : activeTab === "tts" ? (
        <TTSSettingsPage />
      ) : activeTab === "images" ? (
        <ImageSettingsPage />
      ) : (
        <EmbeddingSettingsPage />
      )}
    </Modal>
  );
}
