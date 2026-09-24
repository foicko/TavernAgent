using TavernAgent.Core.Api;
using TavernAgent.Core.Dtos;

namespace TavernAgent.Core.Stores;

// Branch slice: switch / viewAt / back / regenerate / edit / fork / loadOlder
public sealed partial class StoryStore
{
    private async Task<bool> NavigateViewAsync(Func<StoryStore, Task> apply, string label, CancellationToken ct)
    {
        var before = (ActiveBranchId, ViewNodeId, Phase, Error, Memories, MemoryUsage, MemoryError);
        try
        {
            await apply(this).ConfigureAwait(false);
            await ReloadViewAsync(ct).ConfigureAwait(false);
            return true;
        }
        catch (Exception e)
        {
            ActiveBranchId = before.ActiveBranchId;
            ViewNodeId = before.ViewNodeId;
            Phase = before.Phase;
            Error = $"{label}失败，已还原到原位置：{ToErr(e)}";
            Memories = before.Memories;
            MemoryUsage = before.MemoryUsage;
            MemoryError = before.MemoryError;
            return false;
        }
    }

    public async Task SwitchBranchAsync(string branchId, CancellationToken ct = default)
    {
        if (BusyPredicates.IsTurnBusy(PhaseWire)) return;
        var moved = await NavigateViewAsync(s =>
        {
            s.ResetDirector();
            s.LastTurnId = null;
            s.Phase = TurnPhase.Idle;
            s.ActiveBranchId = branchId;
            s.ViewNodeId = null;
            s.DraftBlocks = new();
            s.DraftOptions = new();
            s.Error = null;
            s.Memories = new();
            s.MemoryUsage = null;
            s.MemoryError = null;
            return Task.CompletedTask;
        }, "切换分支", ct).ConfigureAwait(false);
        if (!moved) return;
        var fresh = View;
        if (fresh?.Branch.BranchId == branchId) RecoverActiveTurn(fresh);
    }

    public Task ViewAtAsync(string nodeId, CancellationToken ct = default)
    {
        if (BusyPredicates.IsTurnBusy(PhaseWire)) return Task.CompletedTask;
        return NavigateViewAsync(s =>
        {
            s.ResetDirector();
            s.ViewNodeId = nodeId;
            s.Error = null;
            s.Memories = new();
            s.MemoryUsage = null;
            s.MemoryError = null;
            return Task.CompletedTask;
        }, "查看历史节点", ct);
    }

    public Task BackToHeadAsync(CancellationToken ct = default)
        => NavigateViewAsync(s =>
        {
            s.ResetDirector();
            s.ViewNodeId = null;
            s.Error = null;
            s.Memories = new();
            s.MemoryUsage = null;
            s.MemoryError = null;
            return Task.CompletedTask;
        }, "返回当前进度", ct);

