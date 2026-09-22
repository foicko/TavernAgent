using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.Services;
using TavernAgent.Desktop.ViewModels.Stage;

namespace TavernAgent.Desktop.ViewModels.Left;

/// <summary>Character card roster, server-backed (web: LeftRail characters panel).</summary>
public sealed partial class CharactersPanelViewModel : WorkspaceChildViewModel
{
    private bool _disposed;

    public CharactersPanelViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        Story.PropertyChanged += OnStoryChanged;
    }

    public StoryStore Story => Workspace.Story;

    public ObservableCollection<CardRowViewModel> Cards { get; } = new();

    [ObservableProperty] private bool _isLoading;
    [ObservableProperty] private string? _error;

    public bool HasCards => Cards.Count > 0;
    public bool IsEmpty => !IsLoading && Cards.Count == 0;

    [RelayCommand]
    private void Import() => Workspace.Ui.SetCharImportOpen(true);

    [RelayCommand]
    private async Task Refresh() => await ReloadAsync().ConfigureAwait(true);

    public async Task ReloadAsync()
    {
        IsLoading = true;
        Error = null;
        try
        {
            var list = await CardLibrary.ListAsync(Story.Api).ConfigureAwait(true);
            Cards.Clear();
            foreach (var card in list)
            {
                var row = new CardRowViewModel(this, card);
                Cards.Add(row);
                _ = row.LoadThumbAsync();
            }
        }
        catch (Exception ex)
        {
            Error = ex.Message;
        }
        finally
        {
            IsLoading = false;
            OnPropertyChanged(nameof(HasCards));
            OnPropertyChanged(nameof(IsEmpty));
        }
    }

    public int SessionCountFor(string cardId, string? characterId)
        => Story.Sessions.Count(s =>
            s.CharacterId == cardId || (!string.IsNullOrEmpty(characterId) && s.CharacterId == characterId));

    public bool IsActiveCard(string cardId, string? characterId)
    {
        var cid = Story.View?.CharacterId;
        if (string.IsNullOrEmpty(cid)) return false;
        return cid == cardId || (!string.IsNullOrEmpty(characterId) && cid == characterId);
    }

    public async Task OpenCardAsync(CardRowViewModel row)
    {
        CardPreview? card = null;
        try { card = await Story.Api.GetCardAsync(row.CardId).ConfigureAwait(true); }
        catch { /* fall back to summary */ }

        var characterId = CardLibrary.ExtractCharacterId(card) ?? row.CardId;
        var existing = Story.Sessions.FirstOrDefault(s => s.CharacterId == characterId || s.CharacterId == row.CardId);
        if (existing is not null)
        {
            await Story.OpenSessionAsync(existing.SessionId).ConfigureAwait(true);
            return;
        }

        var opts = new CreateSessionOpts
        {
            Title = $"{row.Name} 的冒险",
            CardId = row.CardId,
            PlayerName = "旅人",
            OpeningVariantId = card?.Openings.FirstOrDefault()?.VariantId,
            OpeningText = card is null || card.Openings.Count > 0 ? null : (string.IsNullOrWhiteSpace(card.FirstMes) ? null : card.FirstMes),
        };
        try
        {
            await Story.CreateSessionAsync(opts).ConfigureAwait(true);
            Workspace.Ui.NotifyQuiet($"已与「{row.Name}」开启新故事");
        }
        catch (Exception ex)
        {
            Workspace.Ui.NotifyQuiet("创建失败：" + ex.Message);
        }
    }

    public async Task DeleteCardAsync(CardRowViewModel row)
    {
        try
        {
            await Story.Api.DeleteCardAsync(row.CardId).ConfigureAwait(true);
            Cards.Remove(row);
            Workspace.Ui.NotifyQuiet($"已从卡库移除「{row.Name}」");
            OnPropertyChanged(nameof(HasCards));
            OnPropertyChanged(nameof(IsEmpty));
        }
        catch
        {
            Workspace.Ui.NotifyQuiet($"移除「{row.Name}」失败，请稍后重试");
        }
    }

    private void OnStoryChanged(object? sender, System.ComponentModel.PropertyChangedEventArgs e)
    {
        if (e.PropertyName is nameof(StoryStore.View) or nameof(StoryStore.Sessions))
        {
            foreach (var row in Cards) row.RefreshState();
        }
    }

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        Story.PropertyChanged -= OnStoryChanged;
    }
}

public sealed partial class CardRowViewModel : ObservableObject
{
    private readonly CharactersPanelViewModel _panel;

    public CardRowViewModel(CharactersPanelViewModel panel, CardLibrarySummary card)
    {
        _panel = panel;
        CardId = card.CardId;
        Name = card.Name;
        ShortName = string.IsNullOrWhiteSpace(card.ShortName) ? NarrativeText.CleanShortName(card.Name) : card.ShortName!;
        Role = card.Role ?? "";
        AvatarSource = card.Avatar;
        Format = string.IsNullOrWhiteSpace(card.Format) ? "V2/V3" : card.Format!;
        RefreshState();
    }

    public string CardId { get; }
    public string Name { get; }
    public string ShortName { get; }
    public string Role { get; }
    public string? AvatarSource { get; }
    public string Format { get; }

    [ObservableProperty] private Avalonia.Media.Imaging.Bitmap? _avatar;
    [ObservableProperty] private bool _isConfirmingDelete;
    [ObservableProperty] private int _sessionCount;
    [ObservableProperty] private bool _isActive;

    public bool HasAvatar => Avatar is not null;
    public string Monogram => string.IsNullOrEmpty(ShortName) ? "🎭" : ShortName[..1];
    public string Badge => IsActive ? "当前主角" : SessionCount > 0 ? $"{SessionCount} 个存档" : Format.ToUpperInvariant();
    public string Desc => string.IsNullOrWhiteSpace(Role) ? "导入的角色卡" : Role;

    public async Task LoadThumbAsync()
    {
        var bmp = await ImageLoader.LoadAsync(AvatarSource, 96).ConfigureAwait(true);
        Avatar = bmp;
        OnPropertyChanged(nameof(HasAvatar));
    }

    public void RefreshState()
    {
        SessionCount = _panel.SessionCountFor(CardId, null);
        IsActive = _panel.IsActiveCard(CardId, null);
        OnPropertyChanged(nameof(Badge));
    }

    [RelayCommand] private async Task Open() => await _panel.OpenCardAsync(this).ConfigureAwait(true);
    [RelayCommand] private void RequestDelete() => IsConfirmingDelete = true;
    [RelayCommand] private void CancelDelete() => IsConfirmingDelete = false;
    [RelayCommand] private async Task ConfirmDelete() => await _panel.DeleteCardAsync(this).ConfigureAwait(true);
}
