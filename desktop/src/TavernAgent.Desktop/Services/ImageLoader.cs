using System.Collections.Concurrent;
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
    private static readonly ConcurrentDictionary<string, Task<Bitmap?>> Cache = new();
    private static readonly ConcurrentQueue<string> InsertionOrder = new();

    public static Task<Bitmap?> LoadAsync(string? source, int? decodeWidth = null)
    {
        if (string.IsNullOrWhiteSpace(source)) return Task.FromResult<Bitmap?>(null);

        // Inline data URLs are large and usually single-use; caching them both
        // wastes memory and evicts real images. Decode without caching.
        if (source.StartsWith("data:", StringComparison.OrdinalIgnoreCase))
            return LoadCoreAsync(source, decodeWidth);

        var key = decodeWidth is null ? source : $"{decodeWidth}|{source}";
        if (Cache.TryGetValue(key, out var existing)) return existing;

        var task = Cache.GetOrAdd(key, _ => LoadAndTrackAsync(key, source, decodeWidth));
        InsertionOrder.Enqueue(key);
        Trim();
        return task;
    }

    private static async Task<Bitmap?> LoadAndTrackAsync(string key, string source, int? decodeWidth)
    {
        var bitmap = await LoadCoreAsync(source, decodeWidth).ConfigureAwait(false);
        if (bitmap is null)
        {
            // Never cache a failure permanently — a transient network error would
            // otherwise stick for the whole session.
            Cache.TryRemove(key, out _);
        }
        return bitmap;
    }

    private static void Trim()
    {
        while (Cache.Count > MaxEntries && InsertionOrder.TryDequeue(out var oldest))
            Cache.TryRemove(oldest, out _);
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
