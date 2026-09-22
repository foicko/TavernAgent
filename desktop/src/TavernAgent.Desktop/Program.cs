using Avalonia;
using TavernAgent.Sidecar;

namespace TavernAgent.Desktop;

internal static class Program
{
    // Avalonia configuration, don't remove; also used by visual designer.
    [STAThread]
    public static void Main(string[] args)
    {
        AppDomain.CurrentDomain.UnhandledException += (_, e) => LogCrash(e.ExceptionObject as Exception, "AppDomain");
        TaskScheduler.UnobservedTaskException += (_, e) => { LogCrash(e.Exception, "Task"); e.SetObserved(); };
        try
        {
            BuildAvaloniaApp().StartWithClassicDesktopLifetime(args);
        }
        catch (Exception ex)
        {
            LogCrash(ex, "Main");
            throw;
        }
    }

    public static AppBuilder BuildAvaloniaApp()
        => AppBuilder.Configure<App>()
            .UsePlatformDetect()
            .LogToTrace();

    /// <summary>Best-effort crash log under &lt;dataDir&gt;/logs/desktop-error.log.</summary>
    private static void LogCrash(Exception? ex, string source)
    {
        try
        {
            var dir = Path.Combine(DesktopPaths.DataDir, "logs");
            Directory.CreateDirectory(dir);
            File.AppendAllText(
                Path.Combine(dir, "desktop-error.log"),
                $"[{DateTime.Now:O}] {source}: {ex}{Environment.NewLine}{Environment.NewLine}");
        }
        catch
        {
            // never let logging take down the process
        }
    }
}
