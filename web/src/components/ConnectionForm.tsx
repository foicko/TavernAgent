// ConnectionForm：一条连接的编辑表单（新建 / 修改 / 删除 / 测试）。
//
// 字段顺序按"用户已经知道什么"排：先服务商预设，再模型名与地址，然后密钥，
// 名称与协议，最后是常显的「生成参数」（上下文窗口 / 最大输出 / 温度 / 思考强度）；
// 窗口下面直接给出派生出的输入预算，省得用户自己算。
import { useEffect, useState } from "react";
import type { ModelInstance, ProbeResult } from "../app/types";
import { effortHint, EFFORT_OPTIONS, inputBudget, maskedKeyHint, PROTOCOL_OPTIONS, tokenLabel } from "../lib/modelLabels";
import { PROVIDER_PRESETS, applyPreset, findPreset, suggestName, type ProviderPreset } from "../lib/providerPresets";
import { useSettings } from "../stores/settingsStore";
import { Banner } from "../ui/Banner";
import { Button, IconButton } from "../ui/Button";
import { Field, NumberInput, Select, TextInput } from "../ui/Field";
import { Pill } from "../ui/Pill";

export interface ConnectionFormProps {
  /** 正在编辑的连接；null 表示新建。 */
  instance: ModelInstance | null;
  instances: ModelInstance[];
  /** 正在引用这条连接的角色名。 */
  usedBy: string[];
  onSaved: (saved: ModelInstance) => void;
  onDeleted: () => void;
  onCancel: () => void;
}

interface FieldErrors {
  name?: string;
  model?: string;
  baseUrl?: string;
  apiKey?: string;
  temperature?: string;
  maxTokens?: string;
  contextWindow?: string;
}

const MAX_CONTEXT_WINDOW = 2_000_000;
const MIN_INPUT_RESERVE = 2048;

function blank(): ModelInstance {
  return {
    id: "",
    name: "",
    kind: "openai-chat",
    baseUrl: "",
    model: "",
    apiKey: "",
    temperature: 0.8,
    maxTokens: 4096,
    contextWindow: 131072,
    reasoningEffort: "",
  };
}

function message(error: unknown): string {
  return error instanceof Error && error.message ? error.message : "操作失败";
}

