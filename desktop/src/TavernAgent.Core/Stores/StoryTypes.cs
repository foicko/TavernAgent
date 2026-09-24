using System.Text.Json;
using TavernAgent.Core.Api;
using TavernAgent.Core.Dtos;

namespace TavernAgent.Core.Stores;

public enum TurnPhase { Idle, Generating, Truncated, AwaitingConfirm, Failed, Committed }

public sealed class OptionView
{
    public string OptionId { get; set; } = "";
    public string Intent { get; set; } = "chaotic";
    public string Text { get; set; } = "";
}

public sealed class StoryMessage
{
    public string Id { get; set; } = "";
    public string? ParentId { get; set; }
    public string Role { get; set; } = "assistant"; // opening | user | assistant
    public List<TextBlock> Blocks { get; set; } = new();
    public List<OptionView> Options { get; set; } = new();
    public bool Draft { get; set; }
    public string? InputText { get; set; }
    public string? InputNote { get; set; }
    public List<CheckResult>? Checks { get; set; }
    public List<StateChange>? Changes { get; set; }
    public List<MemoryRef>? InjectedMemories { get; set; }
    public int? SuppressedOptions { get; set; }
    /// <summary>本轮从正文归一化出的卡片 UI 标记（hud/scene/var）。</summary>
    public List<UIHint>? UiHints { get; set; }
    public int? TurnNumber { get; set; }
    public string? Thinking { get; set; }
}

public sealed class TurnInput
{
    public string Kind { get; set; } = "text";
    public string Text { get; set; } = "";
    public string? Note { get; set; }
    public string? Options { get; set; }
    public string? ActionRef { get; set; }
    public OptionRef? OptionRef { get; set; }
}

public sealed class MemoryPaging
{
    public string Search { get; set; } = "";
    public string Kind { get; set; } = "all";
    public int Index { get; set; }
    public List<string> Cursors { get; set; } = new() { "" };
    public string? NextCursor { get; set; }
    public string? NodeId { get; set; }
    public int Total { get; set; }
    public Dictionary<string, int> Counts { get; set; } = new();

    public static MemoryPaging Empty() => new();
}

public sealed class CreateSessionOpts
{
    public string? IdempotencyKey { get; set; }
    public string? Title { get; set; }
    public string? CardId { get; set; }
    public string? CharacterJson { get; set; }
    public string PlayerName { get; set; } = "";
    public string? PlayerRole { get; set; }
    public string? OpeningVariantId { get; set; }
    public string? OpeningText { get; set; }
}