    public async Task RegenerateAsync(string nodeId, bool recheck = false, string? guidance = null, CancellationToken ct = default)
    {
        var owner = Scope();
        var view = View;
        if (view is null || BusyPredicates.IsTurnBusy(PhaseWire)) return;
        Phase = TurnPhase.Generating;
        Error = null;
        try
        {
            var note = (guidance ?? "").Trim();
            var freq = InteractionSettingsPersisted.OptionsFrequency;
            var res = await _api.RegenerateTurnAsync(view.SessionId, view.Branch.BranchId, new RegenerateRequest
            {
                NodeId = nodeId,
                ExpectedCharacterId = view.CharacterId,
                IdempotencyKey = $"rg_{DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()}_{++_sendSeq}",
                Recheck = recheck,
                Note = string.IsNullOrEmpty(note) ? null : note,
                Options = freq == OptionsFrequency.Auto ? null : freq.ToString().ToLowerInvariant(),
            }, ct).ConfigureAwait(false);
            if (!Owns(owner)) return;
            await OpenDerivedBranchAsync(view.SessionId, res.BranchId, res.TurnId, ct).ConfigureAwait(false);
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

    public async Task EditPlayerInputAsync(string nodeId, string text, CancellationToken ct = default)
    {
        var owner = Scope();
        var view = View;
        if (view is null || BusyPredicates.IsTurnBusy(PhaseWire) || string.IsNullOrWhiteSpace(text)) return;
        Phase = TurnPhase.Generating;
        Error = null;
        try
        {
            var resJson = await _api.EditTurnAsync(view.SessionId, view.Branch.BranchId, new EditTurnRequest
            {
                NodeId = nodeId,
                ExpectedCharacterId = view.CharacterId,
                Input = new TurnSubmitInput { Kind = "text", Text = text.Trim() },
                IdempotencyKey = $"ed_{DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()}_{++_sendSeq}",
            }, ct).ConfigureAwait(false);
            var res = System.Text.Json.JsonSerializer.Deserialize<DeriveResult>(resJson, JsonDefaults.Options);
            if (res is null) throw new InvalidOperationException("服务端未返回有效的分支派生结果");
            if (!Owns(owner)) return;
            await OpenDerivedBranchAsync(view.SessionId, res.BranchId, res.TurnId, ct).ConfigureAwait(false);
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

    public async Task EditAssistantTextAsync(string nodeId, List<TextBlock> blocks, CancellationToken ct = default)
    {
        var owner = Scope();
        var view = View;
        if (view is null || BusyPredicates.IsTurnBusy(PhaseWire)) return;
        Error = null;
        try
        {
            var resJson = await _api.EditTurnAsync(view.SessionId, view.Branch.BranchId, new EditTurnRequest
            {
                NodeId = nodeId,
                ExpectedCharacterId = view.CharacterId,
                Blocks = blocks,
                IdempotencyKey = $"ea_{DateTimeOffset.UtcNow.ToUnixTimeMilliseconds()}_{++_sendSeq}",
            }, ct).ConfigureAwait(false);
            var res = System.Text.Json.JsonSerializer.Deserialize<AssistantEditResult>(resJson, JsonDefaults.Options);
            if (res is null) throw new InvalidOperationException("服务端未返回有效的正文编辑结果");
            if (!Owns(owner)) return;
            await OpenDerivedBranchAsync(view.SessionId, res.BranchId, null, ct).ConfigureAwait(false);
        }
        catch (Exception e)
        {
            if (Owns(owner)) Error = ToErr(e);
        }
    }

    public async Task ForkFromAsync(string nodeId, string? name = null, CancellationToken ct = default)
    {
        var owner = Scope();
        var view = View;
        if (view is null || BusyPredicates.IsTurnBusy(PhaseWire)) return;
        try
        {
            var res = await _api.ForkBranchAsync(view.SessionId, new ForkBranchRequest
            {
                FromNodeId = nodeId,
                Name = name,
                ExpectedCharacterId = view.CharacterId,
            }, ct).ConfigureAwait(false);
            if (!Owns(owner)) return;
            await OpenDerivedBranchAsync(view.SessionId, res.Branch.BranchId, null, ct).ConfigureAwait(false);
        }
        catch (Exception e)
        {
            if (Owns(owner)) Error = ToErr(e);
        }
    }

    public async Task LoadOlderAsync(CancellationToken ct = default)
    {
        var owner = Scope();
        var ticket = _reloadSeq;
        var view = View;
        if (view is null || !HasMoreHistory || LoadingOlder) return;
        LoadingOlder = true;
        try
        {
            var older = await _api.GetSessionAsync(view.SessionId, branchId: ActiveBranchId ?? view.Branch.BranchId, before: OldestTurnId, viewNodeId: ViewNodeId, ct: ct).ConfigureAwait(false);
            if (!Owns(owner) || ticket != _reloadSeq) return;
            var olderMessages = MessagesFromNodes(older, TurnThinking).Where(m => m.Role != "opening").ToList();
            var existing = Messages.Select(m => m.Id).ToHashSet();
            Messages = olderMessages.Where(m => !existing.Contains(m.Id)).Concat(Messages).ToList();
            HasMoreHistory = older.HasMore == true;
            OldestTurnId = older.OldestTurnId;
            LoadingOlder = false;
        }
        catch (Exception e)
        {
            if (Owns(owner))
            {
                LoadingOlder = false;
                Error = "加载历史消息失败：" + ToErr(e);
            }
        }
    }

    internal async Task OpenDerivedBranchAsync(string sessionId, string? branchId, string? turnId, CancellationToken ct)
    {
        ResetDirector();
        if (branchId is not null) ActiveBranchId = branchId;
        ViewNodeId = null;
        LastTurnId = turnId;
        Phase = turnId is not null ? TurnPhase.Generating : TurnPhase.Idle;
        PendingInput = null;
        Memories = new();
        MemoryUsage = null;
        MemoryError = null;
        MemoryLoading = false;
        MemoryPaging = MemoryPaging.Empty();
        Error = null;
        var owner = Scope();
        try
        {
            await ReloadViewAsync(ct).ConfigureAwait(false);
        }
        catch (Exception e)
        {
            if (Owns(owner)) Error = ToErr(e);
        }
        if (turnId is not null && Owns(owner) && LastTurnId == turnId)
            AttachTurnStream(sessionId, turnId);
    }

    private void ResetDirector()
    {
        DirectorView = null;
        DirectorLoading = false;
        DirectorSaving = false;
        DirectorCommanding = false;
        DirectorError = null;
        DirectorScope = "";
        DetachDirectorWatch();
    }
}
