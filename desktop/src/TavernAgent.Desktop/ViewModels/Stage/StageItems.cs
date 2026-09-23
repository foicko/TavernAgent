using Avalonia;
using Avalonia.Media;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Stage;

/// <summary>A single rendered narrative frame (web: block kinds narration/dialogue/inner_monologue).</summary>
public sealed class BlockViewModel
{
    public BlockViewModel(TextBlock block, string charName, string userName)
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
    }

    public string Text { get; }
    public bool IsDialogue { get; }
    public bool IsMonologue { get; }
    public bool IsNarration { get; }
    public bool IsEmpty => string.IsNullOrEmpty(Text);
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
    }

    public string OptionId { get; }
    public string Intent { get; }
    public string Text { get; }
    public string Label { get; }

    /// <summary>Linear icon geometry from Styles/Icons.axaml (web: Icon name).</summary>
    public Geometry? IconGeometry { get; }

    /// <summary>Accent brush resource key (token-driven, no literal color).</summary>
    public string Accent { get; }

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
