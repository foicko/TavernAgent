using TavernAgent.Core.Api;
using TavernAgent.Core.Dtos;

namespace TavernAgent.Core.Stores;

/// <summary>
/// Model instances / slot assignments / interaction prefs (settingsStore.ts parity).
/// </summary>
public sealed class SettingsStore : ObservableStore
{
    public static SettingsStore Instance { get; } = new();

    private readonly TavernApiClient _api;

    public SettingsStore(TavernApiClient? api = null)
    {
        _api = api ?? StoryStore.Instance.Api;
    }

    private List<ModelInstance> _models = new();
    public List<ModelInstance> Models { get => _models; private set => SetProperty(ref _models, value); }

    private List<ProviderConfig> _providers = new();
    public List<ProviderConfig> Providers { get => _providers; private set => SetProperty(ref _providers, value); }

    private bool _loaded;
    public bool Loaded { get => _loaded; private set => SetProperty(ref _loaded, value); }

    private bool _saving;
    public bool Saving { get => _saving; private set => SetProperty(ref _saving, value); }

    public async Task LoadAsync(CancellationToken ct = default)
    {
        try
        {
            var catalog = await _api.ListModelsAsync(ct).ConfigureAwait(false);
            Models = catalog.Models ?? new List<ModelInstance>();
            Providers = catalog.Slots ?? new List<ProviderConfig>();
            Loaded = true;
        }
        catch
        {
            Loaded = true;
        }
    }

    public async Task<ModelInstance> SaveModelAsync(ModelInstance instance, CancellationToken ct = default)
    {
        Saving = true;
        try
        {
            var saved = await _api.SaveModelAsync(instance, ct).ConfigureAwait(false);
            Saving = false;
            await LoadAsync(ct).ConfigureAwait(false);
            return saved;
        }
        catch
        {
            Saving = false;
            throw;
        }
    }

    public async Task DeleteModelAsync(string id, CancellationToken ct = default)
    {
        await _api.DeleteModelAsync(id, ct).ConfigureAwait(false);
        await LoadAsync(ct).ConfigureAwait(false);
    }

    public async Task AssignSlotAsync(string slot, bool enabled, string modelId, CancellationToken ct = default)
    {
        Saving = true;
        try
        {
            Providers = await _api.AssignSlotAsync(slot, enabled, modelId, ct).ConfigureAwait(false);
            Saving = false;
            await LoadAsync(ct).ConfigureAwait(false);
        }
        catch
        {
            Saving = false;
            throw;
        }
    }

    public async Task SwitchPrimaryModelAsync(string modelId, CancellationToken ct = default)
    {
        var primary = Providers.Find(c => c.Slot == "primary");
        await AssignSlotAsync("primary", primary?.Enabled ?? true, modelId, ct).ConfigureAwait(false);
    }

    public async Task SetReasoningEffortAsync(string effort, CancellationToken ct = default)
    {
        var current = ActivePrimaryInstance(Providers, Models);
        if (current is null) throw new Exception("还没有可用的主线连接");
        await SaveModelAsync(new ModelInstance
        {
            Id = current.Id,
            Name = current.Name,
            Kind = current.Kind,
            BaseUrl = current.BaseUrl,
            Model = current.Model,
            ApiKey = "",
            HasApiKey = current.HasApiKey,
            Temperature = current.Temperature,
            MaxTokens = current.MaxTokens,
            ContextWindow = current.ContextWindow,
            ReasoningEffort = effort,
        }, ct).ConfigureAwait(false);
    }

    public static ModelInstance? ActivePrimaryInstance(IReadOnlyList<ProviderConfig> providers, IReadOnlyList<ModelInstance> models)
    {
        var primary = providers.FirstOrDefault(c => c.Slot == "primary");
        var id = primary?.ResolvedModelId ?? primary?.ModelId;
        if (string.IsNullOrEmpty(id)) return null;
        return models.FirstOrDefault(m => m.Id == id);
    }

    public static IReadOnlyList<string> SlotsUsingModel(IReadOnlyList<ProviderConfig> providers, string modelId)
        => providers.Where(c => c.ModelId == modelId).Select(c => ModelLabels.RoleLabel(c.Slot)).ToList();
}
