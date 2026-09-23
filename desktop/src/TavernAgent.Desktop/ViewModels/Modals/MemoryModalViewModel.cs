using System.Collections.ObjectModel;
using System.ComponentModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Modals;

/// <summary>Cognitive memory bank modal (web: MemoryBankModal).</summary>
public sealed partial class MemoryModalViewModel : WorkspaceChildViewModel
{
    private bool _disposed;

    public MemoryModalViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        Workspace.Ui.PropertyChanged += OnUiChanged;
        Story.PropertyChanged += OnStoryChanged;
    }

    public StoryStore Story => Workspace.Story;

    public ObservableCollection<MemoryRowViewModel> Rows { get; } = new();

    [ObservableProperty] private string _search = "";
    [ObservableProperty] private string _kind = "all";
    [ObservableProperty] private bool _isBusy;

    public bool IsOpen => Workspace.Ui.MemoryModalOpen;
    public bool IsEmpty => !IsBusy && Rows.Count == 0;
    public bool IsAll => Kind == "all";
    public bool IsObserved => Kind == "observed";
    public bool IsInferred => Kind == "inferred";
    public bool IsSecret => Kind == "secret";
    public bool HasPrev => Story.MemoryPaging.Index > 0;
    public bool HasNext => !string.IsNullOrEmpty(Story.MemoryPaging.NextCursor);
    public int Total => Story.MemoryPaging.Total;
    public string UsageText
    {
        get
        {
            var u = Story.MemoryUsage;
            return u is null ? "" : $"{u.Used} / {u.Limit}（受保护 {u.Protected}）";
        }
    }

    [RelayCommand] private void Close() => Workspace.Ui.SetMemoryModalOpen(false);

    [RelayCommand] private async Task SearchNow() => await LoadAsync(page: null).ConfigureAwait(true);
    [RelayCommand] private async Task SetAll() => await SetKindAsync("all").ConfigureAwait(true);
    [RelayCommand] private async Task SetObserved() => await SetKindAsync("observed").ConfigureAwait(true);
    [RelayCommand] private async Task SetInferred() => await LoadKind("inferred").ConfigureAwait(true);
    [RelayCommand] private async Task SetSecret() => await LoadKind("secret").ConfigureAwait(true);
    [RelayCommand] private async Task NextPage() => await LoadAsync("next").ConfigureAwait(true);
    [RelayCommand] private async Task PrevPage() => await LoadAsync("previous").ConfigureAwait(true);

    [RelayCommand]
    private async Task Organize()
    {
        try
        {
            await Story.OrganizeMemoriesAsync().ConfigureAwait(true);
            Workspace.Ui.NotifyQuiet("已整理记忆库");
            await LoadAsync(null).ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            Workspace.Ui.NotifyQuiet("整理失败：" + ex.Message);
        }
    }

    public async Task TogglePinAsync(MemoryRowViewModel row)
    {
        try
        {
            await Story.ReviseMemoryAsync(row.Id, null, pinned: !row.Pinned, hidden: null).ConfigureAwait(true);
        }
        catch (Exception ex) { Workspace.Ui.NotifyQuiet(ex.Message); }
    }

    public async Task ToggleHideAsync(MemoryRowViewModel row)
    {
        try
        {
            await Story.ReviseMemoryAsync(row.Id, null, pinned: null, hidden: !row.Hidden).ConfigureAwait(true);
        }
        catch (Exception ex) { Workspace.Ui.NotifyQuiet(ex.Message); }
    }

    private async Task SetKindAsync(string kind)
    {
        Kind = kind;
        RaiseFilters();
        await LoadAsync(null).ConfigureAwait(true);
    }

    private Task LoadKind(string kind) => SetKindAsync(kind);

    public async Task LoadAsync(string? page)
    {
        if (Story.View is null) return;
        IsBusy = true;
        try
        {
            await Story.LoadMemoriesAsync(
                string.IsNullOrWhiteSpace(Search) ? null : Search,
                Kind == "all" ? null : Kind,
                page).ConfigureAwait(true);
            var dispatch = Avalonia.Threading.Dispatcher.UIThread;
            void Populate()
            {
                Rows.Clear();
                foreach (var m in Story.Memories) Rows.Add(new MemoryRowViewModel(this, m));
            }
            if (dispatch.CheckAccess()) Populate();
            else dispatch.Post(Populate);
        }
        catch (Exception ex)
        {
            Workspace.Ui.NotifyQuiet(ex.Message);
        }
        finally
        {
            IsBusy = false;
            RaiseFilters();
        }
    }

    private void RaiseFilters()
    {
        OnPropertyChanged(nameof(IsEmpty));
        OnPropertyChanged(nameof(IsAll));
        OnPropertyChanged(nameof(IsObserved));
        OnPropertyChanged(nameof(IsInferred));
        OnPropertyChanged(nameof(IsSecret));
        OnPropertyChanged(nameof(HasPrev));
        OnPropertyChanged(nameof(HasNext));
        OnPropertyChanged(nameof(Total));
        OnPropertyChanged(nameof(UsageText));
    }

    private void OnUiChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName == nameof(UiStore.MemoryModalOpen))
        {
            var dispatch = Avalonia.Threading.Dispatcher.UIThread;
            void Apply()
            {
                OnPropertyChanged(nameof(IsOpen));
                if (IsOpen) LoadAsync(null).SafeFireAndForget();
            }
            if (dispatch.CheckAccess()) Apply();
            else dispatch.Post(Apply);
        }
    }

    private void OnStoryChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName is nameof(StoryStore.MemoryPaging) or nameof(StoryStore.MemoryUsage))
        {
            var dispatch = Avalonia.Threading.Dispatcher.UIThread;
            if (dispatch.CheckAccess()) RaiseFilters();
            else dispatch.Post(RaiseFilters);
        }
    }

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        Workspace.Ui.PropertyChanged -= OnUiChanged;
        Story.PropertyChanged -= OnStoryChanged;
    }
}

