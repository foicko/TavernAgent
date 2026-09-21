import { useEffect, useState } from "react";
import { getTTSVoices } from "../app/api";
import type { TTSVoice } from "../app/types";
import { ttsPlayer } from "../lib/ttsPlayer";
import { useTTSSettings, type TTSEngine } from "../stores/ttsSettingsStore";
import { Banner } from "../ui/Banner";
import { Button } from "../ui/Button";
import { Field, TextInput, Select } from "../ui/Field";
import { Section } from "../ui/Section";
import { Switch } from "../ui/Switch";
import "./TTSSettings.css";

export function TTSSettingsPage() {
  const engine = useTTSSettings((s) => s.engine);
  const edgeVoice = useTTSSettings((s) => s.edgeVoice);
  const customBaseUrl = useTTSSettings((s) => s.customBaseUrl);
  const customApiKey = useTTSSettings((s) => s.customApiKey);
  const customModel = useTTSSettings((s) => s.customModel);
  const customVoice = useTTSSettings((s) => s.customVoice);
  const speed = useTTSSettings((s) => s.speed);
  const dialogueOnly = useTTSSettings((s) => s.dialogueOnly);
  const directorMode = useTTSSettings((s) => s.directorMode);

  const setEngine = useTTSSettings((s) => s.setEngine);
  const setEdgeVoice = useTTSSettings((s) => s.setEdgeVoice);
  const setCustomBaseUrl = useTTSSettings((s) => s.setCustomBaseUrl);
  const setCustomApiKey = useTTSSettings((s) => s.setCustomApiKey);
  const setCustomModel = useTTSSettings((s) => s.setCustomModel);
  const setCustomVoice = useTTSSettings((s) => s.setCustomVoice);
  const setSpeed = useTTSSettings((s) => s.setSpeed);
  const setDialogueOnly = useTTSSettings((s) => s.setDialogueOnly);
  const setDirectorMode = useTTSSettings((s) => s.setDirectorMode);
  const applyMiMoPreset = useTTSSettings((s) => s.applyMiMoPreset);
  const resetToDefaults = useTTSSettings((s) => s.resetToDefaults);

  const [voices, setVoices] = useState<TTSVoice[]>([]);
  const [testing, setTesting] = useState(false);
  const [testError, setTestError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const fetchVoices = async () => {
      try {
        if (typeof getTTSVoices === "function") {
          const v = await getTTSVoices();
          if (!cancelled && Array.isArray(v) && v.length > 0) {
            setVoices(v);
          }
        }
      } catch {
        // 忽略网络或获取异常
      }
    };
    void fetchVoices();
    return () => {
      cancelled = true;
    };
  }, []);

  const handleTestPlayback = async () => {
    setTestError(null);
    setTesting(true);
    try {
      const sampleText = "你好！很高兴能与你一同开启这段旅程。";
      await ttsPlayer.play(sampleText);
    } catch (err) {
      setTestError(err instanceof Error ? err.message : "试听失败，请检查服务地址与连接");
    } finally {
      setTesting(false);
    }
  };

  const applyLocalPreset = () => {
    setCustomBaseUrl("http://127.0.0.1:9880/v1");
    setCustomApiKey("");
    setCustomModel("tts-1");
  };

  const applyOpenAIPreset = () => {
    setCustomBaseUrl("https://api.openai.com/v1");
    setCustomModel("tts-1");
  };

  return (
    <div className="tts-settings-page">
      <Section title="TTS 语音引擎" description="选择用于朗读剧情台词的语音合成后端。">
        <Field label="引擎类型" hint="支持免配置高品质云端合成、本地开源 TTS 模型或浏览器原生引擎。">
          <Select
            value={engine}
            onChange={(e) => setEngine(e.target.value as TTSEngine)}
          >
            <option value="edge">微软 Edge-TTS (高质量 · 免配置 · 推荐)</option>
            <option value="mimo">小米 MiMo (mimo-v2.5-tts · 电影级导演控制)</option>
            <option value="openai">OpenAI 兼容 / 本地模型 (GPT-SoVITS / CosyVoice / Kokoro 等)</option>
            <option value="web-speech">浏览器原生语音 (离线 · 系统内置音色)</option>
          </Select>
        </Field>
      </Section>

      {engine === "edge" && (
        <Section title="Edge-TTS 声音配置" description="由微软神经语音引擎驱动，无需 API Key。">
          <Field label="音色选择" hint="支持多种自然富有情感的中英文音色。">
            <Select
              value={edgeVoice}
              onChange={(e) => setEdgeVoice(e.target.value)}
            >
              {voices.length > 0 ? (
                voices.map((v) => (
                  <option key={v.id} value={v.id}>
                    {v.name} ({v.locale}) - {v.description}
                  </option>
                ))
              ) : (
                <>
                  <option value="zh-CN-XiaoxiaoNeural">晓晓 (温柔女声) - zh-CN</option>
                  <option value="zh-CN-YunxiNeural">云希 (少年/青年男声) - zh-CN</option>
                  <option value="zh-CN-YunjianNeural">云健 (沉稳旁白男声) - zh-CN</option>
                  <option value="zh-CN-XiaoyiNeural">晓伊 (抒情女声) - zh-CN</option>
                  <option value="en-US-AvaMultilingualNeural">Ava (多语言英文女声) - en-US</option>
                  <option value="en-US-AndrewMultilingualNeural">Andrew (多语言英文男声) - en-US</option>
                </>
              )}
            </Select>
          </Field>
        </Section>
      )}

      {(engine === "openai" || engine === "mimo") && (
        <Section title="自定义端点与本地模型" description="配置 小米 MiMo / OpenAI 兼容语音合成接口与声音模型。">
          <div className="tts-preset-chips">
            <span className="tts-preset-label">快捷填充：</span>
            <Button size="sm" variant="quiet" onClick={applyMiMoPreset}>
              小米 MiMo (mimo-v2.5-tts)
            </Button>
            <Button size="sm" variant="quiet" onClick={applyLocalPreset}>
              本地端点 (GPT-SoVITS / CosyVoice)
            </Button>
            <Button size="sm" variant="quiet" onClick={applyOpenAIPreset}>
              OpenAI 官方端点
            </Button>
          </div>

          <Field label="接口 Base URL" hint="支持小米 MiMo (https://api.xiaomimimo.com/v1)、本地端口或远程端点。">
            <TextInput
              type="text"
              placeholder="https://api.xiaomimimo.com/v1"
              value={customBaseUrl}
              onChange={(e) => setCustomBaseUrl(e.target.value)}
            />
          </Field>

          <Field label="API Key (可选)" hint="本地开源模型通常留空；小米 MiMo 或 OpenAI 在线服务需填入 API Key。">
            <TextInput
              type="password"
              placeholder="sk-..."
              value={customApiKey}
              onChange={(e) => setCustomApiKey(e.target.value)}
            />
          </Field>

          <div className="tts-two-col">
            <Field label="模型标识 (Model)" hint="例如 mimo-v2.5-tts, tts-1, cosyvoice">
              <TextInput
                type="text"
                placeholder="mimo-v2.5-tts"
                value={customModel}
                onChange={(e) => setCustomModel(e.target.value)}
              />
            </Field>

            <Field label="音色标识 (Voice)" hint="小米 MiMo 可填 mimo_default, 冰糖, 茉莉, 苏打, 白桦, Mia, Chloe">
              <TextInput
                type="text"
                placeholder="mimo_default"
                value={customVoice}
                onChange={(e) => setCustomVoice(e.target.value)}
              />
            </Field>
          </div>
        </Section>
      )}

      {engine === "web-speech" && (
        <Section title="系统原生语音" description="直接调用操作系统的文字转语音引擎，完全离线运行。">
          <Banner tone="info">
            已启用浏览器原生 Web Speech API。发音品质取决于操作系统安装的 TTS 语音包。
          </Banner>
        </Section>
      )}

      <Section title="朗读偏好" description="自定义故事播放时的过滤规则与语速。">
        <Switch
          checked={dialogueOnly}
          onChange={setDialogueOnly}
          label="只播报角色对白（跳过旁白）"
          hint="开启后朗读时自动跳过环境描写、旁白与心理活动，只读出角色所说的话。"
        />

        <Switch
          checked={directorMode}
          onChange={setDirectorMode}
          label="情境感知演播 (导演模式)"
          hint="根据角色人设、立绘神态与场景氛围动态生成自然声音指导（含深呼吸、停顿等标签，非 MiMo 引擎将自动剥离保持自然播报）。"
        />

        <Field label={`语速调节 (${speed.toFixed(1)}x)`} hint="0.5x (慢速) ~ 2.0x (倍速)">
          <input
            type="range"
            min="0.5"
            max="2.0"
            step="0.1"
            value={speed}
            onChange={(e) => setSpeed(parseFloat(e.target.value))}
            className="tts-speed-slider"
          />
        </Field>

        <div className="tts-action-row">
          <Button variant="primary" onClick={handleTestPlayback} disabled={testing}>
            {testing ? "正在发音..." : "🔊 试听发音"}
          </Button>
          <Button variant="quiet" onClick={resetToDefaults}>
            恢复默认
          </Button>
        </div>

        {testError && <Banner tone="error">{testError}</Banner>}
      </Section>
    </div>
  );
}
