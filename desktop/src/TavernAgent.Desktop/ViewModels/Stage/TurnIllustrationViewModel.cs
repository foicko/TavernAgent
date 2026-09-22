using Avalonia.Media.Imaging;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.Services;

namespace TavernAgent.Desktop.ViewModels.Stage;

/// <summary>Thumbnail bar for a turn's generated illustration (web: TurnIllustration).</summary>
public sealed partial class TurnIllustrationViewModel : ObservableObject
{
    private readonly StageViewModel _stage;

    public TurnIllustrationViewModel(StageViewModel stage, string turnId, string fallbackText)
    {
        _stage = stage;
        TurnId = turnId;
        FallbackText = fallbackText;
        _ = ReloadAsync();
    }

    public string TurnId { get; }
    public string FallbackText { get; }

    [ObservableProperty] private Bitmap? _image;

    public bool HasImage => Image is not null;
    public string Caption => "✦ 场景插画 · 已保存";

    partial void OnImageChanged(Bitmap? value) => OnPropertyChanged(nameof(HasImage));

    public async Task ReloadAsync()
    {
        var result = IllustrationStore.Instance.Get(TurnId);
        if (result is null)
        {
            Image = null;
            return;
        }
        Image = await ImageLoader.LoadAsync(AbsoluteUrl(result.Url), 480).ConfigureAwait(true);
    }

    [RelayCommand]
    private void Enlarge() => _stage.OpenLightbox(Image);

    [RelayCommand]
    private void Redraw() => _stage.OpenIllustration(TurnId, FallbackText);

    internal static string AbsoluteUrl(string? url)
    {
        if (string.IsNullOrEmpty(url)) return "";
        if (url.StartsWith("http", StringComparison.OrdinalIgnoreCase) ||
            url.StartsWith("data:", StringComparison.OrdinalIgnoreCase) ||
            File.Exists(url)) return url;
        var baseAddress = AppServices.Api.BaseAddress;
        return baseAddress is null ? url : new Uri(baseAddress, url).ToString();
    }
}
