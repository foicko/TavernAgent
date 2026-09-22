using CommunityToolkit.Mvvm.ComponentModel;

namespace TavernAgent.Desktop.ViewModels.Palette;

public static class CommandGroups
{
    public static readonly string[] Order = { "navigation", "story", "view", "panel" };

    public static string Label(string group) => group switch
    {
        "navigation" => "跳转",
        "story" => "剧情",
        "view" => "视图",
        "panel" => "面板",
        _ => group,
    };

    public static int Rank(string group)
    {
        var index = Array.IndexOf(Order, group);
        return index < 0 ? Order.Length : index;
    }
}

/// <summary>One invocable palette entry (web: lib/commands Command).</summary>
public sealed partial class CommandItem : ObservableObject
{
    public CommandItem(string id, string group, string title, Func<Task> run,
        Func<bool>? enabled = null, string? hint = null, string? keywords = null)
    {
        Id = id;
        Group = group;
        Title = title;
        Run = run;
        Enabled = enabled;
        Hint = hint;
        Keywords = keywords ?? "";
    }

    public string Id { get; }
    public string Group { get; }
    public string Title { get; }
    public string? Hint { get; }
    public string Keywords { get; }
    public Func<Task> Run { get; }
    public Func<bool>? Enabled { get; }

    public string GroupLabel => CommandGroups.Label(Group);
    public bool IsEnabled => Enabled?.Invoke() ?? true;
    public bool HasHint => !string.IsNullOrEmpty(Hint);

    [ObservableProperty] private bool _isActive;
}
