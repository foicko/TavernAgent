using System.Text.Json;
using TavernAgent.Core.Api;
using TavernAgent.Core.Dtos;

namespace TavernAgent.Core.Stores;

// Director slice
public sealed partial class StoryStore
{
    public const int DirectorPollMs = 3000;
    private string? _directorWatchRequestId;

    public static string DirectorScopeKey(string sessionId, string branchId, string? nodeId)
        => $"{sessionId}/{branchId}/{(string.IsNullOrEmpty(nodeId) ? "head" : nodeId)}";

    private void DetachDirectorWatch()
    {
        _directorSub?.Dispose();
        _directorSub = null;
        _directorPollTimer?.Dispose();
        _directorPollTimer = null;
        _directorWatchRequestId = null;
    }

    private void AttachDirectorRequest(string requestId)
    {
        if (string.IsNullOrEmpty(requestId) || _directorWatchRequestId == requestId) return;
        DetachDirectorWatch();
        var owner = Scope();
        var checking = false;
        var finished = false;

        async Task ReconcileAsync()
        {
            if (finished || checking) return;
            if (!Owns(owner)) { finished = true; DetachDirectorWatch(); return; }
            checking = true;
            try
            {
                var latest = await _api.GetDirectorRequestAsync(requestId).ConfigureAwait(false);
                if (!Owns(owner)) return;
                if (latest.Status != "generating")
                {
                    finished = true;
                    DetachDirectorWatch();
                    if (DirectorView is not null)
                    {
                        foreach (var r in DirectorView.Requests.Where(r => r.RequestId == requestId).ToList())
                            r.Status = latest.Status;
                        OnPropertyChanged(nameof(DirectorView));
                    }
                    try { await LoadDirectorAsync().ConfigureAwait(false); } catch { /* terminal already written */ }
                    return;
                }
            }
            catch
            {
                // SSE and polling recover independently
            }
            finally
            {
                checking = false;
            }
        }

        _directorSub = _api.SubscribeDirectorEvents(requestId, new SseHandlers
        {
            OnEvent = ev => { if (ev.Event == "director.started") { _ = ReconcileAsync(); } },
            OnReconnect = attempt => { _ = ReconcileAsync(); },
            OnFinalError = err => { _ = ReconcileAsync(); },
        });
        _directorPollTimer = new System.Threading.Timer(_ => _ = ReconcileAsync(), null, DirectorPollMs, DirectorPollMs);
        _directorWatchRequestId = requestId;
        _ = ReconcileAsync();
    }

    public async Task LoadDirectorAsync(CancellationToken ct = default)
    {
        var view = View;
        if (view is null)
        {
            ResetDirector();
            return;
        }
        var owner = Scope();
        var ticket = ++_directorReadSeq;
        var key = DirectorScopeKey(view.SessionId, view.Branch.BranchId, ViewNodeId);
        DirectorLoading = true;
        DirectorError = null;
        if (DirectorScope != key)
        {
            DirectorView = null;
            DirectorScope = key;
        }
        try
        {
            var result = await _api.GetDirectorAsync(view.SessionId, view.Branch.BranchId, ViewNodeId, ct).ConfigureAwait(false);
            if (Owns(owner) && ticket == _directorReadSeq)
            {
                var saved = DirectorScope == key ? DirectorView?.Draft : null;
                if (!result.ReadOnly && saved is not null && saved.Version > (result.Draft?.Version ?? 0))
                    result.Draft = saved;
                DirectorView = result;
                var running = result.Requests.FirstOrDefault(r => r.Status == "generating");
                if (running is not null) AttachDirectorRequest(running.RequestId);
            }
        }
        catch (Exception error)
        {
            if (Owns(owner) && ticket == _directorReadSeq) DirectorError = ToErr(error);
        }
        finally
        {
            if (Owns(owner) && ticket == _directorReadSeq) DirectorLoading = false;
        }
    }

    public async Task<DirectorDraft> SaveDirectorDraftAsync(DirectorPlan plan, int version, string baseRevisionId, CancellationToken ct = default)
    {
        var view = View;
        var owner = Scope();
        if (view is null || ViewNodeId is not null) throw new Exception("历史视图只读，请回到当前节点或分叉");
        _directorReadSeq++;
        DirectorSaving = true;
        DirectorError = null;
        try
        {
            var draft = await _api.SaveDirectorDraftAsync(view.SessionId, view.Branch.BranchId, new SaveDraftRequest
            {
                ExpectedCharacterId = view.CharacterId,
                ExpectedDraftVersion = version,
                BaseRevisionId = baseRevisionId,
                Plan = plan,
            }, ct).ConfigureAwait(false);
            if (Owns(owner) && DirectorView is not null)
            {
                if ((DirectorView.Draft?.Version ?? 0) <= draft.Version)
                    DirectorView.Draft = draft;
            }
            return draft;
        }
        catch (Exception error)
        {
            if (Owns(owner)) DirectorError = ToErr(error);
            throw;
        }
        finally
        {
            if (Owns(owner))
            {
                DirectorSaving = false;
                DirectorLoading = false;
            }
        }
    }

