using Avalonia;
using Avalonia.Threading;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.Services;
using TavernAgent.Sidecar;

namespace TavernAgent.Desktop;

internal static class Program
{
    private static int _uiErrorBurst;

    // Avalonia configuration, don't remove; also used by visual designer.
    [STAThread]
    public static void Main(string[] args)
    {
        AppDomain.CurrentDomain.UnhandledException += (_, e) => CrashLog.Write(e.ExceptionObject as Exception, "AppDomain");
        TaskScheduler.UnobservedTaskException += (_, e) => { CrashLog.Write(e.Exception, "Task"); e.SetObserved(); };
        // Last line of defence: without this, any UI-thread exception silently
        // terminates the process (the reported 闪退).
        Dispatcher.UIThread.UnhandledException += OnUiThreadException;
        // Route every Core store notification through the UI thread so background
        // SSE/timer updates can never mutate bound state off-thread.
        StoreDispatcher.Post = UiDispatcher.Post;
        try
        {
            // Single instance must win BEFORE starting Avalonia or sidecar.
            // If another instance is running, activate its window and exit cleanly.
            using var singleInstance = SingleInstance.TryAcquire(DesktopPaths.DataDir);
            if (!singleInstance.Acquired)
            {
                singleInstance.ActivateExisting();
                return;
            }

            BuildAvaloniaApp().StartWithClassicDesktopLifetime(args);
        }
        catch (Exception ex)
        {
            CrashLog.Write(ex, "Main");
            throw;
        }
    }

    public static AppBuilder BuildAvaloniaApp()
        => AppBuilder.Configure<App>()
            .UsePlatformDetect()
            .LogToTrace();

    /// <summary>
    /// Keep the app alive on an otherwise fatal UI-thread exception: log it and
    /// surface a single quiet notice instead of letting the process die.
    /// </summary>
    private static void OnUiThreadException(object? sender, DispatcherUnhandledExceptionEventArgs e)
    {
        CrashLog.Write(e.Exception, "UIThread");
        e.Handled = true;

        // Rate-limit the toast so a repeated render-loop failure cannot spam it.
        if (Interlocked.Increment(ref _uiErrorBurst) > 5) return;
        try { AppServices.Ui.NotifyQuiet("发生了一个内部错误，已记录到日志。"); }
        catch { /* never throw from the handler */ }
    }
}
