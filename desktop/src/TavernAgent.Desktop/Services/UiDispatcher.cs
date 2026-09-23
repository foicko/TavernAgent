using Avalonia.Threading;

namespace TavernAgent.Desktop.Services;

/// <summary>
/// Single entry point for marshalling work onto the UI thread.
///
/// Every store/event subscription that can be raised from a background thread
/// (SSE, timers, TTS playback) must go through here. Mutating a bound
/// <c>ObservableCollection</c> off-thread crashes Avalonia's container generator
/// with "Collection was modified; enumeration operation may not execute", so this
/// is the one place that guarantees thread affinity.
/// </summary>
public static class UiDispatcher
{
    public static bool IsOnUiThread => Dispatcher.UIThread.CheckAccess();

    /// <summary>Run on the UI thread now if already there, else post it.</summary>
    public static void Post(Action action)
    {
        if (action is null) return;
        if (Dispatcher.UIThread.CheckAccess()) action();
        else Dispatcher.UIThread.Post(action);
    }

    public static void Post(Action action, DispatcherPriority priority)
    {
        if (action is null) return;
        if (Dispatcher.UIThread.CheckAccess()) action();
        else Dispatcher.UIThread.Post(action, priority);
    }

    /// <summary>Awaitable variant for callers that need to observe completion.</summary>
    public static Task InvokeAsync(Action action)
    {
        if (action is null) return Task.CompletedTask;
        if (Dispatcher.UIThread.CheckAccess())
        {
            action();
            return Task.CompletedTask;
        }
        return Dispatcher.UIThread.InvokeAsync(action).GetTask();
    }
}
