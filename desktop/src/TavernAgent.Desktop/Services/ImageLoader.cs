using Avalonia.Media.Imaging;

namespace TavernAgent.Desktop.Services;

/// <summary>
/// Decodes avatars / tachie / thumbnails from data URLs, local files or http(s)
/// into Avalonia bitmaps, with a bounded in-memory cache and optional downscaling.
/// </summary>
public static class ImageLoader
{
    private const int MaxEntries = 160;

    private static readonly HttpClient Http = new() { Timeout = TimeSpan.FromSeconds(15) };
    private static readonly object Gate = new();
    private static readonly Dictionary<string, Entry> Cache = new();

    private sealed class Entry
    {
        public required Task<Bitmap?> Task { get; init; }
        public long LastAccess { get; set; }
    }

    public static Task<Bitmap?> LoadAsync(string? source, int? decodeWidth = null)
    {
        if (string.IsNullOrWhiteSpace(source)) return Task.FromResult<Bitmap?>(null);

        // Inline data URLs are large and usually single-use; caching them both
        // wastes memory and evicts real images. Decode without caching.
        if (source.StartsWith("data:", StringComparison.OrdinalIgnoreCase))
            return LoadCoreAsync(source, decodeWidth);

        var key = decodeWidth is null ? source : $"{decodeWidth}|{source}";
        lock (Gate)
        {
            if (Cache.TryGetValue(key, out var existing))
            {
                existing.LastAccess = Environment.TickCount64;
                return existing.Task;
            }

            var entry = new Entry { Task = LoadAndTrackAsync(key, source, decodeWidth), LastAccess = Environment.TickCount64 };
            Cache[key] = entry;
            TrimLocked();
            return entry.Task;
        }
    }

    private static async Task<Bitmap?> LoadAndTrackAsync(string key, string source, int? decodeWidth)
    {
        var bitmap = await LoadCoreAsync(source, decodeWidth).ConfigureAwait(false);
        if (bitmap is null)
        {
            // Never cache a failure permanently — a transient network error would
            // otherwise stick for the whole session.
            lock (Gate) Cache.Remove(key);
        }
        return bitmap;
    }

    // Evict least-recently-used entries. Kept in sync with the cache so a failed
    // decode (removed above) cannot leave a dangling key that later evicts a live
    // entry, and the bookkeeping cannot grow without bound.
    private static void TrimLocked()
    {
        while (Cache.Count > MaxEntries)
        {
            string? oldestKey = null;
            var oldest = long.MaxValue;
            foreach (var kv in Cache)
            {
                if (kv.Value.LastAccess < oldest)
                {
                    oldest = kv.Value.LastAccess;
                    oldestKey = kv.Key;
                }
            }
            if (oldestKey is null) return;
            Cache.Remove(oldestKey);
        }
    }

    private static async Task<Bitmap?> LoadCoreAsync(string source, int? decodeWidth)
    {
        try
        {
            if (source.StartsWith("data:", StringComparison.OrdinalIgnoreCase))
            {
                var comma = source.IndexOf(',');
                if (comma < 0) return null;
                var meta = source[..comma];
                if (meta.Contains("svg", StringComparison.OrdinalIgnoreCase)) return null; // no SVG decoder
                var payload = source[(comma + 1)..];
                var bytes = meta.Contains("base64", StringComparison.OrdinalIgnoreCase)
                    ? Convert.FromBase64String(payload)
                    : System.Text.Encoding.UTF8.GetBytes(Uri.UnescapeDataString(payload));
                return Decode(bytes, decodeWidth);
            }

            if (source.StartsWith("http", StringComparison.OrdinalIgnoreCase))
            {
                var bytes = await Http.GetByteArrayAsync(source).ConfigureAwait(false);
                return Decode(bytes, decodeWidth);
            }

            if (File.Exists(source))
            {
                var bytes = await File.ReadAllBytesAsync(source).ConfigureAwait(false);
                return Decode(bytes, decodeWidth);
            }
        }
        catch
        {
            // Any decode/network failure simply yields a placeholder.
        }
        return null;
    }

    private static Bitmap? Decode(byte[] bytes, int? decodeWidth)
    {
        using var ms = new MemoryStream(bytes);
        return decodeWidth is > 0 ? Bitmap.DecodeToWidth(ms, decodeWidth.Value) : new Bitmap(ms);
    }
}
