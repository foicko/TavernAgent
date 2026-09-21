import { useState } from "react";
import { generateImage, type GeneratedImageResult } from "../app/api";
import type { TextBlock } from "../app/types";
import { extractImagePrompt } from "../lib/imagePromptExtractor";
import {
  useImageSettings,
  type ImageAspectSize,
  type ImageStylePreset,
} from "../stores/imageSettingsStore";
import { Banner } from "../ui/Banner";
import { Button } from "../ui/Button";
import { Field, Select } from "../ui/Field";
import { Modal } from "../ui/Modal";
import "./ImageSettings.css";
import "./IllustrationModal.css";

export interface IllustrationModalProps {
  open: boolean;
  onClose: () => void;
  turnId: string;
  blocks?: TextBlock[];
  fallbackText?: string;
  characterName?: string;
}

export function IllustrationModal({
  open,
  onClose,
  turnId,
  blocks,
  fallbackText,
  characterName,
}: IllustrationModalProps) {
  const globalSize = useImageSettings((s) => s.size);
  const globalStyle = useImageSettings((s) => s.stylePreset);
  const engine = useImageSettings((s) => s.engine);
  const customBaseUrl = useImageSettings((s) => s.customBaseUrl);
  const customApiKey = useImageSettings((s) => s.customApiKey);
  const customModel = useImageSettings((s) => s.customModel);
  const attachIllustration = useImageSettings((s) => s.attachIllustration);

  const [stylePreset, setStylePreset] = useState<ImageStylePreset>(globalStyle);
  const [size, setSize] = useState<ImageAspectSize>(globalSize);

  const initialExtracted = extractImagePrompt(
    { blocks, text: fallbackText, characterName },
    stylePreset
  );

  const [prompt, setPrompt] = useState(initialExtracted.prompt);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [lastResult, setLastResult] = useState<GeneratedImageResult | null>(null);

  const handleGenerate = async () => {
    setError(null);
    setLoading(true);
    try {
      const res = await generateImage({
        prompt: prompt.trim(),
        engine,
        baseUrl: customBaseUrl,
        apiKey: customApiKey,
        model: customModel,
        size,
        style: stylePreset,
      });

      attachIllustration(turnId, res);
      setLastResult(res);
    } catch (err) {
      setError(err instanceof Error ? err.message : "生图失败，请检查图像配置");
    } finally {
      setLoading(false);
    }
  };

  const handleStyleChange = (nextStyle: ImageStylePreset) => {
    setStylePreset(nextStyle);
    const updated = extractImagePrompt(
      { blocks, text: fallbackText, characterName },
      nextStyle
    );
    setPrompt(updated.prompt);
  };

  if (!open) {
    return null;
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      label="生成场景插画"
      title="🎨 生成本幕场景插画"
      description="系统已根据当前剧情与人物动作提炼画面描述，你可以自由润色提示词后生成。"
      size="lg"
    >
      <div className="illustration-modal-body">
        <Field label="画面提示词 (Prompt)" hint="支持中英文描述，自动附带画质与风格修正词。">
          <textarea
            className="illustration-prompt-textarea"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="输入画面描述词..."
          />
        </Field>

        <div className="image-two-col">
          <Field label="艺术风格" hint="自动注入对应的专业画质正负向词。">
            <Select
              value={stylePreset}
              onChange={(e) => handleStyleChange(e.target.value as ImageStylePreset)}
            >
              <option value="anime">二次元动漫 (Anime Visual Novel)</option>
              <option value="fantasy">暗黑奇幻 (Dark High Fantasy)</option>
              <option value="cinematic">电影质感 (Cinematic 35mm)</option>
              <option value="realistic">超写实摄影 (Realistic Photography)</option>
            </Select>
          </Field>

          <Field label="画面比例" hint="根据场景用途选择最佳画幅。">
            <Select value={size} onChange={(e) => setSize(e.target.value as ImageAspectSize)}>
              <option value="1024x576">16:9 横屏 (1024x576) · 故事场景壁纸</option>
              <option value="1024x1024">1:1 方形 (1024x1024) · 角色肖像与道具</option>
              <option value="576x1024">9:16 竖屏 (576x1024) · 全身立绘与手机屏保</option>
            </Select>
          </Field>
        </div>

        <div className="image-action-row">
          <Button variant="primary" onClick={handleGenerate} disabled={loading || !prompt.trim()}>
            {loading ? "正在绘制画面..." : "🎨 开始绘制插画"}
          </Button>
          <Button variant="quiet" onClick={onClose}>
            完成 / 关闭
          </Button>
        </div>

        {error && <Banner tone="error">{error}</Banner>}

        {lastResult && (
          <div className="image-preview-box">
            <img src={lastResult.url} alt="生成结果" className="image-preview-img" />
            <div className="image-preview-meta">
              ✓ 插画已绘制成功并自动关联至本回合！
            </div>
          </div>
        )}
      </div>
    </Modal>
  );
}
