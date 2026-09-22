using System.Text.Json;
using TavernAgent.Core.Dtos;

namespace TavernAgent.Core.Stores;

/// <summary>
/// Resolve which character preset a session belongs to.
/// Returns null for custom-card sessions (never mislabels as a builtin).
/// </summary>
public static class StoryHelpers
{
    public static string? ResolveCharKeyFromSession(SessionView? view)
    {
        if (view is null) return null;
        if (!string.IsNullOrEmpty(view.CharacterId))
        {
            foreach (var key in CharacterPresets.Presets.Keys)
            {
                if (key == "custom") continue;
                var identity = CharacterPresets.PresetIdentity(key);
                if (view.CharacterId == identity || view.CharacterId == $"card_{key}")
                    return key;
            }
            return null;
        }

        if (view.State?.Characters is { Count: > 0 } chars)
        {
            foreach (var (charId, info) in chars)
            {
                foreach (var key in CharacterPresets.Presets.Keys)
                {
                    if (key == "custom") continue;
                    if (charId == $"npc_{key}" || charId == key)
                        return key;
                }
                foreach (var (key, preset) in CharacterPresets.Presets)
                {
                    if (key == "custom") continue;
                    if ((!string.IsNullOrEmpty(info.Name) &&
                         (info.Name == preset.Name || info.Name == preset.ShortName)) ||
                        (!string.IsNullOrEmpty(info.Description) &&
                         info.Description.Contains(preset.ShortName, StringComparison.Ordinal)))
                    {
                        return key;
                    }
                }
            }
        }

        var title = (view.Title ?? "").ToLowerInvariant();
        foreach (var (key, preset) in CharacterPresets.Presets)
        {
            if (key == "custom") continue;
            if (title.Contains(key.ToLowerInvariant(), StringComparison.Ordinal) ||
                title.Contains(preset.ShortName.ToLowerInvariant(), StringComparison.Ordinal) ||
                title.Contains(preset.Name.ToLowerInvariant(), StringComparison.Ordinal))
            {
                return key;
            }
        }

        return null;
    }

    public static string ParseRoot(PlotNode node)
    {
        try
        {
            using var doc = JsonDocument.Parse(node.ContentJson);
            if (doc.RootElement.ValueKind == JsonValueKind.Object &&
                doc.RootElement.TryGetProperty("openingText", out var ot) &&
                ot.ValueKind == JsonValueKind.String)
            {
                return ot.GetString() ?? "（故事开始）";
            }
        }
        catch (JsonException) { /* fall through */ }
        return "（故事开始）";
    }

    public static TurnContent? ParseTurnContent(PlotNode node)
    {
        try
        {
            return JsonSerializer.Deserialize<TurnContent>(node.ContentJson, JsonDefaults.Options);
        }
        catch (JsonException)
        {
            return null;
        }
    }
}
