// ModelPicker：一个"角色用哪个连接"的选择行。
//
// 角色优先的界面里，用户先回答"哪一段用哪个模型"，地址/密钥/窗口都在连接里，
// 这里只负责选择与显示解析后的状态。
import type { ReactNode } from "react";
import type { ModelInstance } from "../app/types";
import { FOLLOW_PRIMARY, ROLE_HINTS, ROLE_LABELS, optionLabels, type Role } from "../lib/modelLabels";
import { Select } from "../ui/Field";

export interface ModelPickerProps {
  role: Role;
  /** 显式指派的连接 id；空字符串表示未指派（非主线角色即"跟随"）。 */
  instanceId: string;
  instances: ModelInstance[];
  onSelect: (instanceId: string) => void;
  /** 解析后的状态说明（实际会用的连接 / 已关闭 / 跟随主线）。 */
  status: ReactNode;
  disabled?: boolean;
  off?: boolean;
  /** 行尾附加控件，例如"后台记忆"的启用开关。 */
  trailing?: ReactNode;
}

export function ModelPicker({
  role,
  instanceId,
  instances,
  onSelect,
  status,
  disabled = false,
  off = false,
  trailing,
}: ModelPickerProps) {
  const id = `role-${role}`;
  const labels = optionLabels(instances);
  const allowFollow = role !== "primary";

  return (
    <div className={["role-row", off ? "role-row--off" : ""].filter(Boolean).join(" ")}>
      <div className="role-row__head">
        <label className="role-row__label" htmlFor={id}>
          {ROLE_LABELS[role]}
        </label>
        <span className="role-row__hint">{ROLE_HINTS[role]}</span>
        {trailing ? <div className="role-row__toggle">{trailing}</div> : null}
      </div>
      <div className="role-row__control">
        <Select
          id={id}
          value={instanceId}
          disabled={disabled || instances.length === 0}
          onChange={(event) => onSelect(event.target.value)}
        >
          <option value="">{allowFollow ? FOLLOW_PRIMARY : "未选择"}</option>
          {instances.map((instance) => (
            <option key={instance.id} value={instance.id}>
              {labels.get(instance.id)}
            </option>
          ))}
        </Select>
        <span className="role-row__status">{status}</span>
      </div>
    </div>
  );
}
