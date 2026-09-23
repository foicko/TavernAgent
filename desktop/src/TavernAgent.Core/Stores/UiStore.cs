using TavernAgent.Core.Api;
using TavernAgent.Core.Dtos;

namespace TavernAgent.Core.Stores;

/// <summary>
/// Port of web uiStore: viewport mode, rails, theme, modals, micro-interactions.
/// </summary>
public sealed class UiStore : ObservableStore
{
    public static UiStore Instance { get; } = new();

    private ViewportMode _viewMode = ViewportMode.Studio;
    public ViewportMode ViewMode { get => _viewMode; private set => SetProperty(ref _viewMode, value); }

    private bool _leftCollapsed;
    public bool LeftCollapsed { get => _leftCollapsed; private set => SetProperty(ref _leftCollapsed, value); }

    private bool _rightCollapsed;
    public bool RightCollapsed { get => _rightCollapsed; private set => SetProperty(ref _rightCollapsed, value); }

    // Responsive auto-collapse, driven by the window width (web: media queries).
    // Kept separate from the user's manual preference so resizing can re-expand
    // without clobbering an explicit choice.
    private bool _autoLeftCollapsed;
    public bool AutoLeftCollapsed { get => _autoLeftCollapsed; private set => SetProperty(ref _autoLeftCollapsed, value); }

    private bool _autoRightCollapsed;
    public bool AutoRightCollapsed { get => _autoRightCollapsed; private set => SetProperty(ref _autoRightCollapsed, value); }

    private bool _flyoutOpen;
    public bool FlyoutOpen { get => _flyoutOpen; private set => SetProperty(ref _flyoutOpen, value); }

    private LeftSubtab _leftTab = LeftSubtab.Tree;
    public LeftSubtab LeftTab { get => _leftTab; private set => SetProperty(ref _leftTab, value); }

    private ThemeMode _theme = ThemeMode.Dark;
    public ThemeMode Theme { get => _theme; private set => SetProperty(ref _theme, value); }

    private string? _activeCharKey = CharacterPresets.DefaultPresetKey;
    public string? ActiveCharKey { get => _activeCharKey; private set => SetProperty(ref _activeCharKey, value); }

    private TachieExpression _tachieExpression = TachieExpression.Alert;
    public TachieExpression TachieExpression { get => _tachieExpression; private set => SetProperty(ref _tachieExpression, value); }

    private bool _fullTachieOpen;
    public bool FullTachieOpen { get => _fullTachieOpen; private set => SetProperty(ref _fullTachieOpen, value); }

    private bool _worldbookModalOpen;
    public bool WorldbookModalOpen { get => _worldbookModalOpen; private set => SetProperty(ref _worldbookModalOpen, value); }

    private bool _charImportOpen;
    public bool CharImportOpen { get => _charImportOpen; private set => SetProperty(ref _charImportOpen, value); }

    private string? _pendingCardId;
    public string? PendingCardId { get => _pendingCardId; private set => SetProperty(ref _pendingCardId, value); }

    private bool _blueprintOpen;
    public bool BlueprintOpen { get => _blueprintOpen; private set => SetProperty(ref _blueprintOpen, value); }

    private bool _directorOpen;
    public bool DirectorOpen { get => _directorOpen; private set => SetProperty(ref _directorOpen, value); }

    private bool _paletteOpen;
    public bool PaletteOpen { get => _paletteOpen; private set => SetProperty(ref _paletteOpen, value); }

    private bool _dossierModalOpen;
    public bool DossierModalOpen { get => _dossierModalOpen; private set => SetProperty(ref _dossierModalOpen, value); }

    private bool _promisesModalOpen;
    public bool PromisesModalOpen { get => _promisesModalOpen; private set => SetProperty(ref _promisesModalOpen, value); }

    private bool _memoryModalOpen;
    public bool MemoryModalOpen { get => _memoryModalOpen; private set => SetProperty(ref _memoryModalOpen, value); }

    private bool _settingsOpen;
    public bool SettingsOpen { get => _settingsOpen; private set => SetProperty(ref _settingsOpen, value); }

    private string? _toastMessage;
    public string? ToastMessage { get => _toastMessage; private set => SetProperty(ref _toastMessage, value); }

    private string? _injectedComposerText;
    public string? InjectedComposerText { get => _injectedComposerText; private set => SetProperty(ref _injectedComposerText, value); }

    private string? _selectionWrapRequest;
    public string? SelectionWrapRequest { get => _selectionWrapRequest; private set => SetProperty(ref _selectionWrapRequest, value); }

    private (bool Left, bool Right) _savedBeforeReading;

    public void ToggleLeftRail()
    {
        if (ViewMode == ViewportMode.Reading)
        {
            NotifyQuiet("纯净编年阅读模式下侧栏默认整体隐藏");
            return;
        }
        if (!LeftCollapsed && !AutoLeftCollapsed)
        {
            LeftCollapsed = true;
            NotifyQuiet("左栏已收起");
        }
        else
        {
            LeftCollapsed = false;
            AutoLeftCollapsed = false; // an explicit expand wins over the width rule
            NotifyQuiet("左栏已展开");
        }
    }

