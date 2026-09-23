using TavernAgent.Core.Dtos;

namespace TavernAgent.Core.Stores;

/// <summary>
/// Per-turn generated illustrations (web: imageSettingsStore.turnIllustrations).
/// Persisted to the data directory so images survive restarts.
/// </summary>
public sealed class IllustrationStore : ObservableStore
{
    public static IllustrationStore Instance { get; } = new();

    private const string FileName = "illustrations.json";
    private readonly Dictionary<string, GeneratedImageResult> _map;

    public IllustrationStore()
    {
        try
        {
            _map = SettingsPersistence.Load(FileName, new Dictionary<string, GeneratedImageResult>())
                   ?? new Dictionary<string, GeneratedImageResult>();
        }
        catch
        {
            _map = new Dictionary<string, GeneratedImageResult>();
        }
    }

    public event EventHandler? Changed;

    public GeneratedImageResult? Get(string? turnId)
        => string.IsNullOrEmpty(turnId) ? null : (_map.TryGetValue(turnId, out var v) ? v : null);

    public void Attach(string turnId, GeneratedImageResult result)
    {
        if (string.IsNullOrEmpty(turnId)) return;
        _map[turnId] = result;
        Persist();
        StoreDispatcher.Dispatch(() => Changed?.Invoke(this, EventArgs.Empty));
    }

    public void Remove(string turnId)
    {
        if (_map.Remove(turnId))
        {
            Persist();
            StoreDispatcher.Dispatch(() => Changed?.Invoke(this, EventArgs.Empty));
        }
    }

    private void Persist()
    {
        try { SettingsPersistence.Save(FileName, _map); }
        catch { /* persistence is best-effort */ }
    }
}
