import { useState } from "react";
import { useUi } from "../stores/uiStore";
import { INITIAL_LORE_ENTRIES } from "../lib/characterPresets";
import { Modal } from "../ui/Modal";
import { TextInput } from "../ui/Field";
import "./WorldbookModal.css";

// Before a session exists these are examples, not mutable session settings.
export function WorldbookModal() {
  const open = useUi(state => state.worldbookModalOpen);
  const close = () => useUi.getState().setWorldbookModalOpen(false);
  const [query, setQuery] = useState("");
  if (!open) return null;
  const term = query.trim().toLowerCase();
  const entries = INITIAL_LORE_ENTRIES.filter(entry => `${entry.title} ${entry.keys.join(" ")} ${entry.content}`.toLowerCase().includes(term));
  return (
    <Modal
      id="worldbook-modal"
      label="世界书示例"
      title="世界书示例"
      closeLabel="关闭世界书示例"
      size="md"
      onClose={close}
    >
      <p className="worldbook-preview-note">这里展示内置设定示例。故事使用开局时导入的角色卡世界书，可在会话侧栏检索；修改设定后请重新导入角色卡开启故事。</p>
      <TextInput aria-label="检索世界书示例" placeholder="标题、触发词或正文…" value={query} onChange={event => setQuery(event.target.value)} />
      <div className="worldbook-preview-entries">
        {entries.map(entry => <article className="lore-entry-card" key={entry.id}>
          <h3>{entry.title}</h3>
          <div className="lore-keys-row">{entry.keys.map(key => <span className="lore-key-tag" key={key}>{key}</span>)}</div>
          <p>{entry.content}</p>
        </article>)}
        {!entries.length && <p className="worldbook-preview-note">没有匹配的示例词条。</p>}
      </div>
    </Modal>
  );
}
