// 剧情包的浏览器侧文件交互：下载与选择。
//
// 选择框挂到 document.body（隐藏）而不是游离节点：游离的 input 无法被
// 自动化测试定位，也不能保证所有浏览器都接受对它的 programmatic click。

/** 触发浏览器下载。 */
export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.style.display = "none";
  document.body.appendChild(a);
  a.click();
  a.remove();
  // 立刻 revoke 会让部分浏览器来不及取数据，延后释放。
  setTimeout(() => URL.revokeObjectURL(url), 10_000);
}

/** 生成下载文件名：<标题>-<时间戳>.tavernpack。 */
export function packFileName(title: string): string {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, "0");
  const stamp = `${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}-${p(d.getHours())}${p(d.getMinutes())}${p(d.getSeconds())}`;
  const safe = (title || "session").replace(/[/\\:*?"<>|]/g, "_").slice(0, 40);
  return `${safe}-${stamp}.tavernpack`;
}
