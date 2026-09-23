namespace TavernAgent.Core.Stores;

/// <summary>
/// Optional host hook so a UI layer (the Avalonia desktop) can marshal every
/// store notification onto its UI thread.
///
/// Core raises <c>PropertyChanged</c>/<c>StateChanged</c> from SSE, poll timers and
/// HTTP continuations. Without marshalling, a subscriber that mutates a bound
/// collection crashes Avalonia's container generator ("Collection was modified").
/// When <see cref="Post"/> is unset (headless tests), notifications fire inline.
/// </summary>
public static class StoreDispatcher
{
    /// <summary>Set by the desktop host to <c>UiDispatcher.Post</c>.</summary>
    public static Action<Action>? Post { get; set; }

    public static void Dispatch(Action action)
    {
        if (action is null) return;
        var post = Post;
        if (post is null) action();
        else post(action);
    }
}
