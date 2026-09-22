using System.Text.Json;
using TavernAgent.Core.Api;
using TavernAgent.Core.Dtos;

namespace TavernAgent.Core.Stores;

// Session slice
public sealed partial class StoryStore
{
    public async Task LoadSessionsAsync(CancellationToken ct = default)
    {
        var ticket = ++_sessionListSeq;
        var list = await _api.ListSessionsAsync(ct).ConfigureAwait(false);
        if (ticket == _sessionListSeq) Sessions = list ?? new List<SessionSummary>();
    }

    public async Task OpenSessionAsync(string id, CancellationToken ct = default)
    {
        var epoch = BeginNavigation(null);
        var cached = SessionViewCache.TryGet(id);
        if (cached is not null)
        {
            SessionMemory.RememberSession(id);
            var thinking = TurnThinking;
            View = cached;
            Messages = MessagesFromNodes(cached, thinking);
            Hud = cached.State;
            ActiveCharacterId = cached.CharacterId;
            ActiveBranchId = cached.Branch.BranchId;
            HasMoreHistory = cached.HasMore == true;
            OldestTurnId = cached.OldestTurnId;
            SessionLoading = false;
            PendingSessionId = null;
            WatchSession(id, epoch);
            RecoverActiveTurn(cached);
        }
        else
        {
            var target = Sessions.Find(s => s.SessionId == id);
            _ = target;
            ApplyEmptyStory();
            SessionLoading = true;
            PendingSessionId = id;
        }

        try
        {
            var branch = ActiveBranchId;
            var view = await _api.GetSessionAsync(id, branchId: branch, viewNodeId: ViewNodeId, ct: ct).ConfigureAwait(false);
            if (epoch != _navigationEpoch) return;
            SessionViewCache.Set(id, view);
            SessionMemory.RememberSession(id);
            var thinking = TurnThinking;
            View = view;
            Messages = MessagesFromNodes(view, thinking);
            Hud = view.State;
            ActiveCharacterId = view.CharacterId;
            ActiveBranchId = view.Branch.BranchId;
            HasMoreHistory = view.HasMore == true;
            OldestTurnId = view.OldestTurnId;
            SessionLoading = false;
            PendingSessionId = null;
            WatchSession(id, epoch);
            RecoverActiveTurn(view);
            await LoadMemoriesAsync(ct: ct).ConfigureAwait(false);
        }
        catch (Exception e)
        {
            var running = Phase is TurnPhase.Generating or TurnPhase.Truncated or TurnPhase.AwaitingConfirm && LastTurnId is not null;
            if (epoch == _navigationEpoch)
            {
                Error = ToErr(e);
                if (!running) Phase = TurnPhase.Failed;
                SessionLoading = false;
                PendingSessionId = null;
            }
        }
    }

    public async Task DeleteSessionAsync(string id, CancellationToken ct = default)
    {
        try
        {
            await _api.DeleteSessionAsync(id, ct).ConfigureAwait(false);
        }
        catch
        {
            try { await LoadSessionsAsync(ct).ConfigureAwait(false); } catch { /* keep original error */ }
            throw;
        }
        var activeId = View?.SessionId ?? PendingSessionId;
        var deletingActive = activeId == id;
        if (deletingActive)
        {
            BeginNavigation(null);
            ApplyEmptyStory();
            SessionMemory.RememberSession(null);
        }
        SessionViewCache.Clear();
        try
        {
            await LoadSessionsAsync(ct).ConfigureAwait(false);
        }
        catch
        {
            return;
        }
        if (!deletingActive) return;
        var next = Sessions.FirstOrDefault();
        if (next is not null)
            await OpenSessionAsync(next.SessionId, ct).ConfigureAwait(false);
    }

