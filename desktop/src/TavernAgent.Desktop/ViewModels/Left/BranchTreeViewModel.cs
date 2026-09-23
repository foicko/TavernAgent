using System.Collections.ObjectModel;
using System.Text.Json;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Left;

/// <summary>Branch topology tree + snapshot probe (web: LeftRail tree panel).</summary>
public sealed partial class BranchTreeViewModel : WorkspaceChildViewModel
{
    private bool _disposed;

    public BranchTreeViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        Outline = new StoryOutlineViewModel(workspace);
        Story.PropertyChanged += OnStoryChanged;
        Rebuild();
    }

    public StoryStore Story => Workspace.Story;

    public ObservableCollection<BranchNodeViewModel> Nodes { get; } = new();
    public StoryOutlineViewModel Outline { get; }

    [ObservableProperty] private string? _probeNodeId;
    [ObservableProperty] private string? _confirmNodeId;

    public bool HasNodes => Nodes.Count > 0;
    public bool IsEmpty => Nodes.Count == 0;

    public bool HasLiveRelation { get; private set; }
    public double LiveAffection { get; private set; }
    public double LiveTrust { get; private set; }
    public double LiveAlertness { get; private set; }

    [RelayCommand]
    private void OpenStoryMap() => Workspace.LeftRail.StoryMap.Open();

    public void ToggleProbe(BranchNodeViewModel node)
    {
        ProbeNodeId = ProbeNodeId == node.Id ? null : node.Id;
        ConfirmNodeId = null;
        SyncNodeStates();
    }

    public void CloseProbe()
    {
        ProbeNodeId = null;
        ConfirmNodeId = null;
        SyncNodeStates();
    }

    public void RequestFork(BranchNodeViewModel node)
    {
        ConfirmNodeId = node.Id;
        SyncNodeStates();
    }

    public void CancelFork()
    {
        ConfirmNodeId = null;
        SyncNodeStates();
    }

    public async Task ConfirmForkAsync(BranchNodeViewModel node)
    {
        if (!HasNodes) { Workspace.Ui.NotifyQuiet("请先开启故事，再开辟分支。"); return; }
        ConfirmNodeId = null;
        ProbeNodeId = null;
        SyncNodeStates();
        await Story.ForkFromAsync(node.Id, $"平行分支_{node.Tag}").ConfigureAwait(true);
    }

    public async Task ViewAtAsync(BranchNodeViewModel node)
    {
        if (!HasNodes) { Workspace.Ui.NotifyQuiet("请先开启故事，再查看历史快照。"); return; }
        ProbeNodeId = null;
        SyncNodeStates();
        await Story.ViewAtAsync(node.Id).ConfigureAwait(true);
    }

    public async Task RegenerateAsync(BranchNodeViewModel node)
    {
        if (!HasNodes || !node.CanRegenerate) return;
        ProbeNodeId = null;
        SyncNodeStates();
        Workspace.Ui.NotifyQuiet($"🔄 正在重新生成【{node.Tag}】的变体回复… ⚡");
        await Story.RegenerateAsync(node.Id).ConfigureAwait(true);
    }

    private void SyncNodeStates()
    {
        foreach (var n in Nodes)
        {
            n.IsProbeOpen = n.Id == ProbeNodeId;
            n.IsConfirming = n.Id == ConfirmNodeId;
        }
    }

    private void OnStoryChanged(object? sender, System.ComponentModel.PropertyChangedEventArgs e)
    {
        if (e.PropertyName is nameof(StoryStore.View) or nameof(StoryStore.ActiveBranchId) or nameof(StoryStore.Hud))
        {
            var dispatch = Avalonia.Threading.Dispatcher.UIThread;
            if (dispatch.CheckAccess()) Rebuild();
            else dispatch.Post(Rebuild);
        }
    }

    public void Rebuild()
    {
        var view = Story.View;
        UpdateLiveRelation();
        Nodes.Clear();
        if (view?.Nodes is { Count: > 0 } nodes)
        {
            var idx = 0;
            foreach (var n in nodes)
            {
                Nodes.Add(BranchNodeViewModel.FromNode(n, idx++, view, this));
            }
        }
        SyncNodeStates();
        OnPropertyChanged(nameof(HasNodes));
        OnPropertyChanged(nameof(IsEmpty));
        Outline.Rebuild();
    }

    private void UpdateLiveRelation()
    {
        var hud = Story.Hud;
        if (hud is null || hud.Relationships.Count == 0)
        {
            HasLiveRelation = false;
            return;
        }
        var key = Workspace.Ui.ActiveCharKey;
        RelationValue? rel = null;
        if (!string.IsNullOrEmpty(key))
        {
            if (hud.Relationships.TryGetValue("npc_" + key, out var a)) rel = a;
            else if (hud.Relationships.TryGetValue(key, out var b)) rel = b;
        }
        rel ??= hud.Relationships.Values.FirstOrDefault();
        if (rel is null)
        {
            HasLiveRelation = false;
            return;
        }
        HasLiveRelation = true;
        LiveAffection = rel.Affection;
        LiveTrust = rel.Trust;
        LiveAlertness = rel.Alertness;
    }

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        Story.PropertyChanged -= OnStoryChanged;
        Outline.Dispose();
    }
}

public sealed partial class BranchNodeViewModel : ObservableObject
{
    private readonly BranchTreeViewModel _tree;

