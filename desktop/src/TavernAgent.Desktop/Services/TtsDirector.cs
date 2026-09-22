using System.Text;

namespace TavernAgent.Desktop.Services;

/// <summary>
/// MiMo情境演播: audio-tag enrichment + director instruction (web: lib/ttsDirector.ts, simplified).
/// Only meaningful for the MiMo engine; other engines must receive clean text.
/// </summary>
public static class TtsDirector
{
    public static bool IsMiMoEngine(string? engine, string? model, string? baseUrl)
    {
        if (string.Equals(engine, "mimo", StringComparison.OrdinalIgnoreCase)) return true;
        if (!string.IsNullOrEmpty(model) && model.Contains("mimo", StringComparison.OrdinalIgnoreCase)) return true;
        return !string.IsNullOrEmpty(baseUrl) && baseUrl.Contains("xiaomimimo", StringComparison.OrdinalIgnoreCase);
    }

    /// <summary>Prefix a delivery hint and sprinkle a couple of audio tags when none are present.</summary>
    public static string Enrich(string text, string? mood)
    {
        if (string.IsNullOrWhiteSpace(text)) return text;
        var body = text.Trim();
        var hasTags = body.Contains('[') || body.Contains('（');
        var sb = new StringBuilder();
        if (!string.IsNullOrWhiteSpace(mood)) sb.Append('(').Append(mood!.Trim()).Append(')');
        if (!hasTags)
        {
            if (sb.Length > 0) sb.Append(' ');
            sb.Append("[停顿片刻] ");
        }
        sb.Append(body);
        return sb.ToString();
    }

    public static string BuildInstruction(string? characterName, string? scene, string? mood)
    {
        var sb = new StringBuilder();
        if (!string.IsNullOrWhiteSpace(characterName)) sb.Append("【角色】").Append(characterName!.Trim()).Append('\n');
        if (!string.IsNullOrWhiteSpace(scene)) sb.Append("【场景】").Append(scene!.Trim()).Append('\n');
        if (!string.IsNullOrWhiteSpace(mood)) sb.Append("【心境】").Append(mood!.Trim()).Append('\n');
        sb.Append("【指导】以符合角色性格与当下心境的语气自然演播，情绪随剧情起伏，避免机械朗读。");
        return sb.ToString();
    }
}
