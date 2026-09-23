using TavernAgent.Sidecar;

namespace TavernAgent.Desktop.Services;

/// <summary>
/// Best-effort error log under &lt;dataDir&gt;/logs/desktop-error.log.
/// Shared by the process-level handlers and background task helpers so a failure
/// always leaves a trace instead of vanishing with the process.
/// </summary>
public static class CrashLog
{
    private static readonly object Gate = new();

    public static void Write(Exception? ex, string source)
    {
        try
        {
            var dir = Path.Combine(DesktopPaths.DataDir, "logs");
            Directory.CreateDirectory(dir);
            lock (Gate)
            {
                File.AppendAllText(
                    Path.Combine(dir, "desktop-error.log"),
                    $"[{DateTime.Now:O}] {source}: {ex}{Environment.NewLine}{Environment.NewLine}");
            }
        }
        catch
        {
            // never let logging take down the process
        }
    }
}
