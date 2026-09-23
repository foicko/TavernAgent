using System.Collections.ObjectModel;
using System.ComponentModel;
using Avalonia.Threading;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Stage;

/// <summary>Editable beat row in the director plan.</summary>
public sealed partial class DirectorBeatViewModel : ObservableObject
{
    private readonly Action _onChanged;

    public DirectorBeatViewModel(DirectorBeat beat, Action onChanged)
    {
        BeatId = string.IsNullOrEmpty(beat.BeatId) ? Guid.NewGuid().ToString() : beat.BeatId;
        _title = beat.Title;
        _instruction = beat.Instruction;
        _completionCriteria = beat.CompletionCriteria;
        _onChanged = onChanged;
    }

    public string BeatId { get; }

    [ObservableProperty] private int _number;
    [ObservableProperty] private string _title = "";
    [ObservableProperty] private string _instruction = "";
    [ObservableProperty] private string _completionCriteria = "";
    [ObservableProperty] private string _status = "pending";
    [ObservableProperty] private bool _locked;

    public bool IsCurrent => Status == "current";
    public bool IsSettled => Status is "completed" or "skipped";
    public bool IsPending => Status == "pending";
    public string StatusLabel => Status switch
    {
        "current" => "当前阶段",
        "completed" => "已结束",
        "skipped" => "已跳过",
        _ => "待推进",
    };

    public bool Valid => !string.IsNullOrWhiteSpace(Title)
        && !string.IsNullOrWhiteSpace(Instruction)
        && !string.IsNullOrWhiteSpace(CompletionCriteria);

    partial void OnTitleChanged(string value) { _onChanged(); OnPropertyChanged(nameof(Valid)); }
    partial void OnInstructionChanged(string value) { _onChanged(); OnPropertyChanged(nameof(Valid)); }
    partial void OnCompletionCriteriaChanged(string value) { _onChanged(); OnPropertyChanged(nameof(Valid)); }
    partial void OnStatusChanged(string value)
    {
        OnPropertyChanged(nameof(IsCurrent));
        OnPropertyChanged(nameof(IsSettled));
        OnPropertyChanged(nameof(IsPending));
        OnPropertyChanged(nameof(StatusLabel));
    }

    public DirectorBeat ToBeat() => new()
    {
        BeatId = BeatId,
        Title = Title.Trim(),
        Instruction = Instruction.Trim(),
        CompletionCriteria = CompletionCriteria.Trim(),
    };
}

/// <summary>One discussion exchange (user ask + director reply / candidate).</summary>
public sealed class DirectorExchangeViewModel
{
    public DirectorExchangeViewModel(DirectorRequest request)
    {
        RequestId = request.RequestId;
        Text = request.Text;
        Reply = request.Reply ?? "";
        Status = request.Status;
        Error = request.Error;
        Candidate = request.Candidate;
    }

    public string RequestId { get; }
    public string Text { get; }
    public string Reply { get; }
    public string Status { get; }
    public string? Error { get; }
    public DirectorPlan? Candidate { get; }

    public bool IsGenerating => Status == "generating";
    public bool HasReply => !string.IsNullOrEmpty(Reply);
    public bool HasError => !string.IsNullOrEmpty(Error);
    public bool HasCandidate => Candidate is not null;
    public bool CanRetry => Status is "failed" or "cancelled" or "interrupted";
    public string StatusLabel => Status switch
    {
        "generating" => "思考中…",
        "failed" => "失败",
        "cancelled" => "已取消",
        "interrupted" => "已中断",
        _ => "已回复",
    };
}

/// <summary>Director plan editor + discussion workspace (web: DirectorWorkspace + directorSlice).</summary>
public sealed partial class DirectorViewModel : WorkspaceChildViewModel, IDisposable
{
    private readonly Dictionary<string, DirectorPlan> _backups = new();
    private System.Threading.Timer? _debounce;
    private bool _suppress;
    private bool _dirty;
    private bool _saving;
    private string _loadedScope = "";
    private bool _disposed;

