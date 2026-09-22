namespace TavernAgent.Desktop.ViewModels.Modals.Settings;

/// <summary>Generic label/value option for ComboBox bindings (ToString drives the label).</summary>
public sealed record OptionItem(string Value, string Label)
{
    public override string ToString() => Label;
}

/// <summary>Provider connection preset (web: lib/providerPresets.ts).</summary>
public sealed record ProviderPreset(
    string Id,
    string Label,
    string Kind,
    string BaseUrl,
    IReadOnlyList<string> Models,
    string KeyHint);

public static class ProviderPresets
{
    public static readonly IReadOnlyList<ProviderPreset> All = new List<ProviderPreset>
    {
        new("deepseek", "DeepSeek", "openai-chat", "https://api.deepseek.com",
            new[] { "deepseek-chat", "deepseek-reasoner" }, "sk-…（DeepSeek 控制台创建）"),
        new("openai", "OpenAI", "openai-responses", "https://api.openai.com/v1",
            new[] { "gpt-4.1", "gpt-4o" }, "sk-…（platform.openai.com）"),
        new("anthropic", "Anthropic", "anthropic-messages", "https://api.anthropic.com",
            new[] { "claude-sonnet-4-20250514", "claude-3-5-haiku-latest" }, "sk-ant-…（console.anthropic.com）"),
        new("ollama", "本地 Ollama", "openai-chat", "http://127.0.0.1:11434/v1",
            new[] { "qwen2.5:7b", "llama3.1:8b" }, "本地服务通常无需密钥"),
        new("oneapi", "OneAPI / NewAPI", "openai-chat", "http://127.0.0.1:3000/v1",
            new[] { "gpt-4o-mini" }, "填你的中转站密钥"),
        new("custom", "自定义", "openai-chat", "", Array.Empty<string>(), ""),
    };

    public static ProviderPreset Custom => All[^1];

    public static ProviderPreset? Match(string? kind, string? baseUrl)
    {
        var normalized = Normalize(baseUrl);
        foreach (var preset in All)
        {
            if (preset.Kind != kind) continue;
            if (Normalize(preset.BaseUrl) == normalized) return preset;
        }
        return null;
    }

    /// <summary>Suggest a non-colliding name: model → host → "(n)" suffix.</summary>
    public static string SuggestName(ProviderPreset preset, string? baseUrl, string? model, IEnumerable<string> existingNames)
    {
        var taken = new HashSet<string>(existingNames, StringComparer.OrdinalIgnoreCase);
        foreach (var candidate in Candidates(preset, baseUrl, model))
        {
            if (!taken.Contains(candidate)) return candidate;
        }
        var baseName = Candidates(preset, baseUrl, model).FirstOrDefault() ?? "新连接";
        for (var i = 2; i < 100; i++)
        {
            var candidate = $"{baseName} ({i})";
            if (!taken.Contains(candidate)) return candidate;
        }
        return baseName;
    }

    private static IEnumerable<string> Candidates(ProviderPreset preset, string? baseUrl, string? model)
    {
        if (!string.IsNullOrWhiteSpace(model)) yield return model!.Trim();
        foreach (var suggestion in preset.Models) yield return suggestion;
        var host = Host(baseUrl);
        if (!string.IsNullOrEmpty(host)) yield return host;
        if (!string.IsNullOrWhiteSpace(preset.Label)) yield return preset.Label;
    }

    public static string Host(string? baseUrl)
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

    private static string Normalize(string? baseUrl) => (baseUrl ?? "").Trim().TrimEnd('/').ToLowerInvariant();
}
