using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.Services;
using TavernAgent.Desktop.ViewModels.Left;
using TavernAgent.Desktop.ViewModels.Modals;
using TavernAgent.Desktop.ViewModels.Modals.Settings;
using TavernAgent.Desktop.ViewModels.Palette;
using TavernAgent.Desktop.ViewModels.Right;
using TavernAgent.Desktop.ViewModels.Stage;

namespace TavernAgent.Desktop.ViewModels;

/// <summary>
/// Owns the 3-column workspace chrome (header, rails, stage, overlays) and proxies
/// UiStore state into bindable properties. Child feature VMs hang off this.
/// </summary>
public sealed partial class WorkspaceViewModel : ViewModelBase
{
    private readonly UiStore _ui;
    private readonly SettingsStore _settings;
    private bool _disposed;

    public WorkspaceViewModel(UiStore? ui = null, SettingsStore? settings = null)
    {
        _ui = ui ?? AppServices.Ui;
        _settings = settings ?? AppServices.Settings;
        _ui.PropertyChanged += OnUiChanged;
        _settings.PropertyChanged += OnSettingsChanged;
        LeftRail = new LeftRailViewModel(this);
        Stage = new StageViewModel(this);
        RightRail = new RightRailViewModel(this);
        Dossier = new DossierModalViewModel(this);
        Promises = new PromisesModalViewModel(this);
        MemoryModal = new MemoryModalViewModel(this);
        SettingsModal = new SettingsModalViewModel(this);
        CharacterImport = new CharacterImportViewModel(this);
        Worldbook = new WorldbookModalViewModel(this);
        Palette = new CommandPaletteViewModel(this);
    }

    public UiStore Ui => _ui;

    public SettingsStore Settings => _settings;

    public StoryStore Story => AppServices.Story;

    public LeftRailViewModel LeftRail { get; }

    public StageViewModel Stage { get; }

    public RightRailViewModel RightRail { get; }

    public DossierModalViewModel Dossier { get; }

    public PromisesModalViewModel Promises { get; }

    public MemoryModalViewModel MemoryModal { get; }

    public SettingsModalViewModel SettingsModal { get; }

    public CharacterImportViewModel CharacterImport { get; }

    public WorldbookModalViewModel Worldbook { get; }

    public CommandPaletteViewModel Palette { get; }

    // ---- layout geometry (animated via transitions in the view) ----

    public bool IsReadingMode => _ui.ViewMode == ViewportMode.Reading;

    public bool IsStudioMode => !IsReadingMode;

    /// <summary>Width of the persistent left column (224 expanded / 52 icon dock / 0 in reading).</summary>
    public double LeftRailWidth => IsStudioMode ? (_ui.LeftCollapsed ? 52 : 224) : 0;

    /// <summary>Width of the inline right column (304 expanded / 0 collapsed or reading).</summary>
    public double RightRailWidth => IsStudioMode && !_ui.RightCollapsed ? 304 : 0;

    public bool ShowLeftRail => IsStudioMode;

    /// <summary>Inline right rail column visibility (parallels <see cref="RightRailWidth"/>).</summary>
    public bool ShowRightRail => IsStudioMode && !_ui.RightCollapsed;

    public bool ShowRightRailRecall => !IsReadingMode && _ui.RightCollapsed;

    /// <summary>Left panel expanded into the persistent column (web: full rail view).</summary>
    public bool ShowLeftPanel => IsStudioMode && !_ui.LeftCollapsed;

    /// <summary>Collapsed icon dock (web: rail-collapsed-view).</summary>
    public bool ShowIconDock => IsStudioMode && _ui.LeftCollapsed;

    public bool IsLeftCollapsed => _ui.LeftCollapsed;

    public bool IsRightCollapsed => _ui.RightCollapsed;

    public LeftSubtab LeftTab => _ui.LeftTab;
    public bool LeftTabTree => IsStudioMode && _ui.LeftTab == LeftSubtab.Tree;
    public bool LeftTabCharacters => IsStudioMode && _ui.LeftTab == LeftSubtab.Characters;
    public bool LeftTabLore => IsStudioMode && _ui.LeftTab == LeftSubtab.Lore;
    public bool LeftTabSessions => IsStudioMode && _ui.LeftTab == LeftSubtab.Sessions;

    // ---- theme / header ----

    public bool IsDarkTheme => _ui.Theme == ThemeMode.Dark;

    public string ThemeLabel => IsDarkTheme ? "墨黑夜色" : "乳白纸墨";

    public string ActiveModelName
    {
        get
        {
            var instance = SettingsStore.ActivePrimaryInstance(_settings.Providers, _settings.Models);
            return instance?.Name is { Length: > 0 } name ? name : "未配置模型";
        }
    }

