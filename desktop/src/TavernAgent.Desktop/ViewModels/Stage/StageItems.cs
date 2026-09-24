using Avalonia;
using Avalonia.Media;
using Avalonia.Media.Imaging;
using TavernAgent.Core.CardMarkers;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Stage;

/// <summary>A rendered piece of a narrative block: either markdown text or an embedded image.</summary>
public abstract class NarrativeSegment
{
}

/// <summary>Text run rendered through the minimal inline markdown control.</summary>
public sealed class NarrativeTextSegment(string markdown, bool isDialogue, bool isMonologue, bool isNarration) : NarrativeSegment
{
    public string Markdown { get; } = markdown;
    public bool IsDialogue { get; } = isDialogue;
    public bool IsMonologue { get; } = isMonologue;
    public bool IsNarration { get; } = isNarration;
}

/// <summary>An embedded card asset (charx &lt;img cmd="..."&gt;) decoded into a bitmap.</summary>
public sealed class NarrativeImageSegment : NarrativeSegment, System.ComponentModel.INotifyPropertyChanged
{
    public NarrativeImageSegment(string url, string alt)
    {
        Url = url;
        Alt = alt;
        _ = LoadAsync();
    }

    public string Url { get; }
    public string Alt { get; }

    private Bitmap? _image;
    public Bitmap? Image
    {
        get => _image;
        private set
        {
            _image = value;
            PropertyChanged?.Invoke(this, new System.ComponentModel.PropertyChangedEventArgs(nameof(Image)));
        }
    }

    public event System.ComponentModel.PropertyChangedEventHandler? PropertyChanged;

    private async Task LoadAsync()
    {
        Image = await ImageLoader.LoadAsync(TurnIllustrationViewModel.AbsoluteUrl(Url), 480).ConfigureAwait(true);
    }
}

/// <summary>状态面板的一行。</summary>
public sealed record HudRow(string Label, string Value);

/// <summary>A card state panel or scene chip parsed from a UI marker.</summary>
public sealed class NarrativeHudSegment : NarrativeSegment
{
    public NarrativeHudSegment(bool isScene, string title, IReadOnlyDictionary<string, string>? values)
    {
        IsScene = isScene;
        Title = title;
        Values = (values ?? new Dictionary<string, string>())
            .Select(kv => new HudRow(kv.Key, kv.Value))
            .ToList();
    }

    public bool IsScene { get; }
    public string Title { get; }
    public IReadOnlyList<HudRow> Values { get; }
    public bool HasValues => Values.Count > 0;
}

/// <summary>A single rendered narrative frame (web: block kinds narration/dialogue/inner_monologue).</summary>
public sealed class BlockViewModel
{
    public BlockViewModel(TextBlock block, string charName, string userName, IReadOnlyDictionary<string, string>? assets = null)
    {
        var expanded = NarrativeText.ReplaceMacros(block.Text, charName, userName);
        if (block.Kind == "dialogue")
        {
            Text = "「" + NarrativeText.ReplaceMacros(NarrativeText.TrimDialogue(block.Text), charName, userName) + "」";
            IsDialogue = true;
        }
        else if (block.Kind == "inner_monologue")
        {
            Text = "（" + NarrativeText.ReplaceMacros(NarrativeText.TrimMonologue(block.Text), charName, userName) + "）";
            IsMonologue = true;
        }
        else
        {
            Text = expanded;
            IsNarration = true;
        }
        Segments = BuildSegments(Text, assets);
    }

    public string Text { get; }
    public bool IsDialogue { get; }
    public bool IsMonologue { get; }
    public bool IsNarration { get; }
    public bool IsEmpty => string.IsNullOrEmpty(Text);

    /// <summary>Text and embedded images in order; falls back to a single text run.</summary>
    public IReadOnlyList<NarrativeSegment> Segments { get; }

    private IReadOnlyList<NarrativeSegment> BuildSegments(string text, IReadOnlyDictionary<string, string>? assets)
    {
        NarrativeTextSegment TextSegment(string markdown) => new(markdown, IsDialogue, IsMonologue, IsNarration);

        var parsed = CardMarkerParser.Parse(text, assets);
        if (parsed.Count == 0) return new NarrativeSegment[] { TextSegment(text) };

        var segments = new List<NarrativeSegment>();
        foreach (var s in parsed)
        {
            switch (s.Kind)
            {
                case CardMarkerKind.Text:
                    if (s.Text.Length > 0) segments.Add(TextSegment(s.Text));
                    break;
                case CardMarkerKind.Image:
                    segments.Add(new NarrativeImageSegment(s.Url, s.Alt));
                    break;
                case CardMarkerKind.Hud:
                    segments.Add(new NarrativeHudSegment(false, "", s.Values));
                    break;
                case CardMarkerKind.Scene:
                    segments.Add(new NarrativeHudSegment(true, s.Title, null));
                    break;
            }
        }
        return segments.Count > 0 ? segments : new NarrativeSegment[] { TextSegment(text) };
    }
}