    public async Task<CreateSessionResult> CreateSessionAsync(CreateSessionOpts opts, CancellationToken ct = default)
    {
        var owner = Scope();
        var payload = JsonSerializer.Serialize(opts, JsonDefaults.Options);
        if (_pendingSessionPayload != payload || _pendingSessionEpoch != owner.Epoch)
        {
            _pendingSessionPayload = payload;
            _pendingSessionEpoch = owner.Epoch;
            _pendingSessionKey = opts.IdempotencyKey ?? Guid.NewGuid().ToString();
        }
        var requestKey = _pendingSessionKey!;
        Error = null;
        Phase = TurnPhase.Generating;
        try
        {
            var req = new CreateSessionRequest
            {
                IdempotencyKey = requestKey,
                Title = opts.Title,
                CardId = opts.CardId,
                CharacterJson = opts.CharacterJson,
                PlayerName = opts.PlayerName,
                PlayerRole = opts.PlayerRole,
                OpeningVariantId = opts.OpeningVariantId,
                OpeningText = opts.OpeningText,
            };
            var created = await _api.CreateSessionAsync(req, ct).ConfigureAwait(false);
            if (_pendingSessionKey == requestKey) { _pendingSessionPayload = null; _pendingSessionKey = null; }
            if (Owns(owner)) await OpenSessionAsync(created.SessionId, ct).ConfigureAwait(false);
            try { await LoadSessionsAsync(ct).ConfigureAwait(false); } catch { /* non-fatal */ }
            return created;
        }
        catch (Exception e)
        {
            if (Owns(owner))
            {
                Phase = TurnPhase.Idle;
                Error = ToErr(e);
            }
            throw;
        }
    }

    public void ClearPackError() => PackError = null;

    private sealed class SessionViewCache
    {
        private static readonly object Lock = new();
        private static readonly Dictionary<string, SessionView> Map = new();
        public const int Capacity = 12;

        public static SessionView? TryGet(string id)
        {
            lock (Lock) return Map.TryGetValue(id, out var v) ? v : null;
        }

        public static void Set(string id, SessionView view)
        {
            lock (Lock)
            {
                Map.Remove(id);
                Map[id] = view;
                while (Map.Count > Capacity)
                {
                    var first = Map.Keys.First();
                    Map.Remove(first);
                }
            }
        }

        public static void Clear()
        {
            lock (Lock) Map.Clear();
        }
    }

    private void WatchSession(string sessionId, int epoch)
    {
        DetachSessionWatch();
        _watchSessionId = sessionId;
        _watchEpoch = epoch;
        _sessionStalled = false;
        var sameStory = () => _navigationEpoch == epoch && View?.SessionId == sessionId;

        void Subscribe()
        {
            _sessionSub = _api.SubscribeSessionEvents(sessionId, new SseHandlers
            {
                OnEvent = ev =>
                {
                    if (_navigationEpoch != epoch || View?.SessionId != sessionId) return;
                    if (ev.Event != "session.updated") return;
                    if (ev.Data.ValueKind == System.Text.Json.JsonValueKind.Object &&
                        ev.Data.TryGetProperty("branchId", out var b))
                    {
                        var branchId = b.GetString();
                        if (branchId != ActiveBranchId) return;
                    }
                    if (BusyPredicates.IsTurnBusy(PhaseWire))
                    {
                        RequestTurnSync();
                        return;
                    }
                    _ = ReloadViewAsync();
                },
                OnFinalError = _ => { _sessionStalled = true; _sessionSub = null; },
            });
        }

        Subscribe();
        ResumeWatch = () =>
        {
            if (!_sessionStalled || !sameStory()) return;
            _sessionStalled = false;
            Subscribe();
            _ = ReloadViewAsync();
        };
    }

    internal Action? ResumeWatch { get; private set; }

    private void DetachSessionWatch()
    {
        _sessionSub?.Dispose();
        _sessionSub = null;
        ResumeWatch = null;
        _watchSessionId = null;
        _watchEpoch = -1;
        _sessionStalled = false;
    }
}
