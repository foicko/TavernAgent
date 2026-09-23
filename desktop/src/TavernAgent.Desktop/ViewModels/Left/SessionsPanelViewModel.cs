using System.Collections.ObjectModel;
using Avalonia;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.Services;

namespace TavernAgent.Desktop.ViewModels.Left;

public enum SessionFilter { All, Current }

/// <summary>Session list + pack export/import (web: LeftRail sessions panel).</summary>
public sealed partial class SessionsPanelViewModel : WorkspaceChildViewModel
{
    private bool _disposed;

    public SessionsPanelViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        Story.PropertyChanged += OnStoryChanged;
    }

    public StoryStore Story => Workspace.Story;

    public ObservableCollection<SessionRowViewModel> Rows { get; } = new();

    [ObservableProperty] private SessionFilter _filter = SessionFilter.All;

    public int TotalCount => Story.Sessions.Count;
    public string FilterCurrentLabel => $"仅 {Workspace.Stage.CharShortName}";
    public bool IsAllFilter => Filter == SessionFilter.All;
    public bool IsCurrentFilter => Filter == SessionFilter.Current;
    public bool IsEmpty => Rows.Count == 0;
    public bool HasCurrentView => Story.View is not null;
    public string NewStoryLabel => $"与「{Workspace.Stage.CharShortName}」开启新故事";
    public string NewStoryTooltip => $"基于【{Workspace.Stage.CharName}】开辟全新平行故事分支";

    [RelayCommand]
    private void SetAllFilter() { Filter = SessionFilter.All; Rebuild(); }

    [RelayCommand]
    private void SetCurrentFilter() { Filter = SessionFilter.Current; Rebuild(); }

    [RelayCommand]
    private async Task NewStory() => await Workspace.Stage.StartNeutralSessionCommand.ExecuteAsync(null).ConfigureAwait(true);

    [RelayCommand]
    private void GoCharacters() => Workspace.Ui.OpenLeftTabAndExpand(LeftSubtab.Characters);

    [RelayCommand]
    private void GoImport() => Workspace.Ui.SetCharImportOpen(true);

    [RelayCommand]
    private void Refresh() => Rebuild();

    public async Task OpenAsync(SessionRowViewModel row)
        => await Story.OpenSessionAsync(row.SessionId).ConfigureAwait(true);

    public async Task DeleteAsync(SessionRowViewModel row)
    {
        try
        {
            await Story.DeleteSessionAsync(row.SessionId).ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            Workspace.Ui.NotifyQuiet(string.IsNullOrEmpty(ex.Message) ? "删除失败，请稍后重试" : ex.Message);
        }
    }

    public async Task ExportAsync(Visual owner)
    {
        var view = Story.View;
        if (view is null) { Workspace.Ui.NotifyQuiet("请先打开一个会话再导出"); return; }
        try
        {
            Workspace.Ui.NotifyQuiet("正在打包剧情… ⏳");
            var bytes = await Story.Api.ExportSessionPackAsync(view.SessionId).ConfigureAwait(true);
            await FilePickers.SavePackAsync(owner, Sanitize(view.Title) + ".tavernpack", bytes).ConfigureAwait(true);
            Workspace.Ui.NotifyQuiet("剧情包已导出");
        }
        catch (Exception ex)
        {
            Workspace.Ui.NotifyQuiet("导出失败：" + ex.Message);
        }
    }

    public async Task ImportAsync(Visual owner)
    {
        try
        {
            var bytes = await FilePickers.PickPackAsync(owner).ConfigureAwait(true);
            if (bytes is null) return;
            Workspace.Ui.NotifyQuiet("正在导入剧情包… ⏳");
            var result = await Story.Api.ImportSessionPackAsync(bytes).ConfigureAwait(true);
            await Story.LoadSessionsAsync().ConfigureAwait(true);
            if (!string.IsNullOrEmpty(result.SessionId)) await Story.OpenSessionAsync(result.SessionId).ConfigureAwait(true);
            Workspace.Ui.NotifyQuiet($"已导入「{result.Title}」");
        }
        catch (Exception ex)
        {
            Workspace.Ui.NotifyQuiet("导入失败：" + ex.Message);
        }
    }

    private static string Sanitize(string title)
    {
        var t = string.IsNullOrWhiteSpace(title) ? "tavernagent-story" : title.Trim();
        foreach (var c in Path.GetInvalidFileNameChars()) t = t.Replace(c, '_');
        return t;
    }

    private void OnStoryChanged(object? sender, System.ComponentModel.PropertyChangedEventArgs e)
    {
        if (e.PropertyName is nameof(StoryStore.Sessions) or nameof(StoryStore.View) or nameof(StoryStore.PendingSessionId))
        {
            var dispatch = Avalonia.Threading.Dispatcher.UIThread;
            if (dispatch.CheckAccess()) Rebuild();
            else dispatch.Post(Rebuild);
        }
    }

    public void Rebuild()
    {
        Rows.Clear();
        var current = Story.View;
        IEnumerable<SessionSummary> list = Story.Sessions;
        if (Filter == SessionFilter.Current)
        {
            list = current is not null
                ? list.Where(s => s.CharacterId == current.CharacterId)
                : list.Where(s => StoryHelpers.ResolveCharKeyFromSession(null) == Workspace.Ui.ActiveCharKey);
        }
        foreach (var s in list) Rows.Add(new SessionRowViewModel(this, s));

        OnPropertyChanged(nameof(TotalCount));
        OnPropertyChanged(nameof(FilterCurrentLabel));
        OnPropertyChanged(nameof(IsAllFilter));
        OnPropertyChanged(nameof(IsCurrentFilter));
        OnPropertyChanged(nameof(IsEmpty));
        OnPropertyChanged(nameof(HasCurrentView));
        OnPropertyChanged(nameof(NewStoryLabel));
        OnPropertyChanged(nameof(NewStoryTooltip));
    }

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        Story.PropertyChanged -= OnStoryChanged;
    }
}

