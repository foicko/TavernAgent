using System.Text.Json;
using TavernAgent.Core.Api;
using TavernAgent.Core.Dtos;

namespace TavernAgent.Core.Stores;

// Memory slice
public sealed partial class StoryStore
{
    public async Task LoadMemoriesAsync(
        string? search = null,
        string? kind = null,
        string? page = null,
        CancellationToken ct = default)
    {
        var owner = Scope();
        var ticket = ++_memorySeq;
        var view = View;
        if (view is null)
        {
            Memories = new();
            MemoryUsage = null;
            MemoryError = null;
            MemoryLoading = false;
            MemoryPaging = MemoryPaging.Empty();
            return;
        }
        var previous = MemoryPaging;
        var effectiveSearch = search ?? previous.Search;
        var effectiveKind = kind ?? previous.Kind;
        var sameQuery = effectiveSearch == previous.Search && effectiveKind == previous.Kind;
        var moving = sameQuery && (page is "next" or "previous");
        var anchored = sameQuery && page is null && previous.Index > 0;
        if (moving && page == "next" && string.IsNullOrEmpty(previous.NextCursor)) return;
        var index = moving
            ? Math.Max(0, previous.Index + (page == "next" ? 1 : -1))
            : anchored ? previous.Index : 0;
        var cursors = moving || anchored ? previous.Cursors.Take(index + 1).ToList() : new List<string> { "" };
        if (moving && page == "next") cursors[index] = previous.NextCursor!;
        var nodeId = moving || anchored ? previous.NodeId : ViewNodeId;
        MemoryLoading = true;
        MemoryError = null;
        try
        {
            var res = await _api.ListMemoriesAsync(
                view.SessionId, view.Branch.BranchId, nodeId,
                search: string.IsNullOrEmpty(effectiveSearch) ? null : effectiveSearch,
                kind: effectiveKind is "" or "all" ? null : effectiveKind,
                cursor: cursors[index],
                limit: 40,
                ct: ct).ConfigureAwait(false);
            if (Owns(owner) && ticket == _memorySeq)
            {
                Memories = res.Memories;
                MemoryUsage = res.Usage;
                MemoryError = null;
                MemoryPaging = new MemoryPaging
                {
                    Search = effectiveSearch,
                    Kind = effectiveKind,
                    Index = index,
                    Cursors = cursors,
                    NextCursor = res.NextCursor,
                    NodeId = res.NodeId ?? nodeId,
                    Total = res.Total ?? res.Memories.Count,
                    Counts = res.Counts ?? new Dictionary<string, int>(),
                };
            }
        }
        catch (Exception error)
        {
            if (Owns(owner) && ticket == _memorySeq) MemoryError = ToErr(error);
        }
        finally
        {
            if (Owns(owner) && ticket == _memorySeq) MemoryLoading = false;
        }
    }

    public async Task ReviseMemoryAsync(string memoryId, string? content, bool? pinned, bool? hidden, CancellationToken ct = default)
    {
        var owner = Scope();
        var view = View;
        if (view is null || BusyPredicates.IsTurnBusy(PhaseWire) || ViewNodeId is not null || MemoryLoading || MemoryError is not null) return;
        var payload = JsonSerializer.Serialize(new { sessionId = view.SessionId, branchId = view.Branch.BranchId, head = view.Branch.HeadNodeId, memoryId, content, pinned, hidden }, JsonDefaults.Options);
        if (_pendingMemoryPayload != payload)
        {
            _pendingMemoryPayload = payload;
            _pendingMemoryKey = Guid.NewGuid().ToString();
        }
        var requestKey = _pendingMemoryKey!;
        await _api.PatchMemoryAsync(view.SessionId, view.Branch.BranchId, memoryId, new PatchMemoryRequest
        {
            Content = content,
            Pinned = pinned,
            Hidden = hidden,
            ExpectedCharacterId = view.CharacterId,
            ExpectedHeadId = view.Branch.HeadNodeId,
            ExpectedVersion = view.Branch.Version,
            IdempotencyKey = requestKey,
        }, ct).ConfigureAwait(false);
        if (_pendingMemoryKey == requestKey) { _pendingMemoryPayload = null; _pendingMemoryKey = null; }
        if (Owns(owner))
        {
            var search = MemoryPaging.Search;
            var kind = MemoryPaging.Kind;
            MemoryPaging = new MemoryPaging { Search = search, Kind = kind };
            await ReloadViewAsync(ct).ConfigureAwait(false);
        }
    }

    public async Task OrganizeMemoriesAsync(CancellationToken ct = default)
    {
        var owner = Scope();
        var view = View;
        if (view is null || BusyPredicates.IsTurnBusy(PhaseWire) || ViewNodeId is not null || MemoryLoading || MemoryError is not null) return;
        try
        {
            await _api.OrganizeMemoriesAsync(view.SessionId, view.Branch.BranchId, view.CharacterId, ct).ConfigureAwait(false);
            if (Owns(owner)) await ReloadViewAsync(ct).ConfigureAwait(false);
        }
        catch (Exception e)
        {
            if (Owns(owner)) Error = ToErr(e);
        }
    }
}
