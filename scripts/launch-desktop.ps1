param(
    [switch]$Mock = $true,
    [switch]$Rebuild = $false,
    [switch]$Dist = $true
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

# 1. Clean up any existing instances
$existing = Get-Process -Name "TavernAgent.Desktop", "tavernagent" -ErrorAction SilentlyContinue
if ($existing) {
    Write-Host "Terminating existing TavernAgent processes..."
    $existing | Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep -Milliseconds 500
}

# 2. Resolve paths (dist or debug build)
if ($Dist -and (Test-Path (Join-Path $root "dist\desktop-win-x64\TavernAgent.Desktop.exe"))) {
    $desktopExe = Join-Path $root "dist\desktop-win-x64\TavernAgent.Desktop.exe"
    $sidecar = Join-Path $root "dist\desktop-win-x64\sidecar\tavernagent.exe"
    Write-Host "Using published dist: $desktopExe"
} else {
    $sidecar = Join-Path $root "build\tavernagent.exe"
    if ($Rebuild -or -not (Test-Path $sidecar)) {
        Write-Host "Building Go sidecar -> build/tavernagent.exe"
        & go build -trimpath -o $sidecar (Join-Path $root "cmd\tavernagent")
        if ($LASTEXITCODE -ne 0) { throw "go build failed" }
    }

    $desktopCsproj = Join-Path $root "desktop\src\TavernAgent.Desktop\TavernAgent.Desktop.csproj"
    $desktopExe = Join-Path $root "desktop\src\TavernAgent.Desktop\bin\Debug\net10.0\TavernAgent.Desktop.exe"

    if (-not (Test-Path $desktopExe) -or $Rebuild) {
        Write-Host "Building TavernAgent.Desktop..."
        & dotnet build $desktopCsproj -c Debug
        if ($LASTEXITCODE -ne 0) { throw "dotnet build failed" }
    }
}

# 3. Set environment
$env:TAVERNAGENT_SIDECAR = $sidecar
if ($Mock) {
    $env:TAVERNAGENT_PROVIDER = "mock"
}

# 5. Launch on interactive user desktop (WinSta0\Default)
Add-Type @"
using System;
using System.Runtime.InteropServices;

public class DesktopLauncher {
    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    public struct STARTUPINFO {
        public int cb;
        public string lpReserved;
        public string lpDesktop;
        public string lpTitle;
        public int dwX;
        public int dwY;
        public int dwXSize;
        public int dwYSize;
        public int dwXCountChars;
        public int dwYCountChars;
        public int dwFillAttribute;
        public int dwFlags;
        public short wShowWindow;
        public short cbReserved2;
        public IntPtr lpReserved2;
        public IntPtr hStdInput;
        public IntPtr hStdOutput;
        public IntPtr hStdError;
    }

    [StructLayout(LayoutKind.Sequential)]
    public struct PROCESS_INFORMATION {
        public IntPtr hProcess;
        public IntPtr hThread;
        public int dwProcessId;
        public int dwThreadId;
    }

    [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    public static extern bool CreateProcess(
        string lpApplicationName,
        string lpCommandLine,
        IntPtr lpProcessAttributes,
        IntPtr lpThreadAttributes,
        bool bInheritHandles,
        uint dwCreationFlags,
        IntPtr lpEnvironment,
        string lpCurrentDirectory,
        ref STARTUPINFO lpStartupInfo,
        out PROCESS_INFORMATION lpProcessInformation);

    [DllImport("kernel32.dll", SetLastError = true)]
    public static extern bool CloseHandle(IntPtr hObject);

    [DllImport("user32.dll")]
    public static extern bool SetForegroundWindow(IntPtr hWnd);

    [DllImport("user32.dll")]
    public static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);

    public static int Start(string appPath, string workingDir) {
        var si = new STARTUPINFO();
        si.cb = Marshal.SizeOf(si);
        si.lpDesktop = @"WinSta0\Default";
        si.dwFlags = 1;
        si.wShowWindow = 1; // SW_SHOWNORMAL

        var pi = new PROCESS_INFORMATION();
        bool success = CreateProcess(
            appPath,
            null,
            IntPtr.Zero,
            IntPtr.Zero,
            false,
            0,
            IntPtr.Zero,
            workingDir,
            ref si,
            out pi);

        if (!success) {
            int err = Marshal.GetLastWin32Error();
            throw new Exception("CreateProcess failed with win32 error: " + err);
        }

        CloseHandle(pi.hThread);
        CloseHandle(pi.hProcess);
        return pi.dwProcessId;
    }
}
"@

Write-Host "Launching TavernAgent.Desktop onto WinSta0\Default..."
$workingDir = Split-Path -Parent $desktopExe
$launchedPid = [DesktopLauncher]::Start($desktopExe, $workingDir)
Write-Host "Launched process ID: $launchedPid"

# 6. Verify process running
Start-Sleep -Seconds 2
$proc = Get-Process -Id $launchedPid -ErrorAction SilentlyContinue
if ($proc -and -not $proc.HasExited) {
    Write-Host "TavernAgent.Desktop is running (PID: $launchedPid)."
} else {
    Write-Warning "TavernAgent.Desktop exited early. Check logs in %APPDATA%\TavernAgent\logs."
}
