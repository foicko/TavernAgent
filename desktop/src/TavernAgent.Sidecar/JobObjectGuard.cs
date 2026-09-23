using System.Runtime.InteropServices;

namespace TavernAgent.Sidecar;

/// <summary>
/// Windows Job Object with KILL_ON_JOB_CLOSE so the sidecar dies with the parent
/// even when the parent is terminated abruptly (task manager).
/// Non-Windows: no-op (Unix relies on SIGTERM path + supervisor kill).
/// </summary>
internal sealed class JobObjectGuard : IDisposable
{
    private IntPtr _handle = IntPtr.Zero;
    private bool _disposed;

    public static JobObjectGuard? TryCreate()
    {
        if (!OperatingSystem.IsWindows()) return null;
        var handle = CreateJobObjectW(IntPtr.Zero, null);
        if (handle == IntPtr.Zero) return null;

        var info = new JOBOBJECT_EXTENDED_LIMIT_INFORMATION
        {
            BasicLimitInformation = new JOBOBJECT_BASIC_LIMIT_INFORMATION
            {
                LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
            },
        };
        var length = Marshal.SizeOf<JOBOBJECT_EXTENDED_LIMIT_INFORMATION>();
        if (!SetInformationJobObject(handle, JobObjectExtendedLimitInformation, ref info, length))
        {
            CloseHandle(handle);
            return null;
        }
        return new JobObjectGuard { _handle = handle };
    }

    public bool TryAssign(IntPtr processHandle)
    {
        if (_disposed || _handle == IntPtr.Zero || processHandle == IntPtr.Zero) return false;
        try
        {
            return AssignProcessToJobObject(_handle, processHandle);
        }
        catch
        {
            return false;
        }
    }

    public bool TryAssign(int processId)
    {
        if (_disposed || _handle == IntPtr.Zero) return false;
        // PROCESS_SET_QUOTA (0x0100) | PROCESS_TERMINATE (0x0001) is the access required by AssignProcessToJobObject.
        const uint PROCESS_SET_QUOTA = 0x0100;
        const uint PROCESS_TERMINATE = 0x0001;
        var process = OpenProcess(PROCESS_SET_QUOTA | PROCESS_TERMINATE, false, processId);
        if (process == IntPtr.Zero)
        {
            process = OpenProcess(0x001F0FFF, false, processId);
            if (process == IntPtr.Zero) return false;
        }
        try
        {
            return AssignProcessToJobObject(_handle, process);
        }
        finally
        {
            CloseHandle(process);
        }
    }

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_handle != IntPtr.Zero)
        {
            CloseHandle(_handle);
            _handle = IntPtr.Zero;
        }
    }

    private const int JobObjectExtendedLimitInformation = 9;
    private const uint JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE = 0x2000;

    [StructLayout(LayoutKind.Sequential)]
    private struct IO_COUNTERS
    {
        public ulong ReadOperationCount;
        public ulong WriteOperationCount;
        public ulong OtherOperationCount;
        public ulong ReadTransferCount;
        public ulong WriteTransferCount;
        public ulong OtherTransferCount;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct JOBOBJECT_BASIC_LIMIT_INFORMATION
    {
        public long PerProcessUserTimeLimit;
        public long PerJobUserTimeLimit;
        public uint LimitFlags;
        public UIntPtr MinimumWorkingSetSize;
        public UIntPtr MaximumWorkingSetSize;
        public uint ActiveProcessLimit;
        public UIntPtr Affinity;
        public uint PriorityClass;
        public uint SchedulingClass;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct JOBOBJECT_EXTENDED_LIMIT_INFORMATION
    {
        public JOBOBJECT_BASIC_LIMIT_INFORMATION BasicLimitInformation;
        public IO_COUNTERS IoInfo;
        public UIntPtr ProcessMemoryLimit;
        public UIntPtr JobMemoryLimit;
        public UIntPtr PeakProcessMemoryUsed;
        public UIntPtr PeakJobMemoryUsed;
    }

    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern IntPtr CreateJobObjectW(IntPtr lpJobAttributes, string? lpName);

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool SetInformationJobObject(
        IntPtr hJob,
        int jobObjectInfoClass,
        ref JOBOBJECT_EXTENDED_LIMIT_INFORMATION lpJobObjectInfo,
        int cbJobObjectInfoLength);

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool AssignProcessToJobObject(IntPtr hJob, IntPtr hProcess);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern IntPtr OpenProcess(uint dwDesiredAccess, [MarshalAs(UnmanagedType.Bool)] bool bInheritHandle, int dwProcessId);

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool CloseHandle(IntPtr hObject);
}
