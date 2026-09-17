// ModelQuickSwitch：输入台右下角的"当前主线模型 + 一键切换"。
//
// 只改"下一回合用哪个连接"，正在生成的那一段不受影响。
import { useEffect, useMemo, useRef, useState } from "react";
import { connectionSubtitle, connectionTitle, optionLabels, protocolLabel } from "../lib/modelLabels";
import { useDismiss } from "../lib/useDismiss";
import { activePrimaryInstance, useSettings } from "../stores/settingsStore";
import { useUi } from "../stores/uiStore";
import { Button } from "../ui/Button";
import { Menu, MenuItem, MenuSeparator } from "../ui/Menu";
import { StatusDot, Tag } from "../ui/Pill";
import "./ModelQuickSwitch.css";

export function ModelQuickSwitch() {
  const models = useSettings((s) => s.models);
  const providers = useSettings((s) => s.providers);
  const loaded = useSettings((s) => s.loaded);
  const load = useSettings((s) => s.load);
  const openSettings = useSettings((s) => s.openSettings);
  const notify = useUi((s) => s.notifyQuiet);
  const [open, setOpen] = useState(false);
  const [switching, setSwitching] = useState(false);
  const root = useRef<HTMLDivElement>(null);

  const current = useMemo(() => activePrimaryInstance(providers, models), [providers, models]);
  const labels = useMemo(() => optionLabels(models), [models]);
  const close = () => setOpen(false);

  // 目录可能还没被别处加载过：自己补一次，避免下拉是空的。
  useEffect(() => {
    if (!loaded) void load();
  }, [loaded, load]);

  useDismiss({ enabled: open, onDismiss: close, ignore: [root] });

  const choose = async (modelId: string) => {
    close();
    if (modelId === current?.id) return;
    setSwitching(true);
    try {
      await useSettings.getState().switchPrimaryModel(modelId);
      const picked = models.find((item) => item.id === modelId);
      notify(`已切换到「${picked ? connectionTitle(picked) : modelId}」，从下一回合开始生效`);
    } catch (error) {
      notify(`切换失败：${error instanceof Error ? error.message : "请稍后重试"}`);
    } finally {
      setSwitching(false);
    }
  };

  const gotoSettings = () => {
    close();
    openSettings();
  };

  return (
    <div className="model-quick-switch" ref={root}>
      <Button
        variant="quiet"
        size="sm"
        className="model-quick-switch__trigger"
        id="quick-switch"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label="切换主线模型"
        title={current ? `当前：${connectionTitle(current)}（改动对新回合生效）` : "选择主线模型"}
        loading={switching}
        onClick={() => setOpen((value) => !value)}
      >
        <StatusDot tone={current ? "success" : "neutral"} hollow={!current} />
        <span className="model-quick-switch__name">{current ? connectionTitle(current) : "配置模型"}</span>
        <svg
          className="model-quick-switch__caret"
          viewBox="0 0 10 10"
          width="9"
          height="9"
          aria-hidden="true"
        >
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
        label="选择主线模型"
        direction="up"
        align="end"
        onClose={close}
        footer="切换对新回合生效，正在生成的这一段不变。"
      >
        {models.length === 0 ? (
          <MenuItem
            title="还没有连接"
            description="先新建一条连接，再指派给对话生成"
            onSelect={gotoSettings}
          />
        ) : null}
        {models.map((instance) => {
          const active = instance.id === current?.id;
          return (
            <MenuItem
              key={instance.id}
              radio
              selected={active}
              title={labels.get(instance.id) ?? connectionTitle(instance)}
              description={connectionSubtitle(instance) || protocolLabel(instance.kind)}
              meta={
                active ? (
                  <>
                    <span aria-hidden="true">✓</span>
                    <Tag>主线</Tag>
                  </>
                ) : null
              }
              onSelect={() => void choose(instance.id)}
            />
          );
        })}
        <MenuSeparator />
        <MenuItem title="打开模型设置…" description="新建 / 编辑连接，分配其它角色" onSelect={gotoSettings} />
      </Menu>
    </div>
  );
}
