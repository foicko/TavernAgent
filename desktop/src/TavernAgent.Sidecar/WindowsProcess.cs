using System.ComponentModel;
using System.Runtime.InteropServices;
using Microsoft.Win32.SafeHandles;

namespace TavernAgent.Sidecar;

/// <summary>
/// Windows process launcher matching scripts/runtime_smoke.py:
/// CREATE_NEW_CONSOLE | CREATE_NEW_PROCESS_GROUP (hidden) so the child owns a
/// console process group and can receive a targeted CTRL_BREAK, with stdout/stderr
/// on anonymous pipes for the sidecar log.
/// </summary>
internal static partial class WindowsProcess
{
    private const uint CREATE_NEW_CONSOLE = 0x00000010;
    private const uint CREATE_NEW_PROCESS_GROUP = 0x00000200;
    private const uint STARTF_USESHOWWINDOW = 0x00000001;
    private const uint STARTF_USESTDHANDLES = 0x00000100;
    private const short SW_HIDE = 0;
    private const uint PIPE_READMODE_BYTE = 0x00000000;
    private const uint PIPE_WAIT = 0x00000000;
    private const uint GENERIC_READ = 0x80000000;
    private const uint GENERIC_WRITE = 0x40000000;
    private const uint INFINITE = 0xFFFFFFFF;

    public sealed class SpawnedProcess : IDisposable
    {
        public required int ProcessId { get; init; }
        public required SafeProcessHandle ProcessHandle { get; init; }
        public required SafeFileHandle StdoutRead { get; init; }
        public required SafeFileHandle StderrRead { get; init; }
        public Stream StdoutStream { get; init; } = Stream.Null;
        public Stream StderrStream { get; init; } = Stream.Null;

        public void Dispose()
        {
            StdoutStream.Dispose();
            StderrStream.Dispose();
            StdoutRead.Dispose();
            StderrRead.Dispose();
            ProcessHandle.Dispose();
        }
    }

    public static SpawnedProcess Spawn(
        string executable,
        IReadOnlyList<string> args,
        string workingDirectory,
        IDictionary<string, string?>? environment = null)
    {
        var (stdoutRead, stdoutWrite, stderrRead, stderrWrite, stdinRead, stdinWrite) = CreateStdPipes();

        var sa = new SECURITY_ATTRIBUTES { nLength = Marshal.SizeOf<SECURITY_ATTRIBUTES>(), bInheritHandle = true };
        var si = new STARTUPINFOW
        {
            cb = Marshal.SizeOf<STARTUPINFOW>(),
            dwFlags = STARTF_USESHOWWINDOW | STARTF_USESTDHANDLES,
            wShowWindow = SW_HIDE,
            hStdInput = stdinRead.DangerousGetHandle(),
            hStdOutput = stdoutWrite.DangerousGetHandle(),
            hStdError = stderrWrite.DangerousGetHandle(),
        };

        var commandLine = new System.Text.StringBuilder(BuildCommandLine(executable, args));
        var currentDir = string.IsNullOrEmpty(workingDirectory) ? null : workingDirectory;

        uint flags = CREATE_NEW_CONSOLE | CREATE_NEW_PROCESS_GROUP;

        bool ok = CreateProcessW(
            lpApplicationName: executable,
            lpCommandLine: commandLine,
            lpProcessAttributes: ref sa,
            lpThreadAttributes: ref sa,
            bInheritHandles: true,
            dwCreationFlags: flags,
            lpEnvironment: IntPtr.Zero,
            lpCurrentDirectory: currentDir,
            lpStartupInfo: ref si,
            lpProcessInformation: out var pi);

        // Parent must close write ends so ReadLine sees EOF when child exits.
        stdoutWrite.Dispose();
        stderrWrite.Dispose();
        stdinWrite.Dispose();
        stdinRead.Dispose();

        if (!ok)
        {
            stdoutRead.Dispose();
            stderrRead.Dispose();
            var code = Marshal.GetLastWin32Error();
            var sys = new Win32Exception(code).Message;
            throw new Win32Exception(code, $"CreateProcess failed for {executable} (win32={code}: {sys}) cmdline={commandLine}");
        }

        CloseHandle(pi.hThread);
        var processHandle = new SafeProcessHandle(pi.hProcess, ownsHandle: true);

        return new SpawnedProcess
        {
            ProcessId = pi.dwProcessId,
            ProcessHandle = processHandle,
            StdoutRead = stdoutRead,
            StderrRead = stderrRead,
            StdoutStream = new FileStream(stdoutRead, FileAccess.Read, bufferSize: 4096, isAsync: false),
            StderrStream = new FileStream(stderrRead, FileAccess.Read, bufferSize: 4096, isAsync: false),
        };
    }

    public static bool IsAlive(SafeProcessHandle handle, out int exitCode)
    {
        exitCode = -1;
        if (GetExitCodeProcess(handle, out var code))
        {
            const uint STILL_ACTIVE = 259;
            if (code != STILL_ACTIVE)
            {
                exitCode = (int)code;
                return false;
            }
            return true;
        }
        return false;
    }

    public static bool WaitForExit(SafeProcessHandle handle, int millisecondsTimeout)
        => WaitForSingleObject(handle, (uint)millisecondsTimeout) == 0;

    public static void Kill(SafeProcessHandle handle)
    {
        TerminateProcess(handle, 1);
    }

