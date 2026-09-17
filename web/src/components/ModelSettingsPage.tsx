// ModelSettingsPage：角色优先的模型设置页。
//
// 结构只有三段：角色（谁用哪个模型）→ 连接（地址/密钥/窗口，只填一次）→ 写作偏好。
// 「连接」段在列表与编辑表单之间切换，避免两层概念同时铺在屏幕上。
import { useState, type ReactNode } from "react";
import type { ModelInstance } from "../app/types";
import { connectionTitle, windowLabel, type Role } from "../lib/modelLabels";
import { slotsUsingModel, useSettings } from "../stores/settingsStore";
import { Banner } from "../ui/Banner";
import { Button } from "../ui/Button";
import { EmptyState } from "../ui/EmptyState";
import { Section } from "../ui/Section";
import { Switch } from "../ui/Switch";
import { ConnectionForm } from "./ConnectionForm";
import { ConnectionList } from "./ConnectionList";
import { ModelPicker } from "./ModelPicker";
import "./ModelSettings.css";

type Editing = { id: string } | "new" | null;

export function ModelSettingsPage() {
  const models = useSettings((s) => s.models);
  const providers = useSettings((s) => s.providers);
  const saving = useSettings((s) => s.saving);
  const optionMode = useSettings((s) => s.optionMode);
  const autoContinue = useSettings((s) => s.autoContinue);
  const setOptionMode = useSettings((s) => s.setOptionMode);
  const setAutoContinue = useSettings((s) => s.setAutoContinue);

  const [editing, setEditing] = useState<Editing>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [roleError, setRoleError] = useState<string | null>(null);

  const bindingOf = (role: Role) => providers.find((item) => item.slot === role);
  const resolvedOf = (role: Role) => {
    const binding = bindingOf(role);
    return models.find((item) => item.id === (binding?.resolvedModelId || binding?.modelId));
  };
  const follows = (role: Role) => role !== "primary" && !bindingOf(role)?.modelId;

  const statusOf = (role: Role): ReactNode => {
    const binding = bindingOf(role);
    // 只有"后台记忆"有整体开关；其它角色即使 enabled=false 也只是回落到主线，不该显示"已关闭"。
    if (role === "reflection" && binding && !binding.enabled) return "已关闭";
    const resolved = resolvedOf(role);
    if (!resolved) return role === "primary" ? "还没有选择连接" : `跟随对话生成 · 尚未配置连接`;
    const title = connectionTitle(resolved);
    return follows(role) ? `跟随对话生成 · ${title}` : `${title} · ${windowLabel(resolved.contextWindow)}`;
  };

  const assign = async (role: Role, enabled: boolean, modelId: string) => {
    setRoleError(null);
    try {
      await useSettings.getState().assignSlot(role, enabled, modelId);
    } catch (error) {
      setRoleError(error instanceof Error && error.message ? error.message : "保存失败");
    }
  };

  const reflectionEnabled = bindingOf("reflection")?.enabled ?? false;
  const editingInstance: ModelInstance | null =
    editing && editing !== "new" ? (models.find((item) => item.id === editing.id) ?? null) : null;
  const editingUsedBy = editingInstance ? slotsUsingModel(providers, editingInstance.id) : [];

  return (
    <div className="model-settings-page">
      <Section title="角色" description="每个角色用哪个模型。不确定就保持「跟随对话生成」。">
        {models.length === 0 ? (
          <Banner tone="info">还没有连接。先到下面的「连接」新建一个，再回来分配角色。</Banner>
        ) : null}
        <div className="role-list">
          <ModelPicker
            role="primary"
            instanceId={bindingOf("primary")?.modelId ?? ""}
            instances={models}
            disabled={saving}
            status={statusOf("primary")}
            onSelect={(id) => void assign("primary", id !== "", id)}
          />
          <ModelPicker
            role="assist"
            instanceId={bindingOf("assist")?.modelId ?? ""}
            instances={models}
            disabled={saving}
            status={statusOf("assist")}
            onSelect={(id) => void assign("assist", true, id)}
          />
          <ModelPicker
            role="reflection"
            instanceId={bindingOf("reflection")?.modelId ?? ""}
            instances={models}
            disabled={saving || !reflectionEnabled}
            off={!reflectionEnabled}
            status={statusOf("reflection")}
            onSelect={(id) => void assign("reflection", reflectionEnabled, id)}
            trailing={
              <Switch
                id="reflection-enabled"
                label="启用后台记忆"
                checked={reflectionEnabled}
                disabled={saving}
                onChange={(checked) => void assign("reflection", checked, bindingOf("reflection")?.modelId ?? "")}
              />
            }
          />
        </div>
        {roleError ? <Banner tone="error">{roleError}</Banner> : null}
      </Section>

      <Section
        title="连接"
        description="地址、密钥、窗口只在这里填一次，所有角色共用。"
        actions={
          editing === "new" ? undefined : (
            <Button
              disabled={saving}
              onClick={() => {
                setNotice(null);
                setEditing("new");
              }}
            >
              ＋ 新建连接
            </Button>
          )
        }
      >
        {notice ? (
          <Banner tone="ok" onDismiss={() => setNotice(null)}>
            {notice}
          </Banner>
        ) : null}
        {editing ? (
          <ConnectionForm
            key={editing === "new" ? "new" : editing.id}
            instance={editingInstance}
            instances={models}
            usedBy={editingUsedBy}
            onSaved={(saved) => {
              setEditing({ id: saved.id });
              setNotice(`已保存「${connectionTitle(saved)}」。下一次生成就会用它。`);
            }}
            onDeleted={() => {
              setEditing(null);
              setNotice("连接已删除。");
            }}
            onCancel={() => setEditing(null)}
          />
        ) : models.length === 0 ? (
          <EmptyState
            title="还没有连接"
            description="新建一条连接，填上模型名、地址与密钥即可开始使用。"
            action={
              <Button variant="primary" onClick={() => setEditing("new")}>
                ＋ 新建连接
              </Button>
            }
          />
        ) : (
          <ConnectionList
            instances={models}
            providers={providers}
            onEdit={(id) => {
              setNotice(null);
              setEditing({ id });
            }}
          />
        )}
      </Section>

      <Section title="写作偏好">
        <Switch
          label="选项“填入编辑”模式"
          hint="点击选项先填入输入框，不立即发出请求"
          checked={optionMode === "fill-edit"}
          onChange={(checked) => setOptionMode(checked ? "fill-edit" : "direct")}
        />
        <Switch
          label="截断自动续写"
          hint="生成被截断时自动接着写下去"
          checked={autoContinue}
          onChange={setAutoContinue}
        />
      </Section>
    </div>
  );
}
