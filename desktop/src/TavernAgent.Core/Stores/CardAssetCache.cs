using System.Collections.Concurrent;
using TavernAgent.Core.Api;
using TavernAgent.Core.Dtos;

namespace TavernAgent.Core.Stores;

/// <summary>
/// In-memory cache of a card's embedded assets (charx 立绘/图标) keyed by cardId.
/// Only the lightweight name→url map is kept; bytes stay on the server.
/// (web: web/src/stores/cardAssetStore.ts)
/// </summary>
public static class CardAssetCache
{
    private static readonly IReadOnlyDictionary<string, string> Empty =
        new Dictionary<string, string>(StringComparer.Ordinal);

    private static readonly ConcurrentDictionary<string, IReadOnlyDictionary<string, string>> Cache = new(StringComparer.Ordinal);

    public static IReadOnlyDictionary<string, string> Get(string? cardId)
        => !string.IsNullOrEmpty(cardId) && Cache.TryGetValue(cardId, out var map) ? map : Empty;

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
    /// Fetch and cache the asset map for a card. Returns true when the cache changed
    /// (so callers can rebuild views); failures cache an empty map and stop retrying.
    /// </summary>
    public static async Task<bool> EnsureAsync(TavernApiClient api, string? cardId, CancellationToken ct = default)
    {
        if (string.IsNullOrEmpty(cardId) || Cache.ContainsKey(cardId)) return false;
        IReadOnlyDictionary<string, string> map = Empty;
        try
        {
            var preview = await api.GetCardAsync(cardId, ct).ConfigureAwait(false);
            var built = new Dictionary<string, string>(StringComparer.Ordinal);
            foreach (var asset in preview.Assets ?? new List<CardAsset>())
            {
                if (!string.IsNullOrEmpty(asset.Name) && !string.IsNullOrEmpty(asset.Url))
                    built[asset.Name] = asset.Url;
            }
            map = built;
        }
        catch
        {
            // Best-effort: a missing card (built-in preset / deleted) simply has no assets.
        }
        return Cache.TryAdd(cardId, map);
    }
}
