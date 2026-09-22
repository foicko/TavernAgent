using System.Text.Json;
using TavernAgent.Core.Api;
using TavernAgent.Core.Dtos;

namespace TavernAgent.Core.Stores;

// Turn slice: send / stream / cancel / continue / retry / reload
public sealed partial class StoryStore
{
    public const int TurnStatusIntervalMs = 5000;

    private string PhaseForBusy => PhaseWire;

    internal void DetachTurnStream(string? turnId = null)
    {
        if (turnId is not null && _activeTurnSubTurnId != turnId) return;
        _turnSub?.Dispose();
        _turnSub = null;
        _activeTurnSubTurnId = null;
        _synchronizeTurnAsync = null;
        _turnPollTimer?.Dispose();
        _turnPollTimer = null;
    }

    internal void RecoverActiveTurn(SessionView view)
    {
        var id = view.Branch.ActiveTurnId;
        if (string.IsNullOrEmpty(id)) return;
        _turnCursorId = null;
        _turnCursorTurnId = null;
        Phase = TurnPhase.Generating;
        LastTurnId = id;
        DraftBlocks = new();
        DraftOptions = new();
        AttachTurnStream(view.SessionId, id, alreadyCommitted: false, snapshotFirst: true);
    }

    private Func<bool, Task>? _synchronizeTurnAsync;

    internal void RequestTurnSync(bool committed = false)
    {
        var sync = _synchronizeTurnAsync;
        if (sync is not null) _ = sync(committed);
    }

    internal void AttachTurnStream(string sessionId, string turnId, bool alreadyCommitted = false, bool snapshotFirst = false)
    {
        DetachTurnStream();
        var owner = Scope();
        var committed = alreadyCommitted;
        _activeTurnSubTurnId = turnId;

        bool Current() =>
            _activeTurnSubTurnId == turnId && Owns(owner) && LastTurnId == turnId;

        if (!committed)
        {
            _turnSub = _api.SubscribeTurnEvents(turnId, new SseHandlers
            {
                LastEventId = _turnCursorTurnId == turnId ? _turnCursorId : null,
                OnEvent = ev =>
                {
                    if (!Current() || committed) return;
                    if (ev.Id.StartsWith(turnId + ":", StringComparison.Ordinal))
                    {
                        _turnCursorTurnId = turnId;
                        _turnCursorId = ev.Id;
                    }
                    RouteEvent(ev, sessionId, turnId, () => DetachTurnStream(turnId));
                },
                OnReconnect = _ => RequestTurnSync(),
                OnFinalError = _ => RequestTurnSync(),
            });
        }

        async Task SynchronizeTurnAsync(bool knownCommitted)
        {
            if (!Current()) return;
            if (knownCommitted) committed = true;
            try
            {
                TurnView? turn = null;
                if (!committed)
                    turn = await _api.GetTurnAsync(turnId).ConfigureAwait(false);
                if (!Current()) return;
                var status = committed ? "committed" : turn?.Status;
                if (string.IsNullOrEmpty(status)) throw new Exception("服务未返回回合状态");
                if (status == "committed")
                {
                    committed = true;
                    DetachTurnStream(turnId);
                    Phase = TurnPhase.Committed;
                    await ReloadViewAsync().ConfigureAwait(false);
                }
                else if (status is "awaiting_continuation" or "awaiting_approval")
                {
                    var phase = status == "awaiting_continuation" ? TurnPhase.Truncated : TurnPhase.AwaitingConfirm;
                    Phase = phase;
                    if (turn?.Input is not null)
                        LastInput = new TurnInput { Kind = turn.Input.Kind, Text = turn.Input.Text, ActionRef = turn.Input.ActionRef, OptionRef = turn.Input.OptionRef };
                }
                else if (status is "failed" or "cancelled" or "conflicted")
                {
                    RouteEvent(new SSEEvent
                    {
                        Event = "turn." + status,
                        Data = JsonDocument.Parse(JsonSerializer.Serialize(new { message = turn?.FailureMessage }, JsonDefaults.Options)).RootElement.Clone(),
                    }, sessionId, turnId, () => DetachTurnStream(turnId));
                }
                else
                {
                    MergeInFlightDraft(turn?.Draft);
                    Phase = TurnPhase.Generating;
                }
            }
            catch (Exception error)
            {
                if (Current()) Error = "回复进度暂时无法同步，正在重试：" + ToErr(error);
            }
        }

        void Schedule(int delayMs)
        {
            _turnPollTimer?.Dispose();
            if (!Current()) return;
            _turnPollTimer = new System.Threading.Timer(_ =>
            {
                _ = SynchronizeTurnAsync(false);
            }, null, delayMs, Timeout.Infinite);
        }

        _synchronizeTurnAsync = SynchronizeTurnAsync;

        if (committed) RequestTurnSync(true);
        else Schedule(snapshotFirst ? 0 : TurnStatusIntervalMs);

        // keep poll loop alive for generating phases
        if (!committed)
        {
            _turnPollTimer ??= new System.Threading.Timer(_ =>
            {
                if (Current() && Phase is TurnPhase.Generating or TurnPhase.Committed)
                    _ = SynchronizeTurnAsync(false);
            }, null, TurnStatusIntervalMs, TurnStatusIntervalMs);
        }
    }

