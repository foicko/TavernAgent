using System.Text.RegularExpressions;

namespace TavernAgent.Desktop.Services;

public sealed record ExtractedImagePrompt(string Prompt, string NegativePrompt, string Excerpt);

/// <summary>Turn text → image prompt (web: lib/imagePromptExtractor.ts).</summary>
public static partial class ImagePromptExtractor
{
    private static readonly IReadOnlyDictionary<string, (string Positive, string Negative)> StylePrompts =
        new Dictionary<string, (string, string)>
        {
            ["anime"] = (
                "masterpiece, best quality, anime visual novel CG, 16:9 wallpaper, Makoto Shinkai aesthetic, atmospheric lighting, detailed background",
                "low quality, worst quality, deformed hands, extra limbs, bad anatomy, blurry, text, watermark, signature"),
            ["fantasy"] = (
                "masterpiece, dark high fantasy illustration, intricate details, dramatic rim lighting, epic scene, painterly style",
                "worst quality, low quality, modern elements, deformed, blurry, bad hands, artifacts, watermark"),
            ["cinematic"] = (
                "masterpiece, 35mm film still, cinematic lighting, depth of field, atmospheric, volumetric light, 8k wallpaper",
                "anime, cartoon, sketch, worst quality, low quality, deformed, blurry, watermark"),
            ["realistic"] = (
                "masterpiece, ultra realistic photography, natural soft lighting, sharp focus, 8k uhd, highly detailed texture",
                "drawing, painting, illustration, cartoon, anime, worst quality, low quality, deformed, blurry"),
        };

    public static ExtractedImagePrompt Extract(IEnumerable<string>? blocks, string? fallbackText, string stylePreset, string? characterName)
    {
        var content = "";
        var list = blocks?.Where(b => !string.IsNullOrWhiteSpace(b)).ToList();
        content = list is { Count: > 0 } ? string.Join(" ", list) : (fallbackText ?? "").Trim();

        content = RxFullWidth().Replace(content, "");
        content = RxHalfWidth().Replace(content, "");
        content = RxMarkdown().Replace(content, " ");
        content = RxWhitespace().Replace(content, " ").Trim();

        var excerpt = content.Length <= 180 ? content : content[..180];

        var style = StylePrompts.TryGetValue(stylePreset, out var s) ? s : StylePrompts["anime"];
        var prompt = excerpt.Length > 0 ? $"{excerpt}, {style.Positive}" : style.Positive;
        if (!string.IsNullOrWhiteSpace(characterName)) prompt = $"Character: {characterName}, {prompt}";

        return new ExtractedImagePrompt(prompt, style.Negative, excerpt);
    }

    [GeneratedRegex(@"（[^）]*）")]
    private static partial Regex RxFullWidth();

    [GeneratedRegex(@"\([^)]*\)")]
    private static partial Regex RxHalfWidth();

    [GeneratedRegex(@"[*#_`]")]
    private static partial Regex RxMarkdown();

    [GeneratedRegex(@"\s+")]
    private static partial Regex RxWhitespace();
}
