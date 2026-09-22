using System.Collections.ObjectModel;
using System.Xml.Linq;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;

namespace TavernAgent.Desktop.ViewModels.Right;

/// <summary>Parsed "前情回顾与交接" from the active summary (web: storyCheckpoint.ts).</summary>
public sealed partial class StoryCheckpointViewModel : ObservableObject
{
    public ObservableCollection<CheckpointMindset> Mindsets { get; } = new();

    [ObservableProperty] private string _narrativeArc = "";
    [ObservableProperty] private string _hiddenTension = "";
    [ObservableProperty] private string _openLoops = "";
    [ObservableProperty] private string _milestones = "";
    [ObservableProperty] private bool _isExpanded = true;

    public bool HasNarrativeArc => !string.IsNullOrWhiteSpace(NarrativeArc);
    public bool HasMindsets => Mindsets.Count > 0 || !string.IsNullOrWhiteSpace(HiddenTension);
    public bool HasOpenLoops => !string.IsNullOrWhiteSpace(OpenLoops);
    public bool HasMilestones => !string.IsNullOrWhiteSpace(Milestones);
    public bool HasAny => HasNarrativeArc || HasMindsets || HasOpenLoops || HasMilestones;
    public string ToggleLabel => IsExpanded ? "收起 ▲" : "展开 ▼";

    [RelayCommand]
    private void Toggle() => IsExpanded = !IsExpanded;

    public void Load(string? summaryText)
    {
        NarrativeArc = "";
        HiddenTension = "";
        OpenLoops = "";
        Milestones = "";
        Mindsets.Clear();

        var raw = (summaryText ?? "").Trim();
        if (raw.Length == 0)
        {
            RaiseFlags();
            return;
        }

        if (!raw.StartsWith("<", StringComparison.Ordinal))
        {
            NarrativeArc = raw;
            RaiseFlags();
            return;
        }

        try
        {
            var doc = XDocument.Parse(raw);
            var root = doc.Root;
            if (root is null || root.Name.LocalName != "story_checkpoint")
            {
                NarrativeArc = raw;
                RaiseFlags();
                return;
            }

            NarrativeArc = Child(root, "narrative_arc");
            OpenLoops = Child(root, "open_loops");
            Milestones = Child(root, "milestones");
            var dynamics = root.Elements().FirstOrDefault(e => e.Name.LocalName == "character_dynamics");
            if (dynamics is not null)
            {
                foreach (var m in dynamics.Elements().Where(e => e.Name.LocalName == "mindset"))
                    Mindsets.Add(new CheckpointMindset(m.Attribute("character")?.Value ?? "", (m.Value ?? "").Trim()));
                HiddenTension = dynamics.Elements().FirstOrDefault(e => e.Name.LocalName == "hidden_tension")?.Value?.Trim() ?? "";
            }
        }
        catch
        {
            NarrativeArc = raw;
        }

        RaiseFlags();
    }

    private static string Child(XElement parent, string name)
        => parent.Elements().FirstOrDefault(e => e.Name.LocalName == name)?.Value?.Trim() ?? "";

    private void RaiseFlags()
    {
        OnPropertyChanged(nameof(HasNarrativeArc));
        OnPropertyChanged(nameof(HasMindsets));
        OnPropertyChanged(nameof(HasOpenLoops));
        OnPropertyChanged(nameof(HasMilestones));
        OnPropertyChanged(nameof(HasAny));
        OnPropertyChanged(nameof(ToggleLabel));
    }
}

public sealed record CheckpointMindset(string Character, string Content);
