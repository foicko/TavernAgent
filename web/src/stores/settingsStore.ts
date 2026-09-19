// settingsStore：模型实例 / 槽位指派 / 交互偏好（选项双模式/自动续写）。
//
// 配置模型：模型实例是唯一携带连接信息的地方（协议、地址、模型名、密钥、窗口、
// 生成参数），槽位（主线/辅助/反思）只保存 enabled + modelId。输入台的两个快捷
// 控件都落在主线连接上：switchPrimaryModel 换"用哪个实例"，
// setReasoningEffort 调"思考强度"——都只影响下一回合，不打断在途生成。
import { create } from "zustand";
import { api } from "../app/api";
import { roleLabel } from "../lib/modelLabels";
import type { ModelInstance, ProbeResult, ProviderConfig } from "../app/types";

export type OptionMode = "direct" | "fill-edit";

/**
 * 选项呈现频率。默认 auto（只在关键节点给）：选项太密会把玩家训练成“点选项”
 * 而不是扮演，而扮演才是这类产品的留存来源；但完全去掉又会让卡住的玩家没有台阶。
 * 取值必须与后端 domain.OptionsModes 一致。
 */
export type OptionsFrequency = "auto" | "always" | "never";

interface SettingsState {
  // 实例与槽位
  models: ModelInstance[];
  providers: ProviderConfig[];
  loaded: boolean;
  saving: boolean;
  probing: Record<string, boolean>;
  probes: Record<string, ProbeResult>;
  // 偏好
  optionMode: OptionMode;
  optionsFrequency: OptionsFrequency;
  autoContinue: boolean;
  settingsOpen: boolean;

  load: () => Promise<void>;
  saveModel: (instance: ModelInstance) => Promise<ModelInstance>;
  deleteModel: (id: string) => Promise<void>;
  assignSlot: (slot: string, enabled: boolean, modelId: string) => Promise<void>;
  switchPrimaryModel: (modelId: string) => Promise<void>;
  setReasoningEffort: (effort: string) => Promise<void>;
  probeModel: (modelId: string, format: boolean) => Promise<ProbeResult | undefined>;
  setOptionMode: (m: OptionMode) => void;
  setOptionsFrequency: (m: OptionsFrequency) => void;
  setAutoContinue: (v: boolean) => void;
  openSettings: () => void;
  closeSettings: () => void;
}

export const useSettings = create<SettingsState>((set, get) => ({
  models: [],
  providers: [],
  loaded: false,
  saving: false,
  probing: {},
  probes: {},
  optionMode: "direct",
  optionsFrequency: "auto",
  autoContinue: false,
  settingsOpen: false,

  load: async () => {
    try {
      const catalog = await api.listModels();
      set({ models: catalog.models ?? [], providers: catalog.slots ?? [], loaded: true });
    } catch {
      set({ loaded: true });
    }
  },

  saveModel: async (instance) => {
    set({ saving: true });
    try {
      const saved = await api.saveModel(instance);
      // 以服务端返回为准重建列表（ID/脱敏值都由服务端决定），再刷新槽位解析。
      set({ saving: false });
      await get().load();
      return saved;
    } catch (e) {
      set({ saving: false });
      throw e;
    }
  },

  deleteModel: async (id) => {
    await api.deleteModel(id);
    await get().load();
  },

  assignSlot: async (slot, enabled, modelId) => {
    set({ saving: true });
    try {
      const { providers } = await api.assignSlot(slot, enabled, modelId);
      set({ providers, saving: false });
      await get().load();
    } catch (e) {
      set({ saving: false });
      throw e;
    }
  },

  // 输入台快捷切换：只改主线槽位的引用（对新回合生效）。
  switchPrimaryModel: async (modelId) => {
    const primary = get().providers.find((c) => c.slot === "primary");
    await get().assignSlot("primary", primary?.enabled ?? true, modelId);
  },

  // 输入台的思考强度快捷入口：改的是主线连接本身（与温度同类），对新回合生效。
  // 密钥回传空串 = 保留原密钥（服务端约定），避免把脱敏值当新密钥存回去。
  setReasoningEffort: async (effort) => {
    const current = activePrimaryInstance(get().providers, get().models);
    if (!current) throw new Error("还没有可用的主线连接");
    await get().saveModel({ ...current, apiKey: "", reasoningEffort: effort });
  },

  probeModel: async (modelId, format) => {
    set({ probing: { ...get().probing, [modelId]: true } });
    try {
      const res = await api.probeModel(modelId, format);
      set({ probes: { ...get().probes, [modelId]: res } });
      return res;
    } finally {
      const p = { ...get().probing };
      delete p[modelId];
      set({ probing: p });
    }
  },

  setOptionMode: (m) => set({ optionMode: m }),
  setOptionsFrequency: (m) => set({ optionsFrequency: m }),
  setAutoContinue: (v) => set({ autoContinue: v }),
  openSettings: () => set({ settingsOpen: true }),
  closeSettings: () => set({ settingsOpen: false }),
}));

// activePrimaryInstance 返回主线槽位当前解析到的实例（快捷切换按钮显示它）。
export function activePrimaryInstance(
  providers: ProviderConfig[],
  models: ModelInstance[],
): ModelInstance | undefined {
  const primary = providers.find((cfg) => cfg.slot === "primary");
  const id = primary?.resolvedModelId || primary?.modelId;
  return models.find((m) => m.id === id);
}

// slotsUsingModel 返回引用某实例的角色名（列表里标"谁在用"，删除前也用它解释原因）。
export function slotsUsingModel(providers: ProviderConfig[], modelId: string): string[] {
  return providers.filter((cfg) => cfg.modelId === modelId).map((cfg) => roleLabel(cfg.slot));
}
