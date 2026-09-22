namespace TavernAgent.Core.Stores;

/// <summary>
/// Single source of truth for turn busy / read-only gates.
/// Relationships are locked by BusyPredicatesTests (parity with busyPredicates.test.ts).
/// </summary>
public readonly record struct BusyState(string Phase, string? ViewNodeId, bool DirectorCommanding);

public static class BusyPredicates
{
    /// <summary>A committed turn remains read-only until its new head and options are loaded.</summary>
    public static bool IsTurnBusy(string phase)
        => phase is "generating" or "truncated" or "awaiting_confirm" or "committed";

    /// <summary>Viewing a historical node: input and most writes are disabled.</summary>
    public static bool HistoryReadOnly(BusyState state)
        => !string.IsNullOrEmpty(state.ViewNodeId);

    /// <summary>Story-level busy: turn in flight or director command in flight.</summary>
    public static bool StoryBusy(BusyState state)
        => IsTurnBusy(state.Phase) || state.DirectorCommanding;

    public static bool ComposerBusy(BusyState state)
        => HistoryReadOnly(state) || StoryBusy(state);

    public static bool DirectorBusy(BusyState state)
        => HistoryReadOnly(state) || StoryBusy(state);
}
