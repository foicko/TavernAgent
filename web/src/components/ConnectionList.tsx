// ConnectionList：连接清单。一行一条，点行即编辑。
//
// 行内只显示"是什么"（名称 + 地址/模型 + 密钥状态）与"谁在用"，
// 删除放在编辑表单里，避免列表上出现一堆红色按钮。
import type { ModelInstance, ProviderConfig } from "../app/types";
import { connectionSubtitle, connectionTitle, keyStateLabel, protocolLabel, windowLabel } from "../lib/modelLabels";
import { slotsUsingModel } from "../stores/settingsStore";
import { ListRow } from "../ui/ListRow";
import { Pill, StatusDot } from "../ui/Pill";

export interface ConnectionListProps {
  instances: ModelInstance[];
  providers: ProviderConfig[];
  onEdit: (id: string) => void;
}

export function ConnectionList({ instances, providers, onEdit }: ConnectionListProps) {
  return (
    <div className="conn-list">
      {instances.map((instance) => {
        const used = slotsUsingModel(providers, instance.id);
        const subtitle = connectionSubtitle(instance) || protocolLabel(instance.kind);
        return (
          <ListRow
            key={instance.id}
            title={
              <span className="conn-list__meta">
                <StatusDot tone={instance.hasApiKey ? "success" : "neutral"} hollow={!instance.hasApiKey} />
                {connectionTitle(instance)}
              </span>
            }
            subtitle={subtitle}
            meta={
              <>
                {used.map((role) => (
                  <Pill key={role} tone="neutral" className="ui-pill--sm">
                    {role}
                  </Pill>
                ))}
                <Pill tone="neutral" className="ui-pill--sm" title={windowLabel(instance.contextWindow)}>
                  {keyStateLabel(instance.hasApiKey)}
                </Pill>
              </>
            }
            ariaLabel={`编辑连接 ${connectionTitle(instance)}`}
            onSelect={() => onEdit(instance.id)}
          />
        );
      })}
    </div>
  );
}
