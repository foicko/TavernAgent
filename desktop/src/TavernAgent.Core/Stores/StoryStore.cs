using System.Collections.ObjectModel;
using System.ComponentModel;
using System.Runtime.CompilerServices;
using System.Text.Json;
using TavernAgent.Core.Api;
using TavernAgent.Core.Dtos;

namespace TavernAgent.Core.Stores;

/// <summary>
/// Port of web storyStore (session / turn / branch / memory / director slices).
/// Uses explicit events instead of zustand; Desktop VMs subscribe to PropertyChanged.
/// </summary>
public sealed partial class StoryStore : ObservableStore
{
    public static StoryStore Instance { get; } = new();

    private readonly TavernApiClient _api;
    private readonly object _gate = new();

    // navigation / request ownership
    private int _navigationEpoch;
    private int _sendSeq;
    private int _reloadSeq;
    private int _memorySeq;
    private int _sessionListSeq;
    private IDisposable? _sessionSub;
    private IDisposable? _turnSub;
    private IDisposable? _directorSub;
    private System.Threading.Timer? _turnPollTimer;
    private System.Threading.Timer? _directorPollTimer;
    private string? _pendingSessionPayload;
    private string? _pendingSessionKey;
    private int _pendingSessionEpoch = -1;
    private string? _pendingMemoryPayload;
    private string? _pendingMemoryKey;
    private string? _turnCursorId;
    private string? _turnCursorTurnId;
    private int _directorReadSeq;
    private string? _commandPayload;
    private string? _commandKey;
    private string? _messagePayload;
    private string? _messageKey;
    private PendingSubmission? _pendingSubmission;
    private string? _watchSessionId;
    private int _watchEpoch = -1;
    private bool _sessionStalled;
    private string? _activeTurnSubTurnId;

    private sealed record PendingSubmission(int Epoch, SessionView View, string Key, TurnInput Input);

    public StoryStore(TavernApiClient? api = null)
    {
        _api = api ?? new TavernApiClient(null);
    }

    public TavernApiClient Api => _api;

    // ---- state fields ----

    private List<SessionSummary> _sessions = new();
    public List<SessionSummary> Sessions { get => _sessions; private set => SetProperty(ref _sessions, value); }

    private SessionView? _view;
    public SessionView? View { get => _view; private set => SetProperty(ref _view, value); }

    private List<StoryMessage> _messages = new();
    public List<StoryMessage> Messages { get => _messages; private set => SetProperty(ref _messages, value); }

    private List<TextBlock> _draftBlocks = new();
    public List<TextBlock> DraftBlocks { get => _draftBlocks; private set => SetProperty(ref _draftBlocks, value); }

    private List<OptionView> _draftOptions = new();
    public List<OptionView> DraftOptions { get => _draftOptions; private set => SetProperty(ref _draftOptions, value); }

    private TurnPhase _phase = TurnPhase.Idle;
    public TurnPhase Phase { get => _phase; private set { if (SetProperty(ref _phase, value)) OnPropertyChanged(nameof(Busy)); } }

    private string _thinkingText = "";
    public string ThinkingText { get => _thinkingText; private set => SetProperty(ref _thinkingText, value); }

    private Dictionary<string, string> _turnThinking = new();
    public Dictionary<string, string> TurnThinking { get => _turnThinking; private set => SetProperty(ref _turnThinking, value); }

    private string? _lastTurnId;
    public string? LastTurnId { get => _lastTurnId; private set => SetProperty(ref _lastTurnId, value); }

    private string? _error;
    public string? Error { get => _error; private set => SetProperty(ref _error, value); }

    private WorldState? _hud;
    public WorldState? Hud { get => _hud; private set => SetProperty(ref _hud, value); }

    private List<MemoryView> _memories = new();
    public List<MemoryView> Memories { get => _memories; private set => SetProperty(ref _memories, value); }

    private MemoryUsage? _memoryUsage;
    public MemoryUsage? MemoryUsage { get => _memoryUsage; private set => SetProperty(ref _memoryUsage, value); }

    private string? _memoryError;
    public string? MemoryError { get => _memoryError; private set => SetProperty(ref _memoryError, value); }

    private bool _memoryLoading;
    public bool MemoryLoading { get => _memoryLoading; private set => SetProperty(ref _memoryLoading, value); }

