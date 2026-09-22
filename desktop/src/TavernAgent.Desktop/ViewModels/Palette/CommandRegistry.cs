using TavernAgent.Core.Stores;
using TavernAgent.Desktop.ViewModels;

namespace TavernAgent.Desktop.ViewModels.Palette;

/// <summary>Assembles palette commands from the live stores (web: lib/commandRegistry).</summary>
public static class CommandRegistry
{
    public static List<CommandItem> Build(WorkspaceViewModel ws)
    {
        var story = ws.Story;
        var ui = ws.Ui;
        var stage = ws.Stage;
        var list = new List<CommandItem>();

        void Add(string group, string id, string title, Func<Task> run,
            Func<bool>? enabled = null, string? hint = null, string? keywords = null)
            => list.Add(new CommandItem(id, group, title, run, enabled, hint, keywords));

        // ---- navigation ----
        foreach (var session in story.Sessions)
        {
            var id = session.SessionId;
            var title = string.IsNullOrWhiteSpace(session.Title) ? "未命名剧本" : session.Title;
            Add("navigation", $"session:{id}", $"打开会话：{title}",
                () => story.OpenSessionAsync(id), keywords: "session story 切换 会话 故事");
        }

        foreach (var (key, preset) in CharacterPresets.Presets)
        {
            var charKey = key;
            Add("navigation", $"card:{charKey}", $"切换到角色卡：{preset.Name}",
                () => { ui.SwitchActiveCharacter(charKey); return Task.CompletedTask; },
                keywords: "card character 角色卡 切换");
        }

        Add("navigation", "story:back-to-head", "回到最新节点",
            () => story.BackToHeadAsync(),
            () => !string.IsNullOrEmpty(story.ViewNodeId), "浏览历史时可用", "history head 最新 回到");

        Add("navigation", "left-tab:tree", "左栏：因果树", () => { ui.OpenLeftTabAndExpand(LeftSubtab.Tree); return Task.CompletedTask; }, keywords: "tree 分支 因果");
        Add("navigation", "left-tab:characters", "左栏：角色卡库", () => { ui.OpenLeftTabAndExpand(LeftSubtab.Characters); return Task.CompletedTask; }, keywords: "characters 角色 卡库");
        Add("navigation", "left-tab:lore", "左栏：世界书", () => { ui.OpenLeftTabAndExpand(LeftSubtab.Lore); return Task.CompletedTask; }, keywords: "lore worldbook 世界书 词条");
        Add("navigation", "left-tab:sessions", "左栏：历史会话", () => { ui.OpenLeftTabAndExpand(LeftSubtab.Sessions); return Task.CompletedTask; }, keywords: "sessions 会话 历史");

        // ---- story ----
        Add("story", "story:continue", "接着写", () => stage.ContinueAsync(),
            () => stage.IsTruncated, keywords: "continue 续写 接着");
        Add("story", "story:regen", "重新生成最后一段",
            () => stage.RegenerateLatestCommand.ExecuteAsync(null),
            () => !story.IsBusy && story.Messages.Count > 0, keywords: "regenerate 重生成 重写");
        Add("story", "story:retry", "重试上一次提交", () => stage.RetryAsync(),
            () => stage.IsFailed, keywords: "retry 重试 失败");
        Add("story", "story:stop", "停止当前生成", () => stage.CancelAsync(),
            () => stage.IsGenerating, keywords: "stop cancel 停止 中断");
        Add("story", "story:new", "以当前角色开新故事",
            () => stage.StartNeutralSessionCommand.ExecuteAsync(null), keywords: "new story 新故事 开启");
        Add("story", "story:reload", "重新加载当前故事",
            () => stage.RefreshViewCommand.ExecuteAsync(null),
            () => story.View is not null, keywords: "reload refresh 重新加载 刷新");

        // ---- view ----
        Add("view", "view:studio", "切到工作台",
            () => { ui.SwitchViewMode(ViewportMode.Studio); return Task.CompletedTask; },
            () => ui.ViewMode != ViewportMode.Studio, keywords: "studio 工作台 跑团");
        Add("view", "view:reading", "切到纯净编年",
            () => { ui.SwitchViewMode(ViewportMode.Reading); return Task.CompletedTask; },
            () => ui.ViewMode != ViewportMode.Reading, keywords: "reading 纯净 编年 阅读");
        Add("view", "view:theme", ui.Theme == ThemeMode.Dark ? "切换到浅色主题" : "切换到深色主题",
            () => { ui.ToggleTheme(); return Task.CompletedTask; }, keywords: "theme dark light 主题 深浅 明暗");
        Add("view", "view:showcase", "打开独立组件展厅 (beautifului, beui, rareui, transitions, shadcn)",
            () =>
            {
                try { System.Diagnostics.Process.Start(new System.Diagnostics.ProcessStartInfo("http://localhost:5174") { UseShellExecute = true }); }
                catch { }
                return Task.CompletedTask;
            }, keywords: "showcase 组件 展厅 beautifului shadcn rareui transitions beui");
        Add("view", "view:left", "切换左栏", () => { ui.ToggleLeftRail(); return Task.CompletedTask; }, hint: "Ctrl+[", keywords: "left rail 左栏");
        Add("view", "view:right", "切换右栏", () => { ui.ToggleRightRail(); return Task.CompletedTask; }, hint: "Ctrl+]", keywords: "right rail 右栏");

        // ---- panel ----
        Add("panel", "panel:settings", "打开模型与偏好设置",
            () => { ui.SetSettingsOpen(true); return Task.CompletedTask; }, keywords: "settings 设置 模型 连接");
        Add("panel", "panel:worldbook", "打开世界书示例",
            () => { ui.SetWorldbookModalOpen(true); return Task.CompletedTask; }, keywords: "worldbook 世界书 示例");
        Add("panel", "panel:director", "打开导演模式",
            () => { stage.OpenDirector(); return Task.CompletedTask; },
            () => story.View is not null, keywords: "director 导演 大纲 阶段");
        Add("panel", "panel:memory", "打开认知记忆库",
            () => { ui.SetMemoryModalOpen(true); return Task.CompletedTask; },
            () => story.View is not null, keywords: "memory 记忆 认知");
        Add("panel", "panel:dossier", "打开角色档案",
            () => { ui.SetDossierModalOpen(true); return Task.CompletedTask; }, keywords: "dossier 档案 人设");
        Add("panel", "panel:promises", "打开誓约与承诺",
            () => { ui.SetPromisesModalOpen(true); return Task.CompletedTask; }, keywords: "promise 誓约 承诺 契约");
        Add("panel", "panel:tachie", "查看全身立绘",
            () => { ui.SetFullTachieOpen(true); return Task.CompletedTask; }, keywords: "tachie 立绘 全身 角色");
        Add("panel", "panel:import-card", "导入角色卡",
            () => { ui.SetCharImportOpen(true); return Task.CompletedTask; }, keywords: "import card 导入 角色卡");

        return list;
    }
}
