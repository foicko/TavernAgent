using TavernAgent.Core.Api;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.Services;

/// <summary>
/// Composition root: a single place that owns the API client and the Core stores,
/// and connects them to the sidecar's loopback address once it is ready.
/// </summary>
public static class AppServices
{
    public static StoryStore Story { get; } = StoryStore.Instance;

    public static SettingsStore Settings { get; } = SettingsStore.Instance;

    public static UiStore Ui { get; } = UiStore.Instance;

    public static TtsPlayer Tts { get; } = new((req, ct) => Story.Api.SynthesizeTTSAsync(new TTSSynthesizeParams
    {
        Text = req.Text,
        Instruction = req.Instruction,
        Voice = req.Voice,
        Engine = req.Engine,
        BaseUrl = req.BaseUrl,
        ApiKey = req.ApiKey,
        Model = req.Model,
        Speed = req.Speed,
    }, ct));

    public static TavernApiClient Api => Story.Api;

    /// <summary>Point the API client at the running sidecar and load the token.</summary>
    public static void Connect(Uri baseAddress)
    {
        Story.Api.BaseAddress = baseAddress;
        TavernApiClient.LoadAuthToken();
    }

    public static async Task LoadInitialAsync(CancellationToken ct = default)
    {
        await Settings.LoadAsync(ct).ConfigureAwait(false);
        await Story.LoadSessionsAsync(ct).ConfigureAwait(false);
        await OpenStartupSessionAsync(ct).ConfigureAwait(false);
    }

    /// <summary>Restore the last-remembered session (web: startupSession), else the first.</summary>
    public static async Task OpenStartupSessionAsync(CancellationToken ct = default)
    {
        var sessions = Story.Sessions;
        if (sessions.Count == 0) return;
        var remembered = SessionMemory.LastSessionId();
        var pick = sessions.FirstOrDefault(s => s.SessionId == remembered) ?? sessions[0];
        try
        {
            await Story.OpenSessionAsync(pick.SessionId, ct).ConfigureAwait(false);
        }
        catch
        {
            // Opening a stale session must never block boot; the user can pick another.
        }
    }
}
