// desktopShell：网页前端与原生桌面壳之间的最小桥。
//
// 桌面壳把窗口加载在本地 loopback 服务上，而不是 Wails 的资源服务主机名上。
// 原因是 WebView2 的资源响应必须整包交付（wails 的 responsewriter_windows.go
// 把响应体攒进 bytes.Buffer，等 handler 返回才交给 WebView2），SSE 在那里无法
// 流式送达：fetch 连响应头都收不到，既不报错也不重连——生成中的增量与"后台已
// 更新"的通知会静默丢失。
//
// 代价是页面里没有 Wails 注入的 window.runtime，所以这条桥走我们自己的同源接口：
// 浏览器部署下探测得到 desktop=false、主题切换返回 404，全部安静降级，
// web 产物因此不依赖任何桌面依赖，也不影响纯浏览器/局域网第二屏部署。

import { api } from "../app/api";

let probe: Promise<boolean> | null = null;
let known: boolean | null = null;

/** 探测当前是否运行在桌面壳里。结果缓存，浏览器里恒为 false。 */
export function detectDesktopShell(): Promise<boolean> {
  if (probe) return probe;
  probe = api
    .desktopInfo()
    // 探测失败（旧服务端、离线）不得当成"桌面壳"：宁可少一个窗口控件，
    // 也不要在浏览器里调不存在的原生接口。
    .then((info) => (known = info.desktop === true))
    .catch(() => (known = false));
  return probe;
}

/** 已知结果时同步返回；尚未探测完成返回 false（安全默认值）。 */
export function isDesktopShell(): boolean {
  return known === true;
}

/**
 * 让原生标题栏/边框跟随应用主题。
 * 否则切到"墨黑幽夜"时，深色界面顶上会挂着一根突兀的浅色标题栏。
 *
 * 这是显示偏好，失败不打扰读者：浏览器部署与旧服务端返回 404，静默忽略即可。
 */
export function applyNativeTheme(mode: "light" | "dark"): void {
  void api.setDesktopTheme(mode).catch(() => undefined);
}
