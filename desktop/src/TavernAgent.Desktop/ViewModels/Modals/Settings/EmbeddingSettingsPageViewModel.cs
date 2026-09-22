using System.Globalization;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Modals.Settings;

/// <summary>Memory embedding settings tab (web: EmbeddingSettingsPage).</summary>
public sealed partial class EmbeddingSettingsPageViewModel : WorkspaceChildViewModel, ISettingsPage
{
    private readonly EmbeddingSettings _s;
    private bool _loaded;

    public EmbeddingSettingsPageViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        _s = SettingsPersistence.LoadEmbedding();
        _enabled = _s.Enabled;
        _selectedProvider = Providers.FirstOrDefault(o => o.Value == Wire(_s.Provider)) ?? Providers[0];
        _baseUrl = _s.BaseUrl;
        _apiKey = _s.ApiKey;
        _model = _s.Model;
        _dimension = _s.Dimension.ToString(CultureInfo.InvariantCulture);
        _topK = _s.TopK.ToString(CultureInfo.InvariantCulture);
        _weight = _s.HybridRRFWeight;
        _loaded = true;
    }

    public IReadOnlyList<OptionItem> Providers { get; } = new List<OptionItem>
    {
        new("ollama", "本地 Ollama"),
        new("siliconflow", "SiliconFlow"),
        new("openai", "OpenAI"),
        new("custom", "自定义端点"),
    };

    [ObservableProperty] private bool _enabled = true;
    [ObservableProperty] private OptionItem _selectedProvider;
    [ObservableProperty] private string _baseUrl = "";
    [ObservableProperty] private string _apiKey = "";
    [ObservableProperty] private string _model = "";
    [ObservableProperty] private string _dimension = "512";
    [ObservableProperty] private string _topK = "5";
    [ObservableProperty] private double _weight = 0.5;
    [ObservableProperty] private bool _probing;
    [ObservableProperty] private string? _resultText;
    [ObservableProperty] private bool _resultError;
    [ObservableProperty] private string? _notice;

    public bool HasResult => !string.IsNullOrEmpty(ResultText);
    public bool ShowSuccess => HasResult && !ResultError;
    public bool HasNotice => !string.IsNullOrEmpty(Notice);
    public string WeightLabel => $"词法 BM25 {(1 - Weight):0.0} : 向量语义 {Weight:0.0}";

    partial void OnEnabledChanged(bool value) { _s.Enabled = value; Persist(); }
    partial void OnSelectedProviderChanged(OptionItem value) { _s.Provider = Unwire(value.Value); Persist(); }
    partial void OnBaseUrlChanged(string value) { _s.BaseUrl = value; Persist(); }
    partial void OnApiKeyChanged(string value) { _s.ApiKey = value; Persist(); }
    partial void OnModelChanged(string value) { _s.Model = value; Persist(); }
    partial void OnDimensionChanged(string value) { if (int.TryParse(value, out var d) && d > 0) { _s.Dimension = d; Persist(); } }
    partial void OnTopKChanged(string value) { if (int.TryParse(value, out var k) && k > 0) { _s.TopK = k; Persist(); } }
    partial void OnWeightChanged(double value) { _s.HybridRRFWeight = value; Persist(); OnPropertyChanged(nameof(WeightLabel)); }
    partial void OnResultTextChanged(string? value) { OnPropertyChanged(nameof(HasResult)); OnPropertyChanged(nameof(ShowSuccess)); }
    partial void OnResultErrorChanged(bool value) => OnPropertyChanged(nameof(ShowSuccess));
    partial void OnNoticeChanged(string? value) => OnPropertyChanged(nameof(HasNotice));

    public Task LoadAsync() => Task.CompletedTask;

    [RelayCommand]
    private void ApplyBgeSmall() => Apply("ollama", "http://127.0.0.1:11434/v1", "bge-small-zh-v1.5", 512);

    [RelayCommand]
    private void ApplySiliconFlow() => Apply("siliconflow", "https://api.siliconflow.cn/v1", "BAAI/bge-small-zh-v1.5", 512);

    [RelayCommand]
    private void ApplyBgeM3() => Apply("ollama", "http://127.0.0.1:11434/v1", "bge-m3", 1024);

    [RelayCommand]
    private void ApplyNomic() => Apply("ollama", "http://127.0.0.1:11434/v1", "nomic-embed-text", 768);

    [RelayCommand]
    private void ApplyOpenAI() => Apply("openai", "https://api.openai.com/v1", "text-embedding-3-small", 1536);

    [RelayCommand]
    private void ResetDefaults()
    {
        var d = new EmbeddingSettings();
        Enabled = d.Enabled;
        SelectedProvider = Providers.First(o => o.Value == Wire(d.Provider));
        BaseUrl = d.BaseUrl;
        ApiKey = d.ApiKey;
        Model = d.Model;
        Dimension = d.Dimension.ToString(CultureInfo.InvariantCulture);
        TopK = d.TopK.ToString(CultureInfo.InvariantCulture);
        Weight = d.HybridRRFWeight;
        Notice = "已恢复默认向量配置。";
    }

    [RelayCommand]
    private async Task TestProbeAsync()
    {
        Probing = true;
        ResultText = null;
        ResultError = false;
        try
        {
            var res = await Workspace.Story.Api.ProbeEmbeddingAsync(new EmbeddingProbeRequest
            {
                BaseUrl = BaseUrl,
                ApiKey = ApiKey,
                Model = Model,
                SampleText = "这是一段用于测试向量模型的示例文本。",
            }).ConfigureAwait(true);

            if (!res.Ok)
            {
                ResultError = true;
                ResultText = "探测失败：" + (res.Error ?? "未知错误") +
                    "\n排查建议：确认 Ollama 已在 11434 端口运行、模型已 pull、云端密钥有效。";
                return;
            }

            if (res.Dimension > 0 && res.Dimension.ToString(CultureInfo.InvariantCulture) != Dimension)
                Dimension = res.Dimension.ToString(CultureInfo.InvariantCulture);

            var preview = res.Preview is { Count: > 0 }
                ? " · 预览 [" + string.Join(", ", res.Preview.Take(5).Select(x => x.ToString("F5", CultureInfo.InvariantCulture))) + " …]"
                : "";
            ResultText = $"✓ 连通 · 模型 {res.Model} · 维度 {res.Dimension} · 延迟 {res.LatencyMs} ms{preview}";
        }
        catch (Exception ex)
        {
            ResultError = true;
            ResultText = "探测失败：" + ex.Message;
        }
        finally
        {
            Probing = false;
        }
    }

    private void Apply(string provider, string baseUrl, string model, int dimension)
    {
        SelectedProvider = Providers.First(o => o.Value == provider);
        BaseUrl = baseUrl;
        Model = model;
        Dimension = dimension.ToString(CultureInfo.InvariantCulture);
        Notice = $"已应用「{model}」预设。";
    }

    private void Persist() { if (_loaded) SettingsPersistence.SaveEmbedding(_s); }

    private static string Wire(EmbeddingProviderType p) => p switch
    {
        EmbeddingProviderType.SiliconFlow => "siliconflow",
        EmbeddingProviderType.OpenAI => "openai",
        EmbeddingProviderType.Custom => "custom",
        _ => "ollama",
    };

    private static EmbeddingProviderType Unwire(string value) => value switch
    {
        "siliconflow" => EmbeddingProviderType.SiliconFlow,
        "openai" => EmbeddingProviderType.OpenAI,
        "custom" => EmbeddingProviderType.Custom,
        _ => EmbeddingProviderType.Ollama,
    };
}