    public bool HasActiveModel =>
        SettingsStore.ActivePrimaryInstance(_settings.Providers, _settings.Models) is not null;

    public string BreadcrumbTitle
    {
        get
        {
            var view = Story.View;
            if (view is null) return "未载入剧本";
            return string.IsNullOrWhiteSpace(view.Title) ? "未命名剧本" : view.Title;
        }
    }

    public bool HasStory => Story.View is not null;

    // ---- overlays ----

    public string? ToastMessage => _ui.ToastMessage;

    public bool HasToast => !string.IsNullOrEmpty(_ui.ToastMessage);

    public bool IsSettingsOpen => _ui.SettingsOpen;

    public bool IsPaletteOpen => _ui.PaletteOpen;

    // ---- commands ----

    [RelayCommand]
    private void ToggleLeftRail() => _ui.ToggleLeftRail();

    [RelayCommand]
    private void ToggleRightRail() => _ui.ToggleRightRail();

    [RelayCommand]
    private void CloseFlyout() => _ui.CloseFlyout();

    [RelayCommand]
    private void ToggleTheme() => _ui.ToggleTheme();

    [RelayCommand]
    private void EnterStudio() => _ui.SwitchViewMode(ViewportMode.Studio);

    [RelayCommand]
    private void EnterReading() => _ui.SwitchViewMode(ViewportMode.Reading);

    [RelayCommand]
    private void ShowTreeTab() => _ui.OpenLeftTabAndExpand(LeftSubtab.Tree);

    [RelayCommand]
    private void ShowCharactersTab() => _ui.OpenLeftTabAndExpand(LeftSubtab.Characters);

    [RelayCommand]
    private void ShowLoreTab() => _ui.OpenLeftTabAndExpand(LeftSubtab.Lore);

    [RelayCommand]
    private void ShowSessionsTab() => _ui.OpenLeftTabAndExpand(LeftSubtab.Sessions);

    [RelayCommand]
    private void OpenSettings() => _ui.SetSettingsOpen(true);

    [RelayCommand]
    private void CloseSettings() => _ui.SetSettingsOpen(false);

    [RelayCommand]
    private void TogglePalette() => _ui.TogglePalette();

    [RelayCommand]
    private void ClosePalette() => _ui.SetPaletteOpen(false);

    // ---- change plumbing ----

    private void OnUiChanged(object? sender, System.ComponentModel.PropertyChangedEventArgs e)
    {
        OnPropertyChanged(nameof(IsReadingMode));
        OnPropertyChanged(nameof(IsStudioMode));
        OnPropertyChanged(nameof(LeftRailWidth));
        OnPropertyChanged(nameof(RightRailWidth));
        OnPropertyChanged(nameof(ShowLeftRail));
        OnPropertyChanged(nameof(ShowRightRail));
        OnPropertyChanged(nameof(ShowRightRailRecall));
        OnPropertyChanged(nameof(ShowLeftPanel));
        OnPropertyChanged(nameof(ShowIconDock));
        OnPropertyChanged(nameof(IsLeftCollapsed));
        OnPropertyChanged(nameof(IsRightCollapsed));
        OnPropertyChanged(nameof(LeftTab));
        OnPropertyChanged(nameof(LeftTabTree));
        OnPropertyChanged(nameof(LeftTabCharacters));
        OnPropertyChanged(nameof(LeftTabLore));
        OnPropertyChanged(nameof(LeftTabSessions));
        OnPropertyChanged(nameof(IsDarkTheme));
        OnPropertyChanged(nameof(ThemeLabel));
        OnPropertyChanged(nameof(ToastMessage));
        OnPropertyChanged(nameof(HasToast));
        OnPropertyChanged(nameof(IsSettingsOpen));
        OnPropertyChanged(nameof(IsPaletteOpen));
    }

    private void OnSettingsChanged(object? sender, System.ComponentModel.PropertyChangedEventArgs e)
    {
        OnPropertyChanged(nameof(ActiveModelName));
        OnPropertyChanged(nameof(HasActiveModel));
    }

    internal void NotifyStoryChanged()
    {
        OnPropertyChanged(nameof(BreadcrumbTitle));
        OnPropertyChanged(nameof(HasStory));
    }

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        _ui.PropertyChanged -= OnUiChanged;
        _settings.PropertyChanged -= OnSettingsChanged;
        LeftRail.Dispose();
        Stage.Dispose();
        RightRail.Dispose();
        Dossier.Dispose();
        Promises.Dispose();
        MemoryModal.Dispose();
        SettingsModal.Dispose();
        CharacterImport.Dispose();
        Worldbook.Dispose();
        Palette.Dispose();
    }
}
