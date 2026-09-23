using System.ComponentModel;

namespace TavernAgent.Desktop.Services;

/// <summary>Unsubscribes the exact wrapped delegate created by <see cref="UiEvents"/>.</summary>
public sealed class UiSubscription : IDisposable
{
    private Action? _unsubscribe;

    public UiSubscription(Action unsubscribe) => _unsubscribe = unsubscribe;

    public void Dispose()
    {
        var action = Interlocked.Exchange(ref _unsubscribe, null);
        action?.Invoke();
    }
}

/// <summary>
/// Event subscription helpers that guarantee the handler runs on the UI thread.
///
/// A raw <c>+= OnX</c> on a store that raises from SSE/timer threads lets the
/// handler mutate bound collections off-thread, which crashes Avalonia. Always
/// subscribe through here (and keep the returned token to unsubscribe).
/// </summary>
public static class UiEvents
{
    public static IDisposable PropertyChanged(INotifyPropertyChanged source, PropertyChangedEventHandler handler)
    {
        PropertyChangedEventHandler wrapped = (s, e) => UiDispatcher.Post(() => handler(s, e));
        source.PropertyChanged += wrapped;
        return new UiSubscription(() => source.PropertyChanged -= wrapped);
    }

    public static IDisposable Event(Action<EventHandler> add, Action<EventHandler> remove, EventHandler handler)
    {
        EventHandler wrapped = (s, e) => UiDispatcher.Post(() => handler(s, e));
        add(wrapped);
        return new UiSubscription(() => remove(wrapped));
    }
}