public sealed partial class SessionRowViewModel : ObservableObject
{
    private readonly SessionsPanelViewModel _panel;

    public SessionRowViewModel(SessionsPanelViewModel panel, SessionSummary summary)
    {
        _panel = panel;
        SessionId = summary.SessionId;
        Title = string.IsNullOrWhiteSpace(summary.Title) ? "未命名剧本" : summary.Title;
        CharacterId = summary.CharacterId;
        TimeText = FormatSessionTime(summary.CreatedAt);
        IsActive = panel.Story.View?.SessionId == summary.SessionId;
        IsLoading = panel.Story.PendingSessionId == summary.SessionId && panel.Story.SessionLoading;
        StatusText = IsLoading ? "载入中" : IsActive ? "当前" : "存档";
        RoleTag = "存档";
        LoadAvatarAsync(summary).SafeFireAndForget();
    }

    public string SessionId { get; }
    public string Title { get; }
    public string CharacterId { get; }
    public string TimeText { get; }
    public string RoleTag { get; }

    [ObservableProperty] private Avalonia.Media.Imaging.Bitmap? _avatar;
    [ObservableProperty] private bool _isConfirmingDelete;
    [ObservableProperty] private bool _isActive;
    [ObservableProperty] private bool _isLoading;
    [ObservableProperty] private string _statusText = "存档";

    public bool HasAvatar => Avatar is not null;
    public string Monogram => string.IsNullOrEmpty(Title) ? "📜" : Title[..1];

    private async Task LoadAvatarAsync(SessionSummary summary)
    {
        var view = _panel.Story.View;
        string? source = null;
        if (view?.SessionId == summary.SessionId && view.State?.Characters is { Count: > 0 } chars)
        {
            source = chars.Values.FirstOrDefault(c => c.CharacterId != "player")?.Avatar;
        }
        if (string.IsNullOrEmpty(source)) return;
        var bmp = await ImageLoader.LoadAsync(source, 68).ConfigureAwait(true);
        Avatar = bmp;
        OnPropertyChanged(nameof(HasAvatar));
    }

    [RelayCommand] private async Task Open() => await _panel.OpenAsync(this).ConfigureAwait(true);
    [RelayCommand] private void RequestDelete() => IsConfirmingDelete = true;
    [RelayCommand] private void CancelDelete() => IsConfirmingDelete = false;
    [RelayCommand] private async Task ConfirmDelete() => await _panel.DeleteAsync(this).ConfigureAwait(true);

    public static string FormatSessionTime(string? iso)
    {
        if (string.IsNullOrWhiteSpace(iso)) return "最近";
        if (!DateTimeOffset.TryParse(iso, out var when)) return "最近";
        var sec = (DateTimeOffset.UtcNow - when.ToUniversalTime()).TotalSeconds;
        if (sec < 0) sec = 0;
        if (sec < 60) return "刚刚";
        if (sec < 3600) return $"{(int)(sec / 60)} 分钟前";
        if (sec < 86400) return $"{(int)(sec / 3600)} 小时前";
        if (sec < 7 * 86400) return $"{(int)(sec / 86400)} 天前";
        var local = when.ToLocalTime();
        return $"{local.Month + 1}月{local.Day}日";
    }
}
