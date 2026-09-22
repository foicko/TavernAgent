using System.Globalization;

namespace TavernAgent.Core.Stores;

/// <summary>
/// Display copy for model/slot configuration (parity with modelLabels.ts).
/// Hard rules: same model name appears once; no raw slot ids; never render long secrets.
/// </summary>
public static class ModelLabels
{
    public static readonly string[] RoleOrder = { "primary", "assist", "reflection" };

    public static readonly IReadOnlyDictionary<string, string> RoleLabels = new Dictionary<string, string>(StringComparer.Ordinal)
    {
        ["primary"] = "对话生成",
        ["assist"] = "导演协商",
        ["reflection"] = "后台记忆",
    };

    public static readonly IReadOnlyDictionary<string, string> RoleHints = new Dictionary<string, string>(StringComparer.Ordinal)
    {
        ["primary"] = "推进剧情时使用",
        ["assist"] = "导演模式下与模型协商大纲",
        ["reflection"] = "后台抽取记忆、维护摘要",
    };

    public const string FollowPrimary = "跟随对话生成";
    public const string DefaultProtocol = "openai-chat";

    public static readonly IReadOnlyList<(string Value, string Label)> ProtocolOptions = new List<(string, string)>
    {
        ("openai-chat", "OpenAI 兼容"),
        ("openai-responses", "OpenAI Responses"),
        ("anthropic-messages", "Anthropic"),
    };

    public static readonly IReadOnlyList<(string Value, string Label, string Hint)> EffortOptions = new List<(string, string, string)>
    {
        ("", "默认", "不发送思考参数，完全按供应商自己的默认行为"),
        ("low", "低", "思考最少，响应最快"),
        ("medium", "中", "速度与深度的折中"),
        ("high", "高", "思考最充分，响应最慢"),
    };

    public static bool IsRole(string value) => Array.IndexOf(RoleOrder, value) >= 0;

    public static string RoleLabel(string slot) => IsRole(slot) ? RoleLabels[slot] : slot;

    public static string RoleHint(string slot) => IsRole(slot) ? RoleHints[slot] : "";

    public static string ProtocolLabel(string? kind)
    {
        if (string.IsNullOrEmpty(kind)) return "";
        foreach (var (value, label) in ProtocolOptions)
        {
            if (value == kind) return label;
        }
        return kind;
    }

    public static string EffortLabel(string? value)
    {
        if (string.IsNullOrEmpty(value)) return "默认";
        foreach (var (v, label, _) in EffortOptions)
        {
            if (v == value) return label;
        }
        return value;
    }

    public static string EffortHint(string? value)
    {
        foreach (var (v, _, hint) in EffortOptions)
        {
            if (v == (value ?? "")) return hint;
        }
        return "";
    }

    /// <summary>Input budget = window − output − max(512, window/20); null when window unset.</summary>
    public static int? InputBudget(int? contextWindow, int? maxTokens)
    {
        var window = contextWindow ?? 0;
        if (window <= 0) return null;
        var output = maxTokens is > 0 ? maxTokens.Value : 2048;
        var margin = Math.Max(512, window / 20);
        return window - output - margin;
    }

    public static string TokenLabel(int tokens)
        => tokens >= 1024
            ? Math.Round(tokens / 1024.0).ToString(CultureInfo.InvariantCulture) + "k"
            : tokens.ToString(CultureInfo.InvariantCulture);

    public static string ConnectionHost(string? baseUrl)
    {
        var raw = (baseUrl ?? "").Trim();
        if (string.IsNullOrEmpty(raw)) return "";
        try
        {
            if (!Uri.TryCreate(raw.Contains("://", StringComparison.Ordinal) ? raw : "https://" + raw, UriKind.Absolute, out var uri))
                return "";
            return uri.Host;
        }
        catch
        {
            return "";
        }
    }

    public static string ConnectionTitle(string? name, string? model)
    {
        var n = (name ?? "").Trim();
        var m = (model ?? "").Trim();
        if (string.IsNullOrEmpty(n)) return string.IsNullOrEmpty(m) ? "未命名连接" : m;
        if (!string.IsNullOrEmpty(m) && string.Equals(n, m, StringComparison.OrdinalIgnoreCase)) return m;
        return n;
    }

    public static string ConnectionSubtitle(string? name, string? model, string? baseUrl)
    {
        var title = ConnectionTitle(name, model).ToLowerInvariant();
        var m = (model ?? "").Trim();
        var host = ConnectionHost(baseUrl);
        var parts = new List<string>();
        if (!string.IsNullOrEmpty(m) &&
            !string.Equals(m, title, StringComparison.OrdinalIgnoreCase) &&
            !title.Contains(m, StringComparison.OrdinalIgnoreCase))
        {
            parts.Add(m);
        }
        if (!string.IsNullOrEmpty(host) &&
            !title.Contains(host, StringComparison.OrdinalIgnoreCase) &&
            !parts.Any(p => p.Contains(host, StringComparison.OrdinalIgnoreCase)))
        {
            parts.Add(host);
        }
        return string.Join(" · ", parts);
    }

    public static string WindowLabel(int? contextWindow)
    {
        if (contextWindow is null or <= 0) return "窗口未设置";
        if (contextWindow >= 1024 * 1024)
            return Math.Round(contextWindow.Value / (1024.0 * 1024)).ToString(CultureInfo.InvariantCulture) + "M 窗口";
        return Math.Round(contextWindow.Value / 1024.0).ToString(CultureInfo.InvariantCulture) + "k 窗口";
    }

    public static string KeyStateLabel(bool? hasApiKey) => hasApiKey == true ? "密钥已存" : "无密钥";

    public static string MaskedKeyHint(string? masked)
    {
        var value = (masked ?? "").Trim();
        if (string.IsNullOrEmpty(value)) return "已保存";
        if (value.Contains('…')) return value;
        if (value.Length <= 12) return value;
        return value[..4] + "…" + value[^4..];
    }
}
