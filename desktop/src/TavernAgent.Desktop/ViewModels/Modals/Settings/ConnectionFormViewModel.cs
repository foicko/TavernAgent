using System.Globalization;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Modals.Settings;

/// <summary>Create / edit / test / delete a model connection (web: ConnectionForm).</summary>
public sealed partial class ConnectionFormViewModel : ObservableObject
{
    private const int MaxContextWindow = 2_000_000;
    private const int MinInputReserve = 2048;
    private readonly ModelSettingsPageViewModel _page;
    private readonly ModelInstance? _editing;

    public ConnectionFormViewModel(ModelSettingsPageViewModel page, ModelInstance? instance)
    {
        _page = page;
        _editing = instance;
        IsNew = instance is null;
        UsedBy = instance is null ? new List<string>() : page.UsedBy(instance.Id);
        IsUsed = UsedBy.Count > 0;

        if (instance is null)
        {
            Kind = ModelLabels.DefaultProtocol;
            BaseUrl = "";
            Model = "";
            Name = "";
            ApiKey = "";
            Temperature = "0.8";
            MaxTokens = "4096";
            ContextWindow = "131072";
            ReasoningEffort = "";
        }
        else
        {
            Kind = string.IsNullOrEmpty(instance.Kind) ? ModelLabels.DefaultProtocol : instance.Kind;
            BaseUrl = instance.BaseUrl ?? "";
            Model = instance.Model ?? "";
            Name = instance.Name ?? "";
            ApiKey = "";
            Temperature = Num(instance.Temperature, "0.8");
            MaxTokens = Int(instance.MaxTokens, "4096");
            ContextWindow = Int(instance.ContextWindow, "131072");
            ReasoningEffort = instance.ReasoningEffort ?? "";
        }

        ActivePresetId = ProviderPresets.Match(Kind, BaseUrl)?.Id;
    }

    public bool IsNew { get; }
    public string Id => _editing?.Id ?? "";
    public IReadOnlyList<string> UsedBy { get; }
    public bool IsUsed { get; }
    public bool HasUsedBy => UsedBy.Count > 0;
    public string UsedByText => "该连接正被使用：" + string.Join("、", UsedBy);
    public string Title => IsNew ? "新建连接" : "编辑连接：" + ModelLabels.ConnectionTitle(Name, Model);
    public string SaveLabel => IsNew ? "保存连接" : "保存修改";

    public IReadOnlyList<ProviderPreset> Presets => ProviderPresets.All;
    public IReadOnlyList<OptionItem> ProtocolOptions { get; } = ModelLabels.ProtocolOptions
        .Select(o => new OptionItem(o.Value, o.Label)).ToList();
    public IReadOnlyList<OptionItem> EffortOptions { get; } = ModelLabels.EffortOptions
        .Select(o => new OptionItem(o.Value, o.Label)).ToList();

    public OptionItem SelectedProtocol
    {
        get => ProtocolOptions.FirstOrDefault(o => o.Value == Kind) ?? ProtocolOptions[0];
        set { if (value is not null) Kind = value.Value; }
    }

    public OptionItem SelectedEffort
    {
        get => EffortOptions.FirstOrDefault(o => o.Value == ReasoningEffort) ?? EffortOptions[0];
        set { if (value is not null) ReasoningEffort = value.Value; }
    }

    [ObservableProperty] private string _name = "";
    [ObservableProperty] private string _model = "";
    [ObservableProperty] private string _baseUrl = "";
    [ObservableProperty] private string _apiKey = "";
    [ObservableProperty] private string _kind = ModelLabels.DefaultProtocol;
    [ObservableProperty] private string _temperature = "0.8";
    [ObservableProperty] private string _maxTokens = "4096";
    [ObservableProperty] private string _contextWindow = "131072";
    [ObservableProperty] private string _reasoningEffort = "";
    [ObservableProperty] private bool _showKey;
    [ObservableProperty] private string? _activePresetId;
    [ObservableProperty] private string? _alertText;
    [ObservableProperty] private bool _alertError;
    [ObservableProperty] private string? _probeText;
    [ObservableProperty] private bool _probeOk;
    [ObservableProperty] private bool _probing;
    [ObservableProperty] private bool _confirmDelete;

