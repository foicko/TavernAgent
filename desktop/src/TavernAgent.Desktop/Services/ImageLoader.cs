using System.Collections.Concurrent;
using Avalonia.Media.Imaging;

namespace TavernAgent.Desktop.Services;

/// <summary>
/// Decodes avatars / tachie / thumbnails from data URLs, local files or http(s)
/// into Avalonia bitmaps, with an in-memory cache and optional downscaling.
/// </summary>
public static class ImageLoader
{
    private static readonly HttpClient Http = new() { Timeout = TimeSpan.FromSeconds(15) };
    private static readonly ConcurrentDictionary<string, Task<Bitmap?>> Cache = new();

    public static Task<Bitmap?> LoadAsync(string? source, int? decodeWidth = null)
    {
        if (string.IsNullOrWhiteSpace(source)) return Task.FromResult<Bitmap?>(null);
        var key = decodeWidth is null ? source : $"{decodeWidth}|{source}";
        return Cache.GetOrAdd(key, _ => LoadCoreAsync(source, decodeWidth));
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
