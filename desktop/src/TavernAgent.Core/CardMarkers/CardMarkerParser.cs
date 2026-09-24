using System.Text.RegularExpressions;
using TavernAgent.Core.Stores;

namespace TavernAgent.Core.CardMarkers;

/// <summary>卡片 UI 标记片段的种类。</summary>
public enum CardMarkerKind
{
    Text,
    Image,
    Hud,
    Scene,
}

/// <summary>一个已解析的正文片段（文本/图片/状态面板/场景）。</summary>
public sealed class CardMarkerSegment
{
    public CardMarkerKind Kind { get; init; }
    public string Text { get; init; } = "";
    public string Url { get; init; } = "";
    public string Alt { get; init; } = "";
    public IReadOnlyDictionary<string, string>? Values { get; init; }
    public string Title { get; init; } = "";
}

/// <summary>
/// 解析正文里的卡片 UI 标记，供桌面端渲染为原生组件。
/// 与 internal/cardmarkers、web/src/lib/cardMarkers.tsx 保持一致。
///
/// 支持的标记：
///   &lt;img cmd="key"&gt; / [IMG:key]   图片（卡内资源）
///   [HUD:label=value|…]              状态面板
///   [SCENE:title]                    场景
///   [VAR:key=value|…]                会话变量（正文里不展示）
/// 转义：`[[` 表示字面量 `[`。
/// </summary>
public static class CardMarkerParser
{
    private static readonly Regex Marker = new(
        "<img\\s+cmd\\s*=\\s*\"([^\"]*)\"\\s*/?>|\\[(IMG|HUD|SCENE|VAR):([^\\[\\]\\r\\n]*)\\]",
        RegexOptions.IgnoreCase | RegexOptions.Compiled);

    private const string EscapeSentinel = "\u0000LBRACK\u0000";

    public static IReadOnlyList<CardMarkerSegment> Parse(string? text, IReadOnlyDictionary<string, string>? assets)
    {
        var segments = new List<CardMarkerSegment>();
        if (string.IsNullOrEmpty(text))
        {
            return segments;
        }
        var protectedText = text.Replace("[[", EscapeSentinel);
        var last = 0;
        void PushText(string s)
        {
            if (s.Length == 0)
            {
                return;
            }
            var t = s.Replace(EscapeSentinel, "[");
            // 合并相邻文本段：标记被剥离/原样保留时会产生多个文本片，合成一段更干净。
            if (segments.Count > 0 && segments[^1].Kind == CardMarkerKind.Text)
            {
                segments[^1] = new CardMarkerSegment { Kind = CardMarkerKind.Text, Text = segments[^1].Text + t };
            }
            else
            {
                segments.Add(new CardMarkerSegment { Kind = CardMarkerKind.Text, Text = t });
            }
        }

        foreach (Match m in Marker.Matches(protectedText))
        {
            if (m.Index > last)
            {
                PushText(protectedText[last..m.Index]);
            }
            if (m.Groups[1].Success)
            {
                var key = m.Groups[1].Value.Trim();
                var url = Resolve(assets, key);
                if (!string.IsNullOrEmpty(url))
                {
                    segments.Add(new CardMarkerSegment { Kind = CardMarkerKind.Image, Url = url, Alt = key });
                }
                else
                {
                    PushText(m.Value);
                }
            }
            else
            {
                var tag = m.Groups[2].Value.ToUpperInvariant();
                var body = m.Groups[3].Value.Trim();
                switch (tag)
                {
                    case "IMG":
                        var url = Resolve(assets, body);
                        if (!string.IsNullOrEmpty(url))
                        {
                            segments.Add(new CardMarkerSegment { Kind = CardMarkerKind.Image, Url = url, Alt = body });
                        }
                        else
                        {
                            PushText(m.Value);
                        }
                        break;
                    case "HUD":
                        var values = ParsePairs(body);
                        if (values.Count > 0)
                        {
                            segments.Add(new CardMarkerSegment { Kind = CardMarkerKind.Hud, Values = values });
                        }
                        else
                        {
                            PushText(m.Value);
                        }
                        break;
                    case "SCENE":
                        if (body.Length > 0)
                        {
                            segments.Add(new CardMarkerSegment { Kind = CardMarkerKind.Scene, Title = body });
                        }
                        else
                        {
                            PushText(m.Value);
                        }
                        break;
                    // VAR：状态标记，正文里不展示。
                }
            }
            last = m.Index + m.Length;
        }
        if (last < protectedText.Length)
        {
            PushText(protectedText[last..]);
        }
        return segments;
    }

    private static string? Resolve(IReadOnlyDictionary<string, string>? assets, string key)
        => assets is null || string.IsNullOrEmpty(key) ? null : CardAssetCache.Resolve(assets, key);

    private static Dictionary<string, string> ParsePairs(string body)
    {
        var values = new Dictionary<string, string>();
        foreach (var part in body.Split('|'))
        {
            var eq = part.IndexOf('=');
            if (eq < 0)
            {
                continue;
            }
            var k = part[..eq].Trim();
            var v = part[(eq + 1)..].Trim();
            if (k.Length > 0)
            {
                values[k] = v;
            }
        }
        return values;
    }
}
