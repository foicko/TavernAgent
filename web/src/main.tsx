import { createRoot } from "react-dom/client";
import App from "./App";
import { applyThemeToDocument, resolveInitialTheme } from "./stores/uiStore";
import "./styles.css";

// 首帧前落主题，避免刷新时闪白（跟随系统或上次选择）。
applyThemeToDocument(resolveInitialTheme());

createRoot(document.getElementById("root")!).render(<App />);