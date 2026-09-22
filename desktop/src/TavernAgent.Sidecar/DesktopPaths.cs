namespace TavernAgent.Sidecar;

/// <summary>Resolves well-known paths for the desktop shell.</summary>
public static class DesktopPaths
{
    public const string AppFolderName = "TavernAgent";

    /// <summary>%APPDATA%\TavernAgent on Windows, ~/.config/TavernAgent elsewhere.</summary>
    public static string DataDir
    {
        get
        {
            var root = Environment.GetFolderPath(
                Environment.SpecialFolder.ApplicationData,
                Environment.SpecialFolderOption.Create);
            if (string.IsNullOrEmpty(root))
                root = Path.Combine(
                    Environment.GetFolderPath(Environment.SpecialFolder.UserProfile),
                    ".config");
            return Path.Combine(root, AppFolderName);
        }
    }

    public static string LogsDir => Path.Combine(DataDir, "logs");

    public static string DesktopSettingsFile => Path.Combine(DataDir, "desktop-settings.json");

    /// <summary>
    /// Locate the sidecar executable.
    /// Priority: explicit env override → beside the desktop binary → sidecar/ subfolder
    /// → walk up to 8 parent levels looking for &lt;dir&gt;/build/&lt;name&gt; or &lt;dir&gt;/&lt;name&gt;
    /// (covers the repo-root build/ output during development).
    /// </summary>
    public static string FindSidecarExecutable()
    {
        var baseDir = AppContext.BaseDirectory;
        var name = OperatingSystem.IsWindows() ? "tavernagent.exe" : "tavernagent";

        var overridePath = Environment.GetEnvironmentVariable("TAVERNAGENT_SIDECAR");
        if (!string.IsNullOrWhiteSpace(overridePath) && File.Exists(overridePath))
            return overridePath;

        var beside = Path.Combine(baseDir, name);
        var inSub = Path.Combine(baseDir, "sidecar", name);
        if (File.Exists(beside)) return beside;
        if (File.Exists(inSub)) return inSub;

        var dir = new DirectoryInfo(baseDir);
        for (var i = 0; i < 8 && dir is not null; i++, dir = dir.Parent)
        {
            var buildCandidate = Path.Combine(dir.FullName, "build", name);
            if (File.Exists(buildCandidate)) return buildCandidate;
            var rootCandidate = Path.Combine(dir.FullName, name);
            if (File.Exists(rootCandidate)) return rootCandidate;
        }

        return beside;
    }
}