    private MemoryPaging _memoryPaging = MemoryPaging.Empty();
    public MemoryPaging MemoryPaging { get => _memoryPaging; private set => SetProperty(ref _memoryPaging, value); }

    private string? _activeCharacterId;
    public string? ActiveCharacterId { get => _activeCharacterId; private set => SetProperty(ref _activeCharacterId, value); }

    private string? _pendingInput;
    public string? PendingInput { get => _pendingInput; private set => SetProperty(ref _pendingInput, value); }

    private string? _pendingAction;
    public string? PendingAction { get => _pendingAction; private set => SetProperty(ref _pendingAction, value); }

    private TurnInput? _lastInput;
    public TurnInput? LastInput { get => _lastInput; private set => SetProperty(ref _lastInput, value); }

    private string? _activeBranchId;
    public string? ActiveBranchId { get => _activeBranchId; private set => SetProperty(ref _activeBranchId, value); }

    private string? _viewNodeId;
    public string? ViewNodeId { get => _viewNodeId; private set { if (SetProperty(ref _viewNodeId, value)) OnPropertyChanged(nameof(Busy)); } }

    private string? _packError;
    public string? PackError { get => _packError; private set => SetProperty(ref _packError, value); }

    private bool _hasMoreHistory;
    public bool HasMoreHistory { get => _hasMoreHistory; private set => SetProperty(ref _hasMoreHistory, value); }

    private string? _oldestTurnId;
    public string? OldestTurnId { get => _oldestTurnId; private set => SetProperty(ref _oldestTurnId, value); }

    private bool _loadingOlder;
    public bool LoadingOlder { get => _loadingOlder; private set => SetProperty(ref _loadingOlder, value); }

    private bool _sessionLoading;
    public bool SessionLoading { get => _sessionLoading; private set => SetProperty(ref _sessionLoading, value); }

    private string? _pendingSessionId;
    public string? PendingSessionId { get => _pendingSessionId; private set => SetProperty(ref _pendingSessionId, value); }

    // director slice
    private DirectorView? _directorView;
    public DirectorView? DirectorView { get => _directorView; private set => SetProperty(ref _directorView, value); }

    private bool _directorLoading;
    public bool DirectorLoading { get => _directorLoading; private set => SetProperty(ref _directorLoading, value); }

    private bool _directorSaving;
    public bool DirectorSaving { get => _directorSaving; private set => SetProperty(ref _directorSaving, value); }

    private bool _directorCommanding;
    public bool DirectorCommanding { get => _directorCommanding; private set { if (SetProperty(ref _directorCommanding, value)) OnPropertyChanged(nameof(Busy)); } }

    private string? _directorError;
    public string? DirectorError { get => _directorError; private set => SetProperty(ref _directorError, value); }

    private string _directorScope = "";
    public string DirectorScope { get => _directorScope; private set => SetProperty(ref _directorScope, value); }

    public BusyState Busy => new(Phase.ToString().ToLowerInvariant() switch
    {
        "generating" => "generating",
        "truncated" => "truncated",
        "awaitingconfirm" => "awaiting_confirm",
        "committed" => "committed",
        "failed" => "failed",
        _ => "idle",
    }, ViewNodeId, DirectorCommanding);

    public bool IsBusy => BusyPredicates.ComposerBusy(Busy);

    public event EventHandler? StateChanged;
    protected override void OnPropertyChanged([CallerMemberName] string? propertyName = null)
    {
        base.OnPropertyChanged(propertyName);
        if (propertyName is not null and not nameof(PropertyChanged))
            StoreDispatcher.Dispatch(() => StateChanged?.Invoke(this, EventArgs.Empty));
    }

    private string PhaseWire => Phase switch
    {
        TurnPhase.Generating => "generating",
        TurnPhase.Truncated => "truncated",
        TurnPhase.AwaitingConfirm => "awaiting_confirm",
        TurnPhase.Committed => "committed",
        TurnPhase.Failed => "failed",
        _ => "idle",
    };

    private static string ToErr(Exception e) => e is ApiError ? e.Message : e.Message;

    private static readonly JsonSerializerOptions EventJson = new(JsonSerializerDefaults.Web);

    // ---- helpers ----
    private (int Epoch, string? SessionId, string? BranchId, string? NodeId) Scope()
        => (_navigationEpoch, View?.SessionId, ActiveBranchId, ViewNodeId);