    public bool HasAlert => !string.IsNullOrEmpty(AlertText);
    public bool HasProbe => !string.IsNullOrEmpty(ProbeText);
    public string KeyHint => _editing?.HasApiKey == true
        ? "留空 = 保持已保存的密钥（" + ModelLabels.MaskedKeyHint(_editing.ApiKey) + "）"
        : "粘贴完整密钥；只有本机会保存它";
    public string PresetKeyHint => ProviderPresets.All.FirstOrDefault(p => p.Id == ActivePresetId)?.KeyHint ?? "";
    public bool HasPresetKeyHint => !string.IsNullOrEmpty(PresetKeyHint);

    public string InputBudgetText
    {
        get
        {
            if (!int.TryParse(ContextWindow, out var win) || win <= 0) return "填入上下文窗口后可估算可用输入预算";
            if (!int.TryParse(MaxTokens, out var maxOut) || maxOut < 0) maxOut = 2048;
            var budget = ModelLabels.InputBudget(win, maxOut);
            if (budget is null or <= 0) return "⚠ 输入预算不足：窗口几乎被输出与预留占满";
            return $"估算可用输入约 {ModelLabels.TokenLabel(budget.Value)} tokens";
        }
    }

    partial void OnAlertTextChanged(string? value) => OnPropertyChanged(nameof(HasAlert));
    partial void OnProbeTextChanged(string? value) => OnPropertyChanged(nameof(HasProbe));
    partial void OnActivePresetIdChanged(string? value) => OnPropertyChanged(nameof(PresetKeyHint));
    partial void OnContextWindowChanged(string value) => OnPropertyChanged(nameof(InputBudgetText));
    partial void OnMaxTokensChanged(string value) => OnPropertyChanged(nameof(InputBudgetText));
    partial void OnNameChanged(string value) => OnPropertyChanged(nameof(Title));
    partial void OnModelChanged(string value) => OnPropertyChanged(nameof(Title));
    partial void OnKindChanged(string value) => OnPropertyChanged(nameof(SelectedProtocol));
    partial void OnReasoningEffortChanged(string value) => OnPropertyChanged(nameof(SelectedEffort));

    [RelayCommand]
    private void ApplyPreset(ProviderPreset? preset)
    {
        if (preset is null) return;
        var previous = ProviderPresets.All.FirstOrDefault(p => p.Id == ActivePresetId);
        Kind = preset.Kind;
        BaseUrl = preset.BaseUrl;
        if (string.IsNullOrWhiteSpace(Model) || (previous is not null && previous.Models.Contains(Model.Trim())))
            Model = preset.Models.FirstOrDefault() ?? "";
        if (IsNew) Name = ProviderPresets.SuggestName(preset, preset.BaseUrl, Model, _page.ExistingNames());
        ActivePresetId = preset.Id;
        AlertText = null;
    }

    [RelayCommand]
    private void ToggleKey() => ShowKey = !ShowKey;

    [RelayCommand]
    private void Cancel() => _page.CancelEdit();

    [RelayCommand]
    private async Task TestAsync()
    {
        if (IsNew || string.IsNullOrEmpty(Id))
        {
            AlertError = false;
            AlertText = "请先保存连接，再测试它。";
            return;
        }
        Probing = true;
        ProbeText = null;
        AlertText = null;
        try
        {
            var result = await _page.ProbeAsync(Id).ConfigureAwait(true);
            ProbeOk = result.Ok;
            ProbeText = result.Ok
                ? $"连接正常 · 延迟 {result.LatencyMs ?? 0} ms · 模型 {result.Model ?? Model}"
                : $"连接失败：{result.ConnectMsg ?? "未知错误"}";
        }
        catch (Exception ex)
        {
            ProbeOk = false;
            ProbeText = "测试失败：" + ex.Message;
        }
        finally
        {
            Probing = false;
        }
    }