    private void MergeInFlightDraft(TurnDraftSnapshot? draft)
    {
        var inFlight = draft?.InFlight;
        if (inFlight is null || string.IsNullOrEmpty(inFlight.Text)) return;
        var blocks = new List<TextBlock>(DraftBlocks);
        var seq = inFlight.Seq > 0 ? inFlight.Seq : blocks.Count + 1;
        if (seq <= blocks.Count && blocks[seq - 1].Text.Length >= inFlight.Text.Length) return;
        while (blocks.Count < seq - 1) blocks.Add(new TextBlock { Kind = "narration", Text = "" });
        var frame = new TextBlock
        {
            Kind = string.IsNullOrEmpty(inFlight.Kind) ? "narration" : inFlight.Kind,
            SpeakerId = inFlight.SpeakerId,
            Text = inFlight.Text,
        };
        if (seq <= blocks.Count) blocks[seq - 1] = frame;
        else blocks.Add(frame);
        DraftBlocks = blocks;
    }

    internal void RouteEvent(SSEEvent ev, string sessionId, string turnId, Action onEnd)
    {
        if (View?.SessionId != sessionId || LastTurnId != turnId)
        {
            onEnd();
            return;
        }
        switch (ev.Event)
        {
            case "turn.thinking":
            {
                if (ev.Data.ValueKind == JsonValueKind.Object && ev.Data.TryGetProperty("thinking", out var t))
                {
                    var chunk = t.GetString();
                    if (!string.IsNullOrEmpty(chunk))
                    {
                        ThinkingText += chunk;
                        var map = new Dictionary<string, string>(TurnThinking);
                        map.TryGetValue(turnId, out var prev);
                        map[turnId] = (prev ?? "") + chunk;
                        TurnThinking = map;
                    }
                }
                break;
            }
            case "block.delta":
            {
                if (ev.Data.ValueKind != JsonValueKind.Object) break;
                var seq = ev.Data.TryGetProperty("seq", out var s) && s.ValueKind == JsonValueKind.Number ? s.GetInt32() : 1;
                var kind = ev.Data.TryGetProperty("kind", out var k) && k.ValueKind == JsonValueKind.String ? k.GetString() : "narration";
                string? speaker = null;
                if (ev.Data.TryGetProperty("speakerId", out var sp) && sp.ValueKind == JsonValueKind.String) speaker = sp.GetString();
                var delta = ev.Data.TryGetProperty("delta", out var d) ? d.GetString() : null;
                if (string.IsNullOrEmpty(delta)) break;
                var blocks = new List<TextBlock>(DraftBlocks);
                if (seq <= blocks.Count)
                {
                    blocks[seq - 1] = new TextBlock { Kind = blocks[seq - 1].Kind, SpeakerId = blocks[seq - 1].SpeakerId, Text = blocks[seq - 1].Text + delta };
                }
                else
                {
                    while (blocks.Count < seq - 1) blocks.Add(new TextBlock { Kind = "narration", Text = "" });
                    blocks.Add(new TextBlock { Kind = kind ?? "narration", SpeakerId = speaker, Text = delta });
                }
                DraftBlocks = blocks;
                break;
            }
            case "block.appended":
            {
                if (ev.Data.ValueKind != JsonValueKind.Object) break;
                if (!ev.Data.TryGetProperty("frame", out var frameEl)) break;
                var frame = frameEl.Deserialize<TextBlock>(JsonDefaults.Options);
                if (frame is null) break;
                int? seq = null;
                if (ev.Data.TryGetProperty("frameSeq", out var fs) && fs.ValueKind == JsonValueKind.Number) seq = fs.GetInt32();
                else if (frameEl.ValueKind == JsonValueKind.Object && frameEl.TryGetProperty("seq", out var fseq) && fseq.ValueKind == JsonValueKind.Number) seq = fseq.GetInt32();
                var blocks = new List<TextBlock>(DraftBlocks);
                if (seq is > 0 && seq <= blocks.Count) blocks[seq.Value - 1] = frame;
                else blocks.Add(frame);
                DraftBlocks = blocks;
                break;
            }
            case "turn.started":
            case "turn.truncated":
            case "turn.awaiting_continuation":
            case "turn.awaiting_approval":
                RequestTurnSync();
                break;
            case "turn.committed":
                RequestTurnSync(committed: true);
                break;
            case "turn.cancelled":
                onEnd();
                Phase = TurnPhase.Idle;
                DraftBlocks = new();
                DraftOptions = new();
                break;
            case "turn.failed":
            {
                string? msg = null;
                if (ev.Data.ValueKind == JsonValueKind.Object && ev.Data.TryGetProperty("message", out var m) && m.ValueKind == JsonValueKind.String)
                    msg = m.GetString();
                onEnd();
                Phase = TurnPhase.Failed;
                Error = msg ?? "生成失败";
                DraftBlocks = new();
                DraftOptions = new();
                break;
            }
            case "turn.conflicted":
                onEnd();
                Phase = TurnPhase.Failed;
                Error = "分支已变化，请刷新后再试";
                DraftBlocks = new();
                DraftOptions = new();
                break;
        }
    }

