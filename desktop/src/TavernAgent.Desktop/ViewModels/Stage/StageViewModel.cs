using System.Collections.ObjectModel;
using System.ComponentModel;
using Avalonia.Media.Imaging;
using Avalonia.Threading;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.Services;
using TavernAgent.Desktop.ViewModels.Modals;

namespace TavernAgent.Desktop.ViewModels.Stage;

/// <summary>Center stage: scene bar, narrative stream and composer (web: NarrativeStream + ComposerShelf).</summary>
public sealed partial class StageViewModel : WorkspaceChildViewModel
{
    private readonly List<MessageViewModel> _messages = new();
    private bool _disposed;

    public StageViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        Composer = new ComposerViewModel(this);
        Context = new ContextMetricsViewModel();
        Illustration = new IllustrationModalViewModel(workspace);
        Director = new DirectorViewModel(workspace);
        Story.PropertyChanged += OnStoryPropertyChanged;
        IllustrationStore.Instance.Changed += OnIllustrationChanged;
        RebuildAll();
    }

    public StoryStore Story => Workspace.Story;
    public UiStore Ui => Workspace.Ui;

    public ComposerViewModel Composer { get; }
    public ContextMetricsViewModel Context { get; }
    public IllustrationModalViewModel Illustration { get; }

    public DirectorViewModel Director { get; }

    public void OpenDirector()
    {
        if (Story.View is null)
        {
            Ui.NotifyQuiet("请先开启一个故事，再使用导演模式");
            return;
        }
        Ui.SetDirectorOpen(true);
    }

    // ---- illustration lightbox ----

    [ObservableProperty] private bool _lightboxOpen;
    [ObservableProperty] private Bitmap? _lightboxImage;

    public void OpenLightbox(Bitmap? image)
    {
        if (image is null) return;
        LightboxImage = image;
        LightboxOpen = true;
    }

    [RelayCommand]
    private void CloseLightbox() => LightboxOpen = false;

    public void OpenIllustration(string turnId, string fallbackText)
        => Illustration.Open(turnId, fallbackText, CharName);

    private void OnIllustrationChanged(object? sender, EventArgs e) => RunOnUi(() =>
    {
        foreach (var message in _messages) _ = message.Illustration.ReloadAsync();
    });

    public ObservableCollection<MessageViewModel> Messages { get; } = new();
    public ObservableCollection<BlockViewModel> DraftBlocks { get; } = new();

    // ---- character presentation ----

    [ObservableProperty] private string _charName = "未选择角色";
    [ObservableProperty] private string _charShortName = "未选择";
    [ObservableProperty] private string _charRole = "未选择角色卡";
    [ObservableProperty] private string _charStagePill = "暂无角色";
    [ObservableProperty] private string _charStateBadge = "状态：待定";
    [ObservableProperty] private string _charVoiceBadge = "叙事：待定";
    [ObservableProperty] private string? _avatarUrl;
    public string CharMonogram => string.IsNullOrEmpty(CharShortName) ? "?" : CharShortName[..1];
    public bool HasAvatar => !string.IsNullOrEmpty(AvatarUrl);

    // ---- scene / history state ----

    public bool HasStory => Story.View is not null;
    public string SceneTitle => Story.View?.Title ?? "";

    [ObservableProperty] private bool _isThinkingExpanded;
    public string ThinkingText => Story.ThinkingText;
    public bool HasThinking => !string.IsNullOrEmpty(Story.ThinkingText);
    public string ThinkingTitle => DraftBlocks.Count == 0 || DraftBlocks.All(b => b.IsEmpty)
        ? "深度推演与因果思索中..."
        : "思考过程 (Reasoning)";
    public string ThinkingToggleLabel => IsThinkingExpanded ? "收起 ▲" : "展开 ▼";

    public TurnPhase Phase => Story.Phase;
    public bool Busy => Story.IsBusy;
    public bool IsIdle => Story.Phase == TurnPhase.Idle;
    public bool IsGenerating => Story.Phase == TurnPhase.Generating;
    public bool IsCommitted => Story.Phase == TurnPhase.Committed;
    public bool IsTruncated => Story.Phase == TurnPhase.Truncated;
    public bool IsFailed => Story.Phase == TurnPhase.Failed;
    public bool IsHistoryView => !string.IsNullOrEmpty(Story.ViewNodeId);
    public bool IsStudioMode => !Workspace.IsReadingMode;

    public bool IsStreaming => Story.Phase is TurnPhase.Generating or TurnPhase.Committed
        || Story.DraftBlocks.Count > 0
        || (Story.Phase is TurnPhase.Truncated or TurnPhase.AwaitingConfirm && Story.LastInput is not null);

    public bool HasDraftBlocks => DraftBlocks.Any(b => !b.IsEmpty);

    [ObservableProperty] private bool _hasLastInput;
    [ObservableProperty] private string _lastInputText = "";
    [ObservableProperty] private string _lastInputActionLabel = "";
    [ObservableProperty] private bool _hasLastInputAction;
    [ObservableProperty] private int _activeTurnNumber = 1;

    public string? Error => Story.Error;
    public bool HasError => Story.Phase == TurnPhase.Failed || !string.IsNullOrEmpty(Story.Error);

    public bool HasMoreHistory => Story.HasMoreHistory;
    public bool IsLoadingOlder => Story.LoadingOlder;
    public bool IsSessionLoading => Story.SessionLoading;
    public bool ShowSkeleton => IsSessionLoading && Messages.Count == 0;
    public bool ShowPrologue => HasStory && Messages.Count == 0 && !ShowSkeleton;
    public bool HasMessages => Messages.Count > 0;

    public int WordCount => Story.Messages.Sum(m => m.Blocks.Sum(b => b.Text.Length));
    public string WordCountText => WordCount.ToString("N0");
    public int ReadingMinutes => Math.Max(1, (int)Math.Ceiling(WordCount / 450.0));
    public int HeadTurnNumber => Story.View?.HeadNode.TurnNumber ?? 0;

    public bool ShowReadingBanner => Workspace.IsReadingMode && HasStory;
    public string PrologueText { get; private set; } = "";
    public bool HasPrologue => !string.IsNullOrEmpty(PrologueText);

    // ---- commands ----

    public async Task ContinueAsync() => await Story.ContinueTurnAsync().ConfigureAwait(true);

    public async Task RetryAsync() => await Story.RetryAsync().ConfigureAwait(true);

    public async Task CancelAsync() => await Story.CancelAsync().ConfigureAwait(true);

    [RelayCommand]
    private async Task Continue() => await ContinueAsync().ConfigureAwait(true);

    [RelayCommand]
    private async Task Retry() => await RetryAsync().ConfigureAwait(true);

    [RelayCommand]
    private async Task Cancel() => await CancelAsync().ConfigureAwait(true);

    [RelayCommand]
    private async Task LoadOlder() => await Story.LoadOlderAsync().ConfigureAwait(true);

    [RelayCommand]
    private async Task BackToHead() => await Story.BackToHeadAsync().ConfigureAwait(true);

    [RelayCommand]
    private async Task ForkView()
    {
        if (Story.ViewNodeId is { Length: > 0 } nodeId)
            await Story.ForkFromAsync(nodeId, "平行分支_" + DateTime.Now.ToString("HHmm")).ConfigureAwait(true);
    }

    [RelayCommand]
    private async Task RefreshView() => await Story.RefreshViewAsync().ConfigureAwait(true);

    [RelayCommand]
    private async Task RegenerateLatest()
    {
        var latest = _messages.LastOrDefault(m => m.IsAssistant);
        if (latest is not null) await RegenerateAsync(latest, null).ConfigureAwait(true);
    }

    [RelayCommand]
    private void ToggleThinking() => IsThinkingExpanded = !IsThinkingExpanded;

    [RelayCommand]
    private async Task StartNeutralSession()
    {
        if (Busy) return;
        var opts = new CreateSessionOpts
        {
            Title = "未命名剧本",
            CharacterJson = CharacterPresets.GetPresetCardJson(CharacterPresets.DefaultPresetKey),
            PlayerName = NarrativeText.DefaultPlayerName,
            OpeningText = NarrativeText.DefaultOpeningText,
        };
        try
        {
            await Story.CreateSessionAsync(opts).ConfigureAwait(true);
            Ui.NotifyQuiet("已创建新故事");
        }
        catch (Exception ex)
        {
            Ui.NotifyQuiet("创建失败：" + ex.Message);
        }
    }

    // ---- actions invoked by message/composer ----

    public async Task SendAsync(string text, string? actionRef, string? note)
        => await Story.SendAsync(text, actionRef, note).ConfigureAwait(true);

    public async Task ChooseOptionAsync(OptionViewModel option)
        => await Story.ChooseOptionAsync(new OptionView { OptionId = option.OptionId, Intent = option.Intent, Text = option.Text }).ConfigureAwait(true);

    public async Task RegenerateAsync(MessageViewModel message, string? guidance)
    {
        if (message.IsOpening) return;
        await Story.RegenerateAsync(message.Id, recheck: false, guidance: guidance).ConfigureAwait(true);
    }

    public async Task EditTextAsync(MessageViewModel message, List<TextBlock> blocks)
    {
        if (message.IsOpening) return;
        await Story.EditAssistantTextAsync(message.Id, blocks).ConfigureAwait(true);
    }

    public async Task EditInputAsync(MessageViewModel message, string text)
    {
        if (message.IsOpening) return;
        await Story.EditPlayerInputAsync(message.Id, text).ConfigureAwait(true);
    }

    public async Task ViewAtNodeAsync(string nodeId)
        => await Story.ViewAtAsync(nodeId).ConfigureAwait(true);

    public async Task ForkAsync(MessageViewModel message)
    {
        if (message.IsOpening) return;
        await Story.ForkFromAsync(message.Id, "分支_" + DateTime.Now.ToString("HHmm")).ConfigureAwait(true);
    }

    public void InjectQuote(string text)
    {
        Composer.SetText($"> {CharShortName}：「{text}」\n\n");
        Ui.NotifyQuiet("已将原话转换为引文前置到输入框");
    }

    public void OnComposerTextChanged(string text)
    {
        var primary = SettingsStore.ActivePrimaryInstance(AppServices.Settings.Providers, AppServices.Settings.Models);
        Context.Recalculate(Story, primary, text);
    }

    public async Task SwitchModelAsync(string modelId)
    {
        try
        {
            await AppServices.Settings.SwitchPrimaryModelAsync(modelId).ConfigureAwait(true);
            Composer.RefreshModels();
            OnComposerTextChanged(Composer.Text);
            Ui.NotifyQuiet("已切换主线模型");
        }
        catch (Exception ex)
        {
            Ui.NotifyQuiet("切换失败：" + ex.Message);
        }
    }

    public async Task SetEffortAsync(string effort)
    {
        try
        {
            await AppServices.Settings.SetReasoningEffortAsync(effort).ConfigureAwait(true);
            Composer.RefreshModels();
            Ui.NotifyQuiet(effort.Length == 0 ? "思考强度已设为默认" : $"思考强度已设为 {effort}");
        }
        catch (Exception ex)
        {
            Ui.NotifyQuiet(ex.Message);
        }
    }

    // ---- state propagation ----

    private void OnStoryPropertyChanged(object? sender, PropertyChangedEventArgs e)
    {
        void Apply()
        {
            switch (e.PropertyName)
            {
                case nameof(StoryStore.Messages):
                    RebuildMessages();
                    break;
                case nameof(StoryStore.DraftBlocks):
                    RebuildDraft();
                    break;
                case nameof(StoryStore.View):
                    OnViewChanged();
                    break;
                case nameof(StoryStore.ThinkingText):
                    OnPropertyChanged(nameof(ThinkingText));
                    OnPropertyChanged(nameof(HasThinking));
                    OnPropertyChanged(nameof(ThinkingTitle));
                    break;
                case nameof(StoryStore.Phase):
                    RaisePhase();
                    RebuildDraft();
                    break;
                case nameof(StoryStore.Error):
                    OnPropertyChanged(nameof(Error));
                    OnPropertyChanged(nameof(HasError));
                    break;
                case nameof(StoryStore.LastInput):
                    UpdateLastInput();
                    break;
                case nameof(StoryStore.ViewNodeId):
                    OnPropertyChanged(nameof(IsHistoryView));
                    RaisePhase();
                    break;
                case nameof(StoryStore.HasMoreHistory):
                    OnPropertyChanged(nameof(HasMoreHistory));
                    break;
                case nameof(StoryStore.LoadingOlder):
                    OnPropertyChanged(nameof(IsLoadingOlder));
                    break;
                case nameof(StoryStore.SessionLoading):
                    OnPropertyChanged(nameof(IsSessionLoading));
                    OnPropertyChanged(nameof(ShowSkeleton));
                    OnPropertyChanged(nameof(ShowPrologue));
                    break;
                case nameof(StoryStore.PendingInput):
                    if (Story.PendingInput is not null) Composer.SetText(Story.PendingInput);
                    break;
                case nameof(StoryStore.ActiveBranchId):
                    RebuildMessages();
                    break;
            }
        }

        RunOnUi(Apply);
    }

    private void RunOnUi(Action action)
    {
        if (Dispatcher.UIThread.CheckAccess()) action();
        else Dispatcher.UIThread.Post(action);
    }

    private void RebuildAll()
    {
        RebuildCharacter();
        RebuildMessages();
        RebuildDraft();
        UpdateLastInput();
        RaisePhase();
        Composer.Refresh();
        OnComposerTextChanged(Composer.Text);
    }

    private void OnViewChanged()
    {
        RebuildCharacter();
        OnPropertyChanged(nameof(HasStory));
        OnPropertyChanged(nameof(SceneTitle));
        OnPropertyChanged(nameof(ShowPrologue));
        OnPropertyChanged(nameof(ShowSkeleton));
        OnPropertyChanged(nameof(HeadTurnNumber));
        OnPropertyChanged(nameof(ShowReadingBanner));
        Composer.Refresh();
        OnComposerTextChanged(Composer.Text);
    }

    private void RebuildCharacter()
    {
        var view = Story.View;
        CharacterInfo? character = null;
        if (view?.State?.Characters is { Count: > 0 } chars)
            character = chars.Values.FirstOrDefault(c => c.Participant == true) ?? chars.Values.FirstOrDefault(c => c.CharacterId != "player");

        var preset = CharacterPresets.Neutral;
        var name = !string.IsNullOrWhiteSpace(character?.Name) ? character!.Name! : (string.IsNullOrWhiteSpace(view?.Title) ? preset.Name : view!.Title);
        CharName = name;
        CharShortName = CleanShortName(!string.IsNullOrWhiteSpace(character?.Nickname) ? character!.Nickname! : name);
        CharRole = FirstLine(character?.Description) ?? preset.Role;
        CharStagePill = preset.StagePill;
        CharStateBadge = preset.StateBadge;
        CharVoiceBadge = preset.VoiceBadge;

        var avatar = character?.Avatar;
        AvatarUrl = !string.IsNullOrEmpty(avatar) && (avatar.StartsWith("data:", StringComparison.OrdinalIgnoreCase)
            || avatar.StartsWith("http", StringComparison.OrdinalIgnoreCase))
            ? avatar
            : null;
        OnPropertyChanged(nameof(HasAvatar));
        OnPropertyChanged(nameof(CharMonogram));

        PrologueText = NarrativeText.ReplaceMacros(preset.Prologue, CharShortName, PlayerName());
        OnPropertyChanged(nameof(PrologueText));
        OnPropertyChanged(nameof(HasPrologue));
    }

    private void RebuildMessages()
    {
        foreach (var m in _messages) m.Detach();
        _messages.Clear();
        Messages.Clear();

        var view = Story.View;
        var actionLabels = (view?.Actions ?? new List<ActionView>())
            .GroupBy(a => a.ActionId)
            .ToDictionary(g => g.Key, g => g.First().Label);
        var charName = CharShortName;
        var userName = PlayerName();

        var list = view is null ? new List<StoryMessage>() : StoryStore.MessagesFromNodes(view, Story.TurnThinking);
        for (var i = 0; i < list.Count; i++)
        {
            var vm = new MessageViewModel(list[i], i, i == list.Count - 1, this, charName, userName, actionLabels);
            _messages.Add(vm);
            Messages.Add(vm);
        }
        OnPropertyChanged(nameof(HasMessages));
        OnPropertyChanged(nameof(ShowPrologue));
        OnPropertyChanged(nameof(WordCount));
        OnPropertyChanged(nameof(WordCountText));
        OnPropertyChanged(nameof(ReadingMinutes));
    }

    private void RebuildDraft()
    {
        DraftBlocks.Clear();
        var charName = CharShortName;
        var userName = PlayerName();
        foreach (var block in Story.DraftBlocks)
            DraftBlocks.Add(new BlockViewModel(block, charName, userName));
        OnPropertyChanged(nameof(HasDraftBlocks));
        OnPropertyChanged(nameof(IsStreaming));
        OnPropertyChanged(nameof(ThinkingTitle));
    }

    private void UpdateLastInput()
    {
        var input = Story.LastInput;
        HasLastInput = input is not null && !string.IsNullOrEmpty(input.Text);
        LastInputText = input is null ? "" : NarrativeText.ReplaceMacros(input.Text, CharShortName, PlayerName());
        var action = input?.ActionRef;
        HasLastInputAction = !string.IsNullOrEmpty(action);
        var view = Story.View;
        LastInputActionLabel = action is null
            ? ""
            : view?.Actions?.FirstOrDefault(a => a.ActionId == action)?.Label ?? action;
        ActiveTurnNumber = (view?.HeadNode.TurnNumber ?? Messages.Count) + 1;
        OnPropertyChanged(nameof(IsStreaming));
    }

    private void RaisePhase()
    {
        OnPropertyChanged(nameof(Phase));
        OnPropertyChanged(nameof(Busy));
        OnPropertyChanged(nameof(IsIdle));
        OnPropertyChanged(nameof(IsGenerating));
        OnPropertyChanged(nameof(IsCommitted));
        OnPropertyChanged(nameof(IsTruncated));
        OnPropertyChanged(nameof(IsFailed));
        OnPropertyChanged(nameof(IsStreaming));
        OnPropertyChanged(nameof(HasError));
        Composer.RaiseCan();
    }

    private string PlayerName()
    {
        var player = Story.View?.State?.Characters.TryGetValue("player", out var p) == true ? p?.Name : null;
        return string.IsNullOrWhiteSpace(player) ? NarrativeText.DefaultPlayerName : player!;
    }

    private static string CleanShortName(string name)
    {
        var first = name.Split('\n', '\r')[0].Trim();
        return first.Length <= 12 ? first : first[..12];
    }

    private static string? FirstLine(string? text)
    {
        if (string.IsNullOrWhiteSpace(text)) return null;
        var line = text.Split('\n', '\r').FirstOrDefault(l => !string.IsNullOrWhiteSpace(l))?.Trim();
        if (string.IsNullOrEmpty(line)) return null;
        return line.Length <= 42 ? line : line[..42] + "…";
    }

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        Story.PropertyChanged -= OnStoryPropertyChanged;
        IllustrationStore.Instance.Changed -= OnIllustrationChanged;
        Director.Dispose();
        foreach (var m in _messages) m.Detach();
    }
}
