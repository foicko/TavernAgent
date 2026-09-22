using System.Collections.ObjectModel;
using System.ComponentModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Palette;

/// <summary>Command palette host: fuzzy-filter + keyboard-driven command runner (Ctrl+K).</summary>
public sealed partial class CommandPaletteViewModel : WorkspaceChildViewModel
{
    private List<CommandItem> _all = new();
    private bool _disposed;

    public CommandPaletteViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        Workspace.Ui.PropertyChanged += OnUiChanged;
    }

    public ObservableCollection<CommandItem> Results { get; } = new();

    [ObservableProperty] private string _query = "";
    [ObservableProperty] private CommandItem? _active;

    public bool IsOpen => Workspace.Ui.PaletteOpen;
    public bool HasResults => Results.Count > 0;
    public bool IsEmpty => Results.Count == 0;

    partial void OnQueryChanged(string value) => ApplyFilter();

    [RelayCommand]
    private void Close() => Workspace.Ui.SetPaletteOpen(false);

    [RelayCommand]
    private async Task Run(CommandItem? item)
    {
        if (item is null || !item.IsEnabled) return;
        Workspace.Ui.SetPaletteOpen(false);
        try { await item.Run().ConfigureAwait(true); }
        catch { /* surfaced by the underlying feature */ }
    }

    private void OnUiChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName != nameof(UiStore.PaletteOpen)) return;
        OnPropertyChanged(nameof(IsOpen));
        if (IsOpen) Open();
    }

    private void Open()
    {
        Query = "";
        _all = CommandRegistry.Build(Workspace);
        ApplyFilter();
    }

    public void MoveActive(int delta)
    {
        if (Results.Count == 0) return;
        var index = Active is null ? -1 : Results.IndexOf(Active);
        var next = index;
        for (var step = 0; step < Results.Count; step++)
        {
            next += delta;
            if (next < 0) next = Results.Count - 1;
            if (next >= Results.Count) next = 0;
            if (Results[next].IsEnabled) break;
        }
        SetActive(Results[next]);
    }

    public void MoveToEdge(bool last)
    {
        if (Results.Count == 0) return;
        for (var i = 0; i < Results.Count; i++)
        {
            var item = last ? Results[Results.Count - 1 - i] : Results[i];
            if (item.IsEnabled) { SetActive(item); return; }
        }
    }

    public async Task RunActiveAsync()
    {
        if (Active is not null) await Run(Active).ConfigureAwait(true);
    }

    private void ApplyFilter()
    {
        var query = Query.Trim();
        var scored = new List<(CommandItem Item, int Score)>();
        foreach (var item in _all)
        {
            var score = query.Length == 0 ? 0 : Score(item, query);
            if (query.Length == 0 || score > 0) scored.Add((item, score));
        }

        scored.Sort((a, b) =>
        {
            if (b.Score != a.Score) return b.Score.CompareTo(a.Score);
            var group = CommandGroups.Rank(a.Item.Group).CompareTo(CommandGroups.Rank(b.Item.Group));
            return group != 0 ? group : 0;
        });

        Results.Clear();
        foreach (var (item, _) in scored) Results.Add(item);
        OnPropertyChanged(nameof(HasResults));
        OnPropertyChanged(nameof(IsEmpty));

        SetActive(Results.FirstOrDefault(r => r.IsEnabled) ?? Results.FirstOrDefault());
    }

    private void SetActive(CommandItem? item)
    {
        foreach (var result in Results) result.IsActive = ReferenceEquals(result, item);
        Active = item;
    }

    /// <summary>Port of web scoreCommand: exact > prefix > substring > keyword > subsequence.</summary>
    private static int Score(CommandItem item, string query)
    {
        var title = item.Title;
        var q = query.ToLowerInvariant();
        var t = title.ToLowerInvariant();
        if (t == q) return 1000;
        if (t.StartsWith(q, StringComparison.Ordinal)) return 600 - Math.Min(599, title.Length);
        if (t.Contains(q, StringComparison.Ordinal)) return 400 - Math.Min(399, title.Length);

        var keywords = item.Keywords.ToLowerInvariant();
        if (keywords.Length > 0)
        {
            if (keywords.Split(' ').Any(k => k == q)) return 350;
            if (keywords.StartsWith(q, StringComparison.Ordinal)) return 300;
            if (keywords.Contains(q, StringComparison.Ordinal)) return 200;
        }

        return Subsequence(t, q) ? 60 : 0;
    }

    private static bool Subsequence(string text, string query)
    {
        var qi = 0;
        foreach (var ch in text)
        {
            if (qi < query.Length && ch == query[qi]) qi++;
            if (qi == query.Length) return true;
        }
        return qi == query.Length;
    }

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        Workspace.Ui.PropertyChanged -= OnUiChanged;
    }
}
