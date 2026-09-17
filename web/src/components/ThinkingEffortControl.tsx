// ThinkingEffortControl：输入台右下角的"思考强度"快捷调整。
//
// 与 ModelQuickSwitch 同类：改的是主线连接上的一个生成参数（默认/低/中/高），
// 只影响下一回合，正在生成的那一段不变。"默认"档不发送任何思考参数，
// 行为与未设置时完全一致。
import { useEffect, useMemo, useRef, useState } from "react";
import { EFFORT_OPTIONS, effortLabel } from "../lib/modelLabels";
import { useDismiss } from "../lib/useDismiss";
import { activePrimaryInstance, useSettings } from "../stores/settingsStore";
import { useUi } from "../stores/uiStore";
import { Button } from "../ui/Button";
import { Menu, MenuItem, MenuSeparator } from "../ui/Menu";
import "./ThinkingEffortControl.css";

export function ThinkingEffortControl() {
  const models = useSettings((s) => s.models);
  const providers = useSettings((s) => s.providers);
  const loaded = useSettings((s) => s.loaded);
  const load = useSettings((s) => s.load);
  const openSettings = useSettings((s) => s.openSettings);
  const notify = useUi((s) => s.notifyQuiet);
  const [open, setOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const root = useRef<HTMLDivElement>(null);

  const current = useMemo(() => activePrimaryInstance(providers, models), [providers, models]);
  const effort = current?.reasoningEffort ?? "";
  const close = () => setOpen(false);

  // 目录可能还没被别处加载过：自己补一次，避免下拉里读不到当前值。
  useEffect(() => {
    if (!loaded) void load();
  }, [loaded, load]);

  useDismiss({ enabled: open, onDismiss: close, ignore: [root] });

  const choose = async (value: string) => {
    close();
    if (!current || value === effort) return;
    setSaving(true);
    try {
      await useSettings.getState().setReasoningEffort(value);
      notify(`思考强度已设为「${effortLabel(value)}」，从下一回合开始生效`);
    } catch (error) {
      notify(`设置失败：${error instanceof Error ? error.message : "请稍后重试"}`);
    } finally {
      setSaving(false);
    }
  };

  const gotoSettings = () => {
    close();
    openSettings();
  };

  return (
    <div className="thinking-effort" ref={root}>
      <Button
        variant="quiet"
        size="sm"
        className={`thinking-effort__trigger${effort ? " thinking-effort__trigger--set" : ""}`}
        id="thinking-effort"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={`调整思考强度（当前 ${effortLabel(effort)}）`}
        title={current ? `当前 ${effortLabel(effort)}，改动对新回合生效` : "先配置一条模型连接"}
        disabled={!current}
        loading={saving}
        onClick={() => setOpen((value) => !value)}
      >
        <span className="thinking-effort__name">思考 {effortLabel(effort)}</span>
        <svg className="thinking-effort__caret" viewBox="0 0 10 10" width="9" height="9" aria-hidden="true">
          <path
            d="M2 3.5L5 6.5L8 3.5"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      </Button>

      <Menu
        open={open}
        label="选择思考强度"
        direction="up"
        align="end"
        onClose={close}
        footer="只影响新回合，正在生成的这一段不变。"
      >
        {EFFORT_OPTIONS.map((option) => (
          <MenuItem
            key={option.value || "default"}
            radio
            selected={option.value === effort}
            title={option.label}
            description={option.hint}
            onSelect={() => void choose(option.value)}
          />
        ))}
        <MenuSeparator />
        <MenuItem title="打开模型设置…" description="上下文窗口与最大输出也在那里改" onSelect={gotoSettings} />
      </Menu>
    </div>
  );
}
