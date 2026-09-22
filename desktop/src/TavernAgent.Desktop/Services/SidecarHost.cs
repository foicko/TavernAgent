using System.Text.Json;
using System.Text.Json.Serialization;
using TavernAgent.Sidecar;

namespace TavernAgent.Desktop.Services;

/// <summary>
/// Composition root for desktop services: single instance paths, sidecar options,
/// and future API client wiring.
/// </summary>
public static class SidecarHost
{
    public static string DataDir => DesktopPaths.DataDir;

    public static string SidecarExecutable => DesktopPaths.FindSidecarExecutable();

    public static SidecarOptions CreateOptions(string? provider = null)
    {
        // Dev convenience: TAVERNAGENT_PROVIDER=mock launches the offline provider.
        provider ??= Environment.GetEnvironmentVariable("TAVERNAGENT_PROVIDER");
        if (string.IsNullOrWhiteSpace(provider)) provider = null;
        return new SidecarOptions
        {
            ExecutablePath = SidecarExecutable,
            DataDir = DataDir,
            Provider = provider,
            ReadyTimeout = TimeSpan.FromSeconds(15),
            ShutdownGrace = TimeSpan.FromSeconds(5),
            LogDir = DesktopPaths.LogsDir,
        };
    }
}

/// <summary>Persisted desktop UI preferences (theme, window size) — replaces localStorage.</summary>
public sealed class DesktopSettings
{
    /// <summary>Explicit user choice ("light"/"dark"); empty means follow the system theme.</summary>
    public string Theme { get; set; } = "";

    public double WindowWidth { get; set; } = 1440;

    public double WindowHeight { get; set; } = 900;

    public string? LastSessionId { get; set; }

    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        WriteIndented = true,
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
    };

    public static DesktopSettings Load()
    {
        try
        {
            var path = DesktopPaths.DesktopSettingsFile;
            if (File.Exists(path))
            {
                var json = File.ReadAllText(path);
                return JsonSerializer.Deserialize<DesktopSettings>(json) ?? new DesktopSettings();
            }
        }
        catch
        {
            // corrupt file → defaults
        }
        return new DesktopSettings();
    }

    public void Save()
    {
        try
        {
            var path = DesktopPaths.DesktopSettingsFile;
            Directory.CreateDirectory(Path.GetDirectoryName(path)!);
            File.WriteAllText(path, JsonSerializer.Serialize(this, JsonOptions));
        }
        catch
        {
            // best-effort persistence
        }
    }
}