    public DirectorViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        Workspace.Ui.PropertyChanged += OnUiChanged;
        Story.PropertyChanged += OnStoryChanged;
        try { _backups = SettingsPersistence.Load("director_draft_backups.json", new Dictionary<string, DirectorPlan>()) ?? new(); }
        catch { _backups = new Dictionary<string, DirectorPlan>(); }
    }

    public StoryStore Story => Workspace.Story;

    public ObservableCollection<DirectorBeatViewModel> Beats { get; } = new();
    public ObservableCollection<DirectorExchangeViewModel> Exchanges { get; } = new();

    [ObservableProperty] private string _title = "";
    [ObservableProperty] private string _guidance = "";
    [ObservableProperty] private bool _replaceAll;
    [ObservableProperty] private string _discussionText = "";
    [ObservableProperty] private string? _notice;

    public bool IsOpen => Workspace.Ui.DirectorOpen;
    public bool IsReadOnly => !string.IsNullOrEmpty(Story.ViewNodeId) || (Story.DirectorView?.ReadOnly ?? false);
    public bool IsLoading => Story.DirectorLoading;
    public bool IsSaving => Story.DirectorSaving || _saving;
    public bool IsCommanding => Story.DirectorCommanding;
    public string? Error => Story.DirectorError;
    public bool HasError => !string.IsNullOrEmpty(Error);
    public bool HasNotice => !string.IsNullOrEmpty(Notice);

    // ---- summary ----
    public bool HasState => Story.DirectorView?.State is not null;
    public DirectorState? State => Story.DirectorView?.State;
    public string SummaryTitle => State?.Plan.Title ?? "尚未启用导演大纲";
    public string StatusText => State?.Status switch
    {
        "active" => "执行中",
        "paused" => "已暂停",
        "completed" => "已完成",
        _ => "未启用",
    };
    public bool IsActive => State?.Status == "active";
    public bool IsPaused => State?.Status == "paused";
    public bool IsCompleted => State?.Status == "completed";
    public int CompletedCount => State?.Progress.Count(p => p.Value.Status == "completed") ?? 0;
    public int TotalCount => State?.Plan.Beats.Count ?? 0;
    public string ProgressText => TotalCount == 0 ? "暂无阶段" : $"{CompletedCount}/{TotalCount} 阶段";
    public string CurrentBeatText
    {
        get
        {
            var state = State;
            if (state is null) return "";
            if (state.Status == "completed") return "阶段已全部结束，可自由续写";
            var beat = state.Plan.Beats.FirstOrDefault(b => b.BeatId == state.CurrentBeatId);
            return beat is null ? "" : $"当前：{beat.Title}";
        }
    }
    public string? Warning => State?.Warning;
    public bool HasWarning => !string.IsNullOrEmpty(Warning);
    public string ReplaceLabel => ReplaceAll ? "确认替换并启用" : "确认启用大纲";

    // ---- discussion ----
    public bool Running => Story.DirectorView?.Requests.Any(r => r.Status == "generating") ?? false;
    public bool HasExchanges => Exchanges.Count > 0;
    public bool CanDiscuss => !IsReadOnly && !Running && !string.IsNullOrWhiteSpace(DiscussionText);
    public string SendLabel => Running ? "停止讨论" : "发送";

    // ---- editor ----
    public bool HasBeats => Beats.Count > 0;
    public bool CanActivate => !IsReadOnly && !Running && !IsSaving && Title.Trim().Length > 0
        && Beats.Count > 0 && Beats.All(b => b.Valid);
    public string ActivateHint => IsReadOnly
        ? "历史快照为只读，请回到最新节点或先分叉"
        : Running ? "导演正在回复，请稍候" : "启用后从下一回合开始生效";

    partial void OnTitleChanged(string value) => MarkDirty();
    partial void OnGuidanceChanged(string value) => MarkDirty();
    partial void OnDiscussionTextChanged(string value) => RaiseCompose();
    partial void OnReplaceAllChanged(bool value) => OnPropertyChanged(nameof(ReplaceLabel));
    partial void OnNoticeChanged(string? value) => OnPropertyChanged(nameof(HasNotice));

    // ---- lifecycle ----

    private void OnUiChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName != nameof(UiStore.DirectorOpen)) return;
        var dispatch = Avalonia.Threading.Dispatcher.UIThread;
        void Apply()
        {
            OnPropertyChanged(nameof(IsOpen));
            if (IsOpen) _ = OpenAsync();
        }
        if (dispatch.CheckAccess()) Apply();
        else dispatch.Post(Apply);
    }

    public async Task OpenAsync()
    {
        if (Story.View is null) return;
        await Story.LoadDirectorAsync().ConfigureAwait(true);
    }

    [RelayCommand]
    private void Close()
    {
        _ = SaveNowAsync();
        Workspace.Ui.SetDirectorOpen(false);
    }

    private void OnStoryChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName is nameof(StoryStore.DirectorView) or nameof(StoryStore.DirectorScope)
            or nameof(StoryStore.DirectorLoading) or nameof(StoryStore.DirectorSaving)
            or nameof(StoryStore.DirectorCommanding) or nameof(StoryStore.DirectorError))
        {
            var dispatch = Avalonia.Threading.Dispatcher.UIThread;
            if (dispatch.CheckAccess()) SyncFromStore();
            else dispatch.Post(SyncFromStore);
        }
    }

    private void SyncFromStore()
    {
        var scope = Story.DirectorScope;
        if (scope != _loadedScope)
        {
            _loadedScope = scope;
            _dirty = false;
            RebuildEditor();
        }
        else if (!_dirty)
        {
            RebuildEditor();
        }
        RaiseSummary();
        RebuildDiscussion();
    }

    private void RebuildEditor()
    {
        var view = Story.DirectorView;
        var plan = view?.Draft?.Plan ?? view?.State?.Plan;
        if (plan is null && !string.IsNullOrEmpty(_loadedScope) && _backups.TryGetValue(_loadedScope, out var backup))
        {
            plan = backup;
            _dirty = true;
            Notice = "已恢复尚未同步的本地导演草稿。";
        }

        _suppress = true;
        Title = plan?.Title ?? "";
        Guidance = plan?.Guidance ?? "";
        Beats.Clear();
        var currentId = view?.State?.CurrentBeatId ?? "";
        var progress = view?.State?.Progress ?? new Dictionary<string, DirectorProgressEntry>();
        var number = 1;
        foreach (var beat in plan?.Beats ?? new List<DirectorBeat>())
        {
            var status = beat.BeatId == currentId && view?.State?.Status != "completed"
                ? "current"
                : progress.TryGetValue(beat.BeatId, out var entry) ? entry.Status : "pending";
            Beats.Add(new DirectorBeatViewModel(beat, MarkDirty)
            {
                Number = number++,
                Status = status,
                Locked = IsReadOnly || status is "completed" or "skipped",
            });
        }
        _suppress = false;
        OnPropertyChanged(nameof(HasBeats));
        OnPropertyChanged(nameof(CanActivate));
    }

    private void RebuildDiscussion()
    {
        var requests = Story.DirectorView?.Requests ?? new List<DirectorRequest>();
        Exchanges.Clear();
        foreach (var request in requests.OrderBy(r => r.CreatedAt, StringComparer.Ordinal))
            Exchanges.Add(new DirectorExchangeViewModel(request));
        OnPropertyChanged(nameof(HasExchanges));
        OnPropertyChanged(nameof(Running));
        RaiseCompose();
    }

    private void RaiseSummary()
    {
        OnPropertyChanged(nameof(HasState));
        OnPropertyChanged(nameof(State));
        OnPropertyChanged(nameof(SummaryTitle));
        OnPropertyChanged(nameof(StatusText));
        OnPropertyChanged(nameof(IsActive));
        OnPropertyChanged(nameof(IsPaused));
        OnPropertyChanged(nameof(IsCompleted));
        OnPropertyChanged(nameof(CompletedCount));
        OnPropertyChanged(nameof(TotalCount));
        OnPropertyChanged(nameof(ProgressText));
        OnPropertyChanged(nameof(CurrentBeatText));
        OnPropertyChanged(nameof(Warning));
        OnPropertyChanged(nameof(HasWarning));
        OnPropertyChanged(nameof(IsReadOnly));
        OnPropertyChanged(nameof(IsLoading));
        OnPropertyChanged(nameof(IsSaving));
        OnPropertyChanged(nameof(IsCommanding));
        OnPropertyChanged(nameof(Error));
        OnPropertyChanged(nameof(HasError));
        OnPropertyChanged(nameof(CanActivate));
        OnPropertyChanged(nameof(ActivateHint));
    }

    private void RaiseCompose()
    {
        OnPropertyChanged(nameof(CanDiscuss));
        OnPropertyChanged(nameof(SendLabel));
        OnPropertyChanged(nameof(CanActivate));
    }

    // ---- autosave ----

    private void MarkDirty()
    {
        if (_suppress) return;
        _dirty = true;
        OnPropertyChanged(nameof(CanActivate));
        _debounce?.Dispose();
        _debounce = new System.Threading.Timer(
            _ => Dispatcher.UIThread.Post(() => _ = SaveNowAsync()),
            null, 700, Timeout.Infinite);
    }

    public async Task SaveNowAsync()
    {
        if (!_dirty || IsReadOnly || _saving) return;
        var view = Story.DirectorView;
        if (view is null) return;
        _saving = true;
        OnPropertyChanged(nameof(IsSaving));
        try
        {
            var plan = BuildPlan();
            var version = view.Draft?.Version ?? 0;
            var baseRevision = view.Draft?.BaseRevisionId ?? view.State?.Plan.RevisionId ?? "";
            var saved = await Story.SaveDirectorDraftAsync(plan, version, baseRevision).ConfigureAwait(true);
            _dirty = false;
            if (!string.IsNullOrEmpty(_loadedScope) && _backups.Remove(_loadedScope))
                SettingsPersistence.Save("director_draft_backups.json", _backups);
            _ = saved;
        }
        catch
        {
            if (!string.IsNullOrEmpty(_loadedScope))
            {
                _backups[_loadedScope] = BuildPlan();
                SettingsPersistence.Save("director_draft_backups.json", _backups);
            }
            Notice = "草稿保存失败，已在本地备份，稍后会自动重试。";
        }
        finally
        {
            _saving = false;
            OnPropertyChanged(nameof(IsSaving));
        }
    }

    private DirectorPlan BuildPlan()
    {
        var view = Story.DirectorView;
        return new DirectorPlan
        {
            PlanId = view?.Draft?.Plan.PlanId ?? view?.State?.Plan.PlanId,
            RevisionId = view?.Draft?.Plan.RevisionId ?? view?.State?.Plan.RevisionId,
            Title = Title.Trim(),
            Guidance = Guidance.Trim(),
            Beats = Beats.Select(b => b.ToBeat()).ToList(),
        };
    }

    // ---- beat editing ----

    [RelayCommand]
    private void AddBeat()
    {
        if (IsReadOnly || Beats.Count >= 32) return;
        var beat = new DirectorBeatViewModel(new DirectorBeat { Title = "新的阶段" }, MarkDirty) { Number = Beats.Count + 1 };
        Beats.Add(beat);
        OnPropertyChanged(nameof(HasBeats));
        MarkDirty();
    }

    [RelayCommand]
    private void MoveBeatUp(DirectorBeatViewModel? beat)
    {
        if (beat is null || IsReadOnly) return;
        var index = Beats.IndexOf(beat);
        if (index <= 0 || beat.IsSettled || beat.IsCurrent) return;
        Beats.Move(index, index - 1);
        Renumber();
        MarkDirty();
    }

    [RelayCommand]
    private void MoveBeatDown(DirectorBeatViewModel? beat)
    {
        if (beat is null || IsReadOnly) return;
        var index = Beats.IndexOf(beat);
        if (index < 0 || index >= Beats.Count - 1 || beat.IsSettled || beat.IsCurrent) return;
        Beats.Move(index, index + 1);
        Renumber();
        MarkDirty();
    }

    [RelayCommand]
    private void DeleteBeat(DirectorBeatViewModel? beat)
    {
        if (beat is null || IsReadOnly || beat.IsSettled || beat.IsCurrent) return;
        Beats.Remove(beat);
        Renumber();
        OnPropertyChanged(nameof(HasBeats));
        MarkDirty();
    }

    private void Renumber()
    {
        for (var i = 0; i < Beats.Count; i++) Beats[i].Number = i + 1;
    }

    // ---- commands ----

    [RelayCommand]
    private async Task ActivateAsync()
    {
        if (!CanActivate) return;
        await SaveNowAsync().ConfigureAwait(true);
        await Story.CommandDirectorAsync("activate", null, ReplaceAll ? true : null).ConfigureAwait(true);
        ReplaceAll = false;
        Notice = "大纲已启用，从下一回合开始生效。";
    }

    [RelayCommand]
    private async Task PauseAsync() => await Story.CommandDirectorAsync("pause").ConfigureAwait(true);

    [RelayCommand]
    private async Task ResumeAsync() => await Story.CommandDirectorAsync("resume").ConfigureAwait(true);

    [RelayCommand]
    private async Task CompleteBeatAsync()
    {
        var beatId = State?.CurrentBeatId;
        if (!string.IsNullOrEmpty(beatId)) await Story.CommandDirectorAsync("complete", beatId).ConfigureAwait(true);
    }

    [RelayCommand]
    private async Task SkipBeatAsync()
    {
        var beatId = State?.CurrentBeatId;
        if (!string.IsNullOrEmpty(beatId)) await Story.CommandDirectorAsync("skip", beatId).ConfigureAwait(true);
    }

    [RelayCommand]
    private async Task RewindAsync(DirectorBeatViewModel? beat)
    {
        if (beat is null) return;
        await Story.CommandDirectorAsync("rewind", beat.BeatId).ConfigureAwait(true);
    }

    [RelayCommand]
    private async Task SendAsync()
    {
        if (Running) return;
        var text = DiscussionText.Trim();
        if (string.IsNullOrEmpty(text) || IsReadOnly) return;
        DiscussionText = "";
        await Story.DiscussDirectorAsync(text).ConfigureAwait(true);
    }

    [RelayCommand]
    private async Task StopDiscussionAsync()
    {
        var running = Story.DirectorView?.Requests.FirstOrDefault(r => r.Status == "generating");
        if (running is not null) await Story.CancelDirectorAsync(running.RequestId).ConfigureAwait(true);
    }

    [RelayCommand]
    private async Task RetryExchangeAsync(DirectorExchangeViewModel? exchange)
    {
        if (exchange is null || !exchange.CanRetry || IsReadOnly) return;
        await Story.DiscussDirectorAsync(exchange.Text).ConfigureAwait(true);
    }

    [RelayCommand]
    private void LoadCandidate(DirectorExchangeViewModel? exchange)
    {
        if (exchange?.Candidate is null || IsReadOnly) return;
        var candidate = exchange.Candidate;
        _suppress = true;
        Title = candidate.Title;
        Guidance = candidate.Guidance;
        Beats.Clear();
        var number = 1;
        foreach (var beat in candidate.Beats)
            Beats.Add(new DirectorBeatViewModel(beat, MarkDirty) { Number = number++ });
        _suppress = false;
        OnPropertyChanged(nameof(HasBeats));
        Notice = "已将建议载入编辑器，确认后点「启用大纲」生效。";
        MarkDirty();
    }

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        _debounce?.Dispose();
        Workspace.Ui.PropertyChanged -= OnUiChanged;
        Story.PropertyChanged -= OnStoryChanged;
    }
}