public sealed partial class MemoryRowViewModel : ObservableObject
{
    private readonly MemoryModalViewModel _modal;

    public MemoryRowViewModel(MemoryModalViewModel modal, MemoryView m)
    {
        _modal = modal;
        Id = m.MemoryId;
        Content = m.Content;
        Pinned = m.Pinned;
        Hidden = m.Hidden;
        Effective = m.Effective;
        Confidence = m.Confidence;
        Supersedes = m.Supersedes;
        SourceQuote = m.Evidence?.SourceQuote;
        Reasoning = m.Evidence?.Reasoning;
        KindLabel = m.Kind switch
        {
            "observed" => "见证",
            "reported" => "转述",
            "inferred" => "推测",
            "secret" => "秘密",
            _ => m.Kind,
        };
        CreatedTurn = m.CreatedTurn;
    }

    public string Id { get; }
    public string Content { get; }
    public bool Pinned { get; }
    public bool Hidden { get; }
    public bool Effective { get; }
    public double? Confidence { get; }
    public string? Supersedes { get; }
    public string? SourceQuote { get; }
    public string? Reasoning { get; }
    public string KindLabel { get; }
    public int? CreatedTurn { get; }

    public bool HasEvidence => !string.IsNullOrEmpty(SourceQuote) || !string.IsNullOrEmpty(Reasoning);
    public string ConfidenceText => Confidence is null ? "" : $"置信度 {Confidence:0.00}";
    public string PinLabel => Pinned ? "📌 已固定" : "固定";
    public string HideLabel => Hidden ? "👁 已隐藏" : "隐藏";

    [ObservableProperty] private bool _isExpanded;

    [RelayCommand] private void ToggleExpand() => IsExpanded = !IsExpanded;
    [RelayCommand] private async Task TogglePin() => await _modal.TogglePinAsync(this).ConfigureAwait(true);
    [RelayCommand] private async Task ToggleHide() => await _modal.ToggleHideAsync(this).ConfigureAwait(true);
}