    public async Task SendAsync(string text, string? actionRef = null, string? note = null, CancellationToken ct = default)
    {
        if (!string.IsNullOrEmpty(ViewNodeId))
        {
            Error = "历史快照为只读，请先返回最新进度或从此处新建分支";
            return;
        }
        if (string.IsNullOrWhiteSpace(text) || BusyPredicates.IsTurnBusy(PhaseWire) || DirectorCommanding) return;
        var epoch = _navigationEpoch;
        if (View is null)
        {
            Error = "请先打开会话或导入角色卡";
            return;
        }
        var view = View;
        if (view.CharacterId != ActiveCharacterId)
        {
            Error = "当前会话与角色不匹配，请重新打开会话后再发送";
            if (!BusyPredicates.StoryBusy(Busy)) Phase = TurnPhase.Failed;
            return;
        }
        var owner = Scope();
        var trimmedNote = (note ?? "").Trim();
        var optionsFrequency = InteractionSettingsPersisted.OptionsFrequency;
        var input = new TurnInput
        {
            Kind = actionRef is not null ? "action" : "text",
            Text = text,
            ActionRef = actionRef,
            Note = string.IsNullOrEmpty(trimmedNote) ? null : trimmedNote,
            Options = optionsFrequency == OptionsFrequency.Auto ? null : optionsFrequency.ToString().ToLowerInvariant(),
        };
        Phase = TurnPhase.Generating;
        Error = null;
        PendingInput = null;
        LastInput = input;
        try
        {
            await StartTurnAsync(view, $"k_{DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()}_{++_sendSeq}", input, owner, ct).ConfigureAwait(false);
        }
        catch (Exception e)
        {
            if (Owns(owner))
            {
                Phase = TurnPhase.Failed;
                Error = ToErr(e);
            }
        }
    }

    public async Task ChooseOptionAsync(OptionView option, CancellationToken ct = default)
    {
        if (!string.IsNullOrEmpty(ViewNodeId)) return;
        var owner = Scope();
        var view = View;
        if (view is null || BusyPredicates.IsTurnBusy(PhaseWire) || DirectorCommanding) return;
        if (InteractionSettingsPersisted.OptionMode == OptionMode.FillEdit)
        {
            PendingInput = option.Text;
            PendingAction = null;
            return;
        }
        Phase = TurnPhase.Generating;
        Error = null;
        LastInput = new TurnInput { Kind = "option", Text = option.Text };
        var key = $"k_{DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()}_{++_sendSeq}";
        try
        {
            var nodeId = (view.Nodes ?? new List<PlotNode>())
                .Where(n => n.Kind == "turn")
                .Select(n => n.NodeId)
                .LastOrDefault() ?? view.Branch.HeadNodeId;
            await StartTurnAsync(view, key, new TurnInput
            {
                Kind = "option",
                Text = option.Text,
                OptionRef = new OptionRef { NodeId = nodeId, OptionId = option.OptionId },
            }, owner, ct).ConfigureAwait(false);
        }
        catch (Exception e)
        {
            if (Owns(owner))
            {
                Phase = TurnPhase.Failed;
                Error = ToErr(e);
            }
        }
    }

