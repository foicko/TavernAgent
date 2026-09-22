using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using TavernAgent.Core.Dtos;

namespace TavernAgent.Desktop.ViewModels.Right;

/// <summary>One of the three relationship meters (web: .meter-unit).</summary>
public sealed partial class MeterViewModel : ObservableObject
{
    public MeterViewModel(string label, string valueText, double percent, string tone)
    {
        Label = label;
        ValueText = valueText;
        Percent = percent;
        Tone = tone;
    }

    public string Label { get; }
    public string ValueText { get; }
    public double Percent { get; }
    public string Tone { get; } // aff | trust | alert

    public bool IsAffection => Tone == "aff";
    public bool IsTrust => Tone == "trust";
    public bool IsAlert => Tone == "alert";

    public ObservableCollection<MeterDeltaViewModel> Deltas { get; } = new();

    public static double AffectionPercent(double v) => Math.Clamp((v + 100) / 2, 0, 100);
    public static double Clamp(double v) => Math.Min(100, Math.Max(0, v));

    public static MeterViewModel Affection(double v) => new("心意眷顾", v > 0 ? $"+{v:0}" : $"{v:0}", AffectionPercent(v), "aff");
    public static MeterViewModel Trust(double v) => new("信任沉淀", $"{v:0}%", Clamp(v), "trust");
    public static MeterViewModel Alert(double v) => new("防备戒心", $"{v:0}%", Clamp(v), "alert");
}

public sealed partial class MeterDeltaViewModel : ObservableObject
{
    public MeterDeltaViewModel(string text, bool isPositive)
    {
        Text = text;
        IsPositive = isPositive;
    }

    public string Text { get; }
    public bool IsPositive { get; }
    public string Glyph => IsPositive ? "▲" : "▼";
}
