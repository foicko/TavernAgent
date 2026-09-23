using System.Collections.ObjectModel;
using System.ComponentModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.Services;

namespace TavernAgent.Desktop.ViewModels.Modals.Settings;

/// <summary>Models / connections / writing preferences tab (web: ModelSettingsPage).</summary>
public sealed partial class ModelSettingsPageViewModel : WorkspaceChildViewModel, ISettingsPage
{
    private readonly SettingsStore _settings;
    private readonly InteractionSettings _interaction;
    private readonly IDisposable _storeSub;
    private bool _disposed;

    public ModelSettingsPageViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        _settings = workspace.Settings;
        _interaction = SettingsPersistence.LoadInteraction();
        _storeSub = UiEvents.PropertyChanged(_settings, OnStoreChanged);
        foreach (var role in ModelLabels.RoleOrder)
            SlotPickers.Add(new SlotPickerViewModel(this, role));
        _optionModeFillEdit = _interaction.OptionMode == OptionMode.FillEdit;
        _autoContinue = _interaction.AutoContinue;
        _selectedFrequency = Frequencies.First(o => o.Value == WireFrequency(_interaction.OptionsFrequency));
        Rebuild();
    }

    public ObservableCollection<SlotPickerViewModel> SlotPickers { get; } = new();
    public ObservableCollection<ConnectionRowViewModel> Connections { get; } = new();

    [ObservableProperty] private bool _isEditing;
    [ObservableProperty] private ConnectionFormViewModel? _editing;
    [ObservableProperty] private string? _notice;
    [ObservableProperty] private string? _roleError;
    [ObservableProperty] private bool _saving;

    public bool HasConnections => Connections.Count > 0 && !IsEditing;
    public bool ShowEmpty => Connections.Count == 0 && !IsEditing;
    public bool HasNotice => !string.IsNullOrEmpty(Notice);
    public bool HasRoleError => !string.IsNullOrEmpty(RoleError);

    // ---- writing preferences ----

    public IReadOnlyList<OptionItem> Frequencies { get; } = new List<OptionItem>
    {
        new("auto", "关键节点"),
        new("always", "每轮"),
        new("never", "从不"),
    };

    [ObservableProperty] private bool _optionModeFillEdit;
    [ObservableProperty] private bool _autoContinue;
    [ObservableProperty] private OptionItem _selectedFrequency;

    partial void OnOptionModeFillEditChanged(bool value)
    {
        _interaction.OptionMode = value ? OptionMode.FillEdit : OptionMode.Direct;
        SettingsPersistence.SaveInteraction(_interaction);
    }

    partial void OnAutoContinueChanged(bool value)
    {
        _interaction.AutoContinue = value;
        SettingsPersistence.SaveInteraction(_interaction);
    }

    partial void OnSelectedFrequencyChanged(OptionItem value)
    {
        if (value is null) return;
        _interaction.OptionsFrequency = value.Value switch
        {
            "always" => OptionsFrequency.Always,
            "never" => OptionsFrequency.Never,
            _ => OptionsFrequency.Auto,
        };
        SettingsPersistence.SaveInteraction(_interaction);
    }

    public string PreferenceSummary =>
        $"选项：{SelectedFrequency?.Label ?? "关键节点"} · 自动续写：{(AutoContinue ? "开" : "关")}";

    private static string WireFrequency(OptionsFrequency freq) => freq switch
    {
        OptionsFrequency.Always => "always",
        OptionsFrequency.Never => "never",
        _ => "auto",
    };

    // ---- data ----

    public async Task LoadAsync()
    {
        await _settings.LoadAsync().ConfigureAwait(true);
        Rebuild();
    }

    public void Rebuild()
    {
        foreach (var picker in SlotPickers)
            picker.Rebuild(_settings.Models, _settings.Providers.FirstOrDefault(p => p.Slot == picker.Role));

        Connections.Clear();
        foreach (var instance in _settings.Models)
            Connections.Add(new ConnectionRowViewModel(instance, UsedBy(instance.Id)) { OpenCommand = EditConnectionCommand });

        RaiseListState();
    }

    private void RaiseListState()
    {
        OnPropertyChanged(nameof(HasConnections));
        OnPropertyChanged(nameof(ShowEmpty));
    }

    public IReadOnlyList<string> ExistingNames() => _settings.Models.Select(m => m.Name).ToList();

    public IReadOnlyList<string> UsedBy(string modelId) => SettingsStore.SlotsUsingModel(_settings.Providers, modelId);

    public Task<ProbeResult> ProbeAsync(string id) => Workspace.Story.Api.ProbeModelAsync(id, false);

    public async Task<ModelInstance?> SaveAsync(ModelInstance draft)
    {
        Saving = true;
        try
        {
            return await _settings.SaveModelAsync(draft).ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            Notice = "保存失败：" + ex.Message;
            return null;
        }
        finally
        {
            Saving = false;
        }
    }

    public async Task DeleteAsync(string id)
    {
        if (UsedBy(id).Count > 0) return;
        await _settings.DeleteModelAsync(id).ConfigureAwait(true);
    }

    // ---- commands ----

    [RelayCommand]
    private void NewConnection()
    {
        Notice = null;
        Editing = new ConnectionFormViewModel(this, null);
        IsEditing = true;
        RaiseListState();
    }

    [RelayCommand]
    private void EditConnection(ConnectionRowViewModel? row)
    {
        if (row is null) return;
        Notice = null;
        var instance = _settings.Models.FirstOrDefault(m => m.Id == row.Id);
        if (instance is null) return;
        Editing = new ConnectionFormViewModel(this, instance);
        IsEditing = true;
        RaiseListState();
    }

    public void CancelEdit()
    {
        Editing = null;
        IsEditing = false;
        RaiseListState();
    }

    public void OnSaved(ModelInstance saved)
    {
        Notice = $"已保存「{ModelLabels.ConnectionTitle(saved.Name, saved.Model)}」。下一次生成就会用它。";
        Editing = new ConnectionFormViewModel(this, saved);
        Rebuild();
    }

    public void OnDeleted()
    {
        Notice = "连接已删除。";
        Editing = null;
        IsEditing = false;
        Rebuild();
    }

    internal async Task AssignSlotAsync(SlotPickerViewModel picker)
    {
        var modelId = picker.SelectedOption?.Value ?? "";
        var enabled = picker.Role switch
        {
            "primary" => !string.IsNullOrEmpty(modelId),
            "assist" => true,
            _ => picker.Enabled,
        };
        try
        {
            RoleError = null;
            await _settings.AssignSlotAsync(picker.Role, enabled, modelId).ConfigureAwait(true);
            Rebuild();
        }
        catch (Exception ex)
        {
            RoleError = ex.Message;
        }
    }

    private void OnStoreChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName is nameof(SettingsStore.Models) or nameof(SettingsStore.Providers) or nameof(SettingsStore.Loaded))
            Rebuild();
    }

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        _storeSub.Dispose();
    }
}