    private async Task StartTurnAsync(SessionView view, string key, TurnInput input, (int Epoch, string? SessionId, string? BranchId, string? NodeId) owner, CancellationToken ct)
    {
        if (!Owns(owner) || owner.SessionId != view.SessionId) return;
        _pendingSubmission = new PendingSubmission(owner.Epoch, view, key, input);
        LastInput = input;
        LastTurnId = null;
        var req = new AcceptTurnRequest
        {
            IdempotencyKey = key,
            ExpectedHeadId = view.Branch.HeadNodeId,
            ExpectedVersion = view.Branch.Version,
            ExpectedCharacterId = view.CharacterId,
            Input = new TurnSubmitInput
            {
                Kind = input.Kind,
                Text = input.Text,
                Note = input.Note,
                Options = input.Options,
                OptionRef = input.OptionRef,
                ActionRef = input.ActionRef,
            },
        };
        var result = await _api.AcceptTurnAsync(view.SessionId, view.Branch.BranchId, req, ct).ConfigureAwait(false);
        if (!Owns(owner)) return;
        if (_pendingSubmission?.Key == key) _pendingSubmission = null;
        LastTurnId = result.TurnId;
        LastInput = input;
        ThinkingText = "";
        DraftBlocks = new();
        DraftOptions = new();
        AttachTurnStream(view.SessionId, result.TurnId);
    }

    public async Task CancelAsync(CancellationToken ct = default)
    {
        var owner = Scope();
        var id = LastTurnId;
        if (id is null) return;
        try
        {
            var result = await _api.CancelTurnAsync(id, View?.CharacterId, ct).ConfigureAwait(false);
            if (!Owns(owner) || LastTurnId != id) return;
            if (result.Status == "committed")
                AttachTurnStream(View!.SessionId, id, alreadyCommitted: true);
            else if (result.Status == "cancelled")
            {
                DetachTurnStream(id);
                Phase = TurnPhase.Idle;
                DraftBlocks = new();
                DraftOptions = new();
                await RefreshViewAsync(ct).ConfigureAwait(false);
            }
        }
        catch (Exception e)
        {
            if (Owns(owner) && LastTurnId == id) Error = ToErr(e);
        }
    }

    public async Task ContinueTurnAsync(CancellationToken ct = default)
    {
        var owner = Scope();
        var id = LastTurnId;
        var view = View;
        if (id is null || view is null || Phase != TurnPhase.Truncated) return;
        DetachTurnStream(id);
        Phase = TurnPhase.Generating;
        Error = null;
        try
        {
            await _api.ContinueTurnAsync(id, view.CharacterId, ct).ConfigureAwait(false);
            if (!Owns(owner) || LastTurnId != id) return;
        }
        catch (Exception e)
        {
            if (Owns(owner) && LastTurnId == id)
            {
                Phase = TurnPhase.Truncated;
                Error = ToErr(e);
                AttachTurnStream(view.SessionId, id);
            }
            return;
        }
        AttachTurnStream(view.SessionId, id);
    }

    public async Task RetryAsync(CancellationToken ct = default)
    {
        var owner = Scope();
        var last = LastInput;
        var view = View;
        var viewNodeId = ViewNodeId;
        var phase = Phase;
        if (last is null || view is null || viewNodeId is not null || phase is TurnPhase.Generating or TurnPhase.Committed) return;
        if (_pendingSubmission is not null && _pendingSubmission.Epoch == owner.Epoch)
        {
            var pending = _pendingSubmission;
            Phase = TurnPhase.Generating;
            Error = null;
            try { await StartTurnAsync(pending.View, pending.Key, pending.Input, owner, ct).ConfigureAwait(false); }
            catch (Exception e)
            {
                if (Owns(owner)) { Phase = TurnPhase.Failed; Error = ToErr(e); }
            }
            return;
        }
        if (LastTurnId is not null)
        {
            var lastTurnIdAtStart = LastTurnId;
            string status;
            try
            {
                status = (await _api.CancelTurnAsync(LastTurnId, view.CharacterId, ct).ConfigureAwait(false)).Status;
            }
            catch (Exception cancelError)
            {
                if (!Owns(owner) || LastTurnId != lastTurnIdAtStart) return;
                try { status = (await _api.GetTurnAsync(LastTurnId, ct).ConfigureAwait(false)).Status; }
                catch
                {
                    if (Owns(owner)) Error = ToErr(cancelError);
                    return;
                }
            }
            if (!Owns(owner) || LastTurnId != lastTurnIdAtStart) return;
            if (status == "committed")
            {
                AttachTurnStream(view.SessionId, LastTurnId, alreadyCommitted: true);
                return;
            }
            if (status is not ("cancelled" or "failed" or "conflicted"))
            {
                Error = "上一回合尚未结束，请等待取消完成后重试";
                AttachTurnStream(view.SessionId, LastTurnId);
                return;
            }
            DetachTurnStream(LastTurnId);
        }
        if (!Owns(owner)) return;
        Phase = TurnPhase.Generating;
        Error = null;
        DraftBlocks = new();
        DraftOptions = new();
        var key = $"retry_{DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()}_{++_sendSeq}";
        try
        {
            await StartTurnAsync(view, key, last, owner, ct).ConfigureAwait(false);
        }
        catch (Exception e)
        {
            if (Owns(owner)) { Phase = TurnPhase.Failed; Error = ToErr(e); }
        }
    }

