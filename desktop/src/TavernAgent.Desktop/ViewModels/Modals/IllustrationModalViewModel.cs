using Avalonia.Media.Imaging;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.Services;
using TavernAgent.Desktop.ViewModels.Modals.Settings;
using TavernAgent.Desktop.ViewModels.Stage;

namespace TavernAgent.Desktop.ViewModels.Modals;

/// <summary>Generate a scene illustration for a turn (web: IllustrationModal).</summary>
public sealed partial class IllustrationModalViewModel : WorkspaceChildViewModel
{
    private readonly ImageSettings _settings;

    public IllustrationModalViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        _settings = SettingsPersistence.LoadImage();
        _selectedStyle = Styles.FirstOrDefault(o => o.Value == _settings.StylePreset) ?? Styles[0];
        _selectedSize = Sizes.FirstOrDefault(o => o.Value == _settings.Size) ?? Sizes[0];
    }

    public IReadOnlyList<OptionItem> Styles { get; } = new List<OptionItem>
    {
        new("anime", "日系动画"),
        new("fantasy", "奇幻插画"),
        new("cinematic", "电影感"),
        new("realistic", "写实"),
    };

    public IReadOnlyList<OptionItem> Sizes { get; } = new List<OptionItem>
    {
        new("1024x576", "16:9 横版"),
        new("1024x1024", "1:1 方形"),
        new("576x1024", "9:16 竖版"),
    };

    [ObservableProperty] private bool _isOpen;
    [ObservableProperty] private string? _turnId;
    [ObservableProperty] private string _prompt = "";
    [ObservableProperty] private OptionItem _selectedStyle;
    [ObservableProperty] private OptionItem _selectedSize;
    [ObservableProperty] private bool _generating;
    [ObservableProperty] private string? _error;
    [ObservableProperty] private string? _notice;
    [ObservableProperty] private Bitmap? _preview;

    private string? _characterName;

    public bool HasError => !string.IsNullOrEmpty(Error);
    public bool HasNotice => !string.IsNullOrEmpty(Notice);
    public bool HasPreview => Preview is not null;
    public bool CanGenerate => !Generating && !string.IsNullOrWhiteSpace(Prompt);

    partial void OnErrorChanged(string? value) => OnPropertyChanged(nameof(HasError));
    partial void OnNoticeChanged(string? value) => OnPropertyChanged(nameof(HasNotice));
    partial void OnPreviewChanged(Bitmap? value) => OnPropertyChanged(nameof(HasPreview));
    partial void OnPromptChanged(string value) => OnPropertyChanged(nameof(CanGenerate));
    partial void OnGeneratingChanged(bool value) => OnPropertyChanged(nameof(CanGenerate));
    partial void OnSelectedStyleChanged(OptionItem value) => Reprompt();

    public void Open(string turnId, string fallbackText, string? characterName)
    {
        TurnId = turnId;
        _characterName = characterName;
        Reprompt(fallbackText);
        Preview = null;
        Error = null;
        Notice = null;
        IsOpen = true;
    }

    [RelayCommand]
    private void Close() => IsOpen = false;

    [RelayCommand]
    private async Task GenerateAsync()
    {
        if (!CanGenerate) return;
        Generating = true;
        Error = null;
        Notice = null;
        try
        {
            var result = await Workspace.Story.Api.GenerateImageAsync(new GenerateImageParams
            {
                Prompt = Prompt,
                NegativePrompt = ImagePromptExtractor.Extract(null, Prompt, SelectedStyle.Value, _characterName).NegativePrompt,
                Engine = Wire(_settings.Engine),
                BaseUrl = _settings.CustomBaseUrl,
                ApiKey = _settings.CustomApiKey,
                Model = _settings.CustomModel,
                Size = SelectedSize.Value,
                Style = SelectedStyle.Value,
            }).ConfigureAwait(true);

            if (!string.IsNullOrEmpty(TurnId)) IllustrationStore.Instance.Attach(TurnId!, result);
            Preview = await ImageLoader.LoadAsync(TurnIllustrationViewModel.AbsoluteUrl(result.Url), 640).ConfigureAwait(true);
            Notice = "✓ 插画已绘制成功并自动关联至本回合！";
        }
        catch (Exception ex)
        {
            Error = "生图失败，请检查图像配置：" + ex.Message;
        }
        finally
        {
            Generating = false;
        }
    }

    private void Reprompt(string? fallback = null)
    {
        var text = fallback;
        if (text is null && !string.IsNullOrEmpty(TurnId))
        {
            var message = Workspace.Stage.Messages.FirstOrDefault(m => m.Id == TurnId);
            text = message?.PlainText;
        }
        var extracted = ImagePromptExtractor.Extract(null, text, SelectedStyle.Value, _characterName);
        Prompt = extracted.Prompt;
    }

    private static string Wire(ImageEngine e) => e switch
    {
        ImageEngine.OpenAI => "openai",
        ImageEngine.SdWebUI => "sd-webui",
        _ => "comfyui",
    };
}
