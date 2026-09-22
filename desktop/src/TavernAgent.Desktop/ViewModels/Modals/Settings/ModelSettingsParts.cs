using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Modals.Settings;

/// <summary>Role → model picker row (web: ModelPicker for primary/assist/reflection).</summary>
public sealed partial class SlotPickerViewModel : ObservableObject
{
    private readonly ModelSettingsPageViewModel _page;
    private bool _suppress;

    public SlotPickerViewModel(ModelSettingsPageViewModel page, string role)
    {
        _page = page;
        Role = role;
        Label = ModelLabels.RoleLabel(role);
        Hint = ModelLabels.RoleHint(role);
        IsReflection = role == "reflection";
    }

    public string Role { get; }
    public string Label { get; }
    public string Hint { get; }
    public bool IsReflection { get; }

    public ObservableCollection<OptionItem> Options { get; } = new();

    [ObservableProperty] private OptionItem? _selectedOption;
    [ObservableProperty] private bool _enabled = true;
    [ObservableProperty] private string _statusText = "";

    public bool IsPickerEnabled => !IsReflection || Enabled;

    partial void OnSelectedOptionChanged(OptionItem? value)
    {
        if (_suppress || value is null) return;
        _ = _page.AssignSlotAsync(this);
    }

    partial void OnEnabledChanged(bool value)
    {
        OnPropertyChanged(nameof(IsPickerEnabled));
        if (_suppress || !IsReflection) return;
        _ = _page.AssignSlotAsync(this);
    }

    public void Rebuild(IReadOnlyList<ModelInstance> models, ProviderConfig? config)
    {
        _suppress = true;
        Options.Clear();
        Options.Add(new OptionItem("", Role == "primary" ? "未选择" : ModelLabels.FollowPrimary));
        foreach (var m in models)
            Options.Add(new OptionItem(m.Id, UniqueLabel(models, m)));

        var modelId = config?.ModelId ?? "";
        SelectedOption = Options.FirstOrDefault(o => o.Value == modelId) ?? Options[0];
        if (IsReflection) Enabled = config?.Enabled ?? false;

        StatusText = BuildStatus(models, config);
        _suppress = false;
        OnPropertyChanged(nameof(IsPickerEnabled));
    }

    private static string UniqueLabel(IReadOnlyList<ModelInstance> models, ModelInstance m)
    {
        var label = ModelLabels.ConnectionTitle(m.Name, m.Model);
        var dupes = models.Where(x => ModelLabels.ConnectionTitle(x.Name, x.Model) == label).ToList();
        if (dupes.Count <= 1) return label;
        var host = ModelLabels.ConnectionHost(m.BaseUrl);
        return string.IsNullOrEmpty(host) ? $"{label} · {ModelLabels.ProtocolLabel(m.Kind)}" : $"{label} · {host}";
    }

    private string BuildStatus(IReadOnlyList<ModelInstance> models, ProviderConfig? config)
    {
        if (IsReflection && !Enabled) return "已关闭";
        var id = SelectedOption?.Value ?? "";
        var targetId = IsReflection || Role == "primary" ? (config?.ResolvedModelId ?? id) : id;
        if (string.IsNullOrEmpty(targetId))
            return Role == "primary" ? "还没有选择连接" : $"跟随对话生成 · 尚未配置连接";
        var instance = models.FirstOrDefault(m => m.Id == targetId);
        return instance is null
            ? "选择的连接不存在，请重新指派"
            : $"当前：{ModelLabels.ConnectionTitle(instance.Name, instance.Model)}";
    }
}

/// <summary>One saved connection in the list (web: ConnectionList row).</summary>
public sealed class ConnectionRowViewModel
{
    public ConnectionRowViewModel(ModelInstance instance, IReadOnlyList<string> usedBy)
    {
        Id = instance.Id;
        Title = ModelLabels.ConnectionTitle(instance.Name, instance.Model);
        Subtitle = ModelLabels.ConnectionSubtitle(instance.Name, instance.Model, instance.BaseUrl);
        if (string.IsNullOrEmpty(Subtitle)) Subtitle = ModelLabels.ProtocolLabel(instance.Kind);
        HasKey = instance.HasApiKey == true;
        KeyLabel = ModelLabels.KeyStateLabel(instance.HasApiKey);
        WindowLabel = ModelLabels.WindowLabel(instance.ContextWindow);
        UsedBy = usedBy;
        UsesModel = usedBy.Count > 0;
    }

    public string Id { get; }
    public string Title { get; }
    public string Subtitle { get; }
    public bool HasKey { get; }
    public bool NoKey => !HasKey;
    public string KeyLabel { get; }
    public string WindowLabel { get; }
    public IReadOnlyList<string> UsedBy { get; }
    public bool UsesModel { get; }
    public string UsedByLabel => string.Join("·", UsedBy);
    public System.Windows.Input.ICommand? OpenCommand { get; set; }
}
