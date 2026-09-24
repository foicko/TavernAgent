using System.Collections.Concurrent;
using TavernAgent.Core.Api;
using TavernAgent.Core.Dtos;

namespace TavernAgent.Core.Stores;

/// <summary>A card's presentation data fetched from <c>GET /api/v1/cards/{id}</c>.</summary>
public sealed record CardAssetBundle(
    IReadOnlyDictionary<string, string> ByName,
    IReadOnlyList<CardAsset> Typed,
    IReadOnlyDictionary<string, string> Theme,
    IReadOnlyList<string> SdData)
{
    public static readonly CardAssetBundle Empty = new(
        new Dictionary<string, string>(StringComparer.Ordinal),
        Array.Empty<CardAsset>(),
        new Dictionary<string, string>(StringComparer.Ordinal),
        Array.Empty<string>());
}

/// <summary>
/// In-memory cache of a card's embedded assets (charx 立绘/图标) and presentation data
/// (theme tokens, sdData) keyed by cardId. Only lightweight metadata is kept; bytes stay on the server.
/// (web: web/src/stores/cardAssetStore.ts)
/// </summary>
public static class CardAssetCache
{
    private static readonly ConcurrentDictionary<string, CardAssetBundle> Cache = new(StringComparer.Ordinal);

    public static CardAssetBundle GetBundle(string? cardId)
        => !string.IsNullOrEmpty(cardId) && Cache.TryGetValue(cardId, out var bundle) ? bundle : CardAssetBundle.Empty;

    public static IReadOnlyDictionary<string, string> Get(string? cardId) => GetBundle(cardId).ByName;

    public static IReadOnlyList<CardAsset> GetTyped(string? cardId) => GetBundle(cardId).Typed;

    public static IReadOnlyDictionary<string, string> GetTheme(string? cardId) => GetBundle(cardId).Theme;

    public static IReadOnlyList<string> GetSdData(string? cardId) => GetBundle(cardId).SdData;

    /// <summary>
    /// Resolve an <c>&lt;img cmd&gt;</c> key to a URL. Exact match wins; otherwise fall back to
    /// "same name + numeric suffix" variants (RisuAI exports 1/2/3 per expression while the
    /// model usually emits the bare key). Deterministic pick so re-renders don't flicker.
    /// </summary>
    public static string? Resolve(IReadOnlyDictionary<string, string> assets, string cmd)
    {
        if (assets.Count == 0 || string.IsNullOrEmpty(cmd)) return null;
        if (assets.TryGetValue(cmd, out var direct) && !string.IsNullOrEmpty(direct)) return direct;

        List<string>? candidates = null;
        foreach (var key in assets.Keys)
        {
            if (key.Length <= cmd.Length || !key.StartsWith(cmd, StringComparison.Ordinal)) continue;
            var numeric = true;
            for (var i = cmd.Length; i < key.Length; i++)
            {
                if (key[i] is < '0' or > '9') { numeric = false; break; }
            }
            if (!numeric) continue;
            (candidates ??= new List<string>()).Add(key);
        }
        if (candidates is null || candidates.Count == 0) return null;

        candidates.Sort(StringComparer.Ordinal);
        var hash = 0;
        foreach (var ch in cmd) hash = (hash + ch) % candidates.Count;
        return assets[candidates[hash]];
    }

    /// <summary>
    /// Fetch and cache the presentation bundle for a card. Returns true when the cache changed
    /// (so callers can rebuild views); failures cache an empty bundle and stop retrying.
    /// </summary>
    public static async Task<bool> EnsureAsync(TavernApiClient api, string? cardId, CancellationToken ct = default)
    {
        if (string.IsNullOrEmpty(cardId) || Cache.ContainsKey(cardId)) return false;
        var bundle = CardAssetBundle.Empty;
        try
        {
            var preview = await api.GetCardAsync(cardId, ct).ConfigureAwait(false);
            var byName = new Dictionary<string, string>(StringComparer.Ordinal);
            foreach (var asset in preview.Assets ?? new List<CardAsset>())
            {
                if (!string.IsNullOrEmpty(asset.Name) && !string.IsNullOrEmpty(asset.Url))
                    byName[asset.Name] = asset.Url;
            }
            bundle = new CardAssetBundle(
                byName,
                preview.Assets ?? new List<CardAsset>(),
                preview.ThemeTokens ?? new Dictionary<string, string>(StringComparer.Ordinal),
                preview.SdData ?? new List<string>());
        }
        catch
        {
            // Best-effort: a missing card (built-in preset / deleted) simply has no assets.
        }
        return Cache.TryAdd(cardId, bundle);
    }
}
