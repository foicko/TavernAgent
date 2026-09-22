namespace TavernAgent.Core.Stores;

/// <summary>
/// Remember last opened session id across restarts (parity with sessionMemory.ts).
/// Write failures (privacy mode / quota) are ignored.
/// </summary>
public static class SessionMemory
{
    private const string Key = "tavernagent.lastSessionId";

    private static string? _overrideStore;

    /// <summary>Test seam: when set, used instead of the file-backed store.</summary>
    public static string? OverrideStore
    {
        get => _overrideStore;
        set => _overrideStore = value;
    }

    private static string FilePath
    {
        get
        {
            var root = Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData);
            if (string.IsNullOrEmpty(root)) root = Path.GetTempPath();
            return Path.Combine(root, "TavernAgent", "session_memory.txt");
        }
    }

    public static void RememberSession(string? sessionId)
    {
        try
        {
            if (_overrideStore is not null)
            {
                // In-memory mode for tests: null clears.
                _overrideStore = sessionId;
                return;
            }
            if (sessionId is null)
            {
                if (File.Exists(FilePath)) File.Delete(FilePath);
            }
            else
            {
                var dir = Path.GetDirectoryName(FilePath);
                if (!string.IsNullOrEmpty(dir)) Directory.CreateDirectory(dir);
                File.WriteAllText(FilePath, sessionId);
            }
        }
        catch
        {
            // ignore — memory failure must not fail open
        }
    }

    public static string? LastSessionId()
    {
        try
        {
            if (_overrideStore is not null) return _overrideStore;
            if (File.Exists(FilePath))
            {
                var text = File.ReadAllText(FilePath).Trim();
                return string.IsNullOrEmpty(text) ? null : text;
            }
        }
        catch { /* ignore */ }
        return null;
    }

    /// <summary>
    /// Priority: last opened (if still in list) → character match → first story.
    /// </summary>
    public static T? ChooseStartupSession<T>(IReadOnlyList<T> sessions, string? remembered, Func<T, bool> matchesCharacter)
        where T : class, IHasSessionId
    {
        if (sessions.Count == 0) return null;
        if (!string.IsNullOrEmpty(remembered))
        {
            foreach (var s in sessions)
            {
                if (s.SessionId == remembered) return s;
            }
        }
        foreach (var s in sessions)
        {
            if (matchesCharacter(s)) return s;
        }
        return sessions[0];
    }
}

public interface IHasSessionId
{
    string SessionId { get; }
}
