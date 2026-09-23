using System.Collections.ObjectModel;
using System.ComponentModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Modals;

public sealed record LoreExample(string Title, string Keys, string Content);

/// <summary>Built-in worldbook examples viewer (web: WorldbookModal — read-only samples).</summary>
public sealed partial class WorldbookModalViewModel : WorkspaceChildViewModel
{
    private bool _disposed;

    public WorldbookModalViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        Workspace.Ui.PropertyChanged += OnUiChanged;
    }

    // Web ships an empty example list; the real, per-session lorebook lives in the left rail.
    private static readonly IReadOnlyList<LoreExample> All = Array.Empty<LoreExample>();

    public ObservableCollection<LoreExample> Entries { get; } = new();

    [ObservableProperty] private string _search = "";

    public bool IsOpen => Workspace.Ui.WorldbookModalOpen;
    public bool IsEmpty => Entries.Count == 0;

    partial void OnSearchChanged(string value) => Rebuild();

    [RelayCommand]
    private void Close() => Workspace.Ui.SetWorldbookModalOpen(false);

    private void OnUiChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName != nameof(UiStore.WorldbookModalOpen)) return;
        var dispatch = Avalonia.Threading.Dispatcher.UIThread;
        void Apply()
        {
            OnPropertyChanged(nameof(IsOpen));
            if (IsOpen) Rebuild();
        }
        if (dispatch.CheckAccess()) Apply();
        else dispatch.Post(Apply);
    }

    private void Rebuild()
    {
        var query = Search.Trim();
        Entries.Clear();
        foreach (var entry in All)
        {
            if (query.Length == 0 ||
                entry.Title.Contains(query, StringComparison.OrdinalIgnoreCase) ||
                entry.Keys.Contains(query, StringComparison.OrdinalIgnoreCase) ||
                entry.Content.Contains(query, StringComparison.OrdinalIgnoreCase))
            {
                Entries.Add(entry);
            }
        }
        OnPropertyChanged(nameof(IsEmpty));
    }

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        Workspace.Ui.PropertyChanged -= OnUiChanged;
    }
}