    public void ToggleRightRail()
    {
        if (ViewMode == ViewportMode.Reading)
        {
            NotifyQuiet("纯净编年阅读模式下侧栏默认整体隐藏");
            return;
        }
        if (!RightCollapsed && !AutoRightCollapsed)
        {
            RightCollapsed = true;
            NotifyQuiet("右侧角色立绘与状态已收起");
        }
        else
        {
            RightCollapsed = false;
            AutoRightCollapsed = false;
            NotifyQuiet("右侧角色立绘与状态已展开");
        }
    }

    public void SetLeftRailCollapsed(bool collapsed) => LeftCollapsed = collapsed;
    public void SetRightRailCollapsed(bool collapsed) => RightCollapsed = collapsed;

    /// <summary>Window-width driven collapse (does not touch the user's manual preference).</summary>
    public void SetAutoLeftCollapsed(bool collapsed) => AutoLeftCollapsed = collapsed;
    public void SetAutoRightCollapsed(bool collapsed) => AutoRightCollapsed = collapsed;

    public void CloseAllDrawers()
    {
        LeftCollapsed = true;
        RightCollapsed = true;
    }

    public void SwitchLeftTab(LeftSubtab tab) => LeftTab = tab;

    public void OpenLeftTabAndExpand(LeftSubtab tab)
    {
        LeftTab = tab;
        LeftCollapsed = false;
        AutoLeftCollapsed = false;
        FlyoutOpen = true;
    }

    /// <summary>Sidebar behaviour: clicking the active tab again collapses the panel to the icon dock.</summary>
    public void ToggleFlyout(LeftSubtab tab)
    {
        if (!LeftCollapsed && !AutoLeftCollapsed && LeftTab == tab)
        {
            LeftCollapsed = true;
            FlyoutOpen = false;
            return;
        }
        LeftTab = tab;
        LeftCollapsed = false;
        AutoLeftCollapsed = false;
        FlyoutOpen = true;
    }

    public void CloseFlyout()
    {
        FlyoutOpen = false;
        LeftCollapsed = true;
    }

    public void SwitchViewMode(ViewportMode mode)
    {
        if (mode == ViewMode) return;
        if (mode == ViewportMode.Reading)
        {
            _savedBeforeReading = (LeftCollapsed, RightCollapsed);
            FlyoutOpen = false;
            ViewMode = ViewportMode.Reading;
            NotifyQuiet("已进入：纯净编年模式（全视口纯粹文学阅读）");
        }
        else
        {
            ViewMode = ViewportMode.Studio;
            LeftCollapsed = _savedBeforeReading.Left;
            RightCollapsed = _savedBeforeReading.Right;
            NotifyQuiet("已返回：跑团工作台（恢复自由双栏与交互视口）");
        }
    }

    public void ToggleTheme() => SetTheme(Theme == ThemeMode.Dark ? ThemeMode.Light : ThemeMode.Dark);

    public void SetTheme(ThemeMode theme) => Theme = theme;

    public void SwitchActiveCharacter(string key)
    {
        if (CharacterPresets.Presets.ContainsKey(key))
        {
            ActiveCharKey = key;
            TachieExpression = TachieExpression.Alert;
            NotifyQuiet($"已切换角色卡：{CharacterPresets.Presets[key].Name}");
        }
    }

    public void SetActiveCharKey(string? key)
    {
        if (key is null)
        {
            ActiveCharKey = null;
            TachieExpression = TachieExpression.Alert;
            return;
        }
        if (CharacterPresets.Presets.ContainsKey(key))
        {
            ActiveCharKey = key;
            TachieExpression = TachieExpression.Alert;
        }
    }

    public void SetTachieExpression(TachieExpression expr) => TachieExpression = expr;

    public void SetFullTachieOpen(bool open) => FullTachieOpen = open;
    public void SetWorldbookModalOpen(bool open) => WorldbookModalOpen = open;

    public void SetCharImportOpen(bool open)
    {
        CharImportOpen = open;
        if (!open) PendingCardId = null;
    }

    public void OpenCharImportWithCard(string cardId)
    {
        CharImportOpen = true;
        PendingCardId = cardId;
    }

    public void SetBlueprintOpen(bool open) => BlueprintOpen = open;
    public void SetDirectorOpen(bool open) => DirectorOpen = open;
    public void SetPaletteOpen(bool open) => PaletteOpen = open;
    public void TogglePalette() => PaletteOpen = !PaletteOpen;
    public void SetDossierModalOpen(bool open) => DossierModalOpen = open;
    public void SetPromisesModalOpen(bool open) => PromisesModalOpen = open;
    public void SetMemoryModalOpen(bool open) => MemoryModalOpen = open;
    public void SetSettingsOpen(bool open) => SettingsOpen = open;

    private System.Threading.Timer? _toastTimer;

    public void NotifyQuiet(string message)
    {
        ToastMessage = message;
        _toastTimer?.Dispose();
        _toastTimer = new System.Threading.Timer(_ =>
        {
            if (ToastMessage == message) ToastMessage = null;
        }, null, 2200, Timeout.Infinite);
    }

    public void InjectComposerText(string text) => InjectedComposerText = text;
    public void ClearInjectedText() => InjectedComposerText = null;
    public void RequestSelectionWrap(string wrap) => SelectionWrapRequest = wrap;
    public void ClearSelectionWrap() => SelectionWrapRequest = null;
}

public enum ViewportMode { Studio, Reading }
public enum LeftSubtab { Tree, Characters, Lore, Sessions }
public enum TachieExpression { Alert, Calm, Smile, Whisper }
public enum ThemeMode { Light, Dark }