export function ConnectionForm({
  instance,
  instances,
  usedBy,
  onSaved,
  onDeleted,
  onCancel,
}: ConnectionFormProps) {
  const saving = useSettings((s) => s.saving);
  const probing = useSettings((s) => s.probing);
  const store = useSettings.getState;

  const isNew = instance === null;
  const [draft, setDraft] = useState<ModelInstance>(() =>
    instance ? { ...instance, apiKey: "" } : blank(),
  );
  const [presetId, setPresetId] = useState<string>(() => matchPreset(instance)?.id ?? "custom");
  const [showKey, setShowKey] = useState(false);
  const [errors, setErrors] = useState<FieldErrors>({});
  const [alert, setAlert] = useState<{ tone: "ok" | "error"; text: string } | null>(null);
  const [probe, setProbe] = useState<ProbeResult | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);

  // 切换编辑对象时重置本地状态（表单由页面按 id 重新挂载，这里兜底）。
  useEffect(() => {
    setDraft(instance ? { ...instance, apiKey: "" } : blank());
    setPresetId(matchPreset(instance)?.id ?? "custom");
    setErrors({});
    setAlert(null);
    setProbe(null);
    setConfirmDelete(false);
    setShowKey(false);
  }, [instance]);

  const preset = findPreset(presetId);
  const activePreset = matchPreset(draft);
  const patch = (next: Partial<ModelInstance>) => setDraft((prev) => ({ ...prev, ...next }));

  // 后端按「窗口 − 输出 − 安全余量」算输入预算，这里把派生值直接摆给用户看。
  const budget = inputBudget(draft.contextWindow, draft.maxTokens);
  const budgetHint =
    budget === null
      ? "填好窗口与最大输出后，这里会显示实际可用的输入预算"
      : budget > 0
        ? `输入预算约 ${tokenLabel(budget)} tokens（窗口 − 最大输出 − 安全余量）`
        : "输入预算为 0：窗口相对输出太小，请调大窗口或调小最大输出";

  const choosePreset = (next: ProviderPreset) => {
    setPresetId(next.id);
    setErrors({});
    const previous = findPreset(presetId);
    const applied = applyPreset(draft, next, previous);
    // 新建连接时顺带给出一个不与模型名重复的默认名称。
    if (!draft.id) {
      const names = instances.map((item) => item.name).filter(Boolean);
      applied.name = suggestName(next, next.baseUrl, names);
    }
    setDraft(applied);
  };

  const validate = (): FieldErrors => {
    const next: FieldErrors = {};
    if (!draft.name.trim()) next.name = "请填写名称";
    if (!draft.model?.trim()) next.model = "请填写模型名称（探测与生成都要用它）";

    const baseUrl = draft.baseUrl?.trim() ?? "";
    if (baseUrl) {
      try {
        const url = new URL(baseUrl);
        if (!/^https?:$/.test(url.protocol) || !url.host) next.baseUrl = "请填写完整地址，例如 https://api.deepseek.com";
      } catch {
        next.baseUrl = "地址格式不对，应以 http:// 或 https:// 开头";
      }
    }
    if (draft.apiKey && draft.apiKey.includes("…")) next.apiKey = "这是脱敏后的密钥，请重新粘贴完整密钥";

    const temperature = draft.temperature;
    if (temperature === undefined || temperature === null) next.temperature = "请填写";
    else if (temperature < 0 || temperature > 2) next.temperature = "范围 0 ~ 2";

    const maxTokens = draft.maxTokens;
    if (!maxTokens) next.maxTokens = "请填写";
    else if (maxTokens < 0) next.maxTokens = "不能为负数";

    const contextWindow = draft.contextWindow;
    if (!contextWindow) next.contextWindow = "请填写";
    else if (contextWindow > MAX_CONTEXT_WINDOW) next.contextWindow = `不能超过 ${MAX_CONTEXT_WINDOW}`;
    else if (maxTokens && contextWindow - maxTokens < MIN_INPUT_RESERVE) {
      next.contextWindow = `至少要给输入留 ${MIN_INPUT_RESERVE} token（窗口需大于最大输出）`;
    }
    return next;
  };

  const save = async () => {
    const found = validate();
    setErrors(found);
    if (Object.keys(found).length > 0) {
      setAlert({ tone: "error", text: "还有几项需要修正（见字段下方提示）" });
      return;
    }
    try {
      const saved = await store().saveModel({
        ...draft,
        name: draft.name.trim(),
        model: draft.model?.trim() ?? "",
        baseUrl: draft.baseUrl?.trim() ?? "",
      });
      setAlert(null);
      onSaved(saved);
    } catch (error) {
      setAlert({ tone: "error", text: message(error) });
    }
  };

  const runProbe = async () => {
    if (!draft.id) {
      setAlert({ tone: "error", text: "先保存连接，再测试它" });
      return;
    }
    setAlert(null);
    setProbe(null);
    try {
      // 只做连通性检查（不请求格式帧），因此不会产生生成费用。
      const result = await store().probeModel(draft.id, false);
      setProbe(result ?? null);
      if (!result?.ok) setAlert({ tone: "error", text: result?.connectMsg || "连接失败" });
    } catch (error) {
      setAlert({ tone: "error", text: message(error) });
    }
  };

  const remove = async () => {
    if (!draft.id) return;
    if (!confirmDelete) {
      setConfirmDelete(true);
      return;
    }
    try {
      await store().deleteModel(draft.id);
      onDeleted();
    } catch (error) {
      setAlert({ tone: "error", text: message(error) });
      setConfirmDelete(false);
    }
  };

  const title = draft.id ? `编辑连接：${draft.name || draft.model || "未命名"}` : "新建连接";
  const probeBusy = Boolean(draft.id && probing[draft.id]);

  return (
    <div className="conn-form">
      <div className="conn-form__head">
        <span className="conn-form__title">{title}</span>
        <Button variant="ghost" size="sm" onClick={onCancel}>
          返回列表
        </Button>
      </div>

      <div>
        <div
          className="conn-form__presets"
          role="group"
          aria-label="服务商预设"
        >
          {PROVIDER_PRESETS.map((item) => (
            <Button
              key={item.id}
              size="sm"
              variant={activePreset?.id === item.id ? "primary" : "quiet"}
              onClick={() => choosePreset(item)}
            >
              {item.label}
            </Button>
          ))}
        </div>
        {preset?.keyHint ? (
          <span className="conn-form__usage">{preset.label}：{preset.keyHint}</span>
        ) : null}
      </div>

      <div className="conn-form__grid">
        <Field label="模型名称" error={errors.model} hint={preset && preset.models.length > 0 ? `例如 ${preset.models[0]}` : undefined}>
          <TextInput
            value={draft.model ?? ""}
            placeholder="deepseek-chat"
            className="ui-input--mono"
            onChange={(event) => patch({ model: event.target.value })}
          />
        </Field>
        <Field label="接口地址" error={errors.baseUrl} hint="填到版本根即可，程序会自动补全路径">
          <TextInput
            value={draft.baseUrl ?? ""}
            placeholder="https://api.deepseek.com"
            className="ui-input--mono"
            onChange={(event) => patch({ baseUrl: event.target.value })}
          />
        </Field>
      </div>

      <div className="conn-form__grid">
        <Field
          label="API Key"
          error={errors.apiKey}
          hint={
            draft.hasApiKey
              ? draft.apiKey
                ? `已保存密钥 ${maskedKeyHint(draft.apiKey)}（留空则不修改）`
                : "已保存密钥（留空则不修改）"
              : "还没有保存密钥"
          }
        >
          <div className="ui-input-row">
            <TextInput
              type={showKey ? "text" : "password"}
              value={draft.apiKey ?? ""}
              placeholder="留空 = 保持不变"
              className="ui-input--mono"
              autoComplete="off"
              onChange={(event) => patch({ apiKey: event.target.value })}
            />
            <IconButton
              label={showKey ? "隐藏密钥" : "显示密钥"}
              size="sm"
              onClick={() => setShowKey((value) => !value)}
            >
              {showKey ? "🙈" : "👁"}
            </IconButton>
          </div>
        </Field>
        <Field label="名称" error={errors.name} hint="给自己看的名字，会显示在输入台和角色选择里">
          <TextInput
            value={draft.name}
            placeholder="例如：DeepSeek 官方"
            onChange={(event) => patch({ name: event.target.value })}
          />
        </Field>
      </div>

      <Field label="协议" hint="多数国产模型与本地网关都用 OpenAI 兼容">
        <Select value={draft.kind} onChange={(event) => patch({ kind: event.target.value })}>
          {PROTOCOL_OPTIONS.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </Select>
      </Field>

      <div className="conn-form__group">
        <div className="conn-form__caption">生成参数</div>
        <div className="conn-form__grid">
          <Field label="上下文窗口" error={errors.contextWindow} hint="模型一次能装下的输入 + 输出总量">
            <NumberInput
              value={draft.contextWindow}
              suffix="tokens"
              onValueChange={(value) => patch({ contextWindow: value })}
            />
          </Field>
          <Field label="最大输出" error={errors.maxTokens} hint="单次生成的上限，超出会被截断">
            <NumberInput
              value={draft.maxTokens}
              suffix="tokens"
              onValueChange={(value) => patch({ maxTokens: value })}
            />
          </Field>
          <Field label="温度" error={errors.temperature} hint="越高越随机">
            <NumberInput
              value={draft.temperature ?? undefined}
              onValueChange={(value) => patch({ temperature: value ?? null })}
            />
          </Field>
          <Field label="思考强度" hint={effortHint(draft.reasoningEffort)}>
            <Select
              value={draft.reasoningEffort ?? ""}
              onChange={(event) => patch({ reasoningEffort: event.target.value })}
            >
              {EFFORT_OPTIONS.map((option) => (
                <option key={option.value || "default"} value={option.value}>
                  {option.label}
                </option>
              ))}
            </Select>
          </Field>
        </div>
        <span className={`conn-form__usage${budget !== null && budget <= 0 ? " conn-form__usage--warn" : ""}`}>
          {budgetHint}
        </span>
      </div>

      {alert ? <Banner tone={alert.tone}>{alert.text}</Banner> : null}

      {probe?.ok ? (
        <Banner tone="ok">
          连接正常 · 延迟 {probe.latencyMs ?? 0} ms · 模型 {probe.model || draft.model || "-"}
        </Banner>
      ) : null}

      {usedBy.length > 0 ? (
        <Banner tone="info">
          正在被 {usedBy.join(" / ")} 使用。要删除它，请先在这些角色里换成别的连接。
        </Banner>
      ) : null}

      <div className="conn-form__actions">
        <Button loading={probeBusy} disabled={!draft.id || probeBusy} onClick={() => void runProbe()}>
          测试连接
        </Button>
        <Button variant="primary" loading={saving} onClick={() => void save()}>
          {isNew ? "保存连接" : "保存修改"}
        </Button>
        <span className="conn-form__spacer" />
        {usedBy.length > 0 ? <Pill tone="neutral" title={`被 ${usedBy.join("/")} 使用中`}>使用中</Pill> : null}
        <Button
          variant="danger"
          disabled={!draft.id || usedBy.length > 0}
          title={usedBy.length > 0 ? `被 ${usedBy.join("/")} 使用中，无法删除` : "删除这条连接"}
          onClick={() => void remove()}
        >
          {confirmDelete ? "确认删除" : "删除连接"}
        </Button>
      </div>
    </div>
  );
}

/** 反查草稿与哪个预设一致（只比协议与地址，用于回显选中态）。 */
function matchPreset(instance: ModelInstance | null): ProviderPreset | undefined {
  if (!instance) return undefined;
  const baseUrl = (instance.baseUrl ?? "").trim().replace(/\/+$/, "");
  return PROVIDER_PRESETS.find(
    (preset) => preset.kind === instance.kind && preset.baseUrl.replace(/\/+$/, "") === baseUrl,
  );
}
