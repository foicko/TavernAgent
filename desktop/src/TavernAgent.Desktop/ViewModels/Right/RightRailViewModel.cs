using System.Collections.ObjectModel;
using System.ComponentModel;
using Avalonia.Media.Imaging;
using Avalonia.Threading;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.CardMarkers;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.Services;
using TavernAgent.Desktop.ViewModels.Stage;

namespace TavernAgent.Desktop.ViewModels.Right;

/// <summary>Right rail: tachie, meters, inventory and checkpoint (web: RightRail).</summary>
public sealed partial class RightRailViewModel : WorkspaceChildViewModel
{
    private bool _disposed;
    private string? _lastDeltaMessageId;

    public RightRailViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        Story.PropertyChanged += OnStoryChanged;
        Workspace.Ui.PropertyChanged += OnUiChanged;
        Checkpoint = new StoryCheckpointViewModel();
        Rebuild();
    }

    public StoryStore Story => Workspace.Story;
    public UiStore Ui => Workspace.Ui;

    [ObservableProperty] private CharacterPresentation? _character;
    [ObservableProperty] private Bitmap? _tachie;
    [ObservableProperty] private string _expressionTitle = "🎭 神态：警觉审视";
    [ObservableProperty] private double _expressionOverlay = 0.10;

    public bool IsCollapsed => Workspace.IsRightCollapsed;
    public bool ShowContent => !IsCollapsed;
    public bool HasStory => Story.View is not null;
    public bool HasTachie => Tachie is not null;
    public string TachieMonogram => Character?.Monogram ?? "🎭";
    public bool HasTags => Character?.HasTags == true;
    public IReadOnlyList<string> Tags => Character?.Tags ?? Array.Empty<string>();

    public MeterViewModel MeterAffection { get; private set; } = MeterViewModel.Affection(0);
    public MeterViewModel MeterTrust { get; private set; } = MeterViewModel.Trust(0);
    public MeterViewModel MeterAlert { get; private set; } = MeterViewModel.Alert(0);

    public ObservableCollection<InventoryItemViewModel> Inventory { get; } = new();
    public bool HasInventory => Inventory.Count > 0;
    public int InventoryCount => Inventory.Count;

    /// <summary>会话变量（卡片 [VAR]/defaultVariables）：按 key 排序展示。</summary>
    public ObservableCollection<VariableSlotViewModel> Variables { get; } = new();
    public bool HasVariables => Variables.Count > 0;

    public StoryCheckpointViewModel Checkpoint { get; }

    [ObservableProperty] private int _activePromiseCount;
    [ObservableProperty] private int _memoryCount;
    [ObservableProperty] private bool _isFullTachieOpen;

    public bool IsAlertExpr => Ui.TachieExpression == TachieExpression.Alert;
    public bool IsCalmExpr => Ui.TachieExpression == TachieExpression.Calm;
    public bool IsSmileExpr => Ui.TachieExpression == TachieExpression.Smile;
    public bool IsWhisperExpr => Ui.TachieExpression == TachieExpression.Whisper;

    // ---- commands ----

    [RelayCommand] private void ToggleCollapse() => Ui.ToggleRightRail();
    [RelayCommand] private void SetAlert() => Ui.SetTachieExpression(TachieExpression.Alert);
    [RelayCommand] private void SetCalm() => Ui.SetTachieExpression(TachieExpression.Calm);
    [RelayCommand] private void SetSmile() => Ui.SetTachieExpression(TachieExpression.Smile);
    [RelayCommand] private void SetWhisper() => Ui.SetTachieExpression(TachieExpression.Whisper);
    [RelayCommand] private void OpenFullTachie() => IsFullTachieOpen = true;
    [RelayCommand] private void CloseFullTachie() => IsFullTachieOpen = false;
    [RelayCommand] private void OpenDossier() => Ui.SetDossierModalOpen(true);
    [RelayCommand] private void OpenPromises() => Ui.SetPromisesModalOpen(true);
    [RelayCommand] private void OpenMemory() => Ui.SetMemoryModalOpen(true);

    [RelayCommand]
    private void ActivateInventory(InventoryActViewModel? act)
    {
        if (act is null) return;
        Ui.InjectComposerText(act.Text);
        Ui.NotifyQuiet($"已填入「{act.Label}」，发送后推进剧情。");
    }

    // ---- state propagation ----

    private void OnUiChanged(object? sender, PropertyChangedEventArgs e)
    {
        var dispatch = Avalonia.Threading.Dispatcher.UIThread;
        void Apply()
        {
            if (e.PropertyName is nameof(UiStore.TachieExpression))
            {
                UpdateExpression();
                // 有情绪立绘的卡：切换神态需换图，而不只是叠一层滤镜。
                LoadTachieAsync(ResolveTachieSource()).SafeFireAndForget();
                OnPropertyChanged(nameof(IsAlertExpr));
                OnPropertyChanged(nameof(IsCalmExpr));
                OnPropertyChanged(nameof(IsSmileExpr));
                OnPropertyChanged(nameof(IsWhisperExpr));
            }
            else if (e.PropertyName is nameof(UiStore.RightCollapsed) or nameof(UiStore.AutoRightCollapsed))
            {
                OnPropertyChanged(nameof(IsCollapsed));
                OnPropertyChanged(nameof(ShowContent));
            }
        }
        if (dispatch.CheckAccess()) Apply();
        else dispatch.Post(Apply);
    }

    private void OnStoryChanged(object? sender, PropertyChangedEventArgs e)
    {
        void Apply()
        {
            switch (e.PropertyName)
            {
                case nameof(StoryStore.View):
                    Rebuild();
                    break;
                case nameof(StoryStore.Hud):
                    RebuildMeters();
                    RebuildInventory();
                    break;
                case nameof(StoryStore.Memories):
                case nameof(StoryStore.MemoryUsage):
                case nameof(StoryStore.MemoryPaging):
                    UpdateCounts();
                    break;
                case nameof(StoryStore.Messages):
                    RebuildCheckpoint();
                    ProcessDeltas();
                    break;
            }
        }
        if (Dispatcher.UIThread.CheckAccess()) Apply();
        else Dispatcher.UIThread.Post(Apply);
    }

    private void Rebuild()
    {
        Character = CharacterPresentation.Resolve(Ui.ActiveCharKey, Story.View);
        OnPropertyChanged(nameof(HasStory));
        OnPropertyChanged(nameof(HasTags));
        OnPropertyChanged(nameof(Tags));
        OnPropertyChanged(nameof(TachieMonogram));
        LoadTachieAsync(ResolveTachieSource()).SafeFireAndForget();
        RebuildMeters();
        RebuildInventory();
        RebuildVariables();
        RebuildCheckpoint();
        UpdateCounts();
        UpdateExpression();
        ProcessDeltas();
    }

    private async Task LoadTachieAsync(string? source)
    {
        var bmp = await ImageLoader.LoadAsync(source, 520).ConfigureAwait(true);
        Tachie = bmp;
        OnPropertyChanged(nameof(HasTachie));
    }

    private void RebuildMeters()
    {
        var rel = ResolveRelation();
        MeterAffection = rel is null ? MeterViewModel.Affection(0) : MeterViewModel.Affection(rel.Affection);
        MeterTrust = rel is null ? MeterViewModel.Trust(0) : MeterViewModel.Trust(rel.Trust);
        MeterAlert = rel is null ? MeterViewModel.Alert(0) : MeterViewModel.Alert(rel.Alertness);
        OnPropertyChanged(nameof(MeterAffection));
        OnPropertyChanged(nameof(MeterTrust));
        OnPropertyChanged(nameof(MeterAlert));
    }

    private RelationValue? ResolveRelation()
    {
        var hud = Story.Hud;
        if (hud is null || hud.Relationships.Count == 0) return null;
        var key = Ui.ActiveCharKey;
        if (!string.IsNullOrEmpty(key))
        {
            if (hud.Relationships.TryGetValue("npc_" + key, out var a)) return a;
            if (key != "player" && hud.Relationships.TryGetValue(key, out var b)) return b;
        }
        return hud.Relationships.FirstOrDefault(kv => kv.Key != "player").Value
               ?? hud.Relationships.Values.FirstOrDefault();
    }

    private void RebuildInventory()
    {
        Inventory.Clear();
        var items = Story.Hud?.Items;
        if (items is not null)
        {
            foreach (var item in items.Values.Where(i => i.Quantity > 0 && i.OwnerId == "player"))
            {
                var acts = new List<InventoryActViewModel>
                {
                    new("inspect", $"仔细端详 {item.Name}", $"我将【{item.Name}】握在手心仔细端详，思索其背后的渊源与线索..."),
                    new("show", $"向对方出示 {item.Name}", $"我将【{item.Name}】轻轻置于桌面推向对方眼前，观察其神色变化。"),
                };
                var icon = item.Keepsake == true ? "⏱️" : "📦";
                var status = item.Keepsake == true ? "核心信物" : $"数量: {item.Quantity}";
                Inventory.Add(new InventoryItemViewModel(item.InstanceId, item.Name, status, icon, acts));
            }
        }
        OnPropertyChanged(nameof(HasInventory));
        OnPropertyChanged(nameof(InventoryCount));
    }

    private void RebuildVariables()
    {
        Variables.Clear();
        var vars = Story.Hud?.Variables;
        if (vars is not null)
        {
            foreach (var pair in vars.OrderBy(p => p.Key, StringComparer.Ordinal))
                Variables.Add(new VariableSlotViewModel(pair.Key, pair.Value));
        }
        OnPropertyChanged(nameof(HasVariables));
    }

    /// <summary>立绘来源：优先卡内情绪立绘（type=emotion），否则回退角色头像。</summary>
    private string? ResolveTachieSource()
    {
        var url = EmotionAssets.Resolve(CardAssetCache.GetTyped(Story.View?.CharacterId), ExpressionKey());
        return !string.IsNullOrEmpty(url) ? TurnIllustrationViewModel.AbsoluteUrl(url) : Character?.AvatarSource;
    }

    private string ExpressionKey() => Ui.TachieExpression switch
    {
        TachieExpression.Calm => "calm",
        TachieExpression.Smile => "smile",
        TachieExpression.Whisper => "whisper",
        _ => "alert",
    };

    private void RebuildCheckpoint() => Checkpoint.Load(Story.View?.ActiveSummary?.Text);

    private void UpdateCounts()
    {
        var promises = Story.Hud?.Promises?.Values.ToList() ?? new List<PromiseItem>();
        ActivePromiseCount = Story.View is null ? 0 : promises.Count(p => p.State == "active");
        MemoryCount = Story.MemoryPaging.Total > 0
            ? Story.MemoryPaging.Total
            : Story.MemoryUsage?.Used > 0 ? Story.MemoryUsage!.Used : Story.Memories.Count;
    }

    private void UpdateExpression()
    {
        (ExpressionTitle, ExpressionOverlay) = Ui.TachieExpression switch
        {
            TachieExpression.Calm => ("🎭 神态：冷静自持", 0.06),
            TachieExpression.Smile => ("🎭 神态：微露释怀", 0.0),
            TachieExpression.Whisper => ("🎭 神态：动容微温", 0.0),
            _ => ("🎭 神态：警觉审视", 0.14),
        };
    }

    private void ProcessDeltas()
    {
        var latest = Story.Messages.LastOrDefault(m => m.Role == "assistant");
        if (latest is null || latest.Id == _lastDeltaMessageId) return;
        _lastDeltaMessageId = latest.Id;

        foreach (var change in latest.Changes ?? new List<StateChange>())
        {
            if (change.Delta is not { } delta || delta == 0) continue;
            var meter = ResolveMeterForLabel(change.Label);
            if (meter is null) continue;
            var text = string.IsNullOrWhiteSpace(change.Text) ? $"{change.Label} {delta:+0;-0}" : change.Text;
            var vm = new MeterDeltaViewModel(text, delta > 0);
            meter.Deltas.Add(vm);
            _ = RemoveDeltaLater(meter, vm);
        }
    }

    private MeterViewModel? ResolveMeterForLabel(string? label)
    {
        var l = label ?? "";
        if (l.Contains("心意") || l.Contains("眷顾") || l.Contains("aff", StringComparison.OrdinalIgnoreCase)) return MeterAffection;
        if (l.Contains("信任") || l.Contains("trust", StringComparison.OrdinalIgnoreCase)) return MeterTrust;
        if (l.Contains("防备") || l.Contains("戒心") || l.Contains("alert", StringComparison.OrdinalIgnoreCase)) return MeterAlert;
        return null;
    }

    private async Task RemoveDeltaLater(MeterViewModel meter, MeterDeltaViewModel vm)
    {
        await Task.Delay(1400).ConfigureAwait(true);
        if (Dispatcher.UIThread.CheckAccess()) meter.Deltas.Remove(vm);
        else Dispatcher.UIThread.Post(() => meter.Deltas.Remove(vm));
    }

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        Story.PropertyChanged -= OnStoryChanged;
        Workspace.Ui.PropertyChanged -= OnUiChanged;
    }
}

/// <summary>单条会话变量（key / value）。</summary>
public sealed record VariableSlotViewModel(string Key, string Value)
{
    public string DisplayValue => string.IsNullOrEmpty(Value) ? "—" : Value;
}
