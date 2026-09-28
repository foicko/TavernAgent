/**
 * 模态栈判定：共享 `Modal`、`CommandPalette`、`DirectorWorkspace` 都会渲染
 * `role="dialog" aria-modal="true"`。全局快捷键在已有模态时不应再叠加新对话框，
 * 否则两个 `aria-modal` 同时生效、焦点陷阱互相打架。
 */
export function isModalOpen(root: ParentNode = document): boolean {
  return root.querySelector('[role="dialog"][aria-modal="true"]') !== null;
}