    [RelayCommand]
    private async Task SaveAsync()
    {
        var errors = Validate();
        if (errors.Count > 0)
        {
            AlertError = true;
            AlertText = "请检查以下问题：" + string.Join("；", errors.Values);
            OnPropertyChanged(nameof(NameError));
            OnPropertyChanged(nameof(ModelError));
            OnPropertyChanged(nameof(BaseUrlError));
            OnPropertyChanged(nameof(ApiKeyError));
            OnPropertyChanged(nameof(TemperatureError));
            OnPropertyChanged(nameof(MaxTokensError));
            OnPropertyChanged(nameof(ContextWindowError));
            return;
        }

        AlertText = null;
        ParseNumbers(out var temperature, out var maxTokens, out var contextWindow);
        var draft = new ModelInstance
        {
            Id = Id,
            Name = Name.Trim(),
            Kind = Kind,
            BaseUrl = BaseUrl.Trim(),
            Model = Model.Trim(),
            ApiKey = ApiKey,
            Temperature = temperature,
            MaxTokens = maxTokens,
            ContextWindow = contextWindow,
            ReasoningEffort = ReasoningEffort,
        };
        var saved = await _page.SaveAsync(draft).ConfigureAwait(true);
        if (saved is not null) _page.OnSaved(saved);
    }

    [RelayCommand]
    private void AskDelete() => ConfirmDelete = true;

    [RelayCommand]
    private void CancelDelete() => ConfirmDelete = false;

    [RelayCommand]
    private async Task DeleteAsync()
    {
        if (IsUsed || string.IsNullOrEmpty(Id)) return;
        await _page.DeleteAsync(Id).ConfigureAwait(true);
        _page.OnDeleted();
    }

    // ---- field errors ----

    public string? NameError => Error(nameof(Name));
    public string? ModelError => Error(nameof(Model));
    public string? BaseUrlError => Error(nameof(BaseUrl));
    public string? ApiKeyError => Error(nameof(ApiKey));
    public string? TemperatureError => Error(nameof(Temperature));
    public string? MaxTokensError => Error(nameof(MaxTokens));
    public string? ContextWindowError => Error(nameof(ContextWindow));

    private readonly Dictionary<string, string> _errors = new();
    private string? Error(string field) => _errors.TryGetValue(field, out var v) ? v : null;

    private Dictionary<string, string> Validate()
    {
        _errors.Clear();
        if (string.IsNullOrWhiteSpace(Name)) _errors[nameof(Name)] = "名称不能为空";
        if (string.IsNullOrWhiteSpace(Model)) _errors[nameof(Model)] = "模型名称不能为空";

        var url = BaseUrl.Trim();
        if (!string.IsNullOrEmpty(url))
        {
            if (!Uri.TryCreate(url, UriKind.Absolute, out var uri) ||
                (uri.Scheme != Uri.UriSchemeHttp && uri.Scheme != Uri.UriSchemeHttps) ||
                string.IsNullOrEmpty(uri.Host))
                _errors[nameof(BaseUrl)] = "接口地址不是合法的 http(s) URL";
        }

        if (ApiKey.Contains('…')) _errors[nameof(ApiKey)] = "请重新粘贴完整密钥（当前是掩码值）";

        if (!double.TryParse(Temperature, NumberStyles.Float, CultureInfo.InvariantCulture, out var temp) || temp < 0 || temp > 2)
            _errors[nameof(Temperature)] = "温度需在 0 ~ 2 之间";

        if (!int.TryParse(MaxTokens, out var maxTokens) || maxTokens < 0)
            _errors[nameof(MaxTokens)] = "最大输出需为不小于 0 的整数";

        if (!int.TryParse(ContextWindow, out var ctx) || ctx <= 0)
            _errors[nameof(ContextWindow)] = "上下文窗口需为正整数";
        else if (ctx > MaxContextWindow)
            _errors[nameof(ContextWindow)] = "上下文窗口不能超过 2000000";
        else if (maxTokens >= 0 && ctx - maxTokens < MinInputReserve)
            _errors[nameof(ContextWindow)] = $"窗口减去最大输出后须至少留 {MinInputReserve} tokens 输入预算";

        return _errors;
    }

    private void ParseNumbers(out double? temperature, out int? maxTokens, out int? contextWindow)
    {
        temperature = double.TryParse(Temperature, NumberStyles.Float, CultureInfo.InvariantCulture, out var t) ? t : null;
        maxTokens = int.TryParse(MaxTokens, out var m) ? m : null;
        contextWindow = int.TryParse(ContextWindow, out var c) ? c : null;
    }

    private static string Num(double? value, string fallback)
        => value?.ToString("0.##", CultureInfo.InvariantCulture) ?? fallback;

    private static string Int(int? value, string fallback)
        => value?.ToString(CultureInfo.InvariantCulture) ?? fallback;
}
