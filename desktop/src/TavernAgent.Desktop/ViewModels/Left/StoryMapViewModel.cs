using System.Collections.ObjectModel;
using System.Text;
using System.Text.Json;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Left;

/// <summary>Branch graph canvas + node detail (web: StoryMapModal).</summary>
public sealed partial class StoryMapViewModel : WorkspaceChildViewModel
{
    private bool _disposed;

    public StoryMapViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
    }

    public StoryStore Story => Workspace.Story;

    public ObservableCollection<StoryMapNodeViewModel> Nodes { get; } = new();
    public ObservableCollection<StoryMapLinkViewModel> Links { get; } = new();
    public ObservableCollection<BranchView> Branches { get; } = new();

    [ObservableProperty] private bool _isOpen;
    [ObservableProperty] private bool _isLoading;
    [ObservableProperty] private string? _error;
    [ObservableProperty] private bool _truncated;
    [ObservableProperty] private double _canvasWidth = 600;
    [ObservableProperty] private double _canvasHeight = 320;
    [ObservableProperty] private string? _targetNodeId;
    [ObservableProperty] private string? _selectedNodeId;
    [ObservableProperty] private string _statusText = "";

    [ObservableProperty] private string _detailLabel = "加载节点…";
    [ObservableProperty] private string _detailExcerpt = "";
    [ObservableProperty] private bool _detailIsTurn;
    [ObservableProperty] private string? _detailBranchName;
    [ObservableProperty] private string? _detailError;
    [ObservableProperty] private BranchView? _selectedBranch;
    [ObservableProperty] private bool _busy;

    public void Open()
    {
        IsOpen = true;
        var view = Story.View;
        if (view is null) { Error = "请先打开一个会话"; return; }
        TargetNodeId = view.ViewNodeId ?? view.Branch.HeadNodeId;
        SelectedNodeId = TargetNodeId;
        Branches.Clear();
        foreach (var b in view.Branches ?? new List<BranchView> { view.Branch }) Branches.Add(b);
        _ = ReloadAsync();
    }

    [RelayCommand] private void Close() => IsOpen = false;

    [RelayCommand]
    private async Task Retry() => await ReloadAsync().ConfigureAwait(true);

    [RelayCommand]
    private void SelectNode(StoryMapNodeViewModel node)
    {
        SelectedNodeId = node.NodeId;
        UpdateSelection();
        _ = LoadDetailAsync();
    }

    [RelayCommand]
    private async Task ExpandNearby()
    {
        if (SelectedNodeId is null) return;
        TargetNodeId = SelectedNodeId;
        await ReloadAsync().ConfigureAwait(true);
    }

    [RelayCommand]
    private async Task GoHead()
    {
        var view = Story.View;
        if (view is null) return;
        TargetNodeId = view.Branch.HeadNodeId;
        SelectedNodeId = TargetNodeId;
        await ReloadAsync().ConfigureAwait(true);
    }

    [RelayCommand]
    private async Task LocateBranch(BranchView? branch)
    {
        if (branch is null) return;
        TargetNodeId = branch.HeadNodeId;
        SelectedNodeId = branch.HeadNodeId;
        await ReloadAsync().ConfigureAwait(true);
    }

    [RelayCommand]
    private async Task ViewSnapshot()
    {
        if (SelectedNodeId is null) return;
        await PerformAsync(() => Story.ViewAtAsync(SelectedNodeId)).ConfigureAwait(true);
    }

    [RelayCommand]
    private async Task Fork()
    {
        if (SelectedNodeId is null) return;
        await PerformAsync(() => Story.ForkFromAsync(SelectedNodeId)).ConfigureAwait(true);
    }

    [RelayCommand]
    private async Task SwitchBranch()
    {
        if (SelectedBranch is null) return;
        await PerformAsync(() => Story.SwitchBranchAsync(SelectedBranch.BranchId)).ConfigureAwait(true);
    }

    [RelayCommand]
    private async Task Regenerate()
    {
        if (SelectedNodeId is null) return;
        await PerformAsync(() => Story.RegenerateAsync(SelectedNodeId)).ConfigureAwait(true);
    }

    private async Task PerformAsync(Func<Task> action)
    {
        var sessionId = Story.View?.SessionId;
        Busy = true;
        DetailError = null;
        try
        {
            await action().ConfigureAwait(true);
            if (Story.View?.SessionId != sessionId) return;
            if (!string.IsNullOrEmpty(Story.Error)) DetailError = Story.Error;
            else IsOpen = false;
        }
        catch (Exception ex)
        {
            DetailError = ex.Message;
        }
        finally
        {
            Busy = false;
        }
    }

    private async Task ReloadAsync()
    {
        var view = Story.View;
        if (view is null) { Error = "请先打开一个会话"; return; }
        IsLoading = true;
        Error = null;
        try
        {
            var graph = await Story.Api.GetGraphAsync(view.SessionId, TargetNodeId, up: 6, down: 2).ConfigureAwait(true);
            Layout(graph);
            Truncated = graph.Truncated == true;
            StatusText = $"已加载 {Nodes.Count} 个节点";
        }
        catch (Exception ex)
        {
            Error = ex.Message;
        }
        finally
        {
            IsLoading = false;
        }
        await LoadDetailAsync().ConfigureAwait(true);
    }

    private void Layout(GraphView graph)
    {
        Nodes.Clear();
        Links.Clear();

        var ordered = graph.Nodes
            .OrderBy(n => n.Depth)
            .ThenBy(n => n.ParentId ?? "")
            .ThenBy(n => n.NodeId)
            .ToList();
        if (ordered.Count == 0) { CanvasWidth = 600; CanvasHeight = 320; return; }

        var firstDepth = ordered[0].Depth;
        var rows = new Dictionary<int, int>();
        var positions = new Dictionary<string, (double X, double Y)>();
        var branchNames = (Story.View?.Branches ?? new List<BranchView>())
            .GroupBy(b => b.HeadNodeId).ToDictionary(g => g.Key, g => g.First().Name);
        var head = Story.View?.Branch.HeadNodeId;

        var layout = new List<(GraphNode Node, double X, double Y)>();
        foreach (var node in ordered)
        {
            rows.TryGetValue(node.Depth, out var row);
            rows[node.Depth] = row + 1;
            var x = 24 + (node.Depth - firstDepth) * 196;
            var y = 24 + row * 88;
            positions[node.NodeId] = (x, y);
            layout.Add((node, x, y));
        }

        double maxX = 0, maxY = 0;
        foreach (var (node, x, y) in layout)
        {
            maxX = Math.Max(maxX, x + 180);
            maxY = Math.Max(maxY, y + 84);
            var name = branchNames.TryGetValue(node.NodeId, out var bn) ? bn : null;
            Nodes.Add(new StoryMapNodeViewModel
            {
                NodeId = node.NodeId,
                X = x,
                Y = y,
                Label = NodeLabel(node.Kind, node.TurnNumber) + (name is null ? "" : $" · {name}"),
                Subtitle = $"{(node.NodeId == head ? "当前分支头 · " : "")}{node.ChildCount ?? 0} 个子节点{(node.IsCandidate == true ? " · 候选" : "")}",
                IsHead = node.NodeId == head,
                IsSelected = node.NodeId == SelectedNodeId,
            });
        }

        foreach (var (node, x, y) in layout)
        {
            if (string.IsNullOrEmpty(node.ParentId) || !positions.TryGetValue(node.ParentId, out var p)) continue;
            Links.Add(new StoryMapLinkViewModel
            {
                Path = Avalonia.Media.Geometry.Parse(
                    $"M {p.X + 160},{p.Y + 30} C {p.X + 178},{p.Y + 30} {x - 18},{y + 30} {x},{y + 30}"),
            });
        }

        CanvasWidth = Math.Max(600, maxX);
        CanvasHeight = Math.Max(320, maxY);
    }

    private async Task LoadDetailAsync()
    {
        if (SelectedNodeId is null) return;
        DetailError = null;
        try
        {
            var result = await Story.Api.GetNodeAsync(SelectedNodeId).ConfigureAwait(true);
            var node = result.Node;
            DetailLabel = NodeLabel(node.Kind, node.TurnNumber);
            DetailExcerpt = ParseExcerpt(node);
            DetailIsTurn = node.Kind == "turn";
            SelectedBranch = Story.View?.Branches?.FirstOrDefault(b => b.HeadNodeId == SelectedNodeId);
            DetailBranchName = SelectedBranch?.Name;
            UpdateSelection();
        }
        catch (Exception ex)
        {
            DetailError = ex.Message;
        }
    }

    private void UpdateSelection()
    {
        foreach (var n in Nodes) n.IsSelected = n.NodeId == SelectedNodeId;
    }

    private static string NodeLabel(string kind, int? turn) => kind switch
    {
        "root" => "故事起点",
        "memory_change" => "记忆整理 / 修订",
        "director_event" => "导演安排 / 进度调整",
        _ => $"回合 {turn?.ToString() ?? "—"}",
    };

    private static string ParseExcerpt(PlotNode node)
    {
        try
        {
            using var doc = JsonDocument.Parse(node.ContentJson);
            var root = doc.RootElement;
            if (root.TryGetProperty("openingText", out var op) && op.ValueKind == JsonValueKind.String)
                return op.GetString() ?? "";
            var sb = new StringBuilder();
            if (root.TryGetProperty("inputText", out var inp) && inp.ValueKind == JsonValueKind.String)
                sb.Append(inp.GetString());
            if (root.TryGetProperty("blocks", out var blocks) && blocks.ValueKind == JsonValueKind.Array)
            {
                foreach (var b in blocks.EnumerateArray())
                {
                    if (b.TryGetProperty("text", out var t) && t.ValueKind == JsonValueKind.String)
                    {
                        if (sb.Length > 0) sb.Append("\n\n");
                        sb.Append(t.GetString());
                    }
                }
            }
            return sb.Length > 0 ? sb.ToString() : "记忆与状态维护节点";
        }
        catch (JsonException)
        {
            return "此节点没有可展示的正文。";
        }
    }

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
    }
}

public sealed partial class StoryMapNodeViewModel : ObservableObject
{
    public string NodeId { get; init; } = "";
    public double X { get; init; }
    public double Y { get; init; }
    public string Label { get; init; } = "";
    public string Subtitle { get; init; } = "";
    public bool IsHead { get; init; }

    [ObservableProperty] private bool _isSelected;
}

public sealed class StoryMapLinkViewModel
{
    public Avalonia.Media.Geometry Path { get; init; } = Avalonia.Media.Geometry.Parse("M 0,0");
}