    private BranchNodeViewModel(BranchTreeViewModel tree) => _tree = tree;

    public string Id { get; private init; } = "";
    public string Tag { get; private init; } = "";
    public string Desc { get; private init; } = "";
    public bool IsActive { get; private init; }
    public bool IsViewing { get; private init; }
    public bool IsFork { get; private init; }
    public bool CanRegenerate { get; private init; }

    [ObservableProperty] private bool _isProbeOpen;
    [ObservableProperty] private bool _isConfirming;

    public bool ShowMetrics => IsViewing && _tree.HasLiveRelation;
    public string AffText => FormatSigned(_tree.LiveAffection);
    public string TrustText => $"{_tree.LiveTrust:0}%";
    public string AlertText => $"{_tree.LiveAlertness:0}%";
    public string StatePillText => IsActive ? "活跃中 ⚡" : IsFork ? "已封存分支" : "快照";
    public bool IsActiveState => IsActive;
    public bool IsForkState => !IsActive && IsFork;

    public string ConfirmText => $"🌿 确定从【{Tag}】派生新平行因果分支？";

    [RelayCommand] private void ToggleProbe() => _tree.ToggleProbe(this);
    [RelayCommand] private void ViewAt() => _ = _tree.ViewAtAsync(this);
    [RelayCommand] private void RequestFork() => _tree.RequestFork(this);
    [RelayCommand] private void ConfirmFork() => _ = _tree.ConfirmForkAsync(this);
    [RelayCommand] private void CancelFork() => _tree.CancelFork();
    [RelayCommand] private void Regenerate() => _ = _tree.RegenerateAsync(this);

    private static string FormatSigned(double v) => v > 0 ? $"+{v:0}" : $"{v:0}";

    public static BranchNodeViewModel FromNode(PlotNode node, int index, SessionView view, BranchTreeViewModel tree)
    {
        string desc;
        if (node.Kind == "root")
        {
            var opening = StoryHelpers.ParseRoot(node);
            desc = string.IsNullOrWhiteSpace(opening) ? "命运起点 · 酒馆初见" : opening;
        }
        else
        {
            var tc = StoryHelpers.ParseTurnContent(node);
            if (!string.IsNullOrEmpty(tc?.InputText))
                desc = $"玩家: 「{Slice(tc!.InputText, 36)}」";
            else
            {
                var first = tc?.Blocks.FirstOrDefault()?.Text;
                desc = string.IsNullOrEmpty(first) ? "文学演义片段" : Slice(first!, 40);
            }
        }

        return new BranchNodeViewModel(tree)
        {
            Id = node.NodeId,
            Tag = node.Kind == "root" ? "命运起点" : $"回合 #{node.TurnNumber}",
            Desc = desc,
            IsActive = node.NodeId == view.Branch.HeadNodeId,
            IsViewing = node.NodeId == view.ViewNodeId,
            IsFork = false,
            CanRegenerate = node.Kind == "turn",
        };
    }

    private static string Slice(string text, int max)
    {
        var t = text.Replace("\n", " ").Trim();
        return t.Length <= max ? t : t[..max] + "…";
    }
}

/// <summary>Outline section (web: StoryOutlineSection).</summary>
public sealed partial class StoryOutlineViewModel : WorkspaceChildViewModel
{
    private bool _disposed;

    public StoryOutlineViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        Story.PropertyChanged += OnStoryChanged;
        Rebuild();
    }

    public StoryStore Story => Workspace.Story;

    [ObservableProperty] private string? _scene;
    public ObservableCollection<string> Milestones { get; } = new();
    public ObservableCollection<GoalLine> Goals { get; } = new();

    public bool HasScene => !string.IsNullOrWhiteSpace(Scene);
    public bool HasMilestones => Milestones.Count > 0;
    public bool HasGoals => Goals.Count > 0;
    public bool HasOutline => HasScene || HasMilestones || HasGoals;

    public void Rebuild()
    {
        var outline = Story.View?.Outline;
        Scene = outline?.Scene;
        Milestones.Clear();
        foreach (var m in outline?.Milestones ?? new List<OutlineEntry>())
            if (!string.IsNullOrWhiteSpace(m.Description)) Milestones.Add(m.Description);
        Goals.Clear();
        var chars = Story.View?.State?.Characters;
        foreach (var g in outline?.Goals ?? new List<GoalEntry>())
        {
            var name = chars is not null && chars.TryGetValue(g.CharacterId, out var ci) ? ci.Name : g.CharacterId;
            Goals.Add(new GoalLine(name, g.Text));
        }
        OnPropertyChanged(nameof(HasScene));
        OnPropertyChanged(nameof(HasMilestones));
        OnPropertyChanged(nameof(HasGoals));
        OnPropertyChanged(nameof(HasOutline));
    }

    private void OnStoryChanged(object? sender, System.ComponentModel.PropertyChangedEventArgs e)
    {
        if (e.PropertyName == nameof(StoryStore.View))
        {
            var dispatch = Avalonia.Threading.Dispatcher.UIThread;
            if (dispatch.CheckAccess()) Rebuild();
            else dispatch.Post(Rebuild);
        }
    }

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        Story.PropertyChanged -= OnStoryChanged;
    }
}

public sealed record GoalLine(string CharacterName, string Text);
