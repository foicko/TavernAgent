using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Text;

namespace TavernAgent.Sidecar;

/// <summary>
/// Unified handle over a spawned sidecar child.
/// Windows: CREATE_NEW_CONSOLE|CREATE_NEW_PROCESS_GROUP + targeted CTRL_BREAK
/// (same sequence as scripts/runtime_smoke.py).
/// Unix: Process + SIGTERM.
/// </summary>
internal sealed class SidecarChild : IDisposable
{
    private readonly Process? _process;
    private readonly WindowsProcess.SpawnedProcess? _win;
    private readonly JobObjectGuard? _job;
    private bool _disposed;

    private SidecarChild(Process? process, WindowsProcess.SpawnedProcess? win, JobObjectGuard? job)
    {
        _process = process;
        _win = win;
        _job = job;
    }

    public int Id => _win?.ProcessId ?? _process?.Id ?? -1;

    public StreamReader StdoutReader { get; private set; } = StreamReader.Null;

    public StreamReader StderrReader { get; private set; } = StreamReader.Null;

    public bool HasExited
    {
        get
        {
            if (_disposed) return true;
            if (_win is not null)
                return !WindowsProcess.IsAlive(_win.ProcessHandle, out _);
            try { return _process?.HasExited ?? true; } catch { return true; }
        }
    }

    public int ExitCode
    {
        get
        {
            if (_disposed) return -1;
            if (_win is not null)
            {
                WindowsProcess.IsAlive(_win.ProcessHandle, out var code);
                return code;
            }
            try { return _process?.ExitCode ?? -1; } catch { return -1; }
        }
    }

    public static SidecarChild Spawn(SidecarOptions options, string dataDir, string addr)
    {
        var args = new List<string> { "-addr", addr, "-data", dataDir };
        if (!string.IsNullOrEmpty(options.Provider))
        {
            args.Add("-provider");
            args.Add(options.Provider);
        }
        foreach (var a in options.ExtraArgs) args.Add(a);

        var workDir = Path.GetDirectoryName(options.ExecutablePath) ?? Environment.CurrentDirectory;

        if (OperatingSystem.IsWindows())
        {
            var spawned = WindowsProcess.Spawn(options.ExecutablePath, args, workDir);
            var job = JobObjectGuard.TryCreate();
            if (job is not null)
            {
                var assigned = job.TryAssign(spawned.ProcessHandle.DangerousGetHandle())
                    || job.TryAssign(spawned.ProcessId);
                if (!assigned)
                {
                    job.Dispose();
                    job = null;
                }
            }
            return new SidecarChild(null, spawned, job)
            {
                StdoutReader = new StreamReader(spawned.StdoutStream, Encoding.UTF8, detectEncodingFromByteOrderMarks: false, bufferSize: 4096, leaveOpen: true),
                StderrReader = new StreamReader(spawned.StderrStream, Encoding.UTF8, detectEncodingFromByteOrderMarks: false, bufferSize: 4096, leaveOpen: true),
            };
        }

        var psi = new ProcessStartInfo
        {
            FileName = options.ExecutablePath,
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            StandardOutputEncoding = Encoding.UTF8,
            StandardErrorEncoding = Encoding.UTF8,
            WorkingDirectory = workDir,
        };
        foreach (var a in args) psi.ArgumentList.Add(a);

        var process = new Process { StartInfo = psi, EnableRaisingEvents = true };
        if (!process.Start())
            throw new InvalidOperationException("Failed to start sidecar process.");
        return new SidecarChild(process, null, null)
        {
            StdoutReader = process.StandardOutput,
            StderrReader = process.StandardError,
        };
    }

    public bool TrySendGracefulStop()
    {
        if (_disposed || HasExited) return true;
        var id = Id;
        // Never signal pid <= 0: on Unix kill(-1, SIGTERM) would signal every
        // process the user can signal.
        if (id <= 0) return false;
        if (OperatingSystem.IsWindows())
            return GracefulStopWindows.TrySendBreak(id);
        try
        {
            return kill(id, SIGTERM) == 0;
        }
        catch
        {
            return false;
        }
    }

    public void Kill()
    {
        try
        {
            if (_win is not null)
            {
                if (!HasExited) WindowsProcess.Kill(_win.ProcessHandle);
                WindowsProcess.WaitForExit(_win.ProcessHandle, 3000);
                return;
            }
            if (_process is { HasExited: false })
                _process.Kill(entireProcessTree: true);
        }
        catch
        {
            // already gone
        }
    }

    public async Task WaitForExitAsync(TimeSpan timeout)
    {
        var deadline = DateTime.UtcNow + timeout;
        while (DateTime.UtcNow < deadline)
        {
            if (HasExited) return;
            if (_win is not null)
            {
                WindowsProcess.WaitForExit(_win.ProcessHandle, 100);
                continue;
            }
            try
            {
                if (_process is null) return;
                var wait = _process.WaitForExitAsync().WaitAsync(TimeSpan.FromMilliseconds(100));
                await wait.ConfigureAwait(false);
                return;
            }
            catch (TimeoutException)
            {
                // keep polling
            }
            catch
            {
                return;
            }
        }
    }

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        _job?.Dispose();
        try { StdoutReader.Dispose(); } catch { /* ignore */ }
        try { StderrReader.Dispose(); } catch { /* ignore */ }
        _win?.Dispose();
        _process?.Dispose();
    }

    private const int SIGTERM = 15;

    [DllImport("libc", EntryPoint = "kill", SetLastError = true)]
    private static extern int kill(int pid, int sig);
}

/// <summary>
/// Windows CTRL_BREAK delivery — same sequence as scripts/runtime_smoke.py:
/// FreeConsole → AttachConsole(child) → SetConsoleCtrlHandler(NULL, TRUE)
/// → GenerateConsoleCtrlEvent(CTRL_BREAK, childPid) → FreeConsole.
/// </summary>
internal static class GracefulStopWindows
{
    private const uint CTRL_BREAK_EVENT = 1;

    public static bool TrySendBreak(int processId)
    {
        try
        {
            FreeConsole();
            if (!AttachConsole(unchecked((uint)processId)))
                return false;
            SetConsoleCtrlHandler(null, true);
            try
            {
                return GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, unchecked((uint)processId));
            }
            finally
            {
                // Restore default Ctrl+C handling — never leave it ignored.
                SetConsoleCtrlHandler(null, false);
            }
        }
        catch
        {
            return false;
        }
        finally
        {
            FreeConsole();
        }
    }

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool FreeConsole();

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool AttachConsole(uint dwProcessId);

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool SetConsoleCtrlHandler(ConsoleCtrlHandlerRoutine? handlerRoutine, [MarshalAs(UnmanagedType.Bool)] bool add);

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool GenerateConsoleCtrlEvent(uint dwCtrlEvent, uint dwProcessGroupId);

    private delegate bool ConsoleCtrlHandlerRoutine(uint ctrlType);
}
