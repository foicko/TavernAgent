using TavernAgent.Core.Stores;
using Xunit;

namespace TavernAgent.Core.Tests;

/// <summary>
/// Regression cover for the memory-bank paging arithmetic. The desktop port used
/// to assign past the end of <c>List&lt;string&gt;</c> on "next" (Web relied on JS
/// arrays auto-extending), which threw and left pagination stuck on page 0.
/// </summary>
public class MemoryPagingTests
{
    [Fact]
    public void Next_From_First_Page_Pads_Cursor_Chain()
    {
        var previous = new MemoryPaging { Search = "", Kind = "all", Index = 0, Cursors = new() { "" }, NextCursor = "c2" };

        var plan = StoryStore.PlanMemoryPage(previous, "next", "", "all");

        Assert.True(plan.Ok);
        Assert.Equal(1, plan.Index);
        Assert.Equal(new[] { "", "c2" }, plan.Cursors);
        Assert.True(plan.ReusePreviousNode);
    }

    [Fact]
    public void Next_From_Later_Page_Appends_Cursor()
    {
        var previous = new MemoryPaging { Index = 1, Cursors = new() { "", "c2" }, NextCursor = "c3" };

        var plan = StoryStore.PlanMemoryPage(previous, "next", "", "all");

        Assert.True(plan.Ok);
        Assert.Equal(2, plan.Index);
        Assert.Equal(new[] { "", "c2", "c3" }, plan.Cursors);
    }

    [Fact]
    public void Next_Without_Cursor_Is_Rejected()
    {
        var previous = new MemoryPaging { Index = 0, Cursors = new() { "" }, NextCursor = null };

        var plan = StoryStore.PlanMemoryPage(previous, "next", "", "all");

        Assert.False(plan.Ok);
    }

    [Fact]
    public void Previous_Truncates_Cursor_Chain()
    {
        var previous = new MemoryPaging { Index = 2, Cursors = new() { "", "c2", "c3" }, NextCursor = "c4" };

        var plan = StoryStore.PlanMemoryPage(previous, "previous", "", "all");

        Assert.True(plan.Ok);
        Assert.Equal(1, plan.Index);
        Assert.Equal(new[] { "", "c2" }, plan.Cursors);
    }

    [Fact]
    public void New_Query_Resets_To_First_Page()
    {
        var previous = new MemoryPaging { Search = "old", Kind = "all", Index = 3, Cursors = new() { "", "a", "b", "c" } };

        var plan = StoryStore.PlanMemoryPage(previous, null, "new", "all");

        Assert.True(plan.Ok);
        Assert.Equal(0, plan.Index);
        Assert.Equal(new[] { "" }, plan.Cursors);
        Assert.False(plan.ReusePreviousNode);
    }

    [Fact]
    public void Same_Query_Without_Page_Is_Anchored()
    {
        var previous = new MemoryPaging { Search = "key", Kind = "observed", Index = 2, Cursors = new() { "", "a", "b" }, NodeId = "node-1" };

        var plan = StoryStore.PlanMemoryPage(previous, null, "key", "observed");

        Assert.True(plan.Ok);
        Assert.Equal(2, plan.Index);
        Assert.Equal(new[] { "", "a", "b" }, plan.Cursors);
        Assert.True(plan.ReusePreviousNode);
    }
}