    public async Task CommandDirectorAsync(string action, string? beatId = null, bool? replace = null, CancellationToken ct = default)
    {
        var view = View;
        var owner = Scope();
        if (view is null || ViewNodeId is not null || BusyPredicates.IsTurnBusy(PhaseWire) || DirectorCommanding) return;
        var body = new DirectorCommandRequest
        {
            ExpectedCharacterId = view.CharacterId,
            ExpectedHeadId = view.Branch.HeadNodeId,
            ExpectedVersion = view.Branch.Version,
            Action = action,
            BeatId = beatId,
            Replace = replace,
            DraftVersion = DirectorView?.Draft?.Version,
        };
        var payload = JsonSerializer.Serialize(new { sessionId = view.SessionId, branchId = view.Branch.BranchId, body = new { action, beatId, replace, draftVersion = body.DraftVersion } }, JsonDefaults.Options);
        if (_commandPayload != payload)
        {
            _commandPayload = payload;
            _commandKey = Guid.NewGuid().ToString();
        }
        var key = _commandKey!;
        DirectorCommanding = true;
        DirectorError = null;
        try
        {
            body.IdempotencyKey = key;
            await _api.DirectorCommandAsync(view.SessionId, view.Branch.BranchId, body, ct).ConfigureAwait(false);
            if (_commandKey == key) _commandKey = null;
            if (Owns(owner))
            {
                await ReloadViewAsync(ct).ConfigureAwait(false);
                if (Owns(owner)) await LoadDirectorAsync(ct).ConfigureAwait(false);
            }
        }
        catch (Exception error)
        {
            if (Owns(owner)) DirectorError = ToErr(error);
            throw;
        }
        finally
        {
            if (Owns(owner)) DirectorCommanding = false;
        }
    }

    public async Task DiscussDirectorAsync(string text, CancellationToken ct = default)
    {
        var view = View;
        var owner = Scope();
        if (view is null || ViewNodeId is not null || string.IsNullOrWhiteSpace(text)) return;
        var body = new DirectorMessageRequest
        {
            ExpectedCharacterId = view.CharacterId,
            ExpectedHeadId = view.Branch.HeadNodeId,
            ExpectedDraftVersion = DirectorView?.Draft?.Version ?? 0,
            Text = text.Trim(),
        };
        var payload = JsonSerializer.Serialize(new { sessionId = view.SessionId, branchId = view.Branch.BranchId, body = new { body.ExpectedHeadId, body.Text } }, JsonDefaults.Options);
        if (_messagePayload != payload)
        {
            _messagePayload = payload;
            _messageKey = Guid.NewGuid().ToString();
        }
        var key = _messageKey!;
        DirectorError = null;
        try
        {
            body.IdempotencyKey = key;
            var result = await _api.DirectorMessageAsync(view.SessionId, view.Branch.BranchId, body, ct).ConfigureAwait(false);
            if (_messageKey == key) _messageKey = null;
            if (Owns(owner))
            {
                if (DirectorView is not null)
                {
                    DirectorView.Requests.RemoveAll(r => r.RequestId == result.RequestId);
                    DirectorView.Requests.Add(result.Request);
                    OnPropertyChanged(nameof(DirectorView));
                }
                if (result.Request?.Status == "generating") AttachDirectorRequest(result.RequestId);
            }
        }
        catch (Exception error)
        {
            if (Owns(owner)) DirectorError = ToErr(error);
            throw;
        }
    }

    public async Task CancelDirectorAsync(string requestId, CancellationToken ct = default)
    {
        var view = View;
        var owner = Scope();
        if (view is null || ViewNodeId is not null) return;
        try
        {
            await _api.CancelDirectorRequestAsync(requestId, view.CharacterId, ct).ConfigureAwait(false);
            if (Owns(owner)) await LoadDirectorAsync(ct).ConfigureAwait(false);
        }
        catch (Exception error)
        {
            if (Owns(owner)) DirectorError = ToErr(error);
            throw;
        }
    }
}
