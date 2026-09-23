using System.Runtime.InteropServices;

namespace TavernAgent.Sidecar;

/// <summary>
/// Cross-process single-instance guard. Windows uses a named mutex (same
/// ERROR_ALREADY_EXISTS predicate as the old Wails shell). Other platforms use
/// a lock file under the data directory.
/// Must be acquired BEFORE starting the sidecar: the data-dir OS lock would
/// otherwise fire first and show a confusing "directory in use" error.
/// </summary>
public sealed class SingleInstance : IDisposable
{
    public const string MutexName = "TavernAgent-desktop-single-instance-mutex";

    private Mutex? _mutex;
    private FileStream? _lockFile;
    private bool _acquired;

    private SingleInstance() { }

    public bool Acquired => _acquired;

    /// <summary>Try to become the primary instance. When false, another instance is running.</summary>
    public static SingleInstance TryAcquire(string dataDir, string? mutexName = null)
    {
        var si = new SingleInstance();
        if (OperatingSystem.IsWindows())
        {
            var name = string.IsNullOrWhiteSpace(mutexName) ? MutexName : mutexName;
            try
            {
                var mutex = new Mutex(false, name);
                bool acquired = false;
                try
                {
                    acquired = mutex.WaitOne(0, false);
                }
                catch (AbandonedMutexException)
                {
                    // Previous owner exited without releasing the mutex.
                    // Windows grants ownership to this thread.
                    acquired = true;
                }

                if (!acquired)
                {
                    mutex.Dispose();
                    return si;
                }
                si._mutex = mutex;
                si._acquired = true;
                return si;
            }
            catch
            {
                return si;
            }
        }

        try
        {
            Directory.CreateDirectory(dataDir);
            var path = Path.Combine(dataDir, ".desktop-instance.lock");
            var fs = new FileStream(path, FileMode.OpenOrCreate, FileAccess.ReadWrite, FileShare.None);
            si._lockFile = fs;
            si._acquired = true;
        }
        catch (IOException)
        {
            si._acquired = false;
        }
        return si;
    }

    /// <summary>
    /// Windows: find the peer window and bring it forward.
    /// No-op elsewhere (and when Acquired).
    /// </summary>
    public void ActivateExisting()
    {
        if (_acquired || !OperatingSystem.IsWindows()) return;
        TryActivateWindows();
    }

    public void Dispose()
    {
        _lockFile?.Dispose();
        _lockFile = null;
        if (_mutex is not null)
        {
            if (_acquired)
            {
                try { _mutex.ReleaseMutex(); } catch { /* ignore */ }
            }
            _mutex.Dispose();
            _mutex = null;
        }
        _acquired = false;
    }

    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    private static extern IntPtr FindWindowW(string? lpClassName, string lpWindowName);

    [DllImport("user32.dll")]
    private static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);

    [DllImport("user32.dll")]
    private static extern bool SetForegroundWindow(IntPtr hWnd);

    [DllImport("user32.dll")]
    private static extern bool BringWindowToTop(IntPtr hWnd);

    private static void TryActivateWindows()
    {
        // Title must match MainWindow title exactly.
        var hwnd = FindWindowW(null, "TavernAgent");
        if (hwnd == IntPtr.Zero) return;
        const int SW_RESTORE = 9;
        ShowWindow(hwnd, SW_RESTORE);
        BringWindowToTop(hwnd);
        SetForegroundWindow(hwnd);
    }
}
