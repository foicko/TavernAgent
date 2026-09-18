// desktopShell：Web 前端与 Wails 桌面壳之间的最小桥接层。
//
// 只在桌面壳里生效——浏览器或局域网第二屏没有 window.runtime，这里全部是空操作。
// 刻意不引入 Wails 的类型包：只用特性检测调用少数几个运行时方法，
// 这样 web 产物不会被桌面依赖绑死，也不影响纯浏览器部署。

type WailsRuntime = {
  WindowSetLightTheme?: () => void;
  WindowSetDarkTheme?: () => void;
};

function wailsRuntime(): WailsRuntime | null {
  const candidate = (window as unknown as { runtime?: unknown }).runtime;
  return candidate && typeof candidate === "object" ? (candidate as WailsRuntime) : null;
}

/** 是否运行在桌面壳内（可据此显示桌面专属的窗口控件等）。 */
export function isDesktopShell(): boolean {
  return wailsRuntime() !== null;
}

/**
 * 让原生标题栏/边框跟随应用主题。
 * 否则切到"墨黑幽夜"时，深色界面顶上会挂着一根浅色标题栏。
 */
export function applyNativeTheme(mode: "light" | "dark"): void {
  const runtime = wailsRuntime();
  if (!runtime) return;
  if (mode === "dark") runtime.WindowSetDarkTheme?.();
  else runtime.WindowSetLightTheme?.();
}
