using System.Collections.ObjectModel;
using System.ComponentModel;
using Avalonia.Media.Imaging;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.Services;
using TavernAgent.Desktop.ViewModels.Stage;

namespace TavernAgent.Desktop.ViewModels.Modals;

/// <summary>Pick / preview a character card and start a new story (web: CharacterImportModal).</summary>
public sealed partial class CharacterImportViewModel : WorkspaceChildViewModel
{
    private int _seq;
    private bool _disposed;

    public CharacterImportViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        Workspace.Ui.PropertyChanged += OnUiChanged;
    }

    public string DefaultPlayerName => NarrativeText.DefaultPlayerName;

    [ObservableProperty] private CardPreview? _preview;
    [ObservableProperty] private Bitmap? _avatar;
    [ObservableProperty] private bool _parsing;
    [ObservableProperty] private bool _creating;
    [ObservableProperty] private string? _error;
    [ObservableProperty] private string _playerName = NarrativeText.DefaultPlayerName;
    [ObservableProperty] private string _playerRole = "";
    [ObservableProperty] private string _customOpening = "";
    [ObservableProperty] private CardOpening? _selectedOpening;

    public ObservableCollection<CardOpening> Openings { get; } = new();
    public ObservableCollection<string> Tags { get; } = new();

    public bool IsOpen => Workspace.Ui.CharImportOpen;
    public bool HasPreview => Preview is not null;
    public bool HasAvatar => Avatar is not null;
    public bool NoAvatar => Avatar is null;
    public bool HasOpenings => Openings.Count > 0;
    public bool HasTags => Tags.Count > 0;
    public bool HasError => !string.IsNullOrEmpty(Error);
    public bool HasWarnings => Preview is { Warnings.Count: > 0 };
    public string WarningsText => Preview is null ? "" : "兼容提示：" + string.Join("；", Preview.Warnings);
    public string Name => Preview is null ? "" : (string.IsNullOrWhiteSpace(Preview.ShortName) ? Preview.Name : Preview.ShortName!);
    public string FormatBadge => Preview is null ? "" : $"{Preview.Format}{(string.IsNullOrEmpty(Preview.SpecVersion) ? "" : " · " + Preview.SpecVersion)}";
    public string Creator => Preview?.Creator ?? "";
    public bool HasCreator => !string.IsNullOrEmpty(Creator);
    public string OpeningsLabel => SelectedOpening?.Title is { Length: > 0 } t ? t : "开场";
    public bool CanConfirm => !Creating && !Parsing && HasPreview &&
        !string.IsNullOrWhiteSpace(PlayerName) &&
        (HasOpenings || !string.IsNullOrWhiteSpace(CustomOpening));

    partial void OnPreviewChanged(CardPreview? value)
    {
        OnPropertyChanged(nameof(HasPreview));
        OnPropertyChanged(nameof(Name));
        OnPropertyChanged(nameof(FormatBadge));
        OnPropertyChanged(nameof(Creator));
        OnPropertyChanged(nameof(HasCreator));
        OnPropertyChanged(nameof(HasWarnings));
        OnPropertyChanged(nameof(WarningsText));
        OnPropertyChanged(nameof(CanConfirm));
    }

    partial void OnAvatarChanged(Bitmap? value) { OnPropertyChanged(nameof(HasAvatar)); OnPropertyChanged(nameof(NoAvatar)); }
    partial void OnErrorChanged(string? value) => OnPropertyChanged(nameof(HasError));
    partial void OnPlayerNameChanged(string value) => OnPropertyChanged(nameof(CanConfirm));
    partial void OnCustomOpeningChanged(string value) => OnPropertyChanged(nameof(CanConfirm));
    partial void OnCreatingChanged(bool value) => OnPropertyChanged(nameof(CanConfirm));
    partial void OnParsingChanged(bool value) => OnPropertyChanged(nameof(CanConfirm));
    partial void OnSelectedOpeningChanged(CardOpening? value) => OnPropertyChanged(nameof(OpeningsLabel));

    private void OnUiChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName != nameof(UiStore.CharImportOpen)) return;
        OnPropertyChanged(nameof(IsOpen));
        if (IsOpen) OnOpenedAsync().SafeFireAndForget();
    }

    private async Task OnOpenedAsync()
    {
        var pending = Workspace.Ui.PendingCardId;
        if (string.IsNullOrEmpty(pending)) return;
        Parsing = true;
        Error = null;
        try
        {
            var preview = await Workspace.Story.Api.GetCardAsync(pending).ConfigureAwait(true);
            ApplyPreview(preview);
        }
        catch (Exception ex)
        {
            Error = "读取角色卡失败：" + ex.Message;
        }
        finally
        {
            Parsing = false;
        }
    }

    public async Task ImportAsync(string fileName, byte[] data)
    {
        var seq = ++_seq;
        Parsing = true;
        Error = null;
        try
        {
            var contentType = fileName.EndsWith(".png", StringComparison.OrdinalIgnoreCase) ? "image/png" : "application/json";
            var preview = await Workspace.Story.Api.ImportCardFileAsync(fileName, data, contentType).ConfigureAwait(true);
            if (seq != _seq) return;
            ApplyPreview(preview);
        }
        catch (Exception ex)
        {
            if (seq == _seq) Error = "解析角色卡失败：" + ex.Message;
        }
        finally
        {
            if (seq == _seq) Parsing = false;
        }
    }

    private void ApplyPreview(CardPreview preview)
    {
        Preview = preview;
        PlayerName = NarrativeText.DefaultPlayerName;
        PlayerRole = "";
        CustomOpening = "";

        Openings.Clear();
        foreach (var opening in preview.Openings ?? new List<CardOpening>()) Openings.Add(opening);
        SelectedOpening = Openings.FirstOrDefault();
        OnPropertyChanged(nameof(HasOpenings));

        Tags.Clear();
        foreach (var tag in (preview.Tags ?? new List<string>()).Take(5)) Tags.Add(tag);
        OnPropertyChanged(nameof(HasTags));

        LoadAvatarAsync(preview.Avatar).SafeFireAndForget();
    }

    private async Task LoadAvatarAsync(string? source)
    {
        Avatar = await ImageLoader.LoadAsync(source, 96).ConfigureAwait(true);
    }

    [RelayCommand]
    private async Task ConfirmAsync()
    {
        if (!CanConfirm || Preview is null) return;
        Creating = true;
        try
        {
            var hasCardId = !string.IsNullOrEmpty(Preview.CardId);
            var opts = new CreateSessionOpts
            {
                Title = $"{Preview.Name} 的冒险",
                CardId = hasCardId ? Preview.CardId : null,
                CharacterJson = hasCardId ? null : Preview.CharacterJson,
                PlayerName = PlayerName.Trim(),
                PlayerRole = string.IsNullOrWhiteSpace(PlayerRole) ? null : PlayerRole.Trim(),
                OpeningVariantId = HasOpenings ? SelectedOpening?.VariantId : null,
                OpeningText = HasOpenings ? null : CustomOpening.Trim(),
            };
            await Workspace.Story.CreateSessionAsync(opts).ConfigureAwait(true);
            Workspace.Ui.SetCharImportOpen(false);
            Workspace.Ui.NotifyQuiet($"已开启「{Preview.Name}」的新故事");
        }
        catch (Exception ex)
        {
            Error = "创建会话失败：" + ex.Message;
            Workspace.Ui.NotifyQuiet("创建会话失败");
        }
        finally
        {
            Creating = false;
        }
    }

    [RelayCommand]
    private void Close()
    {
        if (Creating) return;
        Workspace.Ui.SetCharImportOpen(false);
        Reset();
    }

    private void Reset()
    {
        _seq++;
        Preview = null;
        Avatar = null;
        Error = null;
        Openings.Clear();
        Tags.Clear();
        CustomOpening = "";
        PlayerRole = "";
        PlayerName = NarrativeText.DefaultPlayerName;
    }

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        Workspace.Ui.PropertyChanged -= OnUiChanged;
    }
}
