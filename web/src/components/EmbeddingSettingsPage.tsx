import { useState } from "react";
import { probeEmbedding, type EmbeddingProbeResponse } from "../app/api";
import {
  useEmbeddingSettings,
  type EmbeddingProviderType,
} from "../stores/embeddingSettingsStore";
import { Banner } from "../ui/Banner";
import { Button } from "../ui/Button";
import { Field, Select, TextInput } from "../ui/Field";
import { Section } from "../ui/Section";
import { Switch } from "../ui/Switch";
import "./EmbeddingSettings.css";

export function EmbeddingSettingsPage() {
  const enabled = useEmbeddingSettings((s) => s.enabled);
  const provider = useEmbeddingSettings((s) => s.provider);
  const baseUrl = useEmbeddingSettings((s) => s.baseUrl);
  const apiKey = useEmbeddingSettings((s) => s.apiKey);
  const model = useEmbeddingSettings((s) => s.model);
  const dimension = useEmbeddingSettings((s) => s.dimension);
  const hybridRRFWeight = useEmbeddingSettings((s) => s.hybridRRFWeight);
  const topK = useEmbeddingSettings((s) => s.topK);

  const setEnabled = useEmbeddingSettings((s) => s.setEnabled);
  const setProvider = useEmbeddingSettings((s) => s.setProvider);
  const setBaseUrl = useEmbeddingSettings((s) => s.setBaseUrl);
  const setApiKey = useEmbeddingSettings((s) => s.setApiKey);
  const setModel = useEmbeddingSettings((s) => s.setModel);
  const setDimension = useEmbeddingSettings((s) => s.setDimension);
  const setHybridRRFWeight = useEmbeddingSettings((s) => s.setHybridRRFWeight);
  const setTopK = useEmbeddingSettings((s) => s.setTopK);

  const applyBgeSmallOllama = useEmbeddingSettings((s) => s.applyBgeSmallOllama);
  const applySiliconFlow = useEmbeddingSettings((s) => s.applySiliconFlow);
  const applyBgeM3Ollama = useEmbeddingSettings((s) => s.applyBgeM3Ollama);
  const applyNomicOllama = useEmbeddingSettings((s) => s.applyNomicOllama);
  const applyOpenAI = useEmbeddingSettings((s) => s.applyOpenAI);
  const resetToDefaults = useEmbeddingSettings((s) => s.resetToDefaults);

  const [testing, setTesting] = useState(false);
  const [probeResult, setProbeResult] = useState<EmbeddingProbeResponse | null>(null);
  const [probeError, setProbeError] = useState<string | null>(null);

  const handleTestProbe = async () => {
    setProbeError(null);
    setTesting(true);
    setProbeResult(null);

    try {
      const res = await probeEmbedding({
        baseUrl,
        apiKey,
        model,
        sampleText: "TavernAgent 记忆向量检索连通性探测测试",
      });

      if (!res.ok) {
        setProbeError(res.error || "服务连通失败，请检查服务地址或模型名称");
      } else {
        setProbeResult(res);
        // 如果探测到的真实维度与当前填写不一致，自动纠正
        if (res.dimension > 0 && res.dimension !== dimension) {
          setDimension(res.dimension);
        }
      }
    } catch (err) {
      setProbeError(err instanceof Error ? err.message : "向量端点探测失败，请确认端点已开启并可访问");
    } finally {
      setTesting(false);
    }
  };

  return (
    <div className="embedding-settings-page">
      <Section title="向量模型与端点" description="配置用于记忆检索与剧情上下文匹配的嵌入（Embedding）模型。">
        <div className="embedding-preset-chips">
          <span className="embedding-preset-label">快捷预设：</span>
          <Button size="sm" variant="quiet" onClick={applyBgeSmallOllama}>
            本地 Ollama (bge-small-zh-v1.5 / 推荐默认)
          </Button>
          <Button size="sm" variant="quiet" onClick={applySiliconFlow}>
            硅基流动 (BAAI/bge-small-zh-v1.5)
          </Button>
          <Button size="sm" variant="quiet" onClick={applyBgeM3Ollama}>
            本地 Ollama (bge-m3 / 1024维)
          </Button>
          <Button size="sm" variant="quiet" onClick={applyNomicOllama}>
            本地 Ollama (nomic-embed / 768维)
          </Button>
          <Button size="sm" variant="quiet" onClick={applyOpenAI}>
            OpenAI (text-embedding-3-small)
          </Button>
        </div>

        <Switch
          checked={enabled}
          onChange={setEnabled}
          label="启用向量语义增强检索 (Hybrid RRF)"
          hint="开启后将结合 FTS5 稀疏词法索引与稠密向量相似度（Reciprocal Rank Fusion），大幅提升长程隐式记忆与代词指代召回率。"
        />

        <div className="embedding-two-col">
          <Field label="服务提供商 / 架构" hint="统一基于通用 OpenAI-compatible /v1/embeddings 协议标准。">
            <Select
              value={provider}
              onChange={(e) => setProvider(e.target.value as EmbeddingProviderType)}
            >
              <option value="ollama">本地 Ollama (127.0.0.1:11434 / 默认)</option>
              <option value="siliconflow">硅基流动 SiliconFlow (云端高速)</option>
              <option value="openai">OpenAI 官方端点</option>
              <option value="custom">自定义 OpenAI 兼容服务 (vLLM / LocalAI / LM Studio)</option>
            </Select>
          </Field>

          <Field label="服务 Base URL" hint="API 根地址（自动兼容尾部 /v1 或 /embeddings）。">
            <TextInput
              type="text"
              placeholder={
                provider === "ollama"
                  ? "http://127.0.0.1:11434/v1"
                  : provider === "siliconflow"
                  ? "https://api.siliconflow.cn/v1"
                  : provider === "openai"
                  ? "https://api.openai.com/v1"
                  : "http://127.0.0.1:8000/v1"
              }
              value={baseUrl}
              onChange={(e) => setBaseUrl(e.target.value)}
            />
          </Field>
        </div>

        <div className="embedding-two-col">
          <Field label="模型标识 (Model)" hint="默认推荐 bge-small-zh-v1.5，极速且仅占用约 90MB 显存/内存。">
            <TextInput
              type="text"
              placeholder="bge-small-zh-v1.5"
              value={model}
              onChange={(e) => setModel(e.target.value)}
            />
          </Field>

          <Field label="API Key (密钥)" hint="本地 Ollama 可留空；云端或鉴权端点需填写。">
            <TextInput
              type="password"
              placeholder="sk-..."
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
            />
          </Field>
        </div>

        <div className="embedding-two-col">
          <Field label="向量维度 (Dimensions)" hint="bge-small 为 512 维，bge-m3 为 1024 维，OpenAI small 为 1536 维。">
            <TextInput
              type="number"
              value={String(dimension)}
              onChange={(e) => {
                const val = parseInt(e.target.value, 10);
                if (!isNaN(val) && val > 0) setDimension(val);
              }}
            />
          </Field>

          <Field label="召回候选上限 (Top-K)" hint="检索出的记忆候选条数（默认 5 条）。">
            <TextInput
              type="number"
              value={String(topK)}
              onChange={(e) => {
                const val = parseInt(e.target.value, 10);
                if (!isNaN(val) && val > 0) setTopK(val);
              }}
            />
          </Field>
        </div>

        <Field
          label={`检索融合权重：词法 BM25 ${(1 - hybridRRFWeight).toFixed(2)} : 向量语义 ${hybridRRFWeight.toFixed(2)}`}
          hint="控制倒数排名融合（RRF）中词法匹配与语义向量的偏向（推荐 0.50 均衡）。"
        >
          <input
            type="range"
            min="0.1"
            max="0.9"
            step="0.05"
            value={hybridRRFWeight}
            className="embedding-slider"
            onChange={(e) => setHybridRRFWeight(parseFloat(e.target.value))}
          />
        </Field>

        <div className="embedding-tip-box">
          💡 <strong>本地部署提示</strong>：若使用本地 Ollama，可在系统终端执行命令拉取模型：
          <br />
          <code>ollama pull bge-small-zh-v1.5</code>
          <br />
          <code>bge-small-zh-v1.5</code> 模型权重约 90MB，推理耗时仅需 5~10ms，无需抢占 GPU 显存。
        </div>

        <div className="embedding-action-row">
          <Button
            variant="primary"
            onClick={() => void handleTestProbe()}
            disabled={testing}
          >
            {testing ? "正在探测连通性..." : "测试连通性 / 探测维度"}
          </Button>

          <Button variant="quiet" onClick={resetToDefaults}>
            恢复默认配置
          </Button>
        </div>

        {probeError ? (
          <Banner tone="error">
            <div>
              <strong>连通性测试未通过</strong>: {probeError}
            </div>
            <div style={{ marginTop: "4px", fontSize: "12px", opacity: 0.9 }}>
              建议排查：1. 本地 Ollama 是否已启动并在 11434 端口监听；2. 是否已执行 <code>ollama pull {model}</code>；3. 云端接口 API Key 是否有效。
            </div>
          </Banner>
        ) : null}

        {probeResult && probeResult.ok ? (
          <div className="embedding-probe-card">
            <div className="embedding-probe-header">
              <span style={{ color: "var(--color-success, #22c55e)" }}>
                ✓ 服务连通成功！模型可用
              </span>
              <span style={{ fontSize: "12px", color: "var(--text-dim)" }}>
                耗时 {probeResult.latencyMs} ms
              </span>
            </div>
            <div className="embedding-probe-meta">
              <span>模型: <strong>{probeResult.model}</strong></span>
              <span>输出维度: <strong>{probeResult.dimension} 维</strong></span>
            </div>
            {probeResult.preview && probeResult.preview.length > 0 ? (
              <div>
                <span style={{ fontSize: "11px", color: "var(--text-dim)" }}>首分量向量样例 (Preview):</span>
                <div className="embedding-preview-code">
                  [{probeResult.preview.map((v) => v.toFixed(5)).join(", ")} ...]
                </div>
              </div>
            ) : null}
          </div>
        ) : null}
      </Section>
    </div>
  );
}
