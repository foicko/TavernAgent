using System.Collections.ObjectModel;
using Avalonia.Media.Imaging;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.Services;

namespace TavernAgent.Desktop.ViewModels.Modals.Settings;

/// <summary>Image generation settings tab (web: ImageSettingsPage).</summary>
public sealed partial class ImageSettingsPageViewModel : WorkspaceChildViewModel, ISettingsPage
{
    private readonly ImageSettings _s;
    private bool _loaded;

    public ImageSettingsPageViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        _s = SettingsPersistence.LoadImage();
        _selectedEngine = Engines.FirstOrDefault(o => o.Value == Wire(_s.Engine)) ?? Engines[0];
        _customBaseUrl = _s.CustomBaseUrl;
        _customApiKey = _s.CustomApiKey;
        _customModel = _s.CustomModel;
        _selectedSize = Sizes.FirstOrDefault(o => o.Value == _s.Size) ?? Sizes[0];
        _selectedStyle = Styles.FirstOrDefault(o => o.Value == _s.StylePreset) ?? Styles[0];
        _loaded = true;
    }

    public IReadOnlyList<OptionItem> Engines { get; } = new List<OptionItem>
    {
        new("comfyui", "ComfyUI"),
        new("openai", "OpenAI 兼容（含 SiliconFlow）"),
        new("sd-webui", "Stable Diffusion WebUI"),
    };

    public IReadOnlyList<OptionItem> Sizes { get; } = new List<OptionItem>
    {
        new("1024x576", "16:9 横版（1024×576）"),
        new("1024x1024", "1:1 方形（1024×1024）"),
        new("576x1024", "9:16 竖版（576×1024）"),
    };

    public IReadOnlyList<OptionItem> Styles { get; } = new List<OptionItem>
    {
        new("anime", "日系动画"),
        new("fantasy", "奇幻插画"),
        new("cinematic", "电影感"),
        new("realistic", "写实"),
    };

    [ObservableProperty] private OptionItem _selectedEngine;
    [ObservableProperty] private string _customBaseUrl = "";
    [ObservableProperty] private string _customApiKey = "";
    [ObservableProperty] private string _customModel = "";
    [ObservableProperty] private OptionItem _selectedSize;
    [ObservableProperty] private OptionItem _selectedStyle;
    [ObservableProperty] private bool _generating;
    [ObservableProperty] private Bitmap? _preview;
    [ObservableProperty] private string? _testMessage;
    [ObservableProperty] private bool _testError;
    [ObservableProperty] private string? _notice;

    public bool HasPreview => Preview is not null;
    public bool HasTestMessage => !string.IsNullOrEmpty(TestMessage);
    public bool ShowTestSuccess => HasTestMessage && !TestError;
    public bool HasNotice => !string.IsNullOrEmpty(Notice);

    partial void OnSelectedEngineChanged(OptionItem value)
    {
        _s.Engine = Unwire(value.Value);
        if (string.IsNullOrEmpty(CustomBaseUrl)) CustomBaseUrl = DefaultBaseUrl(value.Value);
        Persist();
    }

    partial void OnCustomBaseUrlChanged(string value) { _s.CustomBaseUrl = value; Persist(); }
    partial void OnCustomApiKeyChanged(string value) { _s.CustomApiKey = value; Persist(); }
    partial void OnCustomModelChanged(string value) { _s.CustomModel = value; Persist(); }
    partial void OnSelectedSizeChanged(OptionItem value) { _s.Size = value.Value; Persist(); }
    partial void OnSelectedStyleChanged(OptionItem value) { _s.StylePreset = value.Value; Persist(); }
    partial void OnPreviewChanged(Bitmap? value) => OnPropertyChanged(nameof(HasPreview));
    partial void OnTestMessageChanged(string? value) { OnPropertyChanged(nameof(HasTestMessage)); OnPropertyChanged(nameof(ShowTestSuccess)); }
    partial void OnTestErrorChanged(bool value) => OnPropertyChanged(nameof(ShowTestSuccess));
    partial void OnNoticeChanged(string? value) => OnPropertyChanged(nameof(HasNotice));

    public Task LoadAsync() => Task.CompletedTask;

    [RelayCommand]
    private void ApplyComfy() => Apply("comfyui", "http://127.0.0.1:8188", "image_qwen_image_2_1_t2i");

    [RelayCommand]
    private void ApplySiliconFlow() => Apply("openai", "https://api.siliconflow.cn/v1", "black-forest-labs/FLUX.1-schnell");

    [RelayCommand]
    private void ApplyOpenAI() => Apply("openai", "https://api.openai.com/v1", "dall-e-3");

    [RelayCommand]
    private void ApplySdWebUI() => Apply("sd-webui", "http://127.0.0.1:7860", "sd-xl");

    [RelayCommand]
    private void ResetDefaults()
    {
        var d = new ImageSettings();
        SelectedEngine = Engines.First(o => o.Value == Wire(d.Engine));
        CustomBaseUrl = d.CustomBaseUrl;
        CustomApiKey = d.CustomApiKey;
        CustomModel = d.CustomModel;
        SelectedSize = Sizes.First(o => o.Value == d.Size);
        SelectedStyle = Styles.First(o => o.Value == d.StylePreset);
        Notice = "已恢复默认生图设置。";
    }

    [RelayCommand]
    private async Task TestGenerateAsync()
    {
        Generating = true;
        TestMessage = null;
        TestError = false;
        try
        {
            var result = await Workspace.Story.Api.GenerateImageAsync(new GenerateImageParams
            {
                Prompt = "A magical tavern at night, warm lantern light, fantasy anime style",
                Engine = SelectedEngine.Value,
                BaseUrl = CustomBaseUrl,
                ApiKey = CustomApiKey,
                Model = CustomModel,
                Size = SelectedSize.Value,
                Style = SelectedStyle.Value,
            }).ConfigureAwait(true);
            TestMessage = $"✓ 图像生成成功并已本地落盘：{result.Id}";
            Preview = await ImageLoader.LoadAsync(ResolveUrl(result.Url), 480).ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            TestError = true;
            TestMessage = "生图失败，请检查图像配置：" + ex.Message;
        }
        finally
        {
            Generating = false;
        }
    }

    private void Apply(string engine, string baseUrl, string model)
    {
        SelectedEngine = Engines.First(o => o.Value == engine);
        CustomBaseUrl = baseUrl;
        CustomModel = model;
        Notice = "已应用预设。";
    }

    private void Persist() { if (_loaded) SettingsPersistence.SaveImage(_s); }

    private string ResolveUrl(string url)
    {
        if (string.IsNullOrEmpty(url)) return "";
        if (url.StartsWith("http", StringComparison.OrdinalIgnoreCase) ||
            url.StartsWith("data:", StringComparison.OrdinalIgnoreCase) ||
            File.Exists(url)) return url;
        var baseAddress = Workspace.Story.Api.BaseAddress;
        if (baseAddress is null) return url;
        return new Uri(baseAddress, url).ToString();
    }

    private static string DefaultBaseUrl(string engine) => engine switch
    {
        "comfyui" => "http://127.0.0.1:8188",
        "sd-webui" => "http://127.0.0.1:7860",
        _ => "https://api.openai.com/v1",
    };

    private static string Wire(ImageEngine e) => e switch
    {
        ImageEngine.OpenAI => "openai",
        ImageEngine.SdWebUI => "sd-webui",
        _ => "comfyui",
    };

    private static ImageEngine Unwire(string value) => value switch
    {
        "openai" => ImageEngine.OpenAI,
        "sd-webui" => ImageEngine.SdWebUI,
        _ => ImageEngine.ComfyUI,
    };
}
