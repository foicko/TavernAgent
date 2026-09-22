using System.Diagnostics;
using TavernAgent.Sidecar;
using Xunit;

namespace TavernAgent.Core.Tests;

/// <summary>
/// Integration tests that spawn the real Go sidecar when available.
/// Skipped silently when build/tavernagent(.exe) is missing.
/// </summary>
public class SidecarSupervisorIntegrationTests
{
    private static string? FindSidecar()
    {
        var name = OperatingSystem.IsWindows() ? "tavernagent.exe" : "tavernagent";
        // BaseDirectory = desktop/tests/TavernAgent.Core.Tests/bin/Debug/net10.0 → 6 up = repo root
        var candidates = new[]
        {
            Path.GetFullPath(Path.Combine(AppContext.BaseDirectory, "..", "..", "..", "..", "..", "..", "build", name)),
            Path.GetFullPath(Path.Combine(AppContext.BaseDirectory, "..", "..", "..", "..", "..", "build", name)),
            Path.Combine(Environment.CurrentDirectory, "build", name),
            Path.Combine(Environment.CurrentDirectory, name),
        };
        return candidates.FirstOrDefault(File.Exists);
    }

    private static string TempDataDir()
    {
        var dir = Path.Combine(Path.GetTempPath(), "tavernagent-sidecar-" + Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(dir);
        return dir;
    }

    [Fact]
    public async Task Starts_Healthz_And_Shuts_Down_Gracefully()
    {
        var exe = FindSidecar();
        if (exe is null)
        {
            // Build order: Go binary not present in this environment.
            return;
        }

        var dataDir = TempDataDir();
        await using var supervisor = new SidecarSupervisor(new SidecarOptions
        {
            ExecutablePath = exe,
            DataDir = dataDir,
            Provider = "mock",
            ReadyTimeout = TimeSpan.FromSeconds(20),
            ShutdownGrace = TimeSpan.FromSeconds(8),
        });

        await supervisor.StartAsync();
        Assert.True(
            supervisor.State == SidecarState.Ready,
            $"state={supervisor.State} error={supervisor.LastError} log={string.Join("\n", supervisor.Log?.Tail ?? Array.Empty<string>())}");
        Assert.NotNull(supervisor.BaseAddress);

        using var http = new HttpClient { Timeout = TimeSpan.FromSeconds(2) };
        var health = await http.GetAsync(new Uri(supervisor.BaseAddress!, "healthz"));
        Assert.True(health.IsSuccessStatusCode);

        await supervisor.StopAsync();
        Assert.Equal(SidecarState.Stopped, supervisor.State);

        try { Directory.Delete(dataDir, recursive: true); } catch { /* ignore */ }
    }

    [Fact]
    public async Task Missing_Executable_Fails_With_Legible_Error()
    {
        var dataDir = TempDataDir();
        await using var supervisor = new SidecarSupervisor(new SidecarOptions
        {
            ExecutablePath = Path.Combine(dataDir, "does-not-exist.exe"),
            DataDir = dataDir,
        });

        await supervisor.StartAsync();
        Assert.Equal(SidecarState.Failed, supervisor.State);
        Assert.NotNull(supervisor.LastError);
        Assert.Contains("找不到无头服务", supervisor.LastError);

        try { Directory.Delete(dataDir, recursive: true); } catch { /* ignore */ }
    }

    [Fact]
    public async Task Second_Process_Same_DataDir_Is_Rejected_By_Go_Lock()
    {
        var exe = FindSidecar();
        if (exe is null) return;

        var dataDir = TempDataDir();
        await using var first = new SidecarSupervisor(new SidecarOptions
        {
            ExecutablePath = exe,
            DataDir = dataDir,
            Provider = "mock",
            ReadyTimeout = TimeSpan.FromSeconds(20),
        });
        await first.StartAsync();
        Assert.Equal(SidecarState.Ready, first.State);

        // Direct second spawn of the same data dir must fail (datalock).
        var port = PortAllocator.GetFreeLoopbackPort();
        var psi = new ProcessStartInfo
        {
            FileName = exe,
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
        };
        psi.ArgumentList.Add("-data");
        psi.ArgumentList.Add(dataDir);
        psi.ArgumentList.Add("-addr");
        psi.ArgumentList.Add($"127.0.0.1:{port}");
        psi.ArgumentList.Add("-provider");
        psi.ArgumentList.Add("mock");

        using var second = Process.Start(psi)!;
        var stderr = await second.StandardError.ReadToEndAsync();
        var stdout = await second.StandardOutput.ReadToEndAsync();
        await second.WaitForExitAsync().WaitAsync(TimeSpan.FromSeconds(15));

        Assert.NotEqual(0, second.ExitCode);
        var output = stderr + stdout;
        Assert.True(
            output.Contains("数据目录") || output.Contains("独占") || output.Contains("lock"),
            $"expected datalock error, got: {output}");

        await first.StopAsync();
        try { Directory.Delete(dataDir, recursive: true); } catch { /* ignore */ }
    }
}