public sealed class OptionViewModel
{
    public OptionViewModel(OptionView option)
    {
        OptionId = option.OptionId;
        Intent = option.Intent;
        Text = option.Text;
        (Label, IconResourceKey, Accent) = Intent switch
        {
            "aggressive" => ("强势威慑", "IconSwords", "DangerBrush"),
            "clever" => ("机智试探", "IconLightbulb", "InfoBrush"),
            "emotional" => ("共情抚慰", "IconHandshake", "SuccessBrush"),
            _ => ("出其不意", "IconDice", "AccentAmberBrush"),
        };
        IconGeometry = Resolve<Geometry>(IconResourceKey);
        AccentBrush = Resolve<IBrush>(Accent) ?? Resolve<IBrush>("TextSecondaryBrush");
    }

    public string OptionId { get; }
    public string Intent { get; }
    public string Text { get; }
    public string Label { get; }

    /// <summary>Linear icon geometry from Styles/Icons.axaml (web: Icon name).</summary>
    public Geometry? IconGeometry { get; }

    /// <summary>Accent brush resource key (token-driven, no literal color).</summary>
    public string Accent { get; }

    /// <summary>Resolved accent brush for the intent stripe/icon.</summary>
    public IBrush? AccentBrush { get; }

    private string IconResourceKey { get; }

    private static T? Resolve<T>(string key) where T : class =>
        Application.Current is { } app && app.Resources.TryGetResource(key, null, out var value) ? value as T : null;
}

public sealed class CheckEvidenceViewModel
{
    public CheckEvidenceViewModel(CheckResult check, string? actionLabel)
    {
        RollId = check.RollId;
        Attribute = check.Attribute;
        Natural = check.Natural;
        Modifier = check.AttributeModifier;
        Total = check.Total;
        Dc = check.Dc;
        Outcome = check.Outcome;
        PermanentEffect = check.PermanentEffect;
        var attrName = check.Attribute.ToLowerInvariant() switch
        {
            "strength" or "str" => "力量",
            "dexterity" or "dex" => "敏捷",
            "wisdom" or "wis" => "察觉",
            "charisma" or "cha" => "交涉",
            "luck" or "lck" => "运气",
            "constitution" or "con" => "体质",
            "intelligence" or "int" => "智力",
            "perception" or "per" => "感知",
            _ => check.Attribute,
        };
        AttributeName = attrName;
        Label = actionLabel ?? $"{attrName}检定";
        OutcomeLabel = check.Outcome switch
        {
            "success" => "成功",
            "failure" => "失败",
            "critical_success" => "大成功 ✦",
            "critical_failure" => "大失败 ✕",
            _ => check.Outcome,
        };
        IsSuccess = check.Outcome is "success" or "critical_success";
        IsCritSuccess = check.Outcome is "critical_success";
        IsCritFailure = check.Outcome is "critical_failure";
        Detail = $"{Label}（{attrName}）：d20 = {check.Natural}，修正 {(check.AttributeModifier >= 0 ? "+" : "")}{check.AttributeModifier}，总值 {check.Total} / DC {check.Dc} · {OutcomeLabel}";
    }

    public string RollId { get; }
    public string Attribute { get; }
    public string AttributeName { get; }
    public int Natural { get; }
    public int Modifier { get; }
    public int Total { get; }
    public int Dc { get; }
    public string Outcome { get; }
    public string? PermanentEffect { get; }
    public string Label { get; }
    public string OutcomeLabel { get; }
    public bool IsSuccess { get; }
    public bool IsCritSuccess { get; }
    public bool IsCritFailure { get; }
    public string Detail { get; }
    public bool HasPermanentEffect => !string.IsNullOrEmpty(PermanentEffect);
}

public sealed class ChangeEvidenceViewModel
{
    public ChangeEvidenceViewModel(StateChange change)
    {
        Label = change.Label;
        Text = change.Text;
        Delta = change.Delta;
        IsUp = change.Delta > 0;
        IsDown = change.Delta < 0;
    }

    public string Label { get; }
    public string Text { get; }
    public double? Delta { get; }
    public bool IsUp { get; }
    public bool IsDown { get; }
}

public sealed class MemoryEvidenceViewModel
{
    public MemoryEvidenceViewModel(MemoryRef memory)
    {
        MemoryId = memory.MemoryId;
        Text = memory.Text;
        Kind = memory.Kind;
        KindLabel = memory.Kind switch
        {
            "observed" => "见证",
            "reported" => "转述",
            "inferred" => "推测",
            "secret" => "秘密",
            _ => memory.Kind ?? "",
        };
    }

    public string MemoryId { get; }
    public string Text { get; }
    public string? Kind { get; }
    public string KindLabel { get; }
}