    private bool Owns((int Epoch, string? SessionId, string? BranchId, string? NodeId) s)
    {
        var now = Scope();
        return s.Epoch == now.Epoch && s.SessionId == now.SessionId && s.BranchId == now.BranchId && s.NodeId == now.NodeId;
    }

    private int BeginNavigation(Action? detachStream)
    {
        detachStream?.Invoke();
        DetachTurnStream();
        DetachSessionWatch();
        _reloadSeq++;
        _memorySeq++;
        _pendingSessionPayload = null;
        _pendingMemoryPayload = null;
        return ++_navigationEpoch;
    }

    private void ApplyEmptyStory()
    {
        DirectorView = null;
        DirectorLoading = false;
        DirectorSaving = false;
        DirectorCommanding = false;
        DirectorError = null;
        DirectorScope = "";
        View = null;
        Messages = new();
        DraftBlocks = new();
        DraftOptions = new();
        Phase = TurnPhase.Idle;
        ThinkingText = "";
        LastTurnId = null;
        Error = null;
        Hud = null;
        Memories = new();
        MemoryUsage = null;
        MemoryError = null;
        MemoryLoading = false;
        ActiveCharacterId = null;
        PendingInput = null;
        PendingAction = null;
        LastInput = null;
        MemoryPaging = MemoryPaging.Empty();
        ActiveBranchId = null;
        ViewNodeId = null;
        PackError = null;
        HasMoreHistory = false;
        OldestTurnId = null;
        LoadingOlder = false;
        SessionLoading = false;
        PendingSessionId = null;
    }

    public static List<StoryMessage> MessagesFromNodes(SessionView view, IReadOnlyDictionary<string, string>? turnThinkingMap = null)
    {
        var outList = new List<StoryMessage>();
        var thinkingMap = turnTurningFallback(turnThinkingMap);
        var nodes = (view.Nodes ?? new List<PlotNode>())
            .OrderBy(n => n.Depth)
            .ThenBy(n => n.TurnNumber)
            .ToList();
        foreach (var n in nodes)
        {
            if (n.Kind == "root")
            {
                outList.Add(new StoryMessage
                {
                    Id = n.NodeId + ":opening",
                    Role = "opening",
                    Blocks = new List<TextBlock> { new() { Kind = "narration", Text = StoryHelpers.ParseRoot(n) } },
                });
                continue;
            }
            if (n.Kind != "turn") continue;
            var tc = StoryHelpers.ParseTurnContent(n);
            if (tc is null) continue;
            var opts = (tc.Options ?? new List<Option>())
                .Select(o => new OptionView { OptionId = o.OptionId, Intent = o.Intent, Text = o.Text })
                .ToList();
            var turnId = tc.Provenance?.TurnId;
            string? thinking = null;
            if (thinkingMap.TryGetValue(n.NodeId, out var t1)) thinking = t1;
            else if (turnId is not null && thinkingMap.TryGetValue(turnId, out var t2)) thinking = t2;
            outList.Add(new StoryMessage
            {
                Id = n.NodeId,
                ParentId = string.IsNullOrEmpty(n.ParentId) ? null : n.ParentId,
                Role = "assistant",
                Blocks = tc.Blocks ?? new List<TextBlock>(),
                Options = opts,
                InputText = tc.InputText,
                InputNote = tc.InputNote,
                Checks = tc.Checks,
                Changes = tc.Changes,
                InjectedMemories = tc.InjectedMemories,
                SuppressedOptions = tc.SuppressedOptions,
                TurnNumber = n.TurnNumber,
                Thinking = thinking,
            });
        }
        if (outList.Count == 0)
        {
            outList.Add(new StoryMessage
            {
                Id = view.SessionId + ":opening",
                Role = "opening",
                Blocks = new List<TextBlock> { new() { Kind = "narration", Text = StoryHelpers.ParseRoot(view.HeadNode) } },
            });
        }
        return outList;

        static IReadOnlyDictionary<string, string> turnTurningFallback(IReadOnlyDictionary<string, string>? m)
            => m ?? new Dictionary<string, string>();
    }

    private void UpdateBusyRaise()
    {
        OnPropertyChanged(nameof(Busy));
        OnPropertyChanged(nameof(IsBusy));
    }

    partial class ObservableStoreHooks { }
}
