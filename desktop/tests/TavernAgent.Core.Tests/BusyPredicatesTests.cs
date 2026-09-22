using TavernAgent.Core.Stores;
using Xunit;

namespace TavernAgent.Core.Tests;

public class BusyPredicatesTests
{
    private static readonly string[] Phases = { "idle", "generating", "truncated", "awaiting_confirm", "committed", "failed" };

    private static BusyState State(string phase, string? viewNodeId = null, bool directorCommanding = false)
        => new(phase, viewNodeId, directorCommanding);

    [Fact]
    public void Composer_And_Director_Gates_Stay_In_Step_With_Story_Gate()
    {
        foreach (var phase in Phases)
        {
            foreach (var (view, cmd) in new[] { ((string?)null, false), ((string?)null, true), ("n1", false) })
            {
                var s = State(phase, view, cmd);
                if (BusyPredicates.ComposerBusy(s))
                    Assert.True(BusyPredicates.StoryBusy(s) || BusyPredicates.HistoryReadOnly(s));
                if (BusyPredicates.DirectorBusy(s))
                    Assert.True(BusyPredicates.StoryBusy(s) || BusyPredicates.HistoryReadOnly(s));
                if (BusyPredicates.HistoryReadOnly(s))
                    Assert.True(BusyPredicates.ComposerBusy(s) && BusyPredicates.DirectorBusy(s));
                if (BusyPredicates.IsTurnBusy(phase))
                    Assert.True(BusyPredicates.StoryBusy(s) && BusyPredicates.ComposerBusy(s));
            }
        }
    }

    [Fact]
    public void Unfinished_Phases_Are_Busy_And_History_Is_ReadOnly()
    {
        var busyPhases = Phases.Where(BusyPredicates.IsTurnBusy).ToArray();
        Assert.Equal(new[] { "generating", "truncated", "awaiting_confirm", "committed" }, busyPhases);
        Assert.True(BusyPredicates.StoryBusy(State("idle", directorCommanding: true)));
        Assert.False(BusyPredicates.ComposerBusy(State("idle")));
        Assert.False(BusyPredicates.DirectorBusy(State("idle")));
    }

    [Fact]
    public void Does_Not_Block_Normal_Composing_While_Idle_At_Head()
    {
        Assert.False(BusyPredicates.HistoryReadOnly(State("idle")));
        Assert.False(BusyPredicates.ComposerBusy(State("failed")));
    }
}
