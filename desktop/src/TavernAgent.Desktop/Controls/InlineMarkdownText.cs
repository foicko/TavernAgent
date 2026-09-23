using System.Text.RegularExpressions;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Documents;

namespace TavernAgent.Desktop.Controls;

/// <summary>
/// SelectableTextBlock that renders the minimal inline markdown used by the narrative
/// (web: renderInlineMarkdown) — **bold** and *italic* only, no HTML.
/// </summary>
public sealed class InlineMarkdownText : SelectableTextBlock
{
    private static readonly Regex Inline = new(@"(\*\*[^*\n]+\*\*|\*[^*\n]+\*)", RegexOptions.Compiled);

    public static readonly StyledProperty<string?> MarkdownProperty =
        AvaloniaProperty.Register<InlineMarkdownText, string?>(nameof(Markdown));

    static InlineMarkdownText()
    {
        MarkdownProperty.Changed.AddClassHandler<InlineMarkdownText>((c, _) => c.Rebuild());
    }

    public string? Markdown
    {
        get => GetValue(MarkdownProperty);
        set => SetValue(MarkdownProperty, value);
    }

    private void Rebuild()
    {
        var collection = new InlineCollection();
        var text = Markdown ?? "";
        foreach (var part in Inline.Split(text))
        {
            if (part.Length == 0) continue;
            if (part.StartsWith("**", StringComparison.Ordinal) && part.EndsWith("**", StringComparison.Ordinal) && part.Length > 4)
                collection.Add(new Bold { Inlines = new InlineCollection { new Run(part[2..^2]) } });
            else if (part.StartsWith('*') && part.EndsWith('*') && part.Length > 2)
                collection.Add(new Italic { Inlines = new InlineCollection { new Run(part[1..^1]) } });
            else
                collection.Add(new Run(part));
        }
        Inlines = collection;
    }
}
