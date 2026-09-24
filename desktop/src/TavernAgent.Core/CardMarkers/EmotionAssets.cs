using TavernAgent.Core.Dtos;

namespace TavernAgent.Core.CardMarkers;

/// <summary>
/// Picks a card's emotion portrait (assets with <c>type=emotion</c>) for a normalized expression key.
/// (web: emotionMapFromAssets in web/src/stores/cardAssetStore.ts)
/// </summary>
public static class EmotionAssets
{
    // 归一化表情键 → 资源名关键词；与 RightRail 的 TachieExpression 键一致。
    private static readonly Dictionary<string, string[]> Keywords = new(StringComparer.Ordinal)
    {
        ["alert"] = new[] { "alert", "angry", "mad", "guard", "shock", "surprise", "fear", "scared", "worried" },
        ["calm"] = new[] { "calm", "neutral", "normal", "default", "quiet", "serious" },
        ["smile"] = new[] { "smile", "happy", "joy", "glad", "relieved", "laugh", "cheer" },
        ["whisper"] = new[] { "whisper", "blush", "shy", "soft", "moved", "sad", "cry", "sorrow", "gentle" },
    };

    /// <summary>Resolve an emotion portrait URL for the given expression key, or null when none matches.</summary>
    public static string? Resolve(IReadOnlyList<CardAsset>? assets, string? expressionKey)
    {
        if (assets is null || assets.Count == 0 || string.IsNullOrEmpty(expressionKey)) return null;
        if (!Keywords.TryGetValue(expressionKey, out var keywords)) return null;
        foreach (var asset in assets)
        {
            if (asset.Type != "emotion" || string.IsNullOrEmpty(asset.Url)) continue;
            var name = asset.Name.ToLowerInvariant();
            foreach (var keyword in keywords)
            {
                if (name.Contains(keyword, StringComparison.Ordinal)) return asset.Url;
            }
        }
        return null;
    }
}
