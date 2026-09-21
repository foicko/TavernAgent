import { useState } from "react";
import { generateImage, type GeneratedImageResult } from "../app/api";
import {
  useImageSettings,
  type ImageAspectSize,
  type ImageEngine,
  type ImageStylePreset,
} from "../stores/imageSettingsStore";
import { Banner } from "../ui/Banner";
import { Button } from "../ui/Button";
import { Field, Select, TextInput } from "../ui/Field";
import { Section } from "../ui/Section";
import "./ImageSettings.css";

export function ImageSettingsPage() {
  const engine = useImageSettings((s) => s.engine);
  const customBaseUrl = useImageSettings((s) => s.customBaseUrl);
  const customApiKey = useImageSettings((s) => s.customApiKey);
  const customModel = useImageSettings((s) => s.customModel);
  const size = useImageSettings((s) => s.size);
  const stylePreset = useImageSettings((s) => s.stylePreset);

  const setEngine = useImageSettings((s) => s.setEngine);
  const setCustomBaseUrl = useImageSettings((s) => s.setCustomBaseUrl);
  const setCustomApiKey = useImageSettings((s) => s.setCustomApiKey);
  const setCustomModel = useImageSettings((s) => s.setCustomModel);
  const setSize = useImageSettings((s) => s.setSize);
  const setStylePreset = useImageSettings((s) => s.setStylePreset);
  const applyComfyUIPreset = useImageSettings((s) => s.applyComfyUIPreset);
  const applySiliconFlowPreset = useImageSettings((s) => s.applySiliconFlowPreset);
  const applyOpenAIPreset = useImageSettings((s) => s.applyOpenAIPreset);
  const applySDWebUIPreset = useImageSettings((s) => s.applySDWebUIPreset);
  const resetToDefaults = useImageSettings((s) => s.resetToDefaults);

  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<GeneratedImageResult | null>(null);
  const [testError, setTestError] = useState<string | null>(null);

  const handleTestGenerate = async () => {
    setTestError(null);
    setTesting(true);
    setTestResult(null);
    try {
      const res = await generateImage({
        prompt: "A magical tavern at night, warm lantern light, fantasy anime style",
        engine,
        baseUrl: customBaseUrl,
        apiKey: customApiKey,
        model: customModel,
        size,
      });
      setTestResult(res);
    } catch (err) {
      setTestError(err instanceof Error ? err.message : "生图测试失败，请检查服务地址与 API Key");
    } finally {
      setTesting(false);
    }
  };

  return (
    <div className="image-settings-page">
      <Section title="生图引擎配置" description="选择用于生成剧情插画与角色立绘的绘图后端。">
        <div className="image-preset-chips">
          <span className="image-preset-label">快捷预设：</span>
          <Button size="sm" variant="quiet" onClick={applyComfyUIPreset}>
            本地 ComfyUI (Qwen 2.1 / 推荐)
          </Button>
          <Button size="sm" variant="quiet" onClick={applySiliconFlowPreset}>
            硅基流动 (Flux)
          </Button>
          <Button size="sm" variant="quiet" onClick={applyOpenAIPreset}>
            OpenAI DALL-E 3
          </Button>
          <Button size="sm" variant="quiet" onClick={applySDWebUIPreset}>
            本地 SD WebUI (Forge)
          </Button>
        </div>

        <Field label="引擎类型" hint="支持本地 ComfyUI 工作流、OpenAI 兼容云端端点（SiliconFlow / DALL-E）或本地 SD WebUI。">
          <Select value={engine} onChange={(e) => setEngine(e.target.value as ImageEngine)}>
            <option value="comfyui">本地 ComfyUI (Qwen 2.1 极速出图 / 默认)</option>
            <option value="openai">OpenAI 兼容接口 (SiliconFlow / DALL-E 3 / Flux)</option>
            <option value="sd-webui">本地 SD-WebUI / Forge API (127.0.0.1:7860)</option>
          </Select>
        </Field>

        <Field label="服务 Base URL" hint="云端接口或本地 WebUI / ComfyUI 地址（无需输入结尾的 /images/generations 或 /prompt）。">
          <TextInput
            type="text"
            placeholder={
              engine === "comfyui"
                ? "http://127.0.0.1:8188"
                : engine === "sd-webui"
                ? "http://127.0.0.1:7860"
                : "https://api.siliconflow.cn/v1"
            }
            value={customBaseUrl}
            onChange={(e) => setCustomBaseUrl(e.target.value)}
          />
        </Field>

        <div className="image-two-col">
          <Field label="模型标识 (Model / 工作流)" hint="例如 image_qwen_image_2_1_t2i, black-forest-labs/FLUX.1-schnell, dall-e-3">
            <TextInput
              type="text"
              placeholder={engine === "comfyui" ? "image_qwen_image_2_1_t2i" : "black-forest-labs/FLUX.1-schnell"}
              value={customModel}
              onChange={(e) => setCustomModel(e.target.value)}
            />
          </Field>

          <Field label="API Key (密钥)" hint="本地开源模型通常留空；在线服务请填入 Key。">
            <TextInput
              type="password"
              placeholder="sk-..."
              value={customApiKey}
              onChange={(e) => setCustomApiKey(e.target.value)}
            />
          </Field>
        </div>
      </Section>

      <Section title="画面与艺术偏好" description="设定生成场景插图时的默认画幅比例与风格基调。">
        <div className="image-two-col">
          <Field label="默认画面比例" hint="根据场景用途选择最佳画幅。">
            <Select value={size} onChange={(e) => setSize(e.target.value as ImageAspectSize)}>
              <option value="1024x576">16:9 横屏 (1024x576) · 故事场景壁纸</option>
              <option value="1024x1024">1:1 方形 (1024x1024) · 角色肖像与道具</option>
              <option value="576x1024">9:16 竖屏 (576x1024) · 全身立绘与手机屏保</option>
            </Select>
          </Field>

          <Field label="默认艺术风格" hint="自动注入对应的专业画质正负向词。">
            <Select value={stylePreset} onChange={(e) => setStylePreset(e.target.value as ImageStylePreset)}>
              <option value="anime">二次元动漫 (Anime Visual Novel)</option>
              <option value="fantasy">暗黑奇幻 (Dark High Fantasy)</option>
              <option value="cinematic">电影质感 (Cinematic 35mm)</option>
              <option value="realistic">超写实摄影 (Realistic Photography)</option>
            </Select>
          </Field>
        </div>

        <div className="image-action-row">
          <Button variant="primary" onClick={handleTestGenerate} disabled={testing}>
            {testing ? "正在绘制测试图..." : "🎨 试运行生图"}
          </Button>
          <Button variant="quiet" onClick={resetToDefaults}>
            恢复默认
          </Button>
        </div>

        {testError && <Banner tone="error">{testError}</Banner>}

        {testResult && (
          <div className="image-preview-box">
            <img src={testResult.url} alt="生图预览" className="image-preview-img" />
            <div className="image-preview-meta">
              ✓ 图像生成成功并已本地落盘：{testResult.id}
            </div>
          </div>
        )}
      </Section>
    </div>
  );
}
