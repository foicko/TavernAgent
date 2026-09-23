using System.Text.RegularExpressions;

namespace TavernAgent.Desktop.ViewModels.Stage;

/// <summary>Display-layer macro expansion (web: characterMacros.ts).</summary>
public static partial class NarrativeText
{
    public const string DefaultPlayerName = "旅人";
    public const string DefaultCharacterName = "角色";

    /// <summary>Opening text used when starting a session from the neutral skin (no card first_mes).</summary>
    public const string DefaultOpeningText =
        "夜色低垂，灯火在潮湿的木桌上轻轻摇曳。远处传来酒杯碰撞与低语，而你与这个世界的因果，才刚刚开始交织。";

    public static string ReplaceMacros(string? text, string charName, string? userName)
    {
        if (string.IsNullOrEmpty(text)) return "";
        var c = string.IsNullOrEmpty(charName) ? DefaultCharacterName : charName;
        var u = string.IsNullOrEmpty(userName) ? DefaultPlayerName : userName;
        var result = text!;
        result = RxChar().Replace(result, c);
        result = RxUser().Replace(result, u);
        result = RxSub().Replace(result, "他/她");
        result = RxObj().Replace(result, "他/她");
        result = RxPoss().Replace(result, "的");
        result = RxStart().Replace(result, "\n\n--- 【示例对白开始】 ---\n\n");
        return result;
    }

    /// <summary>Trim a single leading/trailing quote or bracket pair from a block (web parity).</summary>
    public static string TrimQuote(string text, char lead, char trail)
    {
        if (string.IsNullOrEmpty(text)) return "";
        var s = text;
        if (s.Length > 0 && (s[0] == lead)) s = s[1..];
        if (s.Length > 0 && (s[^1] == trail)) s = s[..^1];
        return s;
    }

    public static string TrimDialogue(string text)
    {
        var s = TrimQuote(text, '「', '」');
        s = TrimQuote(s, '"', '"');
        s = TrimQuote(s, '\'', '\'');
        return s;
    }

    public static string TrimMonologue(string text)
    {
        var s = TrimQuote(text, '（', '）');
        s = TrimQuote(s, '(', ')');
        return s;
    }

    private static readonly char[] NameDelimiters = { '，', ',', '·', '-', '—', '–', '_', ':', '：', '/', '|' };

    /// <summary>Short display name: primary name token, trimmed, capped (web: cleanShortName).</summary>
    public static string CleanShortName(string name)
    {
        if (string.IsNullOrWhiteSpace(name)) return "自定义";
        var first = name.Split('\n', '\r')[0].Trim();
        var parts = first.Split(NameDelimiters, StringSplitOptions.RemoveEmptyEntries);
        var pick = parts.Length > 0 ? parts[0].Trim() : first;
        return pick.Length <= 10 ? pick : pick[..10];
    }

    /// <summary>First non-empty line, capped with ellipsis.</summary>
    public static string? FirstLine(string? text, int max = 42)
    {
        if (string.IsNullOrWhiteSpace(text)) return null;
        var line = text.Split('\n', '\r').FirstOrDefault(l => !string.IsNullOrWhiteSpace(l))?.Trim();
        if (string.IsNullOrEmpty(line)) return null;
        return line.Length <= max ? line : line[..max] + "…";
    }

    [GeneratedRegex(@"\{\{\s*char\s*\}\}", RegexOptions.IgnoreCase)]
    private static partial Regex RxChar();

    [GeneratedRegex(@"\{\{\s*user\s*\}\}", RegexOptions.IgnoreCase)]
    private static partial Regex RxUser();

    [GeneratedRegex(@"\{\{\s*sub\s*\}\}", RegexOptions.IgnoreCase)]
    private static partial Regex RxSub();

    [GeneratedRegex(@"\{\{\s*obj\s*\}\}", RegexOptions.IgnoreCase)]
    private static partial Regex RxObj();

    [GeneratedRegex(@"\{\{\s*poss\s*\}\}", RegexOptions.IgnoreCase)]
    private static partial Regex RxPoss();

    [GeneratedRegex(@"<START>", RegexOptions.IgnoreCase)]
    private static partial Regex RxStart();
}
