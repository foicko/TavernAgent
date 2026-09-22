using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Left;

/// <summary>Session lorebook search + revealed secrets (web: LorebookPanel).</summary>
public sealed partial class LorePanelViewModel : WorkspaceChildViewModel
{
    private bool _disposed;
    private int _nextOffset;

    public LorePanelViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        Story.PropertyChanged += OnStoryChanged;
    }

    public StoryStore Story => Workspace.Story;

    public ObservableCollection<LorebookMatch> Entries { get; } = new();
    public ObservableCollection<SecretView> Secrets { get; } = new();

    [ObservableProperty] private string _query = "";
    [ObservableProperty] private bool _isLoading;
    [ObservableProperty] private string? _error;
    [ObservableProperty] private bool _hasMore;

    public bool HasSession => Story.View is not null;
    public bool IsEmpty => !IsLoading && Entries.Count == 0;
    public bool HasSecrets => Secrets.Count > 0;

    partial void OnQueryChanged(string value) => _ = SearchAsync();

    [RelayCommand]
    private async Task Search() => await SearchAsync().ConfigureAwait(true);

    [RelayCommand]
    private async Task LoadMore() => await LoadAsync(reset: false).ConfigureAwait(true);

    public async Task SearchAsync() => await LoadAsync(reset: true).ConfigureAwait(true);

    private async Task LoadAsync(bool reset)
    {
        var view = Story.View;
        if (view is null)
        {
            Entries.Clear();
            Secrets.Clear();
            OnPropertyChanged(nameof(IsEmpty));
            OnPropertyChanged(nameof(HasSecrets));
            return;
        }

        if (reset) _nextOffset = 0;
        IsLoading = true;
        Error = null;
        try
        {
            var page = await Story.Api.SearchLorebookAsync(view.SessionId, Query ?? "", _nextOffset).ConfigureAwait(true);
            if (reset) Entries.Clear();
            foreach (var e in page.Entries) Entries.Add(e);
            _nextOffset = page.NextOffset;
            HasMore = page.HasMore;

            Secrets.Clear();
            foreach (var s in view.Secrets ?? new List<SecretView>()) Secrets.Add(s);
        }
        catch (Exception ex)
        {
            Error = ex.Message;
        }
        finally
        {
            IsLoading = false;
            OnPropertyChanged(nameof(IsEmpty));
            OnPropertyChanged(nameof(HasSecrets));
        }
    }

    private void OnStoryChanged(object? sender, System.ComponentModel.PropertyChangedEventArgs e)
    {
        if (e.PropertyName != nameof(StoryStore.View)) return;
        OnPropertyChanged(nameof(HasSession));
        _ = SearchAsync();
    }

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        Story.PropertyChanged -= OnStoryChanged;
    }
}