    /// <summary>
    /// Creates the three std pipes atomically: if any CreatePipe fails, the handles
    /// already created are disposed so the process never leaks them.
    /// </summary>
    private static (SafeFileHandle OutRead, SafeFileHandle OutWrite,
                    SafeFileHandle ErrRead, SafeFileHandle ErrWrite,
                    SafeFileHandle InRead, SafeFileHandle InWrite) CreateStdPipes()
    {
        var outRead = CreatePipe(out var outWrite, inheritRead: false);
        try
        {
            var errRead = CreatePipe(out var errWrite, inheritRead: false);
            try
            {
                var inRead = CreatePipe(out var inWrite, inheritRead: true);
                return (outRead, outWrite, errRead, errWrite, inRead, inWrite);
            }
            catch
            {
                errRead.Dispose();
                errWrite.Dispose();
                throw;
            }
        }
        catch
        {
            outRead.Dispose();
            outWrite.Dispose();
            throw;
        }
    }

    private static SafeFileHandle CreatePipe(out SafeFileHandle write, bool inheritRead)
    {
        var sa = new SECURITY_ATTRIBUTES
        {
            nLength = Marshal.SizeOf<SECURITY_ATTRIBUTES>(),
            bInheritHandle = true,
        };
        if (!CreatePipe(out var read, out write, ref sa, 0))
            throw new Win32Exception(Marshal.GetLastWin32Error(), "CreatePipe failed");

        if (!inheritRead)
        {
            // Ensure read end is NOT inherited; write end is.
            if (!SetHandleInformation(read, HANDLE_FLAG_INHERIT, 0))
            {
                read.Dispose();
                write.Dispose();
                throw new Win32Exception(Marshal.GetLastWin32Error(), "SetHandleInformation failed");
            }
        }
        return read;
    }

    private static string BuildCommandLine(string executable, IReadOnlyList<string> args)
    {
        var sb = new System.Text.StringBuilder();
        sb.Append(Quote(executable));
        foreach (var a in args)
        {
            sb.Append(' ').Append(Quote(a));
        }
        return sb.ToString();
    }

    private static string Quote(string arg)
    {
        if (arg.Length > 0 && !arg.Any(c => char.IsWhiteSpace(c) || c == '"'))
            return arg;

        // CommandLineToArgvW rules: backslashes are literal except immediately
        // before a quote (doubled) or before the closing quote (doubled).
        var sb = new System.Text.StringBuilder("\"");
        for (var i = 0; i < arg.Length; i++)
        {
            var backslashes = 0;
            while (i < arg.Length && arg[i] == '\\') { backslashes++; i++; }

            if (i == arg.Length)
            {
                sb.Append('\\', backslashes * 2);
                break;
            }

            if (arg[i] == '"')
            {
                sb.Append('\\', backslashes * 2 + 1);
                sb.Append('"');
            }
            else
            {
                sb.Append('\\', backslashes);
                sb.Append(arg[i]);
            }
        }
        sb.Append('"');
        return sb.ToString();
    }

    private const uint HANDLE_FLAG_INHERIT = 0x00000001;

    [StructLayout(LayoutKind.Sequential)]
    private struct SECURITY_ATTRIBUTES
    {
        public int nLength;
        public IntPtr lpSecurityDescriptor;
        [MarshalAs(UnmanagedType.Bool)] public bool bInheritHandle;
    }

    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    private struct STARTUPINFOW
    {
        public int cb;
        public IntPtr lpReserved;
        public IntPtr lpDesktop;
        public IntPtr lpTitle;
        public int dwX;
        public int dwY;
        public int dwXSize;
        public int dwYSize;
        public int dwXCountChars;
        public int dwYCountChars;
        public int dwFillAttribute;
        public uint dwFlags;
        public short wShowWindow;
        public short cbReserved2;
        public IntPtr lpReserved2;
        public IntPtr hStdInput;
        public IntPtr hStdOutput;
        public IntPtr hStdError;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct PROCESS_INFORMATION
    {
        public IntPtr hProcess;
        public IntPtr hThread;
        public int dwProcessId;
        public int dwThreadId;
    }

    [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Unicode, EntryPoint = "CreateProcessW")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool CreateProcessW(
        string? lpApplicationName,
        System.Text.StringBuilder lpCommandLine,
        ref SECURITY_ATTRIBUTES lpProcessAttributes,
        ref SECURITY_ATTRIBUTES lpThreadAttributes,
        [MarshalAs(UnmanagedType.Bool)] bool bInheritHandles,
        uint dwCreationFlags,
        IntPtr lpEnvironment,
        string? lpCurrentDirectory,
        ref STARTUPINFOW lpStartupInfo,
        out PROCESS_INFORMATION lpProcessInformation);

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool CreatePipe(
        out SafeFileHandle hReadPipe,
        out SafeFileHandle hWritePipe,
        ref SECURITY_ATTRIBUTES lpPipeAttributes,
        uint nSize);

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool SetHandleInformation(SafeFileHandle hObject, uint dwMask, uint dwFlags);

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool CloseHandle(IntPtr hObject);

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool GetExitCodeProcess(SafeProcessHandle hProcess, out uint lpExitCode);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern uint WaitForSingleObject(SafeProcessHandle hHandle, uint dwMilliseconds);

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool TerminateProcess(SafeProcessHandle hProcess, uint uExitCode);
}

internal sealed class SafeProcessHandle : SafeHandleZeroOrMinusOneIsInvalid
{
    public SafeProcessHandle() : base(true) { }

    public SafeProcessHandle(IntPtr existingHandle, bool ownsHandle) : base(ownsHandle)
    {
        SetHandle(existingHandle);
    }

    protected override bool ReleaseHandle() => CloseNativeHandle(handle);

    [DllImport("kernel32.dll", SetLastError = true, EntryPoint = "CloseHandle")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool CloseNativeHandle(IntPtr hObject);
}