    public void ClearPending()
    {
        PendingInput = null;
        PendingAction = null;
    }

    public void PrepareAction(string text, string actionRef)
    {
        if (View is null || BusyPredicates.IsTurnBusy(PhaseWire) || ViewNodeId is not null) return;
        if (View.Actions is null || !View.Actions.Any(a => a.ActionId == actionRef)) return;
        PendingInput = text;
        PendingAction = actionRef;
    }

    public async Task ReloadViewAsync(CancellationToken ct = default)
    {
        var owner = Scope();
        var ticket = ++_reloadSeq;
        var view = View;
        if (view is null) return;
        var fresh = await _api.GetSessionAsync(view.SessionId, branchId: ActiveBranchId ?? view.Branch.BranchId, viewNodeId: ViewNodeId, ct: ct).ConfigureAwait(false);
        if (!Owns(owner) || ticket != _reloadSeq) return;
        var completedTurn = Phase == TurnPhase.Committed;
        var completedTurnId = LastTurnId;
        var completedThinking = ThinkingText;
        var turnThinking = new Dictionary<string, string>(TurnThinking);
        if (completedTurn && completedTurnId is not null && !string.IsNullOrEmpty(completedThinking))
        {
            turnThinking[completedTurnId] = completedThinking;
            if (!string.IsNullOrEmpty(fresh.HeadNode?.NodeId)) turnThinking[fresh.HeadNode.NodeId] = completedThinking;
        }
        if (completedTurn) DetachTurnStream(LastTurnId);
        if (ViewNodeId is null) SessionViewCache.Set(fresh.SessionId, fresh);
        View = fresh;
        Hud = fresh.State;
        TurnThinking = turnThinking;
        Messages = MessagesFromNodes(fresh, turnThinking);
        if (Phase is not (TurnPhase.Generating or TurnPhase.Truncated or TurnPhase.AwaitingConfirm) || LastTurnId is null)
        {
            DraftBlocks = new();
            DraftOptions = new();
        }
        if (completedTurn)
        {
            Phase = TurnPhase.Idle;
            ThinkingText = "";
        }
        Error = null;
        PendingInput = null;
        HasMoreHistory = fresh.HasMore == true;
        OldestTurnId = fresh.OldestTurnId;
        LoadingOlder = false;
        await LoadMemoriesAsync(ct: ct).ConfigureAwait(false);
    }

    public async Task RefreshViewAsync(CancellationToken ct = default)
    {
        var owner = Scope();
        var turnId = LastTurnId;
        try
        {
            await ReloadViewAsync(ct).ConfigureAwait(false);
            if (Owns(owner) && LastTurnId == turnId && Phase is not (TurnPhase.Generating or TurnPhase.Truncated or TurnPhase.AwaitingConfirm))
                Phase = TurnPhase.Idle;
        }
        catch (Exception e)
        {
            if (Owns(owner)) Error = ToErr(e);
        }
    }
}

internal static class InteractionSettingsPersisted
{
    private static InteractionSettings? _cached;

    public static InteractionSettings Cached
    {
        get => _cached ??= SettingsPersistence.LoadInteraction();
        set
        {
            _cached = value;
            SettingsPersistence.SaveInteraction(value);
        }
    }

    public static OptionMode OptionMode => Cached.OptionMode;
    public static OptionsFrequency OptionsFrequency => Cached.OptionsFrequency;
    public static bool AutoContinue => Cached.AutoContinue;
}
