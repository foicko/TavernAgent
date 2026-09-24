using Avalonia.Controls;
using Avalonia.Media;

namespace TavernAgent.Desktop.Services;

/// <summary>
/// 把卡主题 token（accent/bg/ink）转成画刷覆盖，并同步到卡片 UI 表面（CardHudView / SceneBarView）。
/// 作用域刻意只限这些控件（对应 web 的 --card-* 只被 CardHUD/SceneBar 消费），不波及整体界面。
/// 只解析 #hex；rgb()/hsl() 与字体 token 跳过（避免跨平台字体/解析歧义）。
/// </summary>
public static class CardTheme
{
    // token 键 → 需要覆盖的资源键（一个 token 可能影响多个表面资源）。
    private static readonly Dictionary<string, string[]> TokenToResources = new(StringComparer.Ordinal)
    {
        ["accent"] = new[] { "AccentBrush", "MeterFillBrush" },
        ["bg"] = new[] { "SubtleBrush", "GlassBackgroundBrush", "GlassPillBrush" },
        ["ink"] = new[] { "TextPrimaryBrush" },
    };

    /// <summary>所有可能被覆盖的资源键；切换卡时先清空它们再套用新主题。</summary>
    public static readonly string[] ManagedKeys =
    {
        "AccentBrush", "MeterFillBrush", "SubtleBrush", "GlassBackgroundBrush", "GlassPillBrush", "TextPrimaryBrush",
    };

    /// <summary>构建 资源键 → 画刷；无法解析的 token 跳过。</summary>
    public static IReadOnlyDictionary<string, IBrush> BuildOverrides(IReadOnlyDictionary<string, string>? tokens)
    {
        var map = new Dictionary<string, IBrush>(StringComparer.Ordinal);
        if (tokens is null) return map;
        foreach (var (token, resourceKeys) in TokenToResources)
        {
            if (!tokens.TryGetValue(token, out var value) || string.IsNullOrWhiteSpace(value)) continue;
            if (!Color.TryParse(value.Trim(), out var color)) continue;
            var brush = new SolidColorBrush(color);
            foreach (var key in resourceKeys) map[key] = brush;
        }
        return map;
    }
}

/// <summary>
/// 卡片主题的进程级同步器：StageView 观察到 VM 的 CardThemeTokens 变化后调用 SetTokens，
/// 已挂载的卡片表面（CardHudView / SceneBarView）各自把覆盖写进自己的 Resources。
/// 用 DynamicResource 解析到最近的祖先资源，因此覆盖只作用于该控件子树。
/// </summary>
public static class CardThemeSync
{
    private static IReadOnlyDictionary<string, IBrush> _overrides = new Dictionary<string, IBrush>();
    private static readonly List<WeakReference<Control>> Targets = new();

    public static void SetTokens(IReadOnlyDictionary<string, string>? tokens)
    {
        _overrides = CardTheme.BuildOverrides(tokens);
        foreach (var reference in Targets)
        {
            if (reference.TryGetTarget(out var control)) Apply(control);
        }
    }

    /// <summary>把控件登记为卡片表面，并立即套用当前主题。</summary>
    public static void Attach(Control control)
    {
        Targets.Add(new WeakReference<Control>(control));
        Apply(control);
    }

    private static void Apply(Control control)
    {
        foreach (var key in CardTheme.ManagedKeys) control.Resources.Remove(key);
        foreach (var (key, brush) in _overrides) control.Resources[key] = brush;
    }
}
