using TavernAgent.Core.Stores;
using Xunit;

namespace TavernAgent.Core.Tests;

/// <summary>
/// The host (Avalonia) installs <see cref="StoreDispatcher.Post"/> so store
/// notifications raised from SSE/timer threads are marshalled to the UI thread.
/// These tests pin that contract and the headless (inline) fallback.
/// </summary>
public class StoreDispatcherTests
{
    [Fact]
    public void Notifications_AreMarshalled_WhenHostHookIsSet()
    {
        var queued = new List<Action>();
        StoreDispatcher.Post = queued.Add;
        try
        {
            var store = new UiStore();
            var raised = 0;
            store.PropertyChanged += (_, _) => raised++;

            store.SetTheme(ThemeMode.Light);

            Assert.Equal(0, raised);      // not raised inline
            Assert.Single(queued);
            queued[0]();                  // "pump" the UI thread
            Assert.Equal(1, raised);
        }
        finally
        {
            StoreDispatcher.Post = null;
        }
    }

    [Fact]
    public void Notifications_FireInline_WithoutHost()
    {
        StoreDispatcher.Post = null;
        var store = new UiStore();
        var raised = 0;
        store.PropertyChanged += (_, _) => raised++;

        store.SetTheme(ThemeMode.Light);

        Assert.Equal(1, raised);
    }
}
