using TavernAgent.Core.Api;
using TavernAgent.Core.Stores;
using TavernAgent.Sidecar;
using Xunit;

namespace TavernAgent.Core.Tests;

/// <summary>
/// End-to-end story flow against the real mock sidecar: create → send → commit.
/// Skipped silently when build/tavernagent(.exe) is missing.
/// </summary>
public class StoryFlowIntegrationTests
{
    private static string? FindSidecar()
    {
        var name = OperatingSystem.IsWindows() ? "tavernagent.exe" : "tavernagent";
        var candidates = new[]
        {
            Path.GetFullPath(Path.Combine(AppContext.BaseDirectory, "..", "..", "..", "..", "..", "..", "build", name)),
            Path.GetFullPath(Path.Combine(AppContext.BaseDirectory, "..", "..", "..", "..", "..", "build", name)),
            Path.Combine(Environment.CurrentDirectory, "build", name),
            Path.Combine(Environment.CurrentDirectory, name),
        };
        return candidates.FirstOrDefault(File.Exists);
    }

    [Fact]
    public async Task Create_Session_Send_Turn_And_Commit()
    {
        var exe = FindSidecar();
        if (exe is null) return;

        var dataDir = Path.Combine(Path.GetTempPath(), "tavernagent-flow-" + Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(dataDir);

        await using var supervisor = new SidecarSupervisor(new SidecarOptions
        {
            ExecutablePath = exe,
            DataDir = dataDir,
            Provider = "mock",
            ReadyTimeout = TimeSpan.FromSeconds(20),
            ShutdownGrace = TimeSpan.FromSeconds(8),
        });

        await supervisor.StartAsync();
        Assert.True(supervisor.State == SidecarState.Ready, supervisor.LastError);

        var api = new TavernApiClient(supervisor.BaseAddress);
        var store = new StoryStore(api);

        var created = await store.CreateSessionAsync(new CreateSessionOpts
        {
            Title = "集成测试剧本",
            CharacterJson = CharacterPresets.GetPresetCardJson(CharacterPresets.DefaultPresetKey),
            PlayerName = "旅人",
            OpeningText = "夜色低垂，故事在此开场。",
        });
        Assert.False(string.IsNullOrEmpty(created.SessionId));

        await store.SendAsync("我推门走进酒馆。");

        var deadline = DateTime.UtcNow + TimeSpan.FromSeconds(30);
        while (DateTime.UtcNow < deadline)
        {
            if (store.Phase is TurnPhase.Idle or TurnPhase.Committed && store.Messages.Any(m => m.Role == "assistant"))
                break;
            if (store.Phase == TurnPhase.Failed) break;
            await Task.Delay(200);
        }

        Assert.NotEqual(TurnPhase.Failed, store.Phase);
        Assert.NotEmpty(store.Messages);
        Assert.Contains(store.Messages, m => m.Role == "assistant" && m.Blocks.Count > 0);

        api.Dispose();
        try { Directory.Delete(dataDir, recursive: true); } catch { /* ignore */ }
    }
}
